package proxy

import (
	"strings"
	"testing"
)

type fakeTokenEncoder struct{}

func (fakeTokenEncoder) Count(text string) int {
	return len(text)
}

func TestSSETokenCounterHandlesSplitEvents(t *testing.T) {
	var events []StreamTokenEvent
	counter := newSSETokenCounter(fakeTokenEncoder{}, "test-model", providerOpenAI, func(event StreamTokenEvent) {
		events = append(events, event)
	})

	counter.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n"))
	counter.Write([]byte("\n"))
	counter.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n"))
	final := counter.Finish()

	if final.TotalTokens != 5 {
		t.Fatalf("TotalTokens = %d, want 5", final.TotalTokens)
	}
	if final.TotalTextBytes != 5 {
		t.Fatalf("TotalTextBytes = %d, want 5", final.TotalTextBytes)
	}
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3 including final done event", len(events))
	}
	if !events[len(events)-1].Done {
		t.Fatal("last event was not marked done")
	}
}

func TestExtractStreamTextCoversOpenAIAndToolDeltas(t *testing.T) {
	data := []byte(`{"choices":[{"delta":{"content":"Hello","tool_calls":[{"function":{"name":"lookup","arguments":"{\"q\":\"cost\"}"}}]}}]}`)

	got := strings.Join(extractStreamText(data), "|")
	want := `Hello|lookup|{"q":"cost"}`
	if got != want {
		t.Fatalf("extractStreamText = %q, want %q", got, want)
	}
}

func TestExtractStreamTextCoversAnthropicContentDelta(t *testing.T) {
	data := []byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}`)

	got := strings.Join(extractStreamText(data), "|")
	if got != "hello" {
		t.Fatalf("extractStreamText = %q, want hello", got)
	}
}

func TestJSONUsageExtractionCoversOpenAIAndAnthropic(t *testing.T) {
	openAIUsage, ok := extractUsage([]byte(`{"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`))
	if !ok {
		t.Fatal("extractUsage returned ok=false for OpenAI usage")
	}
	if openAIUsage.InputTokens != 10 || openAIUsage.OutputTokens != 3 {
		t.Fatalf("OpenAI usage = %#v, want input=10 output=3", openAIUsage)
	}

	anthropicUsage, ok := extractUsage([]byte(`{"usage":{"input_tokens":8,"output_tokens":5}}`))
	if !ok {
		t.Fatal("extractUsage returned ok=false for Anthropic usage")
	}
	if anthropicUsage.InputTokens != 8 || anthropicUsage.OutputTokens != 5 {
		t.Fatalf("Anthropic usage = %#v, want input=8 output=5", anthropicUsage)
	}

	orUsage, ok := extractUsage([]byte(`{"usage":{"prompt_tokens":10,"completion_tokens":2,"cost":0.000012}}`))
	if !ok || orUsage.CostUSD != 0.000012 {
		t.Fatalf("OpenRouter cost usage = %#v", orUsage)
	}
}
