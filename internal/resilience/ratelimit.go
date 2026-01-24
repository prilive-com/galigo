package resilience

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter provides global and per-key rate limiting.
type RateLimiter struct {
	global    *rate.Limiter
	perKey    map[string]*rate.Limiter
	mu        sync.RWMutex
	keyRPS    float64
	keyBurst  int
	cleanupCh chan struct{}
}

// RateLimiterConfig holds rate limiter configuration.
type RateLimiterConfig struct {
	GlobalRPS   float64 // Global requests per second
	GlobalBurst int     // Global burst size
	KeyRPS      float64 // Per-key requests per second
	KeyBurst    int     // Per-key burst size
}

// DefaultRateLimiterConfig returns sensible defaults for Telegram.
func DefaultRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		GlobalRPS:   30, // Telegram ~30 msg/s global
		GlobalBurst: 10,
		KeyRPS:      1,  // 1 msg/s per chat recommended
		KeyBurst:    3,
	}
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(cfg RateLimiterConfig) *RateLimiter {
	rl := &RateLimiter{
		global:    rate.NewLimiter(rate.Limit(cfg.GlobalRPS), cfg.GlobalBurst),
		perKey:    make(map[string]*rate.Limiter),
		keyRPS:    cfg.KeyRPS,
		keyBurst:  cfg.KeyBurst,
		cleanupCh: make(chan struct{}),
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// Wait blocks until both global and per-key limits allow.
func (r *RateLimiter) Wait(ctx context.Context, key string) error {
	// Check global limit first
	if err := r.global.Wait(ctx); err != nil {
		return err
	}

	// Check per-key limit
	limiter := r.getOrCreate(key)
	return limiter.Wait(ctx)
}

// Allow returns true if the request is allowed without blocking.
func (r *RateLimiter) Allow(key string) bool {
	if !r.global.Allow() {
		return false
	}
	limiter := r.getOrCreate(key)
	return limiter.Allow()
}

// Reserve returns a reservation for the given key.
func (r *RateLimiter) Reserve(key string) *rate.Reservation {
	// Note: This only reserves the per-key limit
	// Global limit should be checked separately
	limiter := r.getOrCreate(key)
	return limiter.Reserve()
}

// GlobalAllow checks only the global rate limit.
func (r *RateLimiter) GlobalAllow() bool {
	return r.global.Allow()
}

// GlobalWait waits for the global rate limit.
func (r *RateLimiter) GlobalWait(ctx context.Context) error {
	return r.global.Wait(ctx)
}

// SetGlobalLimit updates the global rate limit.
func (r *RateLimiter) SetGlobalLimit(rps float64, burst int) {
	r.global.SetLimit(rate.Limit(rps))
	r.global.SetBurst(burst)
}

// SetKeyLimit updates the per-key rate limit for new keys.
func (r *RateLimiter) SetKeyLimit(rps float64, burst int) {
	r.mu.Lock()
	r.keyRPS = rps
	r.keyBurst = burst
	r.mu.Unlock()
}

// Close stops the cleanup goroutine.
func (r *RateLimiter) Close() {
	close(r.cleanupCh)
}

func (r *RateLimiter) getOrCreate(key string) *rate.Limiter {
	r.mu.RLock()
	limiter, exists := r.perKey[key]
	r.mu.RUnlock()

	if exists {
		return limiter
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists = r.perKey[key]; exists {
		return limiter
	}

	limiter = rate.NewLimiter(rate.Limit(r.keyRPS), r.keyBurst)
	r.perKey[key] = limiter
	return limiter
}

func (r *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.mu.Lock()
			// Remove limiters that have been idle (could track last access, but simple clear is fine)
			// For now, we keep all limiters as Telegram chats are typically long-lived
			r.mu.Unlock()
		case <-r.cleanupCh:
			return
		}
	}
}

// TokenBucket is a simple token bucket for basic rate limiting.
type TokenBucket struct {
	tokens     float64
	capacity   float64
	refillRate float64
	lastRefill time.Time
	mu         sync.Mutex
}

// NewTokenBucket creates a new token bucket.
func NewTokenBucket(capacity float64, refillRate float64) *TokenBucket {
	return &TokenBucket{
		tokens:     capacity,
		capacity:   capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Take attempts to take n tokens. Returns true if successful.
func (tb *TokenBucket) Take(n float64) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= n {
		tb.tokens -= n
		return true
	}
	return false
}

// Tokens returns the current number of tokens.
func (tb *TokenBucket) Tokens() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refill()
	return tb.tokens
}

func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens = min(tb.capacity, tb.tokens+elapsed*tb.refillRate)
	tb.lastRefill = now
}
