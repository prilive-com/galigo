package resilience

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"sync"
	"time"
)

// RetryConfig holds retry configuration.
type RetryConfig struct {
	MaxAttempts int           // Maximum number of attempts (0 = no retries)
	BaseWait    time.Duration // Initial wait duration
	MaxWait     time.Duration // Maximum wait duration
	Multiplier  float64       // Backoff multiplier (e.g., 2.0 for exponential)
	Jitter      float64       // Jitter factor (0.0-1.0)
}

// DefaultRetryConfig returns sensible defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		BaseWait:    time.Second,
		MaxWait:     30 * time.Second,
		Multiplier:  2.0,
		Jitter:      0.2,
	}
}

// RetryableError wraps an error with retry information.
type RetryableError struct {
	Err        error
	RetryAfter time.Duration
}

func (e *RetryableError) Error() string {
	return e.Err.Error()
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// NewRetryableError creates a retryable error.
func NewRetryableError(err error, retryAfter time.Duration) *RetryableError {
	return &RetryableError{Err: err, RetryAfter: retryAfter}
}

// IsRetryable checks if an error should be retried.
func IsRetryable(err error) (time.Duration, bool) {
	var retryErr *RetryableError
	if errors.As(err, &retryErr) {
		return retryErr.RetryAfter, true
	}
	return 0, false
}

// Retry executes fn with retries according to cfg.
func Retry[T any](ctx context.Context, cfg RetryConfig, fn func() (T, error)) (T, error) {
	var zero T
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxAttempts; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if context is done
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}

		// Check if we should retry
		if attempt >= cfg.MaxAttempts {
			break
		}

		// Calculate wait duration
		wait := calculateBackoff(cfg, attempt, lastErr)

		// Wait with context
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(wait):
		}
	}

	return zero, lastErr
}

// RetryWithCallback executes fn with retries and calls onRetry before each retry.
func RetryWithCallback[T any](
	ctx context.Context,
	cfg RetryConfig,
	fn func() (T, error),
	onRetry func(attempt int, err error, wait time.Duration),
) (T, error) {
	var zero T
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxAttempts; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		if ctx.Err() != nil {
			return zero, ctx.Err()
		}

		if attempt >= cfg.MaxAttempts {
			break
		}

		wait := calculateBackoff(cfg, attempt, lastErr)

		if onRetry != nil {
			onRetry(attempt+1, err, wait)
		}

		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(wait):
		}
	}

	return zero, lastErr
}

func calculateBackoff(cfg RetryConfig, attempt int, err error) time.Duration {
	// Check if error specifies retry-after
	if retryAfter, ok := IsRetryable(err); ok && retryAfter > 0 {
		return retryAfter
	}

	// Exponential backoff
	wait := float64(cfg.BaseWait)
	for i := 0; i < attempt; i++ {
		wait *= cfg.Multiplier
	}

	// Apply max wait
	if wait > float64(cfg.MaxWait) {
		wait = float64(cfg.MaxWait)
	}

	// Apply jitter using crypto/rand
	if cfg.Jitter > 0 {
		jitterRange := wait * cfg.Jitter
		n, err := rand.Int(rand.Reader, big.NewInt(int64(jitterRange*2)))
		if err == nil {
			jitter := float64(n.Int64()) - jitterRange
			wait += jitter
		}
	}

	return time.Duration(wait)
}

// SingleFlight prevents duplicate concurrent calls for the same key.
type SingleFlight[T any] struct {
	mu     sync.Mutex
	calls  map[string]*call[T]
}

type call[T any] struct {
	done   chan struct{}
	result T
	err    error
}

// Do executes fn only once for concurrent calls with the same key.
func (sf *SingleFlight[T]) Do(key string, fn func() (T, error)) (T, error) {
	sf.mu.Lock()
	if sf.calls == nil {
		sf.calls = make(map[string]*call[T])
	}

	if c, ok := sf.calls[key]; ok {
		sf.mu.Unlock()
		<-c.done
		return c.result, c.err
	}

	c := &call[T]{done: make(chan struct{})}
	sf.calls[key] = c
	sf.mu.Unlock()

	c.result, c.err = fn()
	close(c.done)

	sf.mu.Lock()
	delete(sf.calls, key)
	sf.mu.Unlock()

	return c.result, c.err
}
