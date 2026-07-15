package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	tiktoken "github.com/pkoukk/tiktoken-go"

	"tokenguard/internal/models"
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

	totalTokens    int64
	inputTokens    int64
	totalTextBytes int64
	costMicroUSD   int64
	hasProviderCost bool
	seenStream     bool
	seenJSON       bool
	truncatedJSON  bool
	finished       bool
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

type streamChunk struct {
	Delta      json.RawMessage `json:"delta"`
	Type       string          `json:"type"`
	Text       string          `json:"text"`
	Completion string          `json:"completion"`
	Content    []contentBlock  `json:"content"`
	Usage      usagePayload    `json:"usage"`
	Message    streamMessage   `json:"message"`
	Choices    []streamChoice  `json:"choices"`
}

type streamChoice struct {
	Delta        json.RawMessage `json:"delta"`
	Text         string          `json:"text"`
	Message      streamMessage   `json:"message"`
	FinishReason string          `json:"finish_reason"`
}

type streamDelta struct {
	Content   any              `json:"content"`
	Text      string           `json:"text"`
	Thinking  string           `json:"thinking"`
	ToolCalls []streamToolCall `json:"tool_calls"`
}

type streamToolCall struct {
	Function streamToolFunction `json:"function"`
}

type streamToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type streamMessage struct {
	Content string       `json:"content"`
	Usage   usagePayload `json:"usage"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type usagePayload struct {
	PromptTokens         int64   `json:"prompt_tokens"`
	CompletionTokens     int64   `json:"completion_tokens"`
	InputTokens          int64   `json:"input_tokens"`
	OutputTokens         int64   `json:"output_tokens"`
	TotalTokens          int64   `json:"total_tokens"`
	PromptTokenCount     int64   `json:"promptTokenCount"`
	CandidatesTokenCount int64   `json:"candidatesTokenCount"`
	TotalTokenCount      int64   `json:"totalTokenCount"`
	Cost                 float64 `json:"cost"` // OpenRouter: USD charged for this request
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
	for _, block := range chunk.Content {
		appendIfNotEmpty(block.Text)
	}
	appendIfNotEmpty(chunk.Message.Content)

	for _, choice := range chunk.Choices {
		appendIfNotEmpty(choice.Text)
		appendIfNotEmpty(choice.Message.Content)
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
	appendContentText(delta.Content, appendText)
	appendText(delta.Text)
	appendText(delta.Thinking)
	for _, toolCall := range delta.ToolCalls {
		appendText(toolCall.Function.Name)
		appendText(toolCall.Function.Arguments)
	}
}

func appendContentText(value any, appendText func(string)) {
	switch typed := value.(type) {
	case nil:
		return
	case string:
		appendText(typed)
	case []any:
		for _, item := range typed {
			appendContentText(item, appendText)
		}
	case map[string]any:
		if text, ok := typed["text"].(string); ok {
			appendText(text)
		}
		if content, ok := typed["content"]; ok {
			appendContentText(content, appendText)
		}
	}
}

func (c *sseTokenCounter) processProviderUsage(raw []byte) {
	usage, ok := extractUsage(raw)
	if !ok {
		// Only fall back to response-text counting for non-SSE JSON bodies.
		// SSE deltas are already counted via extractStreamText in processEvent.
		if c.seenStream {
			return
		}
		for _, text := range extractResponseText(raw) {
			if text == "" || c.encoder == nil {
				continue
			}
			c.totalTokens += int64(c.encoder.Count(text))
		}
		return
	}
	if usage.InputTokens > 0 {
		c.inputTokens = usage.InputTokens
	}
	if usage.OutputTokens > 0 {
		c.totalTokens = usage.OutputTokens
	}
	if usage.CostUSD > 0 {
		c.costMicroUSD = models.USDToMicroUSD(usage.CostUSD)
		c.hasProviderCost = true
	}
}

func extractUsage(raw []byte) (struct {
	InputTokens  int64
	OutputTokens int64
	CostUSD      float64
}, bool) {
	var root struct {
		Usage         usagePayload `json:"usage"`
		UsageMetadata usagePayload `json:"usageMetadata"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return struct {
			InputTokens  int64
			OutputTokens int64
			CostUSD      float64
		}{}, false
	}

	input := firstPositive(root.Usage.InputTokens, root.Usage.PromptTokens, root.UsageMetadata.PromptTokenCount)
	output := firstPositive(root.Usage.OutputTokens, root.Usage.CompletionTokens, root.UsageMetadata.CandidatesTokenCount)
	if output == 0 {
		total := firstPositive(root.Usage.TotalTokens, root.UsageMetadata.TotalTokenCount)
		if total > input {
			output = total - input
		}
	}
	cost := root.Usage.Cost
	if cost <= 0 {
		cost = root.UsageMetadata.Cost
	}
	return struct {
		InputTokens  int64
		OutputTokens int64
		CostUSD      float64
	}{InputTokens: input, OutputTokens: output, CostUSD: cost}, input > 0 || output > 0 || cost > 0
}

func extractResponseText(raw []byte) []string {
	var root streamChunk
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil
	}
	var texts []string
	appendIfNotEmpty := func(text string) {
		if text != "" {
			texts = append(texts, text)
		}
	}
	appendIfNotEmpty(root.Text)
	appendIfNotEmpty(root.Completion)
	appendDeltaText(root.Delta, appendIfNotEmpty)
	for _, block := range root.Content {
		appendIfNotEmpty(block.Text)
	}
	appendIfNotEmpty(root.Message.Content)
	for _, choice := range root.Choices {
		appendIfNotEmpty(choice.Text)
		appendIfNotEmpty(choice.Message.Content)
		appendDeltaText(choice.Delta, appendIfNotEmpty)
	}
	return texts
}

func firstPositive(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
