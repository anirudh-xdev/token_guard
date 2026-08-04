package proxy

import "testing"

func TestClerkFrontendAPIHost(t *testing.T) {
	// base64("safe-hornet-55.clerk.accounts.dev$") with RawStdEncoding
	host := clerkFrontendAPIHost("pk_test_c2FmZS1ob3JuZXQtNTUuY2xlcmsuYWNjb3VudHMuZGV2JA")
	if host != "safe-hornet-55.clerk.accounts.dev" {
		t.Fatalf("host=%q", host)
	}
}
