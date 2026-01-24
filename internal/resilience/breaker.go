package resilience

import (
	"time"

	"github.com/sony/gobreaker/v2"
)

// BreakerConfig holds circuit breaker configuration.
type BreakerConfig struct {
	Name          string
	MaxRequests   uint32        // Max requests in half-open state
	Interval      time.Duration // Counting interval for failures
	Timeout       time.Duration // Timeout before half-open
	Threshold     uint32        // Failures before opening
	FailureRatio  float64       // Ratio threshold (0.5 = 50%)
	MinRequests   uint32        // Minimum requests before checking ratio
	OnStateChange func(name string, from, to string)
}

// DefaultBreakerConfig returns sensible defaults.
func DefaultBreakerConfig(name string) BreakerConfig {
	return BreakerConfig{
		Name:         name,
		MaxRequests:  5,
		Interval:     60 * time.Second,
		Timeout:      30 * time.Second,
		Threshold:    5,
		FailureRatio: 0.5,
		MinRequests:  10,
	}
}

// NewBreaker creates a new circuit breaker with the given configuration.
func NewBreaker[T any](cfg BreakerConfig) *gobreaker.CircuitBreaker[T] {
	settings := gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Trip on consecutive failures
			if counts.ConsecutiveFailures >= cfg.Threshold {
				return true
			}
			// Trip on failure ratio if enough requests
			if counts.Requests >= cfg.MinRequests {
				failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
				return failureRatio >= cfg.FailureRatio
			}
			return false
		},
	}

	if cfg.OnStateChange != nil {
		settings.OnStateChange = func(name string, from, to gobreaker.State) {
			cfg.OnStateChange(name, from.String(), to.String())
		}
	}

	return gobreaker.NewCircuitBreaker[T](settings)
}

// NewDefaultBreaker creates a breaker with default settings.
func NewDefaultBreaker[T any](name string) *gobreaker.CircuitBreaker[T] {
	return NewBreaker[T](DefaultBreakerConfig(name))
}

// IsOpen returns true if the circuit breaker is in the open state.
func IsOpen[T any](cb *gobreaker.CircuitBreaker[T]) bool {
	return cb.State() == gobreaker.StateOpen
}

// Counts returns the current counts from the circuit breaker.
func Counts[T any](cb *gobreaker.CircuitBreaker[T]) gobreaker.Counts {
	return cb.Counts()
}
