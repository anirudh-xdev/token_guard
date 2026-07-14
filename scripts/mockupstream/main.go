package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// Tiny upstream mock for TokenGuard e2e smoke tests.
func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-mock",
			"object":  "chat.completion",
			"model":   "gpt-4o-mini",
			"choices": []map[string]any{{"index": 0, "message": map[string]string{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": 12, "completion_tokens": 3, "total_tokens": 15},
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	log.Print("mock upstream on :19090")
	log.Fatal(http.ListenAndServe("127.0.0.1:19090", mux))
}
