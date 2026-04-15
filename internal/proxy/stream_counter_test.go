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
	counter := newSSETokenCounter(fakeTokenEncoder{}, "test-model", func(event StreamTokenEvent) {
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
