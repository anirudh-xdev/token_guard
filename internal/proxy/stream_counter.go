package proxy

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

const eventStreamContentType = "text/event-stream"

type StreamTokenEvent struct {
	Model           string
	Tokens          int
	TotalTokens     int64
	InputTokens     int64
	TextBytes       int
	TotalTextBytes  int64
	Done            bool
	ProviderUsage   bool
	CostMicroUSD    int64 // provider-reported cost when available (e.g. OpenRouter usage.cost)
	HasProviderCost bool
}

type StreamTokenObserver func(StreamTokenEvent)

type tokenEncoder interface {
	Count(text string) int
}

type tiktokenEncoder struct {
	codec *tiktoken.Tiktoken
}

func newTiktokenEncoder(model string) (*tiktokenEncoder, error) {
	codec, err := tiktoken.EncodingForModel(model)
	if err != nil {
		codec, err = tiktoken.GetEncoding(tiktoken.MODEL_CL100K_BASE)
	}
	if err != nil {
		return nil, fmt.Errorf("load tiktoken encoding: %w", err)
	}
	return &tiktokenEncoder{codec: codec}, nil
}

func (e *tiktokenEncoder) Count(text string) int {
	if text == "" {
		return 0
	}
	return len(e.codec.EncodeOrdinary(text))
}

type sseCountingResponseWriter struct {
	http.ResponseWriter

	counter *sseTokenCounter
	status  int
}

func newSSECountingResponseWriter(w http.ResponseWriter, encoder tokenEncoder, model, provider string, observer StreamTokenObserver) *sseCountingResponseWriter {
	return &sseCountingResponseWriter{
		ResponseWriter: w,
		counter:        newSSETokenCounter(encoder, model, provider, observer),
	}
}

func (w *sseCountingResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *sseCountingResponseWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if n > 0 {
		switch {
		case w.shouldCount():
			w.counter.Write(p[:n])
		case w.shouldCaptureJSON():
			w.counter.CaptureJSON(p[:n])
		}
	}
	return n, err
}

func (w *sseCountingResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *sseCountingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *sseCountingResponseWriter) Finish() StreamTokenEvent {
	return w.counter.Finish()
}

func (w *sseCountingResponseWriter) StatusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *sseCountingResponseWriter) shouldCount() bool {
	if w.status != 0 && (w.status < http.StatusOK || w.status >= http.StatusMultipleChoices) {
		return false
	}
	contentType := strings.ToLower(w.Header().Get("Content-Type"))
	return strings.Contains(contentType, eventStreamContentType)
}

func (w *sseCountingResponseWriter) shouldCaptureJSON() bool {
	if w.status != 0 && (w.status < http.StatusOK || w.status >= http.StatusMultipleChoices) {
		return false
	}
	contentType := strings.ToLower(w.Header().Get("Content-Type"))
	return strings.Contains(contentType, "application/json") || strings.Contains(contentType, "+json")
}

type sseTokenCounter struct {
	encoder  tokenEncoder
	model    string
	provider string
	observer StreamTokenObserver

	line      []byte
	eventData []byte
	jsonBody  []byte

	totalTokens     int64
	inputTokens     int64
	totalTextBytes  int64
	costMicroUSD    int64
	hasProviderCost bool
	seenStream      bool
	seenJSON        bool
	truncatedJSON   bool
	finished        bool
}

const maxUsageJSONBytes = 1 << 20

func newSSETokenCounter(encoder tokenEncoder, model, provider string, observer StreamTokenObserver) *sseTokenCounter {
	return &sseTokenCounter{
		encoder:  encoder,
		model:    model,
		provider: provider,
		observer: observer,
	}
}

func (c *sseTokenCounter) Write(p []byte) {
	if c == nil || c.finished {
		return
	}
	c.seenStream = true

	for len(p) > 0 {
		lineEnd := bytes.IndexByte(p, '\n')
		if lineEnd < 0 {
			c.line = append(c.line, p...)
			return
		}

		c.line = append(c.line, p[:lineEnd]...)
		c.processLine(c.line)
		c.line = c.line[:0]
		p = p[lineEnd+1:]
	}
}

func (c *sseTokenCounter) Finish() StreamTokenEvent {
	if c == nil || c.finished {
		return StreamTokenEvent{}
	}
	c.finished = true

	if len(c.line) > 0 {
		c.processLine(c.line)
		c.line = nil
	}
	if len(c.eventData) > 0 {
		c.processEvent()
	}
	if c.seenJSON && !c.truncatedJSON {
		c.processProviderUsage(c.jsonBody)
	}

	event := StreamTokenEvent{
		Model:           c.model,
		TotalTokens:     c.totalTokens,
		InputTokens:     c.inputTokens,
		TotalTextBytes:  c.totalTextBytes,
		Done:            true,
		ProviderUsage:   c.seenJSON || c.hasProviderCost,
		CostMicroUSD:    c.costMicroUSD,
		HasProviderCost: c.hasProviderCost,
	}
	if (c.seenStream || c.seenJSON) && c.observer != nil {
		c.observer(event)
	}
	return event
}

func (c *sseTokenCounter) CaptureJSON(p []byte) {
	if c == nil || c.finished || len(p) == 0 || c.truncatedJSON {
		return
	}
	c.seenJSON = true
	if len(c.jsonBody)+len(p) > maxUsageJSONBytes {
		c.truncatedJSON = true
		c.jsonBody = nil
		return
	}
	c.jsonBody = append(c.jsonBody, p...)
}

func (c *sseTokenCounter) processLine(line []byte) {
	line = bytes.TrimSuffix(line, []byte{'\r'})
	if len(line) == 0 {
		c.processEvent()
		return
	}

	if bytes.HasPrefix(line, []byte("data:")) {
		data := bytes.TrimPrefix(line, []byte("data:"))
		data = bytes.TrimPrefix(data, []byte(" "))
		if len(c.eventData) > 0 {
			c.eventData = append(c.eventData, '\n')
		}
		c.eventData = append(c.eventData, data...)
	}
}

func (c *sseTokenCounter) processEvent() {
	if len(c.eventData) == 0 {
		return
	}
	data := bytes.TrimSpace(c.eventData)
	c.eventData = c.eventData[:0]

	if bytes.Equal(data, []byte("[DONE]")) {
		return
	}

	c.processProviderUsage(data)

	for _, text := range extractStreamText(data) {
		if text == "" {
			continue
		}
		tokens := c.encoder.Count(text)
		textBytes := len(text)
		c.totalTokens += int64(tokens)
		c.totalTextBytes += int64(textBytes)

		if c.observer != nil {
			c.observer(StreamTokenEvent{
				Model:          c.model,
				Tokens:         tokens,
				TotalTokens:    c.totalTokens,
				TextBytes:      textBytes,
				TotalTextBytes: c.totalTextBytes,
			})
		}
	}
}
