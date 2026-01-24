package tg

import "time"

// Config holds shared configuration for Telegram operations.
// Use functional options or environment variables to customize.
type Config struct {
	// BaseURL is the Telegram Bot API base URL.
	// Default: https://api.telegram.org
	BaseURL string

	// Token is the bot authentication token.
	Token SecretToken

	// Timeouts
	RequestTimeout time.Duration // Default: 30s
	ConnectTimeout time.Duration // Default: 10s

	// Retry settings
	MaxRetries    int           // Default: 3
	RetryBaseWait time.Duration // Default: 1s
	RetryMaxWait  time.Duration // Default: 30s

	// Rate limiting
	RateLimit       float64 // Requests per second, 0 = disabled
	RateLimitBurst  int     // Burst size for rate limiter
	PerChatRPS      float64 // Per-chat rate limit
	PerChatBurst    int     // Per-chat burst size
	GlobalRPS       float64 // Global rate limit
	GlobalBurst     int     // Global burst size

	// Circuit breaker
	CircuitBreakerEnabled   bool
	CircuitBreakerThreshold uint32        // Failure threshold
	CircuitBreakerInterval  time.Duration // Counting interval
	CircuitBreakerTimeout   time.Duration // Reset timeout

	// Default message options
	DefaultParseMode          ParseMode
	DisableWebPagePreview     bool
	DisableNotification       bool
	ProtectContent            bool
	AllowSendingWithoutReply  bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:                   "https://api.telegram.org",
		RequestTimeout:            30 * time.Second,
		ConnectTimeout:            10 * time.Second,
		MaxRetries:                3,
		RetryBaseWait:             time.Second,
		RetryMaxWait:              30 * time.Second,
		RateLimit:                 30, // Telegram limit: ~30 msg/s
		RateLimitBurst:            5,
		PerChatRPS:                1,  // 1 msg/s per chat recommended
		PerChatBurst:              3,
		GlobalRPS:                 30,
		GlobalBurst:               10,
		CircuitBreakerEnabled:     true,
		CircuitBreakerThreshold:   5,
		CircuitBreakerInterval:    60 * time.Second,
		CircuitBreakerTimeout:     30 * time.Second,
		DefaultParseMode:          "",
		DisableWebPagePreview:     false,
		DisableNotification:       false,
		ProtectContent:            false,
		AllowSendingWithoutReply:  true,
	}
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.Token.IsEmpty() {
		return NewValidationError("token", "bot token is required")
	}
	if c.RequestTimeout <= 0 {
		return NewValidationError("request_timeout", "must be positive")
	}
	if c.MaxRetries < 0 {
		return NewValidationError("max_retries", "cannot be negative")
	}
	if c.RateLimit < 0 {
		return NewValidationError("rate_limit", "cannot be negative")
	}
	if !c.DefaultParseMode.IsValid() {
		return NewValidationError("parse_mode", "invalid parse mode")
	}
	return nil
}

// WithToken returns a copy of the config with the given token.
func (c Config) WithToken(token string) Config {
	c.Token = SecretToken(token)
	return c
}

// WithBaseURL returns a copy of the config with the given base URL.
func (c Config) WithBaseURL(url string) Config {
	c.BaseURL = url
	return c
}

// WithTimeout returns a copy of the config with the given request timeout.
func (c Config) WithTimeout(d time.Duration) Config {
	c.RequestTimeout = d
	return c
}

// WithRetries returns a copy of the config with the given max retries.
func (c Config) WithRetries(n int) Config {
	c.MaxRetries = n
	return c
}

// WithParseMode returns a copy of the config with the given default parse mode.
func (c Config) WithParseMode(mode ParseMode) Config {
	c.DefaultParseMode = mode
	return c
}

// WithCircuitBreaker returns a copy with circuit breaker settings.
func (c Config) WithCircuitBreaker(enabled bool, threshold uint32, timeout time.Duration) Config {
	c.CircuitBreakerEnabled = enabled
	c.CircuitBreakerThreshold = threshold
	c.CircuitBreakerTimeout = timeout
	return c
}

// WithRateLimits returns a copy with rate limiting settings.
func (c Config) WithRateLimits(globalRPS float64, perChatRPS float64) Config {
	c.GlobalRPS = globalRPS
	c.PerChatRPS = perChatRPS
	return c
}
