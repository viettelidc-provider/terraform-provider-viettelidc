package client

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// Retry policy. These are vars (not consts) so tests can lower them; do not
// mutate at runtime in production.
var (
	maxRetryAttempts       = 3
	initialBackoffOverride = 1 * time.Second
	maxBackoffOverride     = 10 * time.Second
)

// retryOp executes a single HTTP attempt and returns the HTTP status code
// (0 on transport error) and the error (if any).
type retryOp func() (status int, err error)

// doWithRetry runs op up to maxRetryAttempts+1 times. It retries on:
//   - network/transport errors (status == 0 with non-nil err)
//   - HTTP 5xx
//
// It does NOT retry on:
//   - HTTP 4xx (including 401 — token invalid, retry is pointless)
//   - context cancellation
//   - successful 2xx/3xx
func doWithRetry(ctx context.Context, op retryOp) (int, error) {
	var lastStatus int
	var lastErr error

	for attempt := 0; attempt <= maxRetryAttempts; attempt++ {
		status, err := op()
		lastStatus, lastErr = status, err

		// Transport error → retry
		if err != nil && status == 0 {
			if attempt == maxRetryAttempts {
				return status, err
			}
			if !shouldSleep(ctx, attempt) {
				return status, ctx.Err()
			}
			continue
		}
		// 5xx → retry
		if status >= 500 && status < 600 {
			if attempt == maxRetryAttempts {
				return status, fmt.Errorf("api client: exhausted %d retries, last status %d", maxRetryAttempts, status)
			}
			if !shouldSleep(ctx, attempt) {
				return status, ctx.Err()
			}
			continue
		}
		// 4xx (incl. 401), 2xx, 3xx → return immediately
		return status, err
	}
	return lastStatus, lastErr
}

// shouldSleep waits backoff(attempt) honoring ctx; returns false if ctx
// is cancelled.
func shouldSleep(ctx context.Context, attempt int) bool {
	d := retryBackoff(attempt)
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// retryBackoff returns a duration with full-random jitter:
// base = min(initial * 2^attempt, max); sleep = rand[0, base).
func retryBackoff(attempt int) time.Duration {
	base := initialBackoffOverride << uint(attempt) // 1s, 2s, 4s, 8s
	if base > maxBackoffOverride || base <= 0 {
		base = maxBackoffOverride
	}
	if base <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(base)))
}
