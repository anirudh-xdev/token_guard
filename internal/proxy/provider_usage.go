package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"tokenguard/internal/models"
)

type providerUsageContextKey struct{}

// providerUsageCapture holds usage parsed from the upstream JSON body so
// settlement does not depend on Content-Type quirks or the ResponseWriter tee.
type providerUsageCapture struct {
	mu sync.Mutex

	inputTokens     int64
	outputTokens    int64
	costMicroUSD    int64
	hasProviderCost bool
	ok              bool
}

func withProviderUsageCapture(ctx context.Context) (context.Context, *providerUsageCapture) {
	cap := &providerUsageCapture{}
	return context.WithValue(ctx, providerUsageContextKey{}, cap), cap
}

func providerUsageCaptureFrom(ctx context.Context) *providerUsageCapture {
	cap, _ := ctx.Value(providerUsageContextKey{}).(*providerUsageCapture)
	return cap
}

func (c *providerUsageCapture) store(input, output int64, costUSD float64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if input > 0 {
		c.inputTokens = input
	}
	if output > 0 {
		c.outputTokens = output
	}
	if costUSD > 0 {
		c.costMicroUSD = models.USDToMicroUSD(costUSD)
		c.hasProviderCost = true
	}
	c.ok = c.inputTokens > 0 || c.outputTokens > 0 || c.hasProviderCost
}

func (c *providerUsageCapture) applyTo(event *StreamTokenEvent) {
	if c == nil || event == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.ok {
		return
	}
	if c.inputTokens > 0 {
		event.InputTokens = c.inputTokens
	}
	if c.outputTokens > 0 {
		event.TotalTokens = c.outputTokens
	}
	if c.hasProviderCost && c.costMicroUSD > 0 {
		event.CostMicroUSD = c.costMicroUSD
		event.HasProviderCost = true
	}
	event.ProviderUsage = true
}

func (c *providerUsageCapture) headerValue() string {
	if c == nil {
		return "missing"
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.ok {
		return "missing"
	}
	return fmt.Sprintf("in=%d;out=%d", c.inputTokens, c.outputTokens)
}

const maxProviderUsageBodyBytes = 16 << 20 // 16 MiB

func captureProviderUsageModifyResponse(resp *http.Response) error {
	if resp == nil || resp.Body == nil {
		return nil
	}
	ctx := context.Background()
	if resp.Request != nil {
		ctx = resp.Request.Context()
	}
	return captureProviderUsageInto(ctx, resp)
}

func captureProviderUsageInto(ctx context.Context, resp *http.Response) error {
	cap := providerUsageCaptureFrom(ctx)
	if cap == nil || resp == nil || resp.Body == nil {
		return nil
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, eventStreamContentType) {
		return nil
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderUsageBodyBytes))
	_ = resp.Body.Close()
	if err != nil {
		resp.Body = io.NopCloser(bytes.NewReader(nil))
		return nil
	}

	decoded := data
	ce := strings.ToLower(resp.Header.Get("Content-Encoding"))
	if strings.Contains(ce, "gzip") || looksLikeGzip(data) {
		plain, zipErr := gunzipBytes(data)
		if zipErr != nil {
			log.Printf("provider usage: gzip decode failed: %v", zipErr)
		} else {
			decoded = plain
			resp.Header.Del("Content-Encoding")
			resp.Header.Del("Content-Length")
			resp.ContentLength = int64(len(decoded))
			data = decoded
		}
	}

	resp.Body = io.NopCloser(bytes.NewReader(data))

	if len(bytes.TrimSpace(decoded)) == 0 {
		return nil
	}
	if usage, ok := extractUsageLoose(decoded); ok {
		cap.store(usage.InputTokens, usage.OutputTokens, usage.CostUSD)
		resp.Header.Set("X-TokenGuard-Usage", fmt.Sprintf("in=%d;out=%d", usage.InputTokens, usage.OutputTokens))
	}
	return nil
}

func looksLikeGzip(data []byte) bool {
	return len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b
}

func gunzipBytes(data []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(io.LimitReader(zr, maxProviderUsageBodyBytes))
}

func extractUsageLoose(raw []byte) (struct {
	InputTokens  int64
	OutputTokens int64
	CostUSD      float64
}, bool) {
	if usage, ok := extractUsage(raw); ok {
		return usage, true
	}
	return extractUsageFromTail(raw)
}

func extractUsageFromTail(raw []byte) (struct {
	InputTokens  int64
	OutputTokens int64
	CostUSD      float64
}, bool) {
	empty := struct {
		InputTokens  int64
		OutputTokens int64
		CostUSD      float64
	}{}

	keyIdx := bytes.LastIndex(raw, []byte(`"usage"`))
	if keyIdx < 0 {
		keyIdx = bytes.LastIndex(raw, []byte(`"usageMetadata"`))
	}
	if keyIdx < 0 {
		return empty, false
	}
	rest := raw[keyIdx:]
	brace := bytes.IndexByte(rest, '{')
	if brace < 0 {
		return empty, false
	}
	dec := json.NewDecoder(bytes.NewReader(rest[brace:]))
	var payload usagePayload
	if err := dec.Decode(&payload); err != nil {
		return empty, false
	}
	input := firstPositive(payload.InputTokens, payload.PromptTokens, payload.PromptTokenCount)
	output := firstPositive(payload.OutputTokens, payload.CompletionTokens, payload.CandidatesTokenCount)
	if output == 0 {
		total := firstPositive(payload.TotalTokens, payload.TotalTokenCount)
		if total > input {
			output = total - input
		}
	}
	return struct {
		InputTokens  int64
		OutputTokens int64
		CostUSD      float64
	}{InputTokens: input, OutputTokens: output, CostUSD: payload.Cost}, input > 0 || output > 0 || payload.Cost > 0
}
