package proxy

import (
	"testing"

	"github.com/clerk/clerk-sdk-go/v2"
)

func TestClerkEmailVerified(t *testing.T) {
	t.Parallel()
	if clerkEmailVerified(nil) {
		t.Fatal("nil address must not be verified")
	}
	unverified := &clerk.EmailAddress{
		EmailAddress: "a@example.com",
		Verification: &clerk.Verification{Status: "unverified"},
	}
	if clerkEmailVerified(unverified) {
		t.Fatal("unverified address must not be used for invites")
	}
	verified := &clerk.EmailAddress{
		EmailAddress: "a@example.com",
		Verification: &clerk.Verification{Status: "verified"},
	}
	if !clerkEmailVerified(verified) {
		t.Fatal("verified address should be accepted")
	}
}
