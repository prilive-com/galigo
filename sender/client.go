package sender

import (
	"bytes"
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
	"strconv"
	"sync"
	"time"

	"github.com/prilive-com/galigo/tg"
	"github.com/sony/gobreaker/v2"
	"golang.org/x/time/rate"
)

const (
	maxResponseSize = 10 << 20 // 10MB
)

// Client is the main sender client for Telegram Bot API.
type Client struct {
	config        Config
	httpClient    *http.Client
	logger        *slog.Logger
	globalLimiter *rate.Limiter
	chatLimiters  map[int64]*rate.Limiter
	limiterMu     sync.RWMutex
	breaker       *gobreaker.CircuitBreaker[*apiResponse]
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result,omitempty"`
	ErrorCode   int             `json:"error_code,omitempty"`
	Description string          `json:"description,omitempty"`
}

// Option configures the Client.
type Option func(*Client)

// WithLogger sets a custom logger.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Client) {
		c.logger = logger
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.httpClient = client
	}
}

// WithRateLimit sets rate limiting parameters.
func WithRateLimit(globalRPS float64, burst int) Option {
	return func(c *Client) {
		c.config.GlobalRPS = globalRPS
		c.config.GlobalBurst = burst
		c.globalLimiter = rate.NewLimiter(rate.Limit(globalRPS), burst)
	}
}

// WithRetries sets retry parameters.
func WithRetries(max int) Option {
	return func(c *Client) {
		c.config.MaxRetries = max
	}
}

// New creates a new Client with the given token and options.
func New(token string, opts ...Option) (*Client, error) {
	cfg := DefaultConfig()
	cfg.Token = tg.SecretToken(token)

	if cfg.Token.IsEmpty() {
		return nil, ErrInvalidToken
	}

	c := &Client{
		config:       cfg,
		chatLimiters: make(map[int64]*rate.Limiter),
	}

	// Apply options
	for _, opt := range opts {
		opt(c)
	}

	// Default logger
	if c.logger == nil {
		c.logger = slog.Default()
	}

	// Default HTTP client
	if c.httpClient == nil {
		c.httpClient = &http.Client{
			Timeout: c.config.RequestTimeout,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: c.config.KeepAlive,
				}).DialContext,
				MaxIdleConns:        c.config.MaxIdleConns,
				IdleConnTimeout:     c.config.IdleTimeout,
				TLSHandshakeTimeout: 10 * time.Second,
				ForceAttemptHTTP2:   true,
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
		}
	}

	// Default global limiter
	if c.globalLimiter == nil {
		c.globalLimiter = rate.NewLimiter(rate.Limit(c.config.GlobalRPS), c.config.GlobalBurst)
	}

	// Circuit breaker
	c.breaker = gobreaker.NewCircuitBreaker[*apiResponse](gobreaker.Settings{
		Name:        "galigo-sender",
		MaxRequests: c.config.BreakerMaxRequests,
		Interval:    c.config.BreakerInterval,
		Timeout:     c.config.BreakerTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.Requests < 3 {
				return false
			}
			ratio := float64(counts.TotalFailures) / float64(counts.Requests)
			return ratio >= 0.5
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			c.logger.Info("circuit breaker state changed",
				"name", name,
				"from", from.String(),
				"to", to.String(),
			)
		},
	})

	return c, nil
}

// NewFromConfig creates a Client from a Config.
func NewFromConfig(cfg Config, opts ...Option) (*Client, error) {
	if cfg.Token.IsEmpty() {
		return nil, ErrInvalidToken
	}

	c := &Client{
		config:       cfg,
		chatLimiters: make(map[int64]*rate.Limiter),
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.logger == nil {
		c.logger = slog.Default()
	}

	if c.httpClient == nil {
		c.httpClient = &http.Client{
			Timeout: c.config.RequestTimeout,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: c.config.KeepAlive,
				}).DialContext,
				MaxIdleConns:        c.config.MaxIdleConns,
				IdleConnTimeout:     c.config.IdleTimeout,
				TLSHandshakeTimeout: 10 * time.Second,
				ForceAttemptHTTP2:   true,
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
		}
	}

	if c.globalLimiter == nil {
		c.globalLimiter = rate.NewLimiter(rate.Limit(c.config.GlobalRPS), c.config.GlobalBurst)
	}

	c.breaker = gobreaker.NewCircuitBreaker[*apiResponse](gobreaker.Settings{
		Name:        "galigo-sender",
		MaxRequests: c.config.BreakerMaxRequests,
		Interval:    c.config.BreakerInterval,
		Timeout:     c.config.BreakerTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.Requests < 3 {
				return false
			}
			ratio := float64(counts.TotalFailures) / float64(counts.Requests)
			return ratio >= 0.5
		},
	})

	return c, nil
}

// Close releases resources.
func (c *Client) Close() error {
	return nil
}

// SendMessage sends a text message.
func (c *Client) SendMessage(ctx context.Context, req SendMessageRequest) (*tg.Message, error) {
	return withRetry(c, ctx, req.ChatID, func() (*tg.Message, error) {
		return c.sendMessageOnce(ctx, req)
	})
}

// SendPhoto sends a photo.
func (c *Client) SendPhoto(ctx context.Context, req SendPhotoRequest) (*tg.Message, error) {
	return withRetry(c, ctx, req.ChatID, func() (*tg.Message, error) {
		return c.sendPhotoOnce(ctx, req)
	})
}

// EditMessageText edits message text.
func (c *Client) EditMessageText(ctx context.Context, req EditMessageTextRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "editMessageText", req)
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// EditMessageCaption edits message caption.
func (c *Client) EditMessageCaption(ctx context.Context, req EditMessageCaptionRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "editMessageCaption", req)
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// EditMessageReplyMarkup edits message reply markup.
func (c *Client) EditMessageReplyMarkup(ctx context.Context, req EditMessageReplyMarkupRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "editMessageReplyMarkup", req)
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// DeleteMessage deletes a message.
func (c *Client) DeleteMessage(ctx context.Context, req DeleteMessageRequest) error {
	_, err := c.executeRequest(ctx, "deleteMessage", req)
	return err
}

// ForwardMessage forwards a message.
func (c *Client) ForwardMessage(ctx context.Context, req ForwardMessageRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "forwardMessage", req)
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// CopyMessage copies a message.
func (c *Client) CopyMessage(ctx context.Context, req CopyMessageRequest) (*tg.MessageID, error) {
	resp, err := c.executeRequest(ctx, "copyMessage", req)
	if err != nil {
		return nil, err
	}
	var result tg.MessageID
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}
	return &result, nil
}

// AnswerCallbackQuery answers a callback query.
func (c *Client) AnswerCallbackQuery(ctx context.Context, req AnswerCallbackQueryRequest) error {
	_, err := c.executeRequest(ctx, "answerCallbackQuery", req)
	return err
}

// Edit convenience methods using Editable interface

// Edit edits a message text using Editable.
func (c *Client) Edit(ctx context.Context, e tg.Editable, text string, opts ...EditOption) (*tg.Message, error) {
	msgID, chatID := e.MessageSig()
	req := EditMessageTextRequest{Text: text}
	if chatID != 0 {
		req.ChatID = chatID
		id, _ := strconv.Atoi(msgID)
		req.MessageID = id
	} else {
		req.InlineMessageID = msgID
	}
	for _, opt := range opts {
		opt(&req)
	}
	return c.EditMessageText(ctx, req)
}

// Delete deletes a message using Editable.
func (c *Client) Delete(ctx context.Context, e tg.Editable) error {
	msgID, chatID := e.MessageSig()
	if chatID == 0 {
		return errors.New("cannot delete inline messages")
	}
	id, _ := strconv.Atoi(msgID)
	return c.DeleteMessage(ctx, DeleteMessageRequest{
		ChatID:    chatID,
		MessageID: id,
	})
}

// Forward forwards a message using Editable.
func (c *Client) Forward(ctx context.Context, e tg.Editable, toChatID tg.ChatID, opts ...ForwardOption) (*tg.Message, error) {
	msgID, chatID := e.MessageSig()
	if chatID == 0 {
		return nil, errors.New("cannot forward inline messages")
	}
	id, _ := strconv.Atoi(msgID)
	req := ForwardMessageRequest{
		ChatID:     toChatID,
		FromChatID: chatID,
		MessageID:  id,
	}
	for _, opt := range opts {
		opt(&req)
	}
	return c.ForwardMessage(ctx, req)
}

// Copy copies a message using Editable.
func (c *Client) Copy(ctx context.Context, e tg.Editable, toChatID tg.ChatID, opts ...CopyOption) (*tg.MessageID, error) {
	msgID, chatID := e.MessageSig()
	if chatID == 0 {
		return nil, errors.New("cannot copy inline messages")
	}
	id, _ := strconv.Atoi(msgID)
	req := CopyMessageRequest{
		ChatID:     toChatID,
		FromChatID: chatID,
		MessageID:  id,
	}
	for _, opt := range opts {
		opt(&req)
	}
	return c.CopyMessage(ctx, req)
}

// Answer convenience methods

// Answer answers a callback query.
func (c *Client) Answer(ctx context.Context, cb *tg.CallbackQuery, opts ...AnswerOption) error {
	req := AnswerCallbackQueryRequest{
		CallbackQueryID: cb.ID,
	}
	for _, opt := range opts {
		opt(&req)
	}
	return c.AnswerCallbackQuery(ctx, req)
}

// Acknowledge silently acknowledges a callback query.
func (c *Client) Acknowledge(ctx context.Context, cb *tg.CallbackQuery) error {
	return c.Answer(ctx, cb)
}

// Internal methods

func (c *Client) sendMessageOnce(ctx context.Context, req SendMessageRequest) (*tg.Message, error) {
	chatID := extractChatID(req.ChatID)
	if err := c.waitForRateLimit(ctx, chatID); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRateLimited, err)
	}

	resp, err := c.breaker.Execute(func() (*apiResponse, error) {
		return c.doRequest(ctx, "sendMessage", req)
	})

	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			return nil, fmt.Errorf("%w: %v", ErrCircuitOpen, err)
		}
		return nil, err
	}

	return parseMessage(resp)
}

func (c *Client) sendPhotoOnce(ctx context.Context, req SendPhotoRequest) (*tg.Message, error) {
	chatID := extractChatID(req.ChatID)
	if err := c.waitForRateLimit(ctx, chatID); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRateLimited, err)
	}

	resp, err := c.breaker.Execute(func() (*apiResponse, error) {
		return c.doRequest(ctx, "sendPhoto", req)
	})

	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			return nil, fmt.Errorf("%w: %v", ErrCircuitOpen, err)
		}
		return nil, err
	}

	return parseMessage(resp)
}

func (c *Client) executeRequest(ctx context.Context, method string, payload any) (*apiResponse, error) {
	return c.breaker.Execute(func() (*apiResponse, error) {
		return c.doRequest(ctx, method, payload)
	})
}

func (c *Client) doRequest(ctx context.Context, method string, payload any) (*apiResponse, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/bot%s/%s", c.config.BaseURL, c.config.Token.Value(), method)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	limitedReader := io.LimitReader(resp.Body, maxResponseSize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if len(body) == maxResponseSize {
		return nil, ErrResponseTooLarge
	}

	var apiResp apiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !apiResp.OK {
		retryAfter := resp.Header.Get("Retry-After")
		if apiResp.ErrorCode == 429 && retryAfter != "" {
			if seconds, err := strconv.Atoi(retryAfter); err == nil {
				return nil, NewAPIErrorWithRetry(method, apiResp.ErrorCode, apiResp.Description, time.Duration(seconds)*time.Second)
			}
		}
		return nil, NewAPIError(method, apiResp.ErrorCode, apiResp.Description)
	}

	return &apiResp, nil
}

func (c *Client) waitForRateLimit(ctx context.Context, chatID int64) error {
	limiter := c.getChatLimiter(chatID)
	if err := limiter.Wait(ctx); err != nil {
		return err
	}
	return c.globalLimiter.Wait(ctx)
}

func (c *Client) getChatLimiter(chatID int64) *rate.Limiter {
	c.limiterMu.RLock()
	limiter, exists := c.chatLimiters[chatID]
	c.limiterMu.RUnlock()

	if exists {
		return limiter
	}

	c.limiterMu.Lock()
	defer c.limiterMu.Unlock()

	if limiter, exists = c.chatLimiters[chatID]; exists {
		return limiter
	}

	limiter = rate.NewLimiter(rate.Limit(c.config.PerChatRPS), c.config.PerChatBurst)
	c.chatLimiters[chatID] = limiter
	return limiter
}

func withRetry[T any](c *Client, ctx context.Context, chatID tg.ChatID, fn func() (T, error)) (T, error) {
	var zero T
	var lastErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		if attempt >= c.config.MaxRetries {
			break
		}

		if !isRetryable(err) {
			return zero, err
		}

		backoff := calculateBackoff(c.config, attempt+1, err)

		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(backoff):
		}
	}

	return zero, fmt.Errorf("%w: %v", ErrMaxRetries, lastErr)
}

func isRetryable(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.IsRetryable()
	}

	return false
}

func calculateBackoff(cfg Config, attempt int, err error) time.Duration {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
		return apiErr.RetryAfter
	}

	backoff := float64(cfg.RetryBaseWait) * math.Pow(cfg.RetryFactor, float64(attempt-1))
	if backoff > float64(cfg.RetryMaxWait) {
		backoff = float64(cfg.RetryMaxWait)
	}

	// Add jitter
	jitterRange := int64(backoff * 0.2)
	if jitterRange > 0 {
		jitter, err := rand.Int(rand.Reader, big.NewInt(jitterRange*2))
		if err == nil {
			backoff += float64(jitter.Int64()) - float64(jitterRange)
		}
	}

	return time.Duration(backoff)
}

func extractChatID(chatID tg.ChatID) int64 {
	switch v := chatID.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	default:
		return 0
	}
}

func parseMessage(resp *apiResponse) (*tg.Message, error) {
	var msg tg.Message
	if err := json.Unmarshal(resp.Result, &msg); err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}
	return &msg, nil
}
