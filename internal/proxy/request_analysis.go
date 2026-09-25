package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	tokenGuardAPIKeyHeader    = "X-TokenGuard-API-Key"
	tokenGuardAPIKeyAltHeader = "X-TokenGuard-Key"
	tokenGuardSessionHeader   = "X-TokenGuard-Session-ID"
	tokenGuardTeamIDHeader    = "X-TokenGuard-Team-ID"
	sessionIDHeader           = "X-Session-ID"
)

type requestAnalysis struct {
	Provider        string
	Model           string
	InputTokens     int64
	MaxOutputTokens int64
	SessionID       string
	SemanticPayload []byte
}

func readRequestBody(r *http.Request, maxBytes int64) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxRequestBytes
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if err := r.Body.Close(); err != nil {
		return nil, fmt.Errorf("close request body: %w", err)
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("request body exceeds %d bytes", maxBytes)
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	r.ContentLength = int64(len(raw))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(raw)), nil
	}
	return raw, nil
}

func restoreRequestBody(r *http.Request, body []byte) {
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
}

func tokenGuardAPIKey(r *http.Request) string {
	if key := strings.TrimSpace(r.Header.Get(tokenGuardAPIKeyHeader)); key != "" {
		return key
	}
	return strings.TrimSpace(r.Header.Get(tokenGuardAPIKeyAltHeader))
}

// applyCombinedBearer supports clients that can send only Authorization,
// such as Cursor's Override OpenAI Base URL. Format:
//
//	Authorization: Bearer tg_<key>:<provider-key>
//
// The tg_ half becomes the budget identity. The provider half is what
// upstream receives. A missing session id is scoped to that tg_ key so
// loop detection still runs, and stays separate per key.
func applyCombinedBearer(r *http.Request) {
	if tokenGuardAPIKey(r) != "" {
		return
	}
	if tgKey, providerKey, ok := splitCombinedBearer(r.Header.Get("Authorization")); ok {
		r.Header.Set(tokenGuardAPIKeyHeader, tgKey)
		r.Header.Set("Authorization", "Bearer "+providerKey)
		setSingleHeaderSession(r, tgKey)
		return
	}
	// Claude Code and other Anthropic clients put the only secret in x-api-key.
	if tgKey, providerKey, ok := splitCombinedToken(r.Header.Get("x-api-key")); ok {
		r.Header.Set(tokenGuardAPIKeyHeader, tgKey)
		r.Header.Set("x-api-key", providerKey)
		setSingleHeaderSession(r, tgKey)
	}
}

func setSingleHeaderSession(r *http.Request, tgKey string) {
	if sessionIDFromHeaders(r.Header) == "" {
		r.Header.Set(tokenGuardSessionHeader, cursorSessionID(tgKey))
	}
}

func splitCombinedBearer(header string) (string, string, bool) {
	value := strings.TrimSpace(header)
	const scheme = "bearer "
	if len(value) < len(scheme) || !strings.EqualFold(value[:len(scheme)], scheme) {
		return "", "", false
	}
	return splitCombinedToken(strings.TrimSpace(value[len(scheme):]))
}

func splitCombinedToken(token string) (string, string, bool) {
	tgKey, providerKey, found := strings.Cut(strings.TrimSpace(token), ":")
	if !found || !strings.HasPrefix(tgKey, "tg_") || len(tgKey) <= len("tg_") || providerKey == "" {
		return "", "", false
	}
	return tgKey, providerKey, true
}

func cursorSessionID(tgKey string) string {
	sum := sha256.Sum256([]byte(tgKey))
	return "cursor-" + hex.EncodeToString(sum[:8])
}

func stripTokenGuardHeaders(r *http.Request) {
	r.Header.Del(tokenGuardAPIKeyHeader)
	r.Header.Del(tokenGuardAPIKeyAltHeader)
	r.Header.Del(tokenGuardSessionHeader)
	r.Header.Del(tokenGuardProviderHeader)
	r.Header.Del(tokenGuardTeamIDHeader)
}

func analyzeRequest(r *http.Request, body []byte, encoder tokenEncoder, defaultMaxOutputTokens int64) (requestAnalysis, error) {
	route := providerFromContext(r.Context())
	analysis := requestAnalysis{
		Provider:        route.Name,
		SessionID:       sessionIDFromHeaders(r.Header),
		MaxOutputTokens: defaultMaxOutputTokens,
		SemanticPayload: compactOrCopy(body),
	}
	if analysis.Provider == "" {
		analysis.Provider = providerOpenAI
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return analysis, nil
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return requestAnalysis{}, errors.New("request body must be valid JSON for TokenGuard checks")
	}

	analysis.Model = jsonString(root["model"])
	if maxOutputTokens, ok := jsonInt64(root["max_completion_tokens"]); ok {
		analysis.MaxOutputTokens = maxOutputTokens
	} else if maxOutputTokens, ok := jsonInt64(root["max_tokens"]); ok {
		analysis.MaxOutputTokens = maxOutputTokens
	} else if maxOutputTokens, ok := jsonInt64(root["max_output_tokens"]); ok {
		analysis.MaxOutputTokens = maxOutputTokens
	}
	if analysis.MaxOutputTokens < 0 {
		return requestAnalysis{}, errors.New("max output tokens cannot be negative")
	}
	if analysis.SessionID == "" {
		analysis.SessionID = jsonString(root["session_id"])
	}
	if analysis.SessionID == "" {
		analysis.SessionID = sessionIDFromMetadata(root["metadata"])
	}

	analysis.InputTokens = countRequestInputTokens(root, body, encoder)
	analysis.SemanticPayload = semanticPayload(root, body)
	return analysis, nil
}

func clampMaxOutputTokens(body []byte, maxOutput int64) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(maxOutput)
	if err != nil {
		return nil, err
	}
	set := false
	for _, field := range []string{"max_completion_tokens", "max_tokens", "max_output_tokens"} {
		if _, ok := root[field]; ok {
			root[field] = raw
			set = true
		}
	}
	if !set {
		root["max_tokens"] = raw
	}
	return json.Marshal(root)
}

func sessionIDFromHeaders(header http.Header) string {
	if sessionID := strings.TrimSpace(header.Get(tokenGuardSessionHeader)); sessionID != "" {
		return sessionID
	}
	return strings.TrimSpace(header.Get(sessionIDHeader))
}

func sessionIDFromMetadata(raw json.RawMessage) string {
	var metadata map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &metadata) != nil {
		return ""
	}
	return jsonString(metadata["session_id"])
}

func countRequestInputTokens(root map[string]json.RawMessage, body []byte, encoder tokenEncoder) int64 {
	if encoder == nil {
		return 0
	}

	var total int64
	for _, field := range []string{"messages", "input", "prompt", "instructions", "system", "tools", "functions"} {
		total += countJSONText(root[field], encoder)
	}
	if total > 0 {
		return total
	}

	return int64(encoder.Count(string(compactOrCopy(body))))
}

func countJSONText(raw json.RawMessage, encoder tokenEncoder) int64 {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return 0
	}

	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0
	}
	return countAnyText(value, encoder)
}

func countAnyText(value any, encoder tokenEncoder) int64 {
	switch typed := value.(type) {
	case string:
		return int64(encoder.Count(typed))
	case []any:
		var total int64
		for _, item := range typed {
			total += countAnyText(item, encoder)
		}
		return total
	case map[string]any:
		var total int64
		for _, item := range typed {
			total += countAnyText(item, encoder)
		}
		return total
	default:
		return 0
	}
}

func semanticPayload(root map[string]json.RawMessage, body []byte) []byte {
	semantic := make(map[string]json.RawMessage, 8)
	for _, field := range []string{"model", "messages", "input", "prompt", "instructions", "system", "tools", "tool_choice", "functions"} {
		if raw := bytes.TrimSpace(root[field]); len(raw) > 0 {
			semantic[field] = append(json.RawMessage(nil), raw...)
		}
	}
	if len(semantic) == 0 {
		return compactOrCopy(body)
	}

	raw, err := json.Marshal(semantic)
	if err != nil {
		return compactOrCopy(body)
	}
	return compactOrCopy(raw)
}

func compactOrCopy(raw []byte) []byte {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err == nil {
		return buf.Bytes()
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return out
}

func jsonString(raw json.RawMessage) string {
	var out string
	if len(raw) == 0 || json.Unmarshal(raw, &out) != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func jsonInt64(raw json.RawMessage) (int64, bool) {
	var out int64
	if len(raw) == 0 || json.Unmarshal(raw, &out) != nil {
		return 0, false
	}
	return out, true
}
