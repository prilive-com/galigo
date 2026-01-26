package testutil

import (
	"testing"
	"time"

	"github.com/prilive-com/galigo/sender"
	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/require"
)

// CircuitBreakerNeverTrip returns settings where breaker never opens.
// Use for retry tests that need to verify retry behavior without breaker interference.
func CircuitBreakerNeverTrip() sender.CircuitBreakerSettings {
	return sender.CircuitBreakerSettings{
		MaxRequests: 100,
		Interval:    0,
		Timeout:     time.Hour,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return false // Never trip
		},
	}
}

// CircuitBreakerAggressiveTrip returns settings for testing breaker behavior.
// Trips after just 2 consecutive failures.
func CircuitBreakerAggressiveTrip() sender.CircuitBreakerSettings {
	return sender.CircuitBreakerSettings{
		MaxRequests: 1,
		Interval:    0,
		Timeout:     2 * time.Second, // Long enough to stay open during test assertions
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 2
		},
	}
}

// NewRetryTestClient creates a client for testing retry behavior.
// Circuit breaker is configured to never trip.
func NewRetryTestClient(t *testing.T, baseURL string, sleeper *FakeSleeper, opts ...sender.Option) *sender.Client {
	t.Helper()

	defaultOpts := []sender.Option{
		sender.WithBaseURL(baseURL),
		sender.WithCircuitBreakerSettings(CircuitBreakerNeverTrip()),
	}

	if sleeper != nil {
		defaultOpts = append(defaultOpts, sender.WithSleeper(sleeper))
	}

	client, err := sender.New(TestToken, append(defaultOpts, opts...)...)
	require.NoError(t, err)

	t.Cleanup(func() { client.Close() })
	return client
}

// NewBreakerTestClient creates a client for testing circuit breaker behavior.
// Circuit breaker trips aggressively for fast testing.
func NewBreakerTestClient(t *testing.T, baseURL string, opts ...sender.Option) *sender.Client {
	t.Helper()

	defaultOpts := []sender.Option{
		sender.WithBaseURL(baseURL),
		sender.WithCircuitBreakerSettings(CircuitBreakerAggressiveTrip()),
		sender.WithRetries(0), // No retries - test breaker directly
	}

	client, err := sender.New(TestToken, append(defaultOpts, opts...)...)
	require.NoError(t, err)

	t.Cleanup(func() { client.Close() })
	return client
}

// NewTestClient creates a standard test client with sensible defaults.
func NewTestClient(t *testing.T, baseURL string, opts ...sender.Option) *sender.Client {
	t.Helper()

	defaultOpts := []sender.Option{
		sender.WithBaseURL(baseURL),
		sender.WithRetries(0), // No retries by default for simple tests
	}

	client, err := sender.New(TestToken, append(defaultOpts, opts...)...)
	require.NoError(t, err)

	t.Cleanup(func() { client.Close() })
	return client
}
