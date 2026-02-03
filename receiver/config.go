package receiver

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prilive-com/galigo/tg"
)

// Mode defines how the receiver gets updates from Telegram.
type Mode string

const (
	ModeWebhook     Mode = "webhook"
	ModeLongPolling Mode = "longpolling"
)

// UpdateDeliveryPolicy defines how updates are handled when the channel is full.
type UpdateDeliveryPolicy int

const (
	// DeliveryPolicyBlock waits for channel space (with timeout).
	// This is the safest default - no updates lost unless timeout.
	DeliveryPolicyBlock UpdateDeliveryPolicy = iota

	// DeliveryPolicyDropNewest drops the current update if channel is full.
	// Offset advances - update is lost but polling continues.
	DeliveryPolicyDropNewest

	// DeliveryPolicyDropOldest drops oldest update to make room.
	// Uses non-blocking receive/send pattern.
	DeliveryPolicyDropOldest
)

// Config holds receiver configuration.
type Config struct {
	// Mode selection
	Mode Mode

	// Bot token
	Token tg.SecretToken

	// API URL (defaults to https://api.telegram.org/bot)
	BaseURL string

	// Webhook configuration
	WebhookPort   int
	TLSCertPath   string
	TLSKeyPath    string
	WebhookSecret string
	AllowedDomain string
	WebhookURL    string // Public URL for auto-registration

	// Long polling configuration
	PollingTimeout     int           // Seconds to wait (0-60)
	PollingLimit       int           // Max updates per request (1-100)
	PollingMaxErrors   int           // Max consecutive errors (0 = unlimited)
	DeleteWebhookFirst bool          // Delete webhook before starting
	AllowedUpdates     []string      // Filter update types
	RetryInitialDelay  time.Duration // Initial retry delay
	RetryMaxDelay      time.Duration // Maximum retry delay
	RetryBackoffFactor float64       // Backoff multiplier

	// Common configuration
	UpdateBufferSize  int     // Channel buffer size
	RateLimitRequests float64 // Requests per second
	RateLimitBurst    int     // Burst size
	MaxBodySize       int64   // Max webhook body size

	// Update delivery policy (for long polling)
	UpdateDeliveryPolicy  UpdateDeliveryPolicy // Behavior when update channel is full
	UpdateDeliveryTimeout time.Duration        // Max time to wait in Block mode (0 = block forever)
	OnUpdateDropped       func(int, string)    // Callback when update is dropped (updateID, reason)

	// Circuit breaker
	BreakerMaxRequests uint32
	BreakerInterval    time.Duration
	BreakerTimeout     time.Duration

	// Server timeouts
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration

	// Shutdown
	DrainDelay      time.Duration // Wait for LB before shutdown
	ShutdownTimeout time.Duration
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Mode:                  ModeWebhook,
		WebhookPort:           8443,
		PollingTimeout:        30,
		PollingLimit:          100,
		PollingMaxErrors:      10,
		DeleteWebhookFirst:    false,
		RetryInitialDelay:     time.Second,
		RetryMaxDelay:         60 * time.Second,
		RetryBackoffFactor:    2.0,
		UpdateBufferSize:      100,
		RateLimitRequests:     10,
		RateLimitBurst:        20,
		MaxBodySize:           1 << 20, // 1MB
		UpdateDeliveryPolicy:  DeliveryPolicyBlock,
		UpdateDeliveryTimeout: 5 * time.Second,
		BreakerMaxRequests:    5,
		BreakerInterval:       2 * time.Minute,
		BreakerTimeout:        60 * time.Second,
		ReadTimeout:           10 * time.Second,
		ReadHeaderTimeout:     2 * time.Second,
		WriteTimeout:          15 * time.Second,
		IdleTimeout:           120 * time.Second,
		DrainDelay:            5 * time.Second,
		ShutdownTimeout:       15 * time.Second,
	}
}

// LoadConfig loads configuration from environment variables.
func LoadConfig() (*Config, error) {
	cfg := DefaultConfig()

	// Mode
	modeStr := getEnv("RECEIVER_MODE", "webhook")
	switch strings.ToLower(modeStr) {
	case "webhook":
		cfg.Mode = ModeWebhook
	case "longpolling":
		cfg.Mode = ModeLongPolling
	default:
		return nil, tg.NewValidationError("RECEIVER_MODE", "must be 'webhook' or 'longpolling'")
	}

	// Token
	cfg.Token = tg.SecretToken(getEnv("TELEGRAM_BOT_TOKEN", ""))

	// Webhook settings
	if port, err := strconv.Atoi(getEnv("WEBHOOK_PORT", "8443")); err == nil {
		cfg.WebhookPort = port
	}
	cfg.TLSCertPath = getEnv("TLS_CERT_PATH", "")
	cfg.TLSKeyPath = getEnv("TLS_KEY_PATH", "")
	cfg.WebhookSecret = getEnv("WEBHOOK_SECRET", "")
	cfg.AllowedDomain = getEnv("ALLOWED_DOMAIN", "")
	cfg.WebhookURL = getEnv("WEBHOOK_URL", "")

	// Validate webhook URL if provided
	if cfg.WebhookURL != "" && !strings.HasPrefix(cfg.WebhookURL, "https://") {
		return nil, tg.NewValidationError("WEBHOOK_URL", "must start with https://")
	}

	// Polling settings
	if timeout, err := strconv.Atoi(getEnv("POLLING_TIMEOUT", "30")); err == nil {
		if timeout < 0 || timeout > 60 {
			return nil, tg.NewValidationError("POLLING_TIMEOUT", "must be 0-60")
		}
		cfg.PollingTimeout = timeout
	}

	if limit, err := strconv.Atoi(getEnv("POLLING_LIMIT", "100")); err == nil {
		if limit < 1 || limit > 100 {
			return nil, tg.NewValidationError("POLLING_LIMIT", "must be 1-100")
		}
		cfg.PollingLimit = limit
	}

	if maxErrors, err := strconv.Atoi(getEnv("POLLING_MAX_ERRORS", "10")); err == nil {
		cfg.PollingMaxErrors = maxErrors
	}

	cfg.DeleteWebhookFirst = strings.ToLower(getEnv("POLLING_DELETE_WEBHOOK", "false")) == "true"

	// Allowed updates
	if updates := getEnv("ALLOWED_UPDATES", ""); updates != "" {
		for _, u := range strings.Split(updates, ",") {
			if trimmed := strings.TrimSpace(u); trimmed != "" {
				cfg.AllowedUpdates = append(cfg.AllowedUpdates, trimmed)
			}
		}
	}

	// Retry config
	if d, err := time.ParseDuration(getEnv("POLLING_RETRY_INITIAL_DELAY", "1s")); err == nil {
		cfg.RetryInitialDelay = d
	}
	if d, err := time.ParseDuration(getEnv("POLLING_RETRY_MAX_DELAY", "60s")); err == nil {
		cfg.RetryMaxDelay = d
	}
	if f, err := strconv.ParseFloat(getEnv("POLLING_RETRY_BACKOFF_FACTOR", "2.0"), 64); err == nil {
		cfg.RetryBackoffFactor = f
	}

	// Rate limiting
	if f, err := strconv.ParseFloat(getEnv("RATE_LIMIT_REQUESTS", "10"), 64); err == nil {
		cfg.RateLimitRequests = f
	}
	if i, err := strconv.Atoi(getEnv("RATE_LIMIT_BURST", "20")); err == nil {
		cfg.RateLimitBurst = i
	}

	// Body size
	if i, err := strconv.ParseInt(getEnv("MAX_BODY_SIZE", "1048576"), 10, 64); err == nil {
		cfg.MaxBodySize = i
	}

	// Update delivery policy
	policyStr := strings.ToLower(getEnv("UPDATE_DELIVERY_POLICY", "block"))
	switch policyStr {
	case "block":
		cfg.UpdateDeliveryPolicy = DeliveryPolicyBlock
	case "drop_newest", "dropnewest":
		cfg.UpdateDeliveryPolicy = DeliveryPolicyDropNewest
	case "drop_oldest", "dropoldest":
		cfg.UpdateDeliveryPolicy = DeliveryPolicyDropOldest
	default:
		return nil, tg.NewValidationError("UPDATE_DELIVERY_POLICY", "must be 'block', 'drop_newest', or 'drop_oldest'")
	}

	if d, err := time.ParseDuration(getEnv("UPDATE_DELIVERY_TIMEOUT", "5s")); err == nil {
		cfg.UpdateDeliveryTimeout = d
	}

	// Circuit breaker
	if i, err := strconv.ParseUint(getEnv("BREAKER_MAX_REQUESTS", "5"), 10, 32); err == nil {
		cfg.BreakerMaxRequests = uint32(i)
	}
	if d, err := time.ParseDuration(getEnv("BREAKER_INTERVAL", "2m")); err == nil {
		cfg.BreakerInterval = d
	}
	if d, err := time.ParseDuration(getEnv("BREAKER_TIMEOUT", "60s")); err == nil {
		cfg.BreakerTimeout = d
	}

	// Server timeouts
	if d, err := time.ParseDuration(getEnv("READ_TIMEOUT", "10s")); err == nil {
		cfg.ReadTimeout = d
	}
	if d, err := time.ParseDuration(getEnv("READ_HEADER_TIMEOUT", "2s")); err == nil {
		cfg.ReadHeaderTimeout = d
	}
	if d, err := time.ParseDuration(getEnv("WRITE_TIMEOUT", "15s")); err == nil {
		cfg.WriteTimeout = d
	}
	if d, err := time.ParseDuration(getEnv("IDLE_TIMEOUT", "120s")); err == nil {
		cfg.IdleTimeout = d
	}

	// Shutdown
	if d, err := time.ParseDuration(getEnv("DRAIN_DELAY", "5s")); err == nil {
		cfg.DrainDelay = d
	}
	if d, err := time.ParseDuration(getEnv("SHUTDOWN_TIMEOUT", "15s")); err == nil {
		cfg.ShutdownTimeout = d
	}

	return &cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
