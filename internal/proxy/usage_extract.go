package proxy

import (
	"bytes"
	"encoding/json"

	"tokenguard/internal/models"
)

// JSON/SSE payload shapes and helpers for extracting completion text and usage
// (including OpenRouter usage.cost) from provider responses.

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

func (c *sseTokenCounter) processProviderUsage(raw []byte) {
	usage, ok := extractUsageLoose(raw)
	if ok {
		c.hasProviderUsage = true
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
	// Some gateways omit completion_tokens or send 0 while still returning message
	// content. For non-SSE JSON, count response text as a fallback.
	if c.totalTokens == 0 && !c.seenStream && c.encoder != nil && !c.partialJSON {
		for _, text := range extractResponseText(raw) {
			if text == "" {
				continue
			}
			c.totalTokens += int64(c.encoder.Count(text))
		}
	}
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
