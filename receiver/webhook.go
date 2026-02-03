package receiver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

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
	updatesBidi   chan tg.Update // bidirectional ref for DropOldest; may be nil

	deliveryPolicy  UpdateDeliveryPolicy
	deliveryTimeout time.Duration
	onUpdateDropped func(int, string)

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
// The updates channel must be bidirectional (chan tg.Update) if using DeliveryPolicyDropOldest.
func NewWebhookHandler(
	logger *slog.Logger,
	updates chan tg.Update,
	cfg Config,
	opts ...WebhookOption,
) *WebhookHandler {
	if logger == nil {
		logger = slog.Default()
	}

	h := &WebhookHandler{
		logger:          logger,
		webhookSecret:   cfg.WebhookSecret,
		allowedDomain:   cfg.AllowedDomain,
		updates:         updates,
		updatesBidi:     updates,
		deliveryPolicy:  cfg.UpdateDeliveryPolicy,
		deliveryTimeout: cfg.UpdateDeliveryTimeout,
		onUpdateDropped: cfg.OnUpdateDropped,
		limiter:         rate.NewLimiter(rate.Limit(cfg.RateLimitRequests), cfg.RateLimitBurst),
		maxBodySize:     cfg.MaxBodySize,
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
	// Rate limit check (outside breaker)
	if !h.limiter.Allow() {
		h.fail(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// P0.6 FIX: Authentication checks OUTSIDE circuit breaker
	// This prevents attackers from tripping the breaker with bad credentials

	// Domain validation
	if h.allowedDomain != "" {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host != h.allowedDomain {
			h.fail(w, "forbidden", http.StatusForbidden)
			return
		}
	}

	// Secret validation (constant-time comparison)
	if h.webhookSecret != "" {
		secret := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		if subtle.ConstantTimeCompare([]byte(secret), []byte(h.webhookSecret)) != 1 {
			h.fail(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Method validation
	if r.Method != http.MethodPost {
		h.fail(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Only downstream processing inside circuit breaker
	_, err := h.breaker.Execute(func() (interface{}, error) {
		return nil, h.processUpdate(w, r)
	})

	if err != nil {
		var webhookErr *WebhookError
		if errors.As(err, &webhookErr) {
			h.fail(w, webhookErr.Message, webhookErr.Code)
		} else if errors.Is(err, ErrChannelBlocked) {
			h.fail(w, "service unavailable", http.StatusServiceUnavailable)
		} else {
			h.fail(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}

// processUpdate handles the actual update processing (inside circuit breaker)
func (h *WebhookHandler) processUpdate(w http.ResponseWriter, r *http.Request) error {
	// Get pooled buffer
	bufPtr := h.bufferPool.Get().(*[]byte)
	buffer := *bufPtr
	defer h.bufferPool.Put(bufPtr)

	// Read body with size limit
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		// P0.5 FIX: Return 413 for oversized body
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return &WebhookError{Code: http.StatusRequestEntityTooLarge, Message: "payload too large", Err: err}
		}
		return &WebhookError{Code: http.StatusInternalServerError, Message: "failed to read body", Err: err}
	}
	defer r.Body.Close()

	// Copy to buffer for potential reuse
	n := copy(buffer, body)

	// Parse update
	var update tg.Update
	if err := json.Unmarshal(buffer[:n], &update); err != nil {
		return &WebhookError{Code: http.StatusBadRequest, Message: "invalid JSON", Err: err}
	}

	// Deliver using configured policy.
	// Unlike polling, webhook ALWAYS returns nil (200 OK) to Telegram,
	// even when dropping updates — returning non-200 causes Telegram retry storms.
	return h.deliverUpdate(r.Context(), update)
}

// deliverUpdate delivers a single update using the configured policy.
// Always returns nil to ensure Telegram gets 200 OK (prevents retry storms).
// Errors are only returned for actual processing failures (bad JSON, oversized body).
func (h *WebhookHandler) deliverUpdate(ctx context.Context, update tg.Update) error {
	switch h.deliveryPolicy {
	case DeliveryPolicyBlock:
		return h.webhookDeliverBlocking(ctx, update)
	case DeliveryPolicyDropNewest:
		return h.webhookDeliverDropNewest(update)
	case DeliveryPolicyDropOldest:
		return h.webhookDeliverDropOldest(ctx, update)
	default:
		return h.webhookDeliverBlocking(ctx, update)
	}
}

// webhookDeliverBlocking waits for channel space with optional timeout.
func (h *WebhookHandler) webhookDeliverBlocking(ctx context.Context, update tg.Update) error {
	deliveryCtx := ctx
	var cancel context.CancelFunc

	if h.deliveryTimeout > 0 {
		deliveryCtx, cancel = context.WithTimeout(ctx, h.deliveryTimeout)
		defer cancel()
	}

	select {
	case h.updates <- update:
		h.logger.Debug("update forwarded", "update_id", update.UpdateID)
		return nil
	case <-deliveryCtx.Done():
		if ctx.Err() != nil {
			// Client disconnected — Telegram will retry, that's fine
			return nil
		}
		// Delivery timeout — drop and return 200 OK to prevent retry storm
		h.logger.Warn("webhook delivery timeout, dropping",
			"update_id", update.UpdateID,
			"timeout", h.deliveryTimeout,
		)
		if h.onUpdateDropped != nil {
			h.onUpdateDropped(update.UpdateID, "webhook_delivery_timeout")
		}
		return nil
	}
}

// webhookDeliverDropNewest drops the current update if channel is full.
func (h *WebhookHandler) webhookDeliverDropNewest(update tg.Update) error {
	select {
	case h.updates <- update:
		h.logger.Debug("update forwarded", "update_id", update.UpdateID)
	default:
		h.logger.Warn("webhook channel full, dropping newest",
			"update_id", update.UpdateID,
		)
		if h.onUpdateDropped != nil {
			h.onUpdateDropped(update.UpdateID, "webhook_channel_full_drop_newest")
		}
	}
	return nil
}

// webhookDeliverDropOldest drops the oldest update to make room.
func (h *WebhookHandler) webhookDeliverDropOldest(ctx context.Context, update tg.Update) error {
	if h.updatesBidi == nil {
		// Fallback: can't drain from send-only channel
		return h.webhookDeliverDropNewest(update)
	}
	for {
		select {
		case h.updates <- update:
			h.logger.Debug("update forwarded", "update_id", update.UpdateID)
			return nil
		default:
			select {
			case dropped := <-h.updatesBidi:
				h.logger.Warn("webhook channel full, dropping oldest",
					"dropped_id", dropped.UpdateID,
					"new_id", update.UpdateID,
				)
				if h.onUpdateDropped != nil {
					h.onUpdateDropped(dropped.UpdateID, "webhook_channel_full_drop_oldest")
				}
			case <-ctx.Done():
				return nil
			}
		}
	}
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
