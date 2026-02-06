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
	"sync/atomic"
	"time"

	"github.com/sony/gobreaker/v2"
	"golang.org/x/time/rate"

	"github.com/prilive-com/galigo/internal/scrub"
	"github.com/prilive-com/galigo/tg"
)

const (
	maxResponseSize = 10 << 20 // 10MB
)

// Sleeper abstracts time-based waiting for testing.
type Sleeper interface {
	Sleep(ctx context.Context, d time.Duration) error
}

// CircuitBreakerSettings configures the circuit breaker behavior.
type CircuitBreakerSettings struct {
	// MaxRequests is the maximum number of requests allowed in half-open state.
	MaxRequests uint32

	// Interval is the cyclic period of the closed state.
	// If 0, internal counts never reset in closed state.
	Interval time.Duration

	// Timeout is the duration of the open state before transitioning to half-open.
	Timeout time.Duration

	// ReadyToTrip determines if breaker should trip based on failure counts.
	// If nil, uses default (50% failure rate after 3 requests).
	ReadyToTrip func(counts gobreaker.Counts) bool
}

// DefaultCircuitBreakerSettings returns production-ready defaults.
func DefaultCircuitBreakerSettings() CircuitBreakerSettings {
	return CircuitBreakerSettings{
		MaxRequests: 5,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.Requests < 3 {
				return false
			}
			ratio := float64(counts.TotalFailures) / float64(counts.Requests)
			return ratio >= 0.5
		},
	}
}

// realSleeper uses actual time.
type realSleeper struct{}

func (realSleeper) Sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// Client is the main sender client for Telegram Bot API.
type Client struct {
	config          Config
	httpClient      *http.Client
	logger          *slog.Logger
	globalLimiter   *rate.Limiter
	chatLimiters    map[string]*chatLimiterEntry // P1.2: Track last used time
	limiterMu       sync.RWMutex
	breaker         *gobreaker.CircuitBreaker[*apiResponse]
	breakerSettings CircuitBreakerSettings
	sleeper         Sleeper // For testing retry logic

	// P1.2: Cleanup
	cleanupTicker *time.Ticker
	cleanupDone   chan struct{}
}

// chatLimiterEntry wraps a rate limiter with last used timestamp.
// lastUsed uses atomic.Int64 (Unix nanos) to avoid write-lock contention on the hot path.
type chatLimiterEntry struct {
	limiter  *rate.Limiter
	lastUsed atomic.Int64 // UnixNano timestamp
}

type apiResponse struct {
	OK          bool                `json:"ok"`
	Result      json.RawMessage     `json:"result,omitempty"`
	ErrorCode   int                 `json:"error_code,omitempty"`
	Description string              `json:"description,omitempty"`
	Parameters  *responseParameters `json:"parameters,omitempty"` // P0.3: For retry_after
}

// responseParameters contains special parameters returned by Telegram API
type responseParameters struct {
	RetryAfter      int   `json:"retry_after,omitempty"`
	MigrateToChatID int64 `json:"migrate_to_chat_id,omitempty"`
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

// WithBaseURL sets the API base URL (useful for testing).
func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.config.BaseURL = url
	}
}

// WithSleeper sets a custom sleeper for retry timing (useful for testing).
func WithSleeper(s Sleeper) Option {
	return func(c *Client) {
		c.sleeper = s
	}
}

// WithPerChatRateLimit sets per-chat rate limiting parameters.
func WithPerChatRateLimit(rps float64, burst int) Option {
	return func(c *Client) {
		c.config.PerChatRPS = rps
		c.config.PerChatBurst = burst
	}
}

// WithGroupRateLimit sets the per-chat rate limit for group chats (negative chat IDs).
// Telegram limits groups to ~20 messages/minute. Default: 0.33 RPS, burst 2.
func WithGroupRateLimit(rps float64, burst int) Option {
	return func(c *Client) {
		c.config.GroupRPS = rps
		c.config.GroupBurst = burst
	}
}

// WithCircuitBreakerSettings configures the circuit breaker.
func WithCircuitBreakerSettings(settings CircuitBreakerSettings) Option {
	return func(c *Client) {
		c.breakerSettings = settings
	}
}

// P1.5 FIX: Deduplicated HTTP client creation
func createHTTPClient(cfg Config) *http.Client {
	return &http.Client{
		Timeout: cfg.RequestTimeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: cfg.KeepAlive,
			}).DialContext,
			MaxIdleConns:          cfg.MaxIdleConns,
			IdleConnTimeout:       cfg.IdleTimeout,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second, // Time to receive response headers; shorter than total timeout
			ForceAttemptHTTP2:     true,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		},
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
		chatLimiters: make(map[string]*chatLimiterEntry), // P1.2: Use entry type
	}

	// Apply options
	for _, opt := range opts {
		opt(c)
	}

	// Default logger
	if c.logger == nil {
		c.logger = slog.Default()
	}

	// Default HTTP client (P1.5: Use helper function)
	if c.httpClient == nil {
		c.httpClient = createHTTPClient(c.config)
	}

	// Default global limiter
	if c.globalLimiter == nil {
		c.globalLimiter = rate.NewLimiter(rate.Limit(c.config.GlobalRPS), c.config.GlobalBurst)
	}

	// Default sleeper
	if c.sleeper == nil {
		c.sleeper = realSleeper{}
	}

	// Default circuit breaker settings
	if c.breakerSettings.ReadyToTrip == nil {
		c.breakerSettings = DefaultCircuitBreakerSettings()
	}

	// Circuit breaker
	c.breaker = gobreaker.NewCircuitBreaker[*apiResponse](gobreaker.Settings{
		Name:         "galigo-sender",
		MaxRequests:  c.breakerSettings.MaxRequests,
		Interval:     c.breakerSettings.Interval,
		Timeout:      c.breakerSettings.Timeout,
		ReadyToTrip:  c.breakerSettings.ReadyToTrip,
		IsSuccessful: isBreakerSuccess,
		OnStateChange: func(name string, from, to gobreaker.State) {
			c.logger.Info("circuit breaker state changed",
				"name", name,
				"from", from.String(),
				"to", to.String(),
			)
		},
	})

	// P1.2: Start chat limiter cleanup goroutine
	c.startLimiterCleanup()

	return c, nil
}

// NewFromConfig creates a Client from a Config.
func NewFromConfig(cfg Config, opts ...Option) (*Client, error) {
	if cfg.Token.IsEmpty() {
		return nil, ErrInvalidToken
	}

	c := &Client{
		config:       cfg,
		chatLimiters: make(map[string]*chatLimiterEntry), // P1.2: Use entry type
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.logger == nil {
		c.logger = slog.Default()
	}

	// P1.5 FIX: Use helper function
	if c.httpClient == nil {
		c.httpClient = createHTTPClient(c.config)
	}

	if c.globalLimiter == nil {
		c.globalLimiter = rate.NewLimiter(rate.Limit(c.config.GlobalRPS), c.config.GlobalBurst)
	}

	// Default sleeper
	if c.sleeper == nil {
		c.sleeper = realSleeper{}
	}

	// Default circuit breaker settings
	if c.breakerSettings.ReadyToTrip == nil {
		c.breakerSettings = DefaultCircuitBreakerSettings()
	}

	c.breaker = gobreaker.NewCircuitBreaker[*apiResponse](gobreaker.Settings{
		Name:         "galigo-sender",
		MaxRequests:  c.breakerSettings.MaxRequests,
		Interval:     c.breakerSettings.Interval,
		Timeout:      c.breakerSettings.Timeout,
		ReadyToTrip:  c.breakerSettings.ReadyToTrip,
		IsSuccessful: isBreakerSuccess,
	})

	// P1.2: Start chat limiter cleanup goroutine
	c.startLimiterCleanup()

	return c, nil
}

// Close releases resources used by the client.
// It is safe to call Close concurrently with other methods;
// in-flight requests will complete normally or with context errors.
// Close should be called only once; subsequent calls are no-ops.
func (c *Client) Close() error {
	// P1.6 FIX: Actually close resources

	// Stop limiter cleanup goroutine
	if c.cleanupTicker != nil {
		c.cleanupTicker.Stop()
		close(c.cleanupDone)
	}

	// Close idle HTTP connections
	if t, ok := c.httpClient.Transport.(*http.Transport); ok {
		t.CloseIdleConnections()
	}

	return nil
}

// P1.2: Start background goroutine to cleanup stale chat limiters
func (c *Client) startLimiterCleanup() {
	c.cleanupTicker = time.NewTicker(5 * time.Minute)
	c.cleanupDone = make(chan struct{})

	go func() {
		for {
			select {
			case <-c.cleanupDone:
				return
			case <-c.cleanupTicker.C:
				c.cleanupStaleLimiters()
			}
		}
	}()
}

// cleanupStaleLimiters removes chat limiters that haven't been used in 10 minutes
func (c *Client) cleanupStaleLimiters() {
	c.limiterMu.Lock()
	defer c.limiterMu.Unlock()

	threshold := time.Now().Add(-10 * time.Minute).UnixNano()
	for chatID, entry := range c.chatLimiters {
		if entry.lastUsed.Load() < threshold {
			delete(c.chatLimiters, chatID)
		}
	}
}

// ChatLimiterCount returns the number of active per-chat limiters.
// Useful for monitoring and testing.
func (c *Client) ChatLimiterCount() int {
	c.limiterMu.RLock()
	defer c.limiterMu.RUnlock()
	return len(c.chatLimiters)
}

// SendMessage sends a text message.
func (c *Client) SendMessage(ctx context.Context, req SendMessageRequest) (*tg.Message, error) {
	if err := validateChatID(req.ChatID); err != nil {
		return nil, err
	}
	return withRetry(c, ctx, req.ChatID, func() (*tg.Message, error) {
		return c.sendMessageOnce(ctx, req)
	})
}

// SendPhoto sends a photo.
func (c *Client) SendPhoto(ctx context.Context, req SendPhotoRequest) (*tg.Message, error) {
	if err := validateChatID(req.ChatID); err != nil {
		return nil, err
	}
	return withRetry(c, ctx, req.ChatID, func() (*tg.Message, error) {
		return c.sendPhotoOnce(ctx, req)
	})
}

// EditMessageText edits message text.
func (c *Client) EditMessageText(ctx context.Context, req EditMessageTextRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "editMessageText", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// EditMessageCaption edits message caption.
func (c *Client) EditMessageCaption(ctx context.Context, req EditMessageCaptionRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "editMessageCaption", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// EditMessageReplyMarkup edits message reply markup.
func (c *Client) EditMessageReplyMarkup(ctx context.Context, req EditMessageReplyMarkupRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "editMessageReplyMarkup", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// EditMessageMedia edits the media content of a message.
func (c *Client) EditMessageMedia(ctx context.Context, req EditMessageMediaRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "editMessageMedia", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// DeleteMessage deletes a message.
func (c *Client) DeleteMessage(ctx context.Context, req DeleteMessageRequest) error {
	_, err := c.executeRequest(ctx, "deleteMessage", req, extractChatID(req.ChatID))
	return err
}

// ForwardMessage forwards a message.
func (c *Client) ForwardMessage(ctx context.Context, req ForwardMessageRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "forwardMessage", req, extractChatID(req.ChatID))
	if err != nil {
		return nil, err
	}
	return parseMessage(resp)
}

// CopyMessage copies a message.
func (c *Client) CopyMessage(ctx context.Context, req CopyMessageRequest) (*tg.MessageID, error) {
	resp, err := c.executeRequest(ctx, "copyMessage", req, extractChatID(req.ChatID))
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
	resp, err := c.executeRequest(ctx, "sendMessage", req, extractChatID(req.ChatID))
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			return nil, fmt.Errorf("%w: %w", ErrCircuitOpen, err)
		}
		return nil, err
	}
	return parseMessage(resp)
}

func (c *Client) sendPhotoOnce(ctx context.Context, req SendPhotoRequest) (*tg.Message, error) {
	resp, err := c.executeRequest(ctx, "sendPhoto", req, extractChatID(req.ChatID))
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			return nil, fmt.Errorf("%w: %w", ErrCircuitOpen, err)
		}
		return nil, err
	}
	return parseMessage(resp)
}

func (c *Client) executeRequest(ctx context.Context, method string, payload any, chatIDs ...string) (*apiResponse, error) {
	// Apply rate limiting if a chatID is provided
	if len(chatIDs) > 0 && chatIDs[0] != "" {
		if err := c.waitForRateLimit(ctx, chatIDs[0]); err != nil {
			return nil, err
		}
	}
	return c.breaker.Execute(func() (*apiResponse, error) {
		return c.doRequest(ctx, method, payload)
	})
}

func (c *Client) doRequest(ctx context.Context, method string, payload any) (*apiResponse, error) {
	url := fmt.Sprintf("%s/bot%s/%s", c.config.BaseURL, c.config.Token.Value(), method)

	// Check if this request needs multipart encoding (has file uploads)
	multipartReq, err := BuildMultipartRequest(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	var req *http.Request

	if multipartReq.HasUploads() {
		// Use multipart/form-data for file uploads — streamed via io.Pipe
		pr, pw := io.Pipe()
		encoder := NewMultipartEncoder(pw)
		contentType := encoder.ContentType()

		// Encode in a goroutine so the HTTP request streams as data is written
		go func() {
			var encErr error
			if encErr = encoder.Encode(multipartReq); encErr != nil {
				pw.CloseWithError(fmt.Errorf("failed to encode multipart request: %w", encErr))
				return
			}
			if encErr = encoder.Close(); encErr != nil {
				pw.CloseWithError(fmt.Errorf("failed to close multipart encoder: %w", encErr))
				return
			}
			pw.Close()
		}()

		req, err = http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
		if err != nil {
			pr.Close() // Ensure pipe is cleaned up
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", contentType)
	} else {
		// Use JSON for simple requests (no file uploads)
		jsonData, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", marshalErr)
		}

		req, err = http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", scrub.TokenFromError(err, c.config.Token))
	}
	defer resp.Body.Close()

	// P0.8 FIX: Read maxResponseSize+1 to detect overflow without false positive
	limitedReader := io.LimitReader(resp.Body, maxResponseSize+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if int64(len(body)) > maxResponseSize {
		return nil, ErrResponseTooLarge
	}

	var apiResp apiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !apiResp.OK {
		// Parse retry_after: JSON body (primary) + HTTP header (fallback)
		retryAfter := parseRetryAfter(&apiResp, resp)
		if retryAfter > 0 {
			return nil, NewAPIErrorWithRetry(method, apiResp.ErrorCode, apiResp.Description, retryAfter)
		}
		return nil, NewAPIError(method, apiResp.ErrorCode, apiResp.Description)
	}

	return &apiResp, nil
}

func (c *Client) waitForRateLimit(ctx context.Context, chatID string) error {
	limiter := c.getChatLimiter(chatID)
	if err := limiter.Wait(ctx); err != nil {
		return err
	}
	return c.globalLimiter.Wait(ctx)
}

func (c *Client) getChatLimiter(chatID string) *rate.Limiter {
	now := time.Now().UnixNano()

	c.limiterMu.RLock()
	entry, exists := c.chatLimiters[chatID]
	c.limiterMu.RUnlock()

	if exists {
		entry.lastUsed.Store(now) // Lock-free atomic update
		return entry.limiter
	}

	c.limiterMu.Lock()
	defer c.limiterMu.Unlock()

	// Double-check after acquiring write lock
	if entry, exists = c.chatLimiters[chatID]; exists {
		entry.lastUsed.Store(now)
		return entry.limiter
	}

	// Use lower rate for group chats (negative numeric IDs)
	rps := c.config.PerChatRPS
	burst := c.config.PerChatBurst
	if c.config.GroupRPS > 0 {
		if id, err := strconv.ParseInt(chatID, 10, 64); err == nil && id < 0 {
			rps = c.config.GroupRPS
			burst = c.config.GroupBurst
		}
	}

	// Evict oldest if at capacity
	maxLimiters := c.config.MaxChatLimiters
	if maxLimiters <= 0 {
		maxLimiters = 10000
	}
	if len(c.chatLimiters) >= maxLimiters {
		var oldestKey string
		oldestTime := now
		for k, e := range c.chatLimiters {
			if t := e.lastUsed.Load(); t < oldestTime {
				oldestTime = t
				oldestKey = k
			}
		}
		if oldestKey != "" {
			delete(c.chatLimiters, oldestKey)
		}
	}

	// Create new entry with limiter
	entry = &chatLimiterEntry{
		limiter: rate.NewLimiter(rate.Limit(rps), burst),
	}
	entry.lastUsed.Store(now)
	c.chatLimiters[chatID] = entry
	return entry.limiter
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

		// Non-retryable errors return immediately (not wrapped in ErrMaxRetries)
		if !isRetryable(err) {
			return zero, err
		}

		// Check if we've exhausted retries
		if attempt >= c.config.MaxRetries {
			break
		}

		backoff := calculateBackoff(c.config, attempt+1, err)

		// Use sleeper for testable timing
		if err := c.sleeper.Sleep(ctx, backoff); err != nil {
			return zero, err
		}
	}

	return zero, fmt.Errorf("%w: %w", ErrMaxRetries, lastErr)
}

func isRetryable(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}

	// Circuit breaker errors are not retryable
	if errors.Is(err, ErrCircuitOpen) {
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

func extractChatID(chatID tg.ChatID) string {
	switch v := chatID.(type) {
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func parseMessage(resp *apiResponse) (*tg.Message, error) {
	var msg tg.Message
	if err := json.Unmarshal(resp.Result, &msg); err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}
	return &msg, nil
}

// isBreakerSuccess determines if an error should count as a circuit breaker failure.
// Only server errors (5xx) and network errors trip the breaker.
// Client errors (4xx) including 429 are NOT breaker failures.
// 429 is rate pressure (self-inflicted), not service degradation — handle via retry_after.
func isBreakerSuccess(err error) bool {
	if err == nil {
		return true
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		// All 4xx = client-side issues, don't trip breaker.
		// 429 = rate limited — handle via retry_after, not breaker.
		// 5xx = server failure → trip breaker.
		return apiErr.Code >= 400 && apiErr.Code < 500
	}
	// Context cancellation is not a service failure
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// Network errors, timeouts → breaker failure
	return false
}

// parseRetryAfter extracts retry_after from JSON body (primary) or HTTP header (fallback).
func parseRetryAfter(apiResp *apiResponse, httpResp *http.Response) time.Duration {
	// Primary source: JSON response body
	if apiResp.Parameters != nil && apiResp.Parameters.RetryAfter > 0 {
		return time.Duration(apiResp.Parameters.RetryAfter) * time.Second
	}

	// Fallback: HTTP Retry-After header
	if httpResp != nil {
		if retryHeader := httpResp.Header.Get("Retry-After"); retryHeader != "" {
			if seconds, err := strconv.Atoi(retryHeader); err == nil && seconds > 0 {
				return time.Duration(seconds) * time.Second
			}
		}
	}

	return 0
}
