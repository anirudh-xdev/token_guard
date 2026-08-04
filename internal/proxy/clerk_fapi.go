package proxy

import (
	"encoding/base64"
	"strings"
)

// clerkFrontendAPIHost extracts the Frontend API host from a Clerk publishable key.
// Keys look like pk_test_<base64> where base64 decodes to "<host>$...".
func clerkFrontendAPIHost(publishableKey string) string {
	pk := strings.TrimSpace(publishableKey)
	for _, prefix := range []string{"pk_test_", "pk_live_"} {
		if !strings.HasPrefix(pk, prefix) {
			continue
		}
		raw := strings.TrimPrefix(pk, prefix)
		decoded, err := base64.RawStdEncoding.DecodeString(raw)
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(raw)
		}
		if err != nil {
			return ""
		}
		host, _, _ := strings.Cut(string(decoded), "$")
		return strings.TrimSpace(host)
	}
	return ""
}
