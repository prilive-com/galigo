package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds testbot configuration.
type Config struct {
	// Required
	Token  string
	ChatID int64
	Admins []int64

	// General
	Mode       string // "polling" or "webhook"
	StorageDir string
	LogLevel   string

	// Safety limits
	MaxMessagesPerRun int
	SendInterval      time.Duration
	JitterInterval    time.Duration
	RetryOn429        bool
	Max429Retries     int
	AllowStress       bool

	// Webhook (for future use)
	WebhookPublicURL   string
	WebhookSecretToken string
	ListenAddr         string

	// Derived
	Timeout time.Duration
}

// Load loads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		Token:             os.Getenv("TESTBOT_TOKEN"),
		Mode:              getEnvDefault("TESTBOT_MODE", "polling"),
		StorageDir:        getEnvDefault("TESTBOT_STORAGE_DIR", "./var"),
		LogLevel:          getEnvDefault("TESTBOT_LOG_LEVEL", "info"),
		MaxMessagesPerRun: getEnvIntDefault("TESTBOT_MAX_MESSAGES_PER_RUN", 40),
		SendInterval:      getEnvDurationDefault("TESTBOT_SEND_INTERVAL", 350*time.Millisecond),
		JitterInterval:    getEnvDurationDefault("TESTBOT_JITTER_INTERVAL", 150*time.Millisecond),
		RetryOn429:        getEnvDefault("TESTBOT_RETRY_429", "true") == "true",
		Max429Retries:     getEnvIntDefault("TESTBOT_MAX_429_RETRIES", 2),
		AllowStress:       os.Getenv("TESTBOT_ALLOW_STRESS") == "true",
		ListenAddr:        getEnvDefault("TESTBOT_LISTEN_ADDR", ":8080"),
		Timeout:           30 * time.Second,
	}

	if cfg.Token == "" {
		return nil, fmt.Errorf("TESTBOT_TOKEN required")
	}

	// Parse chat ID
	chatIDStr := os.Getenv("TESTBOT_CHAT_ID")
	if chatIDStr == "" {
		return nil, fmt.Errorf("TESTBOT_CHAT_ID required")
	}
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid TESTBOT_CHAT_ID: %w", err)
	}
	cfg.ChatID = chatID

	// Parse admins
	adminsStr := os.Getenv("TESTBOT_ADMINS")
	if adminsStr == "" {
		return nil, fmt.Errorf("TESTBOT_ADMINS required")
	}
	for _, s := range strings.Split(adminsStr, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			continue
		}
		cfg.Admins = append(cfg.Admins, id)
	}
	if len(cfg.Admins) == 0 {
		return nil, fmt.Errorf("at least one admin required in TESTBOT_ADMINS")
	}

	// Webhook config (optional)
	cfg.WebhookPublicURL = os.Getenv("TESTBOT_WEBHOOK_PUBLIC_URL")
	cfg.WebhookSecretToken = os.Getenv("TESTBOT_WEBHOOK_SECRET_TOKEN")

	return cfg, nil
}

// IsAdmin checks if a user ID is in the admin list.
func (c *Config) IsAdmin(userID int64) bool {
	for _, id := range c.Admins {
		if id == userID {
			return true
		}
	}
	return false
}

func getEnvDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvIntDefault(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvDurationDefault(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
