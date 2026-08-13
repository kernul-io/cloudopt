package awsinventory

import (
	"context"
	"math/rand"
	"time"
)

// RetryConfig bounds retried read-only calls.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

func defaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   200 * time.Millisecond,
		MaxDelay:    8 * time.Second,
	}
}

func withRetry(ctx context.Context, cfg RetryConfig, fn func() error, retryable func(error) bool) error {
	if cfg.MaxAttempts <= 0 {
		cfg = defaultRetryConfig()
	}
	var last error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return ErrCancelled
		}
		last = fn()
		if last == nil {
			return nil
		}
		if !retryable(last) || attempt == cfg.MaxAttempts {
			return last
		}
		delay := cfg.BaseDelay * time.Duration(1<<(attempt-1))
		if delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}
		if delay <= 0 {
			delay = time.Millisecond
		}
		jitterSpan := int64(delay / 2)
		if jitterSpan <= 0 {
			jitterSpan = 1
		}
		jitter := time.Duration(rand.Int63n(jitterSpan))
		timer := time.NewTimer(delay/2 + jitter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ErrCancelled
		case <-timer.C:
		}
	}
	return last
}

func retryableAPIErr(err error) bool {
	var api *APIError
	if errorsAsAPI(err, &api) {
		return api.Retryable
	}
	return false
}

func errorsAsAPI(err error, target **APIError) bool {
	if err == nil {
		return false
	}
	if api, ok := err.(*APIError); ok {
		*target = api
		return true
	}
	return false
}
