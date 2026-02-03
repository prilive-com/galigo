package sender

import (
	"os"
	"strconv"
	"time"

	"github.com/prilive-com/galigo/tg"
)

// Config holds sender configuration.
type Config struct {
	// Bot token
	Token tg.SecretToken

	// API settings
	BaseURL        string
	RequestTimeout time.Duration
	KeepAlive      time.Duration
	MaxIdleConns   int
	IdleTimeout    time.Duration

	// Rate limiting
	GlobalRPS    float64
	GlobalBurst  int
	PerChatRPS   float64
	PerChatBurst int
	GroupRPS        float64 // Rate limit for group chats (negative chat IDs). 0 = use PerChatRPS.
	GroupBurst      int     // Burst for group chats. 0 = use PerChatBurst.
	MaxChatLimiters int     // Maximum number of per-chat limiters to prevent memory exhaustion. 0 = 10000.

	// Circuit breaker
	BreakerMaxRequests uint32
	BreakerInterval    time.Duration
	BreakerTimeout     time.Duration

	// Retry settings
	MaxRetries    int
	RetryBaseWait time.Duration
	RetryMaxWait  time.Duration
	RetryFactor   float64

	// Content limits
	MaxTextLength    int
	MaxCaptionLength int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:            "https://api.telegram.org",
		RequestTimeout:     30 * time.Second,
		KeepAlive:          30 * time.Second,
		MaxIdleConns:       100,
		IdleTimeout:        90 * time.Second,
		GlobalRPS:          30,
		GlobalBurst:        10,
		PerChatRPS:         1,
		PerChatBurst:       3,
		GroupRPS:           0.33, // ~20/min — Telegram's group chat limit
		GroupBurst:         2,
		MaxChatLimiters:    10000,
		BreakerMaxRequests: 5,
		BreakerInterval:    60 * time.Second,
		BreakerTimeout:     30 * time.Second,
		MaxRetries:         3,
		RetryBaseWait:      time.Second,
		RetryMaxWait:       30 * time.Second,
		RetryFactor:        2.0,
		MaxTextLength:      4096,
		MaxCaptionLength:   1024,
	}
}

// LoadConfig loads configuration from environment variables.
func LoadConfig() (*Config, error) {
	cfg := DefaultConfig()

	cfg.Token = tg.SecretToken(getEnv("TELEGRAM_BOT_TOKEN", ""))

	if url := getEnv("TELEGRAM_API_BASE_URL", ""); url != "" {
		cfg.BaseURL = url
	}

	if d, err := time.ParseDuration(getEnv("REQUEST_TIMEOUT", "30s")); err == nil {
		cfg.RequestTimeout = d
	}

	if f, err := strconv.ParseFloat(getEnv("RATE_LIMIT_REQUESTS", "30"), 64); err == nil {
		cfg.GlobalRPS = f
	}

	if i, err := strconv.Atoi(getEnv("RATE_LIMIT_BURST", "10")); err == nil {
		cfg.GlobalBurst = i
	}

	if f, err := strconv.ParseFloat(getEnv("PER_CHAT_RPS", "1"), 64); err == nil {
		cfg.PerChatRPS = f
	}

	if i, err := strconv.Atoi(getEnv("PER_CHAT_BURST", "3")); err == nil {
		cfg.PerChatBurst = i
	}

	if f, err := strconv.ParseFloat(getEnv("GROUP_RPS", "0.33"), 64); err == nil {
		cfg.GroupRPS = f
	}

	if i, err := strconv.Atoi(getEnv("GROUP_BURST", "2")); err == nil {
		cfg.GroupBurst = i
	}

	if i, err := strconv.ParseUint(getEnv("BREAKER_MAX_REQUESTS", "5"), 10, 32); err == nil {
		cfg.BreakerMaxRequests = uint32(i)
	}

	if d, err := time.ParseDuration(getEnv("BREAKER_INTERVAL", "60s")); err == nil {
		cfg.BreakerInterval = d
	}

	if d, err := time.ParseDuration(getEnv("BREAKER_TIMEOUT", "30s")); err == nil {
		cfg.BreakerTimeout = d
	}

	if i, err := strconv.Atoi(getEnv("MAX_RETRIES", "3")); err == nil {
		cfg.MaxRetries = i
	}

	if d, err := time.ParseDuration(getEnv("RETRY_BASE_WAIT", "1s")); err == nil {
		cfg.RetryBaseWait = d
	}

	if d, err := time.ParseDuration(getEnv("RETRY_MAX_WAIT", "30s")); err == nil {
		cfg.RetryMaxWait = d
	}

	if f, err := strconv.ParseFloat(getEnv("RETRY_FACTOR", "2.0"), 64); err == nil {
		cfg.RetryFactor = f
	}

	if i, err := strconv.Atoi(getEnv("MAX_TEXT_LENGTH", "4096")); err == nil {
		cfg.MaxTextLength = i
	}

	if i, err := strconv.Atoi(getEnv("MAX_CAPTION_LENGTH", "1024")); err == nil {
		cfg.MaxCaptionLength = i
	}

	return &cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
