package receiver

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/prilive-com/galigo/tg"
	"github.com/sony/gobreaker/v2"
	"golang.org/x/time/rate"
)

var _ http.Handler = (*WebhookHandler)(nil)

// WebhookHandler implements http.Handler for Telegram webhook callbacks.
type WebhookHandler struct {
	logger        *slog.Logger
	webhookSecret string
	allowedDomain string
	updates       chan<- tg.Update

	limiter     *rate.Limiter
	breaker     *gobreaker.CircuitBreaker[any]
	bufferPool  sync.Pool
	maxBodySize int64
}

// WebhookOption configures the WebhookHandler.
type WebhookOption func(*WebhookHandler)

// WithWebhookRateLimit sets rate limiting parameters.
func WithWebhookRateLimit(rps float64, burst int) WebhookOption {
	return func(h *WebhookHandler) {
		h.limiter = rate.NewLimiter(rate.Limit(rps), burst)
	}
}

// WithWebhookCircuitBreaker sets a custom circuit breaker.
func WithWebhookCircuitBreaker(breaker *gobreaker.CircuitBreaker[any]) WebhookOption {
	return func(h *WebhookHandler) {
		h.breaker = breaker
	}
}

// WithWebhookMaxBodySize sets the maximum request body size.
func WithWebhookMaxBodySize(size int64) WebhookOption {
	return func(h *WebhookHandler) {
		h.maxBodySize = size
		h.bufferPool = sync.Pool{
			New: func() interface{} {
				b := make([]byte, size)
				return &b
			},
		}
	}
}

// NewWebhookHandler creates a new webhook handler.
func NewWebhookHandler(
	logger *slog.Logger,
	updates chan<- tg.Update,
	cfg Config,
	opts ...WebhookOption,
) *WebhookHandler {
	h := &WebhookHandler{
		logger:        logger,
		webhookSecret: cfg.WebhookSecret,
		allowedDomain: cfg.AllowedDomain,
		updates:       updates,
		limiter:       rate.NewLimiter(rate.Limit(cfg.RateLimitRequests), cfg.RateLimitBurst),
		maxBodySize:   cfg.MaxBodySize,
		bufferPool: sync.Pool{
			New: func() interface{} {
				b := make([]byte, cfg.MaxBodySize)
				return &b
			},
		},
	}

	// Default circuit breaker
	h.breaker = gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		Name:        "galigo-webhook",
		MaxRequests: cfg.BreakerMaxRequests,
		Interval:    cfg.BreakerInterval,
		Timeout:     cfg.BreakerTimeout,
	})

	for _, opt := range opts {
		opt(h)
	}

	return h
}

// ServeHTTP implements http.Handler.
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Rate limit check
	if !h.limiter.Allow() {
		h.fail(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// Process through circuit breaker
	_, err := h.breaker.Execute(func() (interface{}, error) {
		// Domain validation
		if h.allowedDomain != "" && r.Host != h.allowedDomain {
			return nil, ErrForbidden
		}

		// Secret validation (constant-time comparison)
		if h.webhookSecret != "" {
			secret := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
			if subtle.ConstantTimeCompare([]byte(secret), []byte(h.webhookSecret)) != 1 {
				return nil, ErrUnauthorized
			}
		}

		// Method validation
		if r.Method != http.MethodPost {
			return nil, ErrMethodNotAllowed
		}

		// Get pooled buffer
		bufPtr := h.bufferPool.Get().(*[]byte)
		buffer := *bufPtr
		defer h.bufferPool.Put(bufPtr)

		// Read body with size limit
		r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)
		n, err := io.ReadFull(r.Body, buffer)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return nil, &WebhookError{Code: 500, Message: "failed to read body", Err: err}
		}
		defer r.Body.Close()

		// Parse update
		var update tg.Update
		if err := json.Unmarshal(buffer[:n], &update); err != nil {
			return nil, &WebhookError{Code: 400, Message: "invalid JSON", Err: err}
		}

		// Send to channel
		select {
		case h.updates <- update:
			h.logger.Info("update forwarded", "update_id", update.UpdateID)
		default:
			return nil, ErrChannelBlocked
		}

		return nil, nil
	})

	if err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			h.fail(w, "forbidden", http.StatusForbidden)
		case errors.Is(err, ErrUnauthorized):
			h.fail(w, "unauthorized", http.StatusUnauthorized)
		case errors.Is(err, ErrMethodNotAllowed):
			h.fail(w, "method not allowed", http.StatusMethodNotAllowed)
		case errors.Is(err, ErrChannelBlocked):
			h.fail(w, "service unavailable", http.StatusServiceUnavailable)
		default:
			var webhookErr *WebhookError
			if errors.As(err, &webhookErr) {
				h.fail(w, webhookErr.Message, webhookErr.Code)
			} else {
				h.fail(w, err.Error(), http.StatusInternalServerError)
			}
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *WebhookHandler) fail(w http.ResponseWriter, msg string, code int) {
	h.logger.Error(msg, "code", code)
	http.Error(w, msg, code)
}

// HealthHandler returns HTTP handlers for health checks.
type HealthHandler struct {
	ready *readinessState
}

type readinessState struct {
	ready atomic.Bool
}

// NewHealthHandler creates health check handlers.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{
		ready: &readinessState{},
	}
}

// SetReady marks the service as ready.
func (h *HealthHandler) SetReady(ready bool) {
	h.ready.ready.Store(ready)
}

// LivenessHandler returns the liveness probe handler.
func (h *HealthHandler) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}

// ReadinessHandler returns the readiness probe handler.
func (h *HealthHandler) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ready.ready.Load() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Ready"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Not Ready"))
		}
	}
}
