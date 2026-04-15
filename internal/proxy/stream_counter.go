package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

const eventStreamContentType = "text/event-stream"

type StreamTokenEvent struct {
	Model          string
	Tokens         int
	TotalTokens    int64
	TextBytes      int
	TotalTextBytes int64
	Done           bool
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

func newSSECountingResponseWriter(w http.ResponseWriter, encoder tokenEncoder, model string, observer StreamTokenObserver) *sseCountingResponseWriter {
	return &sseCountingResponseWriter{
		ResponseWriter: w,
		counter:        newSSETokenCounter(encoder, model, observer),
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
	if n > 0 && w.shouldCount() {
		w.counter.Write(p[:n])
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

type sseTokenCounter struct {
	encoder  tokenEncoder
	model    string
	observer StreamTokenObserver

	line      []byte
	eventData []byte

	totalTokens    int64
	totalTextBytes int64
	seenStream     bool
	finished       bool
}

func newSSETokenCounter(encoder tokenEncoder, model string, observer StreamTokenObserver) *sseTokenCounter {
	return &sseTokenCounter{
		encoder:  encoder,
		model:    model,
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

	event := StreamTokenEvent{
		Model:          c.model,
		TotalTokens:    c.totalTokens,
		TotalTextBytes: c.totalTextBytes,
		Done:           true,
	}
	if c.seenStream && c.observer != nil {
		c.observer(event)
	}
	return event
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

type streamChunk struct {
	Delta      json.RawMessage `json:"delta"`
	Text       string          `json:"text"`
	Completion string          `json:"completion"`
	Choices    []streamChoice  `json:"choices"`
}

type streamChoice struct {
	Delta json.RawMessage `json:"delta"`
	Text  string          `json:"text"`
}

type streamDelta struct {
	Content   string           `json:"content"`
	Text      string           `json:"text"`
	ToolCalls []streamToolCall `json:"tool_calls"`
}

type streamToolCall struct {
	Function streamToolFunction `json:"function"`
}

type streamToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func extractStreamText(data []byte) []string {
	var chunk streamChunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		return nil
	}

	var texts []string
	appendIfNotEmpty := func(text string) {
		if text != "" {
			texts = append(texts, text)
		}
	}

	appendIfNotEmpty(chunk.Text)
	appendIfNotEmpty(chunk.Completion)
	appendDeltaText(chunk.Delta, appendIfNotEmpty)

	for _, choice := range chunk.Choices {
		appendIfNotEmpty(choice.Text)
		appendDeltaText(choice.Delta, appendIfNotEmpty)
	}

	return texts
}

func appendDeltaText(raw json.RawMessage, appendText func(string)) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		appendText(text)
		return
	}

	var delta streamDelta
	if err := json.Unmarshal(raw, &delta); err != nil {
		return
	}
	appendText(delta.Content)
	appendText(delta.Text)
	for _, toolCall := range delta.ToolCalls {
		appendText(toolCall.Function.Name)
		appendText(toolCall.Function.Arguments)
	}
}
