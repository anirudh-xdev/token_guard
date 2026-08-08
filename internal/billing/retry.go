package billing

import (
	"context"
	"errors"
	"strings"
	"time"
)

// IsTransientStoreError reports network / timeout failures that are safe to retry.
func IsTransientStoreError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"deadline exceeded",
		"timeout",
		"temporarily unavailable",
		"connection reset",
		"connection refused",
		"broken pipe",
		"eof",
		"i/o timeout",
		"tls handshake timeout",
		"server closed idle connection",
		"http2: client connection force closed",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// WithTimeout returns a child context with a hard deadline, detached from the
// parent cancel so a client disconnect during preflight doesn't abort Turso I/O mid-flight.
// Parent values (e.g. SpendTeamID) are preserved via WithoutCancel.
// The parent's err is still observed if already done.
func WithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = 15 * time.Second
	}
	if parent == nil {
		parent = context.Background()
	}
	if parent.Err() != nil {
		ctx, cancel := context.WithCancel(parent)
		return ctx, cancel
	}
	return context.WithTimeout(context.WithoutCancel(parent), d)
}

// Retry runs fn up to attempts times when Turso/network errors look transient.
func Retry(ctx context.Context, attempts int, fn func(context.Context) error) error {
	if attempts < 1 {
		attempts = 1
	}
	var last error
	for i := 0; i < attempts; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		last = fn(ctx)
		if last == nil || !IsTransientStoreError(last) || i == attempts-1 {
			return last
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(i+1) * 250 * time.Millisecond):
		}
	}
	return last
}
