package receiver

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prilive-com/galigo/tg"
	"github.com/sony/gobreaker/v2"
)

const (
	telegramAPIBaseURL     = "https://api.telegram.org/bot"
	maxPollResponseSize    = 50 << 20 // 50MB for updates
)

// PollingClient polls Telegram's getUpdates API for updates.
type PollingClient struct {
	token   tg.SecretToken
	baseURL string
	updates chan tg.Update // Changed to bidirectional for DropOldest policy
	logger  *slog.Logger

	// Configuration
	timeout              int
	limit                int
	maxErrors            int
	allowedUpdates       []string
	deleteWebhookOnStart bool

	// Retry configuration
	retryInitialDelay  time.Duration
	retryMaxDelay      time.Duration
	retryBackoffFactor float64

	// Update delivery policy
	deliveryPolicy  UpdateDeliveryPolicy
	deliveryTimeout time.Duration
	onUpdateDropped func(int, string)

	// HTTP client
	client *http.Client

	// Circuit breaker
	breaker *gobreaker.CircuitBreaker[[]byte]

	// State
	running           atomic.Bool
	offset            atomic.Int64 // P1.1: Use atomic for thread-safe access
	consecutiveErrors atomic.Int32
	stopCh            chan struct{}
	stopped           atomic.Bool  // P1.3: Track if stopped for restart capability
	mu                sync.Mutex   // P1.3: Protects stopCh recreation
	wg                sync.WaitGroup
}

// PollingOption configures the PollingClient.
type PollingOption func(*PollingClient)

// WithPollingHTTPClient sets a custom HTTP client.
func WithPollingHTTPClient(client *http.Client) PollingOption {
	return func(c *PollingClient) {
		c.client = client
	}
}

// WithPollingCircuitBreaker sets a custom circuit breaker.
func WithPollingCircuitBreaker(breaker *gobreaker.CircuitBreaker[[]byte]) PollingOption {
	return func(c *PollingClient) {
		c.breaker = breaker
	}
}

// WithPollingMaxErrors sets maximum consecutive errors before stopping.
func WithPollingMaxErrors(max int) PollingOption {
	return func(c *PollingClient) {
		c.maxErrors = max
	}
}

// WithPollingAllowedUpdates sets the update types to receive.
func WithPollingAllowedUpdates(types []string) PollingOption {
	return func(c *PollingClient) {
		c.allowedUpdates = types
	}
}

// WithPollingDeleteWebhook enables webhook deletion before starting.
func WithPollingDeleteWebhook(delete bool) PollingOption {
	return func(c *PollingClient) {
		c.deleteWebhookOnStart = delete
	}
}

// WithPollingRetryConfig sets exponential backoff parameters.
func WithPollingRetryConfig(initial, max time.Duration, factor float64) PollingOption {
	return func(c *PollingClient) {
		if initial > 0 {
			c.retryInitialDelay = initial
		}
		if max > 0 {
			c.retryMaxDelay = max
		}
		if factor > 1.0 {
			c.retryBackoffFactor = factor
		}
	}
}

// WithDeliveryPolicy sets the update delivery policy.
func WithDeliveryPolicy(policy UpdateDeliveryPolicy) PollingOption {
	return func(c *PollingClient) {
		c.deliveryPolicy = policy
	}
}

// WithDeliveryTimeout sets the timeout for blocking delivery policy.
func WithDeliveryTimeout(timeout time.Duration) PollingOption {
	return func(c *PollingClient) {
		c.deliveryTimeout = timeout
	}
}

// WithUpdateDroppedCallback sets the callback for dropped updates.
func WithUpdateDroppedCallback(fn func(updateID int, reason string)) PollingOption {
	return func(c *PollingClient) {
		c.onUpdateDropped = fn
	}
}

// NewPollingClient creates a new long polling client.
// Note: The updates channel must be bidirectional (chan tg.Update) if using DeliveryPolicyDropOldest.
func NewPollingClient(
	token tg.SecretToken,
	updates chan tg.Update,
	logger *slog.Logger,
	cfg Config,
	opts ...PollingOption,
) *PollingClient {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = telegramAPIBaseURL
	}

	if logger == nil {
		logger = slog.Default()
	}

	c := &PollingClient{
		token:              token,
		baseURL:            baseURL,
		updates:            updates,
		logger:             logger,
		timeout:            cfg.PollingTimeout,
		limit:              cfg.PollingLimit,
		maxErrors:          cfg.PollingMaxErrors,
		retryInitialDelay:  cfg.RetryInitialDelay,
		retryMaxDelay:      cfg.RetryMaxDelay,
		retryBackoffFactor: cfg.RetryBackoffFactor,
		deliveryPolicy:     cfg.UpdateDeliveryPolicy,
		deliveryTimeout:    cfg.UpdateDeliveryTimeout,
		onUpdateDropped:    cfg.OnUpdateDropped,
		client:             defaultPollingHTTPClient(cfg.PollingTimeout),
		stopCh:             make(chan struct{}),
	}

	// Default circuit breaker
	c.breaker = gobreaker.NewCircuitBreaker[[]byte](gobreaker.Settings{
		Name:        "galigo-polling",
		MaxRequests: cfg.BreakerMaxRequests,
		Interval:    cfg.BreakerInterval,
		Timeout:     cfg.BreakerTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.Requests < 3 {
				return false
			}
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRatio >= 0.6
		},
		IsSuccessful: func(err error) bool {
			if err == nil {
				return true
			}
			// 4xx API errors are client issues, not server failures
			var apiErr *APIError
			if errors.As(err, &apiErr) && apiErr.Code >= 400 && apiErr.Code < 500 {
				return true
			}
			return false
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			logger.Info("circuit breaker state changed",
				"name", name,
				"from", from.String(),
				"to", to.String(),
			)
		},
	})

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func defaultPollingHTTPClient(timeoutSeconds int) *http.Client {
	httpTimeout := time.Duration(timeoutSeconds+10) * time.Second
	return &http.Client{
		Timeout: httpTimeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
			TLSHandshakeTimeout:   10 * time.Second,
			MaxIdleConns:          10,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       90 * time.Second,
			ResponseHeaderTimeout: time.Duration(timeoutSeconds+5) * time.Second,
			ForceAttemptHTTP2:     true,
		},
	}
}

// Start begins polling for updates.
func (c *PollingClient) Start(ctx context.Context) error {
	if !c.running.CompareAndSwap(false, true) {
		return ErrAlreadyRunning
	}

	// P1.3: Support restart by recreating stopCh if previously stopped
	c.mu.Lock()
	if c.stopped.Load() {
		c.stopCh = make(chan struct{})
		c.stopped.Store(false)
	}
	c.mu.Unlock()

	if c.deleteWebhookOnStart {
		c.logger.Info("deleting existing webhook")
		if err := DeleteWebhook(ctx, c.client, c.token, false); err != nil {
			c.running.Store(false)
			return fmt.Errorf("failed to delete webhook: %w", err)
		}
	}

	// P1.4: Use sync.WaitGroup.Go() for Go 1.25
	c.wg.Go(func() {
		c.pollLoop(ctx)
	})

	c.logger.Info("long polling started",
		"timeout", c.timeout,
		"limit", c.limit,
		"max_errors", c.maxErrors,
	)

	return nil
}

// Stop gracefully stops the polling client.
func (c *PollingClient) Stop() {
	if !c.running.CompareAndSwap(true, false) {
		return
	}

	c.mu.Lock()
	select {
	case <-c.stopCh:
		// Already closed
	default:
		close(c.stopCh)
	}
	c.stopped.Store(true)
	c.mu.Unlock()

	c.wg.Wait()
	c.logger.Info("long polling stopped")
}

// Running returns true if polling is active.
func (c *PollingClient) Running() bool {
	return c.running.Load()
}

// IsHealthy returns health status for K8s probes.
func (c *PollingClient) IsHealthy() bool {
	if c.maxErrors == 0 {
		return c.running.Load()
	}
	return c.running.Load() && int(c.consecutiveErrors.Load()) < c.maxErrors
}

// ConsecutiveErrors returns the current error count.
func (c *PollingClient) ConsecutiveErrors() int32 {
	return c.consecutiveErrors.Load()
}

// Offset returns the current update offset.
func (c *PollingClient) Offset() int64 {
	return c.offset.Load()
}

func (c *PollingClient) pollLoop(ctx context.Context) {
	// Note: No defer c.wg.Done() needed when using wg.Go()
	defer c.running.Store(false)

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("polling stopped: context cancelled")
			return
		case <-c.stopCh:
			c.logger.Info("polling stopped: stop signal")
			return
		default:
		}

		updates, err := c.fetchUpdates(ctx)
		if err != nil {
			errCount := c.consecutiveErrors.Add(1)
			backoff := c.calculateBackoff(errCount)
			c.logger.Error("fetch updates failed",
				"error", err,
				"consecutive_errors", errCount,
				"retry_delay", backoff,
			)

			if c.maxErrors > 0 && int(errCount) >= c.maxErrors {
				c.logger.Error("max consecutive errors exceeded", "max_errors", c.maxErrors)
				return
			}

			select {
			case <-ctx.Done():
				return
			case <-c.stopCh:
				return
			case <-time.After(backoff):
				continue
			}
		}

		c.consecutiveErrors.Store(0)

		// Deliver updates using configured policy
		if err := c.deliverUpdates(ctx, updates); err != nil {
			if err == context.Canceled || errors.Is(err, context.Canceled) {
				c.logger.Info("stopping update delivery: context cancelled")
			} else {
				c.logger.Info("stopping update delivery: stop signal")
			}
			return
		}
	}
}

// deliverUpdates delivers updates using the configured delivery policy.
func (c *PollingClient) deliverUpdates(ctx context.Context, updates []tg.Update) error {
	for _, update := range updates {
		if err := c.deliverUpdate(ctx, update); err != nil {
			return err
		}
	}
	return nil
}

// deliverUpdate delivers a single update using the configured policy.
func (c *PollingClient) deliverUpdate(ctx context.Context, update tg.Update) error {
	switch c.deliveryPolicy {
	case DeliveryPolicyBlock:
		return c.deliverBlocking(ctx, update)
	case DeliveryPolicyDropNewest:
		return c.deliverDropNewest(ctx, update)
	case DeliveryPolicyDropOldest:
		return c.deliverDropOldest(ctx, update)
	default:
		return c.deliverBlocking(ctx, update)
	}
}

// deliverBlocking waits for channel space with optional timeout.
func (c *PollingClient) deliverBlocking(ctx context.Context, update tg.Update) error {
	// Create delivery context with timeout (if configured)
	deliveryCtx := ctx
	var cancel context.CancelFunc

	if c.deliveryTimeout > 0 {
		deliveryCtx, cancel = context.WithTimeout(ctx, c.deliveryTimeout)
		defer cancel()
	}

	select {
	case c.updates <- update:
		// Only advance offset after successful delivery
		c.advanceOffset(update.UpdateID)
		c.logger.Debug("update sent", "update_id", update.UpdateID)
		return nil

	case <-deliveryCtx.Done():
		if ctx.Err() != nil {
			// Parent context cancelled - normal shutdown
			return ctx.Err()
		}

		// Delivery timeout - drop update, advance offset, continue
		c.logger.Warn("update delivery timeout, dropping",
			"update_id", update.UpdateID,
			"timeout", c.deliveryTimeout,
		)

		if c.onUpdateDropped != nil {
			c.onUpdateDropped(update.UpdateID, "delivery_timeout")
		}

		// Advance offset to prevent infinite retry loop
		c.advanceOffset(update.UpdateID)
		return nil

	case <-c.stopCh:
		// Don't advance offset - updates will be redelivered on restart
		return errors.New("stop signal received")
	}
}

// deliverDropNewest drops the current update if channel is full.
func (c *PollingClient) deliverDropNewest(ctx context.Context, update tg.Update) error {
	select {
	case c.updates <- update:
		c.advanceOffset(update.UpdateID)
		c.logger.Debug("update sent", "update_id", update.UpdateID)
		return nil

	default:
		// Channel full - drop this update
		c.logger.Warn("channel full, dropping newest update",
			"update_id", update.UpdateID,
		)

		if c.onUpdateDropped != nil {
			c.onUpdateDropped(update.UpdateID, "channel_full_drop_newest")
		}

		// Advance offset - intentionally dropping
		c.advanceOffset(update.UpdateID)
		return nil
	}
}

// deliverDropOldest drops oldest update to make room for new one.
func (c *PollingClient) deliverDropOldest(ctx context.Context, update tg.Update) error {
	for {
		select {
		case c.updates <- update:
			c.advanceOffset(update.UpdateID)
			c.logger.Debug("update sent", "update_id", update.UpdateID)
			return nil

		default:
			// Channel full - try to drain oldest
			select {
			case dropped := <-c.updates:
				c.logger.Warn("channel full, dropping oldest update",
					"dropped_id", dropped.UpdateID,
					"new_id", update.UpdateID,
				)
				if c.onUpdateDropped != nil {
					c.onUpdateDropped(dropped.UpdateID, "channel_full_drop_oldest")
				}
				// Loop and try to send again

			case <-ctx.Done():
				return ctx.Err()

			case <-c.stopCh:
				return errors.New("stop signal received")
			}
		}
	}
}

// advanceOffset updates the offset if the update ID is >= current offset.
func (c *PollingClient) advanceOffset(updateID int) {
	if int64(updateID) >= c.offset.Load() {
		c.offset.Store(int64(updateID) + 1)
	}
}

type getUpdatesResponse struct {
	OK          bool        `json:"ok"`
	Result      []tg.Update `json:"result,omitempty"`
	ErrorCode   int         `json:"error_code,omitempty"`
	Description string      `json:"description,omitempty"`
}

func (c *PollingClient) fetchUpdates(ctx context.Context) ([]tg.Update, error) {
	// P0.2 FIX: Use url.Values for proper URL encoding
	params := url.Values{}
	params.Set("timeout", strconv.Itoa(c.timeout))
	params.Set("limit", strconv.Itoa(c.limit))
	params.Set("offset", strconv.FormatInt(c.offset.Load(), 10))

	if len(c.allowedUpdates) > 0 {
		encoded, err := json.Marshal(c.allowedUpdates)
		if err == nil {
			params.Set("allowed_updates", string(encoded))
		}
	}

	apiURL := fmt.Sprintf("%s%s/getUpdates?%s",
		c.baseURL,
		c.token.Value(),
		params.Encode(),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, &APIError{Description: "failed to create request", Err: err}
	}

	respBody, err := c.breaker.Execute(func() ([]byte, error) {
		resp, err := c.client.Do(req)
		if err != nil {
			return nil, scrubTokenFromError(err, c.token)
		}
		defer func() {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()

		// P0.9 FIX: Add response size limit to prevent memory exhaustion
		limitedReader := io.LimitReader(resp.Body, maxPollResponseSize+1)
		body, err := io.ReadAll(limitedReader)
		if err != nil {
			return nil, err
		}

		if int64(len(body)) > maxPollResponseSize {
			return nil, errors.New("response too large")
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		return body, nil
	})

	if err != nil {
		return nil, &APIError{Description: "request failed", Err: err}
	}

	var response getUpdatesResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, &APIError{Description: "failed to parse response", Err: err}
	}

	if !response.OK {
		return nil, &APIError{
			Code:        response.ErrorCode,
			Description: response.Description,
		}
	}

	return response.Result, nil
}

// scrubbedError wraps an error with a token-scrubbed message while preserving the error chain.
type scrubbedError struct {
	msg string
	err error
}

func (e *scrubbedError) Error() string { return e.msg }
func (e *scrubbedError) Unwrap() error { return e.err }

// scrubTokenFromError removes the bot token from error messages.
// Preserves the error chain for errors.Is/As.
func scrubTokenFromError(err error, token tg.SecretToken) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	tokenVal := token.Value()
	if tokenVal != "" && strings.Contains(msg, tokenVal) {
		return &scrubbedError{
			msg: strings.ReplaceAll(msg, tokenVal, "[REDACTED]"),
			err: err,
		}
	}
	return err
}

func (c *PollingClient) calculateBackoff(attempt int32) time.Duration {
	baseDelay := float64(c.retryInitialDelay) * math.Pow(c.retryBackoffFactor, float64(attempt-1))

	if baseDelay > float64(c.retryMaxDelay) {
		baseDelay = float64(c.retryMaxDelay)
	}

	// Add cryptographic jitter (0-25%)
	jitterRange := int64(baseDelay * 0.25)
	if jitterRange > 0 {
		jitterBig, err := rand.Int(rand.Reader, big.NewInt(jitterRange))
		if err == nil {
			baseDelay += float64(jitterBig.Int64())
		}
	}

	return time.Duration(baseDelay)
}
