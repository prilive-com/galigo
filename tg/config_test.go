package tg_test

import (
	"testing"
	"time"

	"github.com/prilive-com/galigo/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== DefaultConfig ====================

func TestDefaultConfig_HasSensibleDefaults(t *testing.T) {
	cfg := tg.DefaultConfig()

	assert.Equal(t, "https://api.telegram.org", cfg.BaseURL)
	assert.Equal(t, 30*time.Second, cfg.RequestTimeout)
	assert.Equal(t, 10*time.Second, cfg.ConnectTimeout)
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, time.Second, cfg.RetryBaseWait)
	assert.Equal(t, 30*time.Second, cfg.RetryMaxWait)
	assert.Equal(t, float64(30), cfg.RateLimit)
	assert.Equal(t, 5, cfg.RateLimitBurst)
	assert.Equal(t, float64(1), cfg.PerChatRPS)
	assert.Equal(t, 3, cfg.PerChatBurst)
	assert.Equal(t, float64(30), cfg.GlobalRPS)
	assert.Equal(t, 10, cfg.GlobalBurst)
	assert.True(t, cfg.CircuitBreakerEnabled)
	assert.Equal(t, uint32(5), cfg.CircuitBreakerThreshold)
	assert.Equal(t, 60*time.Second, cfg.CircuitBreakerInterval)
	assert.Equal(t, 30*time.Second, cfg.CircuitBreakerTimeout)
	assert.Equal(t, tg.ParseMode(""), cfg.DefaultParseMode)
	assert.True(t, cfg.AllowSendingWithoutReply)
}

// ==================== Validate ====================

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := tg.DefaultConfig()
	cfg.Token = tg.SecretToken("123456:ABC")

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestConfig_Validate_MissingToken(t *testing.T) {
	cfg := tg.DefaultConfig()
	// Token not set

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token")
}

func TestConfig_Validate_InvalidTimeout(t *testing.T) {
	cfg := tg.DefaultConfig()
	cfg.Token = tg.SecretToken("123456:ABC")
	cfg.RequestTimeout = 0

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request_timeout")
}

func TestConfig_Validate_NegativeRetries(t *testing.T) {
	cfg := tg.DefaultConfig()
	cfg.Token = tg.SecretToken("123456:ABC")
	cfg.MaxRetries = -1

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_retries")
}

func TestConfig_Validate_NegativeRateLimit(t *testing.T) {
	cfg := tg.DefaultConfig()
	cfg.Token = tg.SecretToken("123456:ABC")
	cfg.RateLimit = -1

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate_limit")
}

func TestConfig_Validate_InvalidParseMode(t *testing.T) {
	cfg := tg.DefaultConfig()
	cfg.Token = tg.SecretToken("123456:ABC")
	cfg.DefaultParseMode = tg.ParseMode("invalid")

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse_mode")
}

// ==================== With* Methods ====================

func TestConfig_WithToken(t *testing.T) {
	cfg := tg.DefaultConfig()
	newCfg := cfg.WithToken("new:token")

	// Original unchanged
	assert.True(t, cfg.Token.IsEmpty())
	// New config has token
	assert.Equal(t, "new:token", newCfg.Token.Value())
}

func TestConfig_WithBaseURL(t *testing.T) {
	cfg := tg.DefaultConfig()
	newCfg := cfg.WithBaseURL("https://custom.api.com")

	assert.Equal(t, "https://api.telegram.org", cfg.BaseURL)
	assert.Equal(t, "https://custom.api.com", newCfg.BaseURL)
}

func TestConfig_WithTimeout(t *testing.T) {
	cfg := tg.DefaultConfig()
	newCfg := cfg.WithTimeout(60 * time.Second)

	assert.Equal(t, 30*time.Second, cfg.RequestTimeout)
	assert.Equal(t, 60*time.Second, newCfg.RequestTimeout)
}

func TestConfig_WithRetries(t *testing.T) {
	cfg := tg.DefaultConfig()
	newCfg := cfg.WithRetries(5)

	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 5, newCfg.MaxRetries)
}

func TestConfig_WithParseMode(t *testing.T) {
	cfg := tg.DefaultConfig()
	newCfg := cfg.WithParseMode(tg.ParseModeHTML)

	assert.Equal(t, tg.ParseMode(""), cfg.DefaultParseMode)
	assert.Equal(t, tg.ParseModeHTML, newCfg.DefaultParseMode)
}

func TestConfig_WithCircuitBreaker(t *testing.T) {
	cfg := tg.DefaultConfig()
	newCfg := cfg.WithCircuitBreaker(false, 10, 60*time.Second)

	// Original unchanged
	assert.True(t, cfg.CircuitBreakerEnabled)
	assert.Equal(t, uint32(5), cfg.CircuitBreakerThreshold)

	// New config updated
	assert.False(t, newCfg.CircuitBreakerEnabled)
	assert.Equal(t, uint32(10), newCfg.CircuitBreakerThreshold)
	assert.Equal(t, 60*time.Second, newCfg.CircuitBreakerTimeout)
}

func TestConfig_WithRateLimits(t *testing.T) {
	cfg := tg.DefaultConfig()
	newCfg := cfg.WithRateLimits(60, 2)

	assert.Equal(t, float64(30), cfg.GlobalRPS)
	assert.Equal(t, float64(1), cfg.PerChatRPS)

	assert.Equal(t, float64(60), newCfg.GlobalRPS)
	assert.Equal(t, float64(2), newCfg.PerChatRPS)
}

func TestConfig_MethodChaining(t *testing.T) {
	cfg := tg.DefaultConfig().
		WithToken("123:ABC").
		WithBaseURL("https://custom.api.com").
		WithTimeout(45 * time.Second).
		WithRetries(5).
		WithParseMode(tg.ParseModeMarkdownV2)

	assert.Equal(t, "123:ABC", cfg.Token.Value())
	assert.Equal(t, "https://custom.api.com", cfg.BaseURL)
	assert.Equal(t, 45*time.Second, cfg.RequestTimeout)
	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, tg.ParseModeMarkdownV2, cfg.DefaultParseMode)
}
