package billing

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestIsTransientStoreError(t *testing.T) {
	if !IsTransientStoreError(context.DeadlineExceeded) {
		t.Fatal("deadline should be transient")
	}
	if !IsTransientStoreError(errors.New(`Post "https://x.turso.io": context deadline exceeded`)) {
		t.Fatal("turso deadline string should be transient")
	}
	if IsTransientStoreError(errors.New("no such table: users")) {
		t.Fatal("schema error should not be transient")
	}
}

func TestRetryEventuallySucceeds(t *testing.T) {
	attempts := 0
	err := Retry(context.Background(), 3, func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return context.DeadlineExceeded
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Retry error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts=%d want 3", attempts)
	}
}

func TestWithTimeoutDetachesLaterCancel(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	ctx, stop := WithTimeout(parent, time.Second)
	defer stop()
	cancel()
	if ctx.Err() != nil {
		t.Fatal("db timeout context must not cancel when the HTTP request context cancels")
	}
}

func TestWithTimeoutPreservesValues(t *testing.T) {
	parent := WithSpendTeamID(context.Background(), "team_abc")
	ctx, stop := WithTimeout(parent, time.Second)
	defer stop()
	if got := SpendTeamIDFromContext(ctx); got != "team_abc" {
		t.Fatalf("SpendTeamIDFromContext = %q, want team_abc", got)
	}
}
