# galigo Testing Implementation Plan v3.1

## Comprehensive Test Strategy for 80%+ Coverage (Final Consolidated)

**Version:** 3.1 (Final with Test Fixes)  
**Target Coverage:** ≥80% (critical packages higher)  
**Go Version:** 1.21+ (compatible, no Go 1.25 assumptions)  
**Testing Framework:** Standard `testing` + `testify` + Go fuzzing  
**Estimated Effort:** 3-4 weeks (9 PRs)

---

## Executive Summary

This final consolidated plan combines multiple independent analyses into a production-ready testing strategy. All known concerns have been addressed with idiomatic Go solutions.

### Key Decisions in v3.0

| Issue | Decision | Rationale |
|-------|----------|-----------|
| `WaitGroup.Go()` | **Don't use** - standard `wg.Add(1)/go func()` | Not in stable Go, future-proof |
| Encapsulation | **Option A+B**: `ChatLimiterCount()` + `chatLimiterStore` extraction | Clean API, testable internals |
| Time injection | **Sleeper interface** for retry, **`cleanup(now)`** for rate limits | Deterministic, fast tests |
| Error goroutines | **`errgroup.Group`** where errors matter | Context-aware, cleaner |
| `fmt` in testutil | **`strconv.Itoa`** instead | Simpler, no fmt dependency |
| Missing coverage | **Add PR2.5 (httpclient) + PR3.5 (breaker)** | Complete internal coverage |

---

## Package Structure & Targets

```
galigo/
├── internal/
│   ├── testutil/           # NEW: Test infrastructure
│   ├── httpclient/         # HTTP client factory (add tests)
│   ├── resilience/         # Breaker, retry, ratelimit (add tests)
│   └── validate/           # Token validation (100% coverage)
├── receiver/               # Webhook + polling (85% coverage)
├── sender/                 # API client (85% coverage)
├── tg/                     # Types, keyboards (90% coverage)
└── bot.go                  # Facade (80% coverage)
```

### Coverage Targets by Package

| Package | Target | Priority |
|---------|--------|----------|
| `sender/` | 85% | Critical |
| `receiver/` | 85% | Critical |
| `tg/` | 90% | High |
| `internal/validate/` | 100% | High |
| `internal/resilience/` | 80% | Medium |
| `internal/httpclient/` | 75% | Medium |
| `bot.go` | 80% | Medium |

---

## Known Test Issues & Solutions

### Issue 1: Empty Body After Capture

**Problem:** Mock server reads `r.Body` for capture, handler can't re-read.

**Solution:** Restore body with `io.NopCloser(bytes.NewReader(body))` after capture.

### Issue 2: Circuit Breaker Opens During Retry Tests

**Problem:** Breaker trips before retry completes, tests fail with wrong error.

**Solution:** 
- Add `WithCircuitBreakerSettings()` (public, clean API)
- Create test helpers: `NewRetryTestClient()` (breaker never trips), `NewBreakerTestClient()` (trips quickly)
- **Do NOT** add `WithCircuitBreakerDisabled()` - bad API design

### Issue 3: Context Cancel Returns ErrRateLimited

**Problem:** Context cancellation during limiter wait wraps as `ErrRateLimited`.

**Solution:** Return context error directly from limiter wait path. Reserve `ErrRateLimited` for Telegram 429 only.

See `galigo_test_fixes_final.md` for complete implementation details.

---

## Critical Refactors for Testability

### 1. Sleeper Interface (Retry Testing)

```go
// internal/resilience/sleeper.go
package resilience

import (
    "context"
    "time"
)

// Sleeper abstracts time-based waiting for deterministic testing.
type Sleeper interface {
    Sleep(ctx context.Context, d time.Duration) error
}

// RealSleeper uses actual time (production).
type RealSleeper struct{}

func (RealSleeper) Sleep(ctx context.Context, d time.Duration) error {
    select {
    case <-ctx.Done():
        return ctx.Err()
    case <-time.After(d):
        return nil
    }
}
```

### 2. ChatLimiterStore Extraction (Rate Limit Testing)

```go
// sender/limiter_store.go
package sender

import (
    "sync"
    "time"

    "golang.org/x/time/rate"
)

// chatLimiterStore manages per-chat rate limiters with cleanup.
type chatLimiterStore struct {
    mu  sync.RWMutex
    m   map[int64]*chatLimiterEntry
    rps rate.Limit
    burst int
    ttl time.Duration
}

type chatLimiterEntry struct {
    limiter  *rate.Limiter
    lastUsed time.Time
}

func newChatLimiterStore(rps float64, burst int, ttl time.Duration) *chatLimiterStore {
    return &chatLimiterStore{
        m:     make(map[int64]*chatLimiterEntry),
        rps:   rate.Limit(rps),
        burst: burst,
        ttl:   ttl,
    }
}

func (s *chatLimiterStore) get(chatID int64) *rate.Limiter {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    entry, exists := s.m[chatID]
    if exists {
        entry.lastUsed = time.Now()
        return entry.limiter
    }
    
    limiter := rate.NewLimiter(s.rps, s.burst)
    s.m[chatID] = &chatLimiterEntry{
        limiter:  limiter,
        lastUsed: time.Now(),
    }
    return limiter
}

// cleanup removes stale entries. Returns count of removed entries.
func (s *chatLimiterStore) cleanup(now time.Time) int {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    removed := 0
    for chatID, entry := range s.m {
        if now.Sub(entry.lastUsed) > s.ttl {
            delete(s.m, chatID)
            removed++
        }
    }
    return removed
}

// count returns the number of active limiters (for monitoring/testing).
func (s *chatLimiterStore) count() int {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return len(s.m)
}

// reset clears all limiters (for test isolation).
func (s *chatLimiterStore) reset() {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.m = make(map[int64]*chatLimiterEntry)
}
```

### 3. Client Integration

```go
// sender/client.go modifications

type Client struct {
    // ... existing fields ...
    chatLimiters    *chatLimiterStore      // Changed from map
    sleeper         resilience.Sleeper     // For retry testing
    breakerSettings CircuitBreakerSettings // Configurable breaker
}

// ChatLimiterCount returns the number of active per-chat limiters.
// Safe for monitoring dashboards and testing.
func (c *Client) ChatLimiterCount() int {
    return c.chatLimiters.count()
}

// Option to inject sleeper for testing
func WithSleeper(s resilience.Sleeper) Option {
    return func(c *Client) {
        c.sleeper = s
    }
}
```

### 4. Circuit Breaker Settings (Configurable for Testing)

```go
// sender/options.go

import "github.com/sony/gobreaker/v2"

// CircuitBreakerSettings configures the circuit breaker behavior.
type CircuitBreakerSettings struct {
    MaxRequests uint32
    Interval    time.Duration
    Timeout     time.Duration
    ReadyToTrip func(counts gobreaker.Counts) bool
}

// DefaultCircuitBreakerSettings returns production-ready defaults.
func DefaultCircuitBreakerSettings() CircuitBreakerSettings {
    return CircuitBreakerSettings{
        MaxRequests: 1,
        Interval:    0,
        Timeout:     60 * time.Second,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            if counts.Requests < 3 {
                return false
            }
            ratio := float64(counts.TotalFailures) / float64(counts.Requests)
            return ratio >= 0.5
        },
    }
}

// WithCircuitBreakerSettings configures the circuit breaker.
func WithCircuitBreakerSettings(settings CircuitBreakerSettings) Option {
    return func(c *Client) {
        c.breakerSettings = settings
    }
}
```

### 5. Test Client Helpers

```go
// internal/testutil/client.go

package testutil

import (
    "testing"
    "time"

    "github.com/sony/gobreaker/v2"
    "github.com/prilive-com/galigo/sender"
    "github.com/stretchr/testify/require"
)

// circuitBreakerNeverTrip returns settings where breaker never opens.
func circuitBreakerNeverTrip() sender.CircuitBreakerSettings {
    return sender.CircuitBreakerSettings{
        MaxRequests: 100,
        Interval:    0,
        Timeout:     time.Hour,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return false // Never trip
        },
    }
}

// circuitBreakerAggressiveTrip returns settings for testing breaker behavior.
func circuitBreakerAggressiveTrip() sender.CircuitBreakerSettings {
    return sender.CircuitBreakerSettings{
        MaxRequests: 1,
        Interval:    0,
        Timeout:     10 * time.Millisecond,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures >= 2
        },
    }
}

// NewRetryTestClient creates a client for testing retry behavior.
// Circuit breaker is configured to never trip.
func NewRetryTestClient(t *testing.T, baseURL string, sleeper *FakeSleeper, opts ...sender.Option) *sender.Client {
    t.Helper()
    
    defaultOpts := []sender.Option{
        sender.WithBaseURL(baseURL),
        sender.WithCircuitBreakerSettings(circuitBreakerNeverTrip()),
    }
    
    if sleeper != nil {
        defaultOpts = append(defaultOpts, sender.WithSleeper(sleeper))
    }
    
    client, err := sender.New(TestToken, append(defaultOpts, opts...)...)
    require.NoError(t, err)
    
    t.Cleanup(func() { client.Close() })
    return client
}

// NewBreakerTestClient creates a client for testing circuit breaker behavior.
func NewBreakerTestClient(t *testing.T, baseURL string, opts ...sender.Option) *sender.Client {
    t.Helper()
    
    defaultOpts := []sender.Option{
        sender.WithBaseURL(baseURL),
        sender.WithCircuitBreakerSettings(circuitBreakerAggressiveTrip()),
        sender.WithRetries(0),
    }
    
    client, err := sender.New(TestToken, append(defaultOpts, opts...)...)
    require.NoError(t, err)
    
    t.Cleanup(func() { client.Close() })
    return client
}

// NewTestClient creates a standard test client with default settings.
func NewTestClient(t *testing.T, baseURL string, opts ...sender.Option) *sender.Client {
    t.Helper()
    
    defaultOpts := []sender.Option{
        sender.WithBaseURL(baseURL),
    }
    
    client, err := sender.New(TestToken, append(defaultOpts, opts...)...)
    require.NoError(t, err)
    
    t.Cleanup(func() { client.Close() })
    return client
}
```

### 6. Context Error Handling Fix

```go
// sender/client.go - Fix context error return

func (c *Client) waitForRateLimit(ctx context.Context, chatID int64) error {
    // Global limiter
    if c.globalLimiter != nil {
        if err := c.globalLimiter.Wait(ctx); err != nil {
            // ✅ Return context error directly, not wrapped
            return err
        }
    }
    
    // Per-chat limiter
    if chatID != 0 {
        limiter := c.chatLimiters.get(chatID)
        if err := limiter.Wait(ctx); err != nil {
            return err
        }
    }
    
    return nil
}
```

```go
// sender/errors.go - Error helpers

// IsContextError returns true if the error is due to context cancellation or timeout.
func IsContextError(err error) bool {
    return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// IsTelegramRateLimit returns true if Telegram returned 429.
func IsTelegramRateLimit(err error) bool {
    return errors.Is(err, ErrRateLimited)
}
```

---

## Test File Structure

```
galigo/
├── internal/
│   ├── testutil/
│   │   ├── server.go           # Mock Telegram API server
│   │   ├── capture.go          # Request capture & assertions
│   │   ├── replies.go          # Standard Telegram responses
│   │   ├── fixtures.go         # Test data fixtures
│   │   └── sleeper.go          # FakeSleeper for tests
│   │
│   ├── httpclient/
│   │   └── client_test.go      # HTTP factory tests
│   │
│   ├── resilience/
│   │   ├── breaker_test.go     # Circuit breaker tests
│   │   ├── retry_test.go       # Retry logic tests
│   │   └── sleeper_test.go     # Sleeper tests
│   │
│   └── validate/
│       └── validate_test.go    # Validation tests (100%)
│
├── tg/
│   ├── types_test.go           # Type serialization
│   ├── keyboard_test.go        # Keyboard builders
│   ├── errors_test.go          # Error sentinels
│   ├── secret_test.go          # Redaction tests
│   ├── update_test.go          # Update parsing
│   ├── fuzz_test.go            # Fuzzing targets
│   └── testdata/
│       └── updates/            # Golden JSON fixtures
│
├── sender/
│   ├── executor_test.go        # Core request/response
│   ├── retry_test.go           # 429, 5xx, backoff
│   ├── ratelimit_test.go       # Global + per-chat
│   ├── limiter_store_test.go   # Store + cleanup
│   ├── messages_test.go        # sendMessage, edit, delete
│   ├── media_test.go           # sendPhoto, editCaption
│   ├── forward_copy_test.go    # forward, copy
│   ├── callback_test.go        # answerCallbackQuery
│   └── config_test.go          # Config validation
│
├── receiver/
│   ├── polling_test.go         # Basic polling
│   ├── polling_offset_test.go  # Offset progression
│   ├── polling_stop_test.go    # Graceful shutdown
│   ├── webhook_test.go         # Handler tests
│   ├── webhook_auth_test.go    # Secret validation
│   ├── api_test.go             # setWebhook, deleteWebhook
│   └── config_test.go          # Config tests
│
├── bot_test.go                 # Facade tests
└── integration_test.go         # E2E tests (build tag)
```

---

## PR Structure (9 PRs)

| PR | Focus | Hours | Coverage Impact |
|----|-------|-------|-----------------|
| **PR1** | Test Infrastructure | 6-8 | Foundation |
| **PR2** | Sender Executor + Retry | 6-8 | +15% sender |
| **PR2.5** | internal/httpclient Tests | 2-3 | +75% httpclient |
| **PR3** | Rate Limiting + Store | 4-6 | +10% sender |
| **PR3.5** | Circuit Breaker Tests | 3-4 | +80% resilience |
| **PR4** | Sender Method Tests | 6-8 | +20% sender |
| **PR5** | Receiver Tests | 6-8 | +85% receiver |
| **PR6** | tg Package + Fuzzing | 5-7 | +90% tg |
| **PR7** | Bot Facade + CI Gate | 4-5 | ≥80% overall |
| **TOTAL** | | **43-57** | **≥80%** |

---

## PR1: Test Infrastructure

**Goal:** Build reusable test harness.  
**Estimated Time:** 6-8 hours

### 1.1 Mock Server (`internal/testutil/server.go`)

```go
package testutil

import (
    "bytes"
    "encoding/json"
    "io"
    "net/http"
    "net/http/httptest"
    "sync"
    "testing"
    "time"
)

// Capture represents a captured HTTP request with timestamp.
type Capture struct {
    Method      string
    Path        string
    Query       map[string][]string
    Headers     http.Header
    Body        []byte
    ContentType string
    Timestamp   time.Time // For rate-limit verification
}

// MockTelegramServer provides a mock Telegram Bot API server.
type MockTelegramServer struct {
    *httptest.Server
    t        *testing.T
    mu       sync.Mutex
    handlers map[string]http.HandlerFunc
    captures []Capture
}

// NewMockServer creates a mock Telegram API server.
func NewMockServer(t *testing.T) *MockTelegramServer {
    t.Helper()
    
    m := &MockTelegramServer{
        t:        t,
        handlers: make(map[string]http.HandlerFunc),
        captures: make([]Capture, 0),
    }
    
    m.Server = httptest.NewServer(http.HandlerFunc(m.handle))
    t.Cleanup(m.Server.Close)
    return m
}

func (m *MockTelegramServer) handle(w http.ResponseWriter, r *http.Request) {
    // Read body once
    body, _ := io.ReadAll(r.Body)
    r.Body.Close()
    
    // ✅ KEY FIX: Restore body for downstream handler
    r.Body = io.NopCloser(bytes.NewReader(body))
    
    // Capture request with timestamp
    m.mu.Lock()
    m.captures = append(m.captures, Capture{
        Method:      r.Method,
        Path:        r.URL.Path,
        Query:       r.URL.Query(),
        Headers:     r.Header.Clone(),
        Body:        body,
        ContentType: r.Header.Get("Content-Type"),
        Timestamp:   time.Now(),
    })
    m.mu.Unlock()
    
    // Find and call handler (can now decode r.Body if needed)
    key := r.Method + ":" + r.URL.Path
    m.mu.Lock()
    handler, exists := m.handlers[key]
    m.mu.Unlock()
    
    if exists {
        handler(w, r)
        return
    }
    
    // Default: success response
    ReplyOK(w, map[string]any{})
}

// OnMethod registers a handler for a specific API method.
func (m *MockTelegramServer) OnMethod(method, path string, handler http.HandlerFunc) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.handlers[method+":"+path] = handler
}

// Captures returns all captured requests.
func (m *MockTelegramServer) Captures() []Capture {
    m.mu.Lock()
    defer m.mu.Unlock()
    return append([]Capture{}, m.captures...)
}

// LastCapture returns the most recent request.
func (m *MockTelegramServer) LastCapture() *Capture {
    m.mu.Lock()
    defer m.mu.Unlock()
    if len(m.captures) == 0 {
        return nil
    }
    return &m.captures[len(m.captures)-1]
}

// CaptureCount returns total captured requests.
func (m *MockTelegramServer) CaptureCount() int {
    m.mu.Lock()
    defer m.mu.Unlock()
    return len(m.captures)
}

// Reset clears all captures and handlers.
func (m *MockTelegramServer) Reset() {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.captures = m.captures[:0]
    m.handlers = make(map[string]http.HandlerFunc)
}

// TimeBetweenCaptures returns duration between two captures (for rate-limit tests).
func (m *MockTelegramServer) TimeBetweenCaptures(i, j int) time.Duration {
    m.mu.Lock()
    defer m.mu.Unlock()
    if i >= len(m.captures) || j >= len(m.captures) {
        return 0
    }
    return m.captures[j].Timestamp.Sub(m.captures[i].Timestamp)
}

// BaseURL returns the server's base URL.
func (m *MockTelegramServer) BaseURL() string {
    return m.Server.URL
}
```

### 1.2 Telegram Replies (`internal/testutil/replies.go`)

```go
package testutil

import (
    "encoding/json"
    "net/http"
    "strconv"
)

// TelegramEnvelope is the standard Telegram API response format.
type TelegramEnvelope struct {
    OK          bool        `json:"ok"`
    Result      any         `json:"result,omitempty"`
    ErrorCode   int         `json:"error_code,omitempty"`
    Description string      `json:"description,omitempty"`
    Parameters  *Parameters `json:"parameters,omitempty"`
}

// Parameters contains optional error parameters (e.g., retry_after).
type Parameters struct {
    RetryAfter      int   `json:"retry_after,omitempty"`
    MigrateToChatID int64 `json:"migrate_to_chat_id,omitempty"`
}

// ReplyOK writes a successful Telegram API response.
func ReplyOK(w http.ResponseWriter, result any) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(TelegramEnvelope{
        OK:     true,
        Result: result,
    })
}

// ReplyError writes a Telegram API error response.
func ReplyError(w http.ResponseWriter, code int, description string, params *Parameters) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(TelegramEnvelope{
        OK:          false,
        ErrorCode:   code,
        Description: description,
        Parameters:  params,
    })
}

// ReplyRateLimit writes a 429 rate limit response with retry_after.
func ReplyRateLimit(w http.ResponseWriter, retryAfter int) {
    w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
    ReplyError(w, 429, "Too Many Requests: retry after "+strconv.Itoa(retryAfter), &Parameters{
        RetryAfter: retryAfter,
    })
}

// ReplyServerError writes a 5xx server error.
func ReplyServerError(w http.ResponseWriter, code int, description string) {
    ReplyError(w, code, description, nil)
}

// ReplyMessage writes a successful message response.
func ReplyMessage(w http.ResponseWriter, messageID int) {
    ReplyOK(w, map[string]any{
        "message_id": messageID,
        "date":       1234567890,
        "chat": map[string]any{
            "id":   TestChatID,
            "type": "private",
        },
        "text": "Test message",
    })
}

// ReplyBool writes a successful boolean response (for deleteMessage, etc.).
func ReplyBool(w http.ResponseWriter, result bool) {
    ReplyOK(w, result)
}

// Test constants
const (
    TestToken    = "123456789:ABCdefGHIjklMNOpqrsTUVwxyz"
    TestChatID   = int64(123456789)
    TestUserID   = int64(987654321)
    TestUsername = "@testuser"
)
```

### 1.3 FakeSleeper (`internal/testutil/sleeper.go`)

```go
package testutil

import (
    "context"
    "sync"
    "time"
)

// FakeSleeper records sleep calls without waiting (for deterministic tests).
type FakeSleeper struct {
    mu    sync.Mutex
    calls []time.Duration
}

// Sleep records the duration without actually sleeping.
func (f *FakeSleeper) Sleep(ctx context.Context, d time.Duration) error {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
        f.mu.Lock()
        f.calls = append(f.calls, d)
        f.mu.Unlock()
        return nil
    }
}

// Calls returns all recorded sleep durations.
func (f *FakeSleeper) Calls() []time.Duration {
    f.mu.Lock()
    defer f.mu.Unlock()
    return append([]time.Duration{}, f.calls...)
}

// TotalDuration returns the sum of all sleep durations.
func (f *FakeSleeper) TotalDuration() time.Duration {
    f.mu.Lock()
    defer f.mu.Unlock()
    var total time.Duration
    for _, d := range f.calls {
        total += d
    }
    return total
}

// CallCount returns the number of sleep calls.
func (f *FakeSleeper) CallCount() int {
    f.mu.Lock()
    defer f.mu.Unlock()
    return len(f.calls)
}

// LastCall returns the most recent sleep duration.
func (f *FakeSleeper) LastCall() time.Duration {
    f.mu.Lock()
    defer f.mu.Unlock()
    if len(f.calls) == 0 {
        return 0
    }
    return f.calls[len(f.calls)-1]
}

// Reset clears all recorded calls.
func (f *FakeSleeper) Reset() {
    f.mu.Lock()
    defer f.mu.Unlock()
    f.calls = f.calls[:0]
}
```

### 1.4 Capture Assertions (`internal/testutil/capture.go`)

```go
package testutil

import (
    "encoding/json"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// AssertPath verifies the request path.
func (c *Capture) AssertPath(t *testing.T, expected string) {
    t.Helper()
    assert.Equal(t, expected, c.Path, "unexpected path")
}

// AssertMethod verifies the HTTP method.
func (c *Capture) AssertMethod(t *testing.T, expected string) {
    t.Helper()
    assert.Equal(t, expected, c.Method, "unexpected method")
}

// AssertContentType verifies the Content-Type header.
func (c *Capture) AssertContentType(t *testing.T, expected string) {
    t.Helper()
    assert.Contains(t, c.ContentType, expected, "unexpected content-type")
}

// AssertHeader verifies a specific header value.
func (c *Capture) AssertHeader(t *testing.T, key, expected string) {
    t.Helper()
    assert.Equal(t, expected, c.Headers.Get(key), "unexpected header "+key)
}

// AssertJSONField verifies a field in the JSON body.
func (c *Capture) AssertJSONField(t *testing.T, field string, expected any) {
    t.Helper()
    var body map[string]any
    require.NoError(t, json.Unmarshal(c.Body, &body), "failed to parse JSON body")
    assert.Equal(t, expected, body[field], "unexpected value for field "+field)
}

// AssertJSONFieldExists verifies a field exists in the JSON body.
func (c *Capture) AssertJSONFieldExists(t *testing.T, field string) {
    t.Helper()
    var body map[string]any
    require.NoError(t, json.Unmarshal(c.Body, &body), "failed to parse JSON body")
    assert.Contains(t, body, field, "field should exist: "+field)
}

// AssertJSONFieldAbsent verifies a field does NOT exist in the JSON body.
func (c *Capture) AssertJSONFieldAbsent(t *testing.T, field string) {
    t.Helper()
    var body map[string]any
    require.NoError(t, json.Unmarshal(c.Body, &body), "failed to parse JSON body")
    assert.NotContains(t, body, field, "field should be absent: "+field)
}

// BodyJSON decodes the body as JSON into target.
func (c *Capture) BodyJSON(t *testing.T, target any) {
    t.Helper()
    require.NoError(t, json.Unmarshal(c.Body, target), "failed to decode JSON body")
}

// BodyMap returns the body as a map (convenience).
func (c *Capture) BodyMap(t *testing.T) map[string]any {
    t.Helper()
    var m map[string]any
    require.NoError(t, json.Unmarshal(c.Body, &m), "failed to decode JSON body")
    return m
}
```

### 1.5 Test Fixtures (`internal/testutil/fixtures.go`)

```go
package testutil

import "github.com/prilive-com/galigo/tg"

// TestUser returns a test user fixture.
func TestUser() *tg.User {
    return &tg.User{
        ID:        TestUserID,
        IsBot:     false,
        FirstName: "Test",
        LastName:  "User",
        Username:  "testuser",
    }
}

// TestChat returns a test chat fixture.
func TestChat() *tg.Chat {
    return &tg.Chat{
        ID:        TestChatID,
        Type:      "private",
        FirstName: "Test",
        LastName:  "User",
        Username:  "testuser",
    }
}

// TestMessage returns a test message fixture.
func TestMessage(messageID int, text string) *tg.Message {
    return &tg.Message{
        MessageID: messageID,
        Date:      1234567890,
        Chat:      TestChat(),
        From:      TestUser(),
        Text:      text,
    }
}

// TestUpdate returns a test update fixture.
func TestUpdate(updateID int, text string) tg.Update {
    return tg.Update{
        UpdateID: updateID,
        Message:  TestMessage(1, text),
    }
}

// TestCallbackQuery returns a test callback query fixture.
func TestCallbackQuery(id, data string) *tg.CallbackQuery {
    return &tg.CallbackQuery{
        ID:           id,
        From:         TestUser(),
        Message:      TestMessage(1, "Original"),
        ChatInstance: "instance_123",
        Data:         data,
    }
}
```

### Definition of Done (PR1)

- [ ] `internal/testutil/` package created
- [ ] MockTelegramServer with capture + timestamps
- [ ] Telegram reply helpers (using `strconv`)
- [ ] FakeSleeper for deterministic timing
- [ ] Capture assertion helpers
- [ ] Test fixtures
- [ ] Demo test proving harness works

---

## PR2: Sender Executor + Retry Tests

**Goal:** Test core request/response handling with deterministic retry.  
**Estimated Time:** 6-8 hours

### 2.1 Executor Tests (`sender/executor_test.go`)

```go
package sender_test

import (
    "context"
    "errors"
    "net/http"
    "testing"
    "time"

    "github.com/prilive-com/galigo/internal/testutil"
    "github.com/prilive-com/galigo/sender"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestExecutor_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.OnMethod("POST", "/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyMessage(w, 123)
    })
    
    client := newTestClient(t, server.BaseURL())
    ctx := context.Background()
    
    msg, err := client.SendMessage(ctx, sender.SendMessageRequest{
        ChatID: testutil.TestChatID,
        Text:   "Hello",
    })
    
    require.NoError(t, err)
    assert.Equal(t, 123, msg.MessageID)
    
    // Verify request
    cap := server.LastCapture()
    require.NotNil(t, cap)
    cap.AssertMethod(t, "POST")
    cap.AssertContentType(t, "application/json")
    cap.AssertJSONField(t, "chat_id", float64(testutil.TestChatID))
    cap.AssertJSONField(t, "text", "Hello")
}

func TestExecutor_TelegramError(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.OnMethod("POST", "/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyError(w, 400, "Bad Request: chat not found", nil)
    })
    
    client := newTestClient(t, server.BaseURL())
    
    _, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
        ChatID: testutil.TestChatID,
        Text:   "Hello",
    })
    
    require.Error(t, err)
    
    var apiErr *sender.APIError
    require.ErrorAs(t, err, &apiErr)
    assert.Equal(t, 400, apiErr.Code)
    assert.Contains(t, apiErr.Description, "chat not found")
}

func TestExecutor_ContextCancellation(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.OnMethod("POST", "/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(5 * time.Second)
        testutil.ReplyMessage(w, 123)
    })
    
    client := newTestClient(t, server.BaseURL())
    
    ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
    defer cancel()
    
    _, err := client.SendMessage(ctx, sender.SendMessageRequest{
        ChatID: testutil.TestChatID,
        Text:   "Hello",
    })
    
    require.Error(t, err)
    assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled))
}

func TestExecutor_NonJSONResponse(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.OnMethod("POST", "/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusInternalServerError)
        w.Write([]byte("Internal Server Error"))
    })
    
    client := newTestClient(t, server.BaseURL())
    
    _, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
        ChatID: testutil.TestChatID,
        Text:   "Hello",
    })
    
    require.Error(t, err)
}
```

### 2.2 Retry Tests (`sender/retry_test.go`)

```go
package sender_test

import (
    "context"
    "errors"
    "net/http"
    "sync/atomic"
    "testing"
    "time"

    "github.com/prilive-com/galigo/internal/testutil"
    "github.com/prilive-com/galigo/sender"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestRetry_429WithRetryAfter(t *testing.T) {
    var attempts int32
    
    server := testutil.NewMockServer(t)
    server.OnMethod("POST", "/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        if atomic.AddInt32(&attempts, 1) == 1 {
            testutil.ReplyRateLimit(w, 2) // 2 seconds
            return
        }
        testutil.ReplyMessage(w, 123)
    })
    
    sleeper := &testutil.FakeSleeper{}
    // ✅ Use retry test client (breaker won't interfere)
    client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper)
    
    msg, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
        ChatID: testutil.TestChatID,
        Text:   "Hello",
    })
    
    require.NoError(t, err)
    assert.Equal(t, 123, msg.MessageID)
    assert.Equal(t, int32(2), atomic.LoadInt32(&attempts))
    
    // Verify sleeper used Telegram's retry_after
    assert.Equal(t, 1, sleeper.CallCount())
    assert.Equal(t, 2*time.Second, sleeper.LastCall())
}

func TestRetry_5xxWithExponentialBackoff(t *testing.T) {
    var attempts int32
    
    server := testutil.NewMockServer(t)
    server.OnMethod("POST", "/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        if atomic.AddInt32(&attempts, 1) <= 2 {
            testutil.ReplyServerError(w, 502, "Bad Gateway")
            return
        }
        testutil.ReplyMessage(w, 123)
    })
    
    sleeper := &testutil.FakeSleeper{}
    client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRetries(3))
    
    msg, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
        ChatID: testutil.TestChatID,
        Text:   "Hello",
    })
    
    require.NoError(t, err)
    assert.Equal(t, 123, msg.MessageID)
    assert.Equal(t, int32(3), atomic.LoadInt32(&attempts))
    
    // Verify exponential backoff (second sleep > first)
    calls := sleeper.Calls()
    assert.Equal(t, 2, len(calls))
    assert.Less(t, calls[0], calls[1], "backoff should increase")
}

func TestRetry_NoRetryOn4xx(t *testing.T) {
    var attempts int32
    
    server := testutil.NewMockServer(t)
    server.OnMethod("POST", "/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        atomic.AddInt32(&attempts, 1)
        testutil.ReplyError(w, 400, "Bad Request", nil)
    })
    
    client := testutil.NewRetryTestClient(t, server.BaseURL(), nil, sender.WithRetries(3))
    
    _, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
        ChatID: testutil.TestChatID,
        Text:   "Hello",
    })
    
    require.Error(t, err)
    assert.Equal(t, int32(1), atomic.LoadInt32(&attempts), "should not retry 4xx")
}

func TestRetry_ContextCancelStopsRetry(t *testing.T) {
    var attempts int32
    
    server := testutil.NewMockServer(t)
    server.OnMethod("POST", "/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        atomic.AddInt32(&attempts, 1)
        testutil.ReplyRateLimit(w, 60) // Long retry_after
    })
    
    sleeper := &testutil.FakeSleeper{}
    client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper)
    
    ctx, cancel := context.WithCancel(context.Background())
    
    // Cancel context shortly after first attempt
    go func() {
        time.Sleep(10 * time.Millisecond)
        cancel()
    }()
    
    start := time.Now()
    _, err := client.SendMessage(ctx, sender.SendMessageRequest{
        ChatID: testutil.TestChatID,
        Text:   "Hello",
    })
    elapsed := time.Since(start)
    
    require.Error(t, err)
    
    // ✅ Clean assertion: context error returned directly
    assert.True(t, errors.Is(err, context.Canceled), "expected context.Canceled, got: %v", err)
    assert.Less(t, elapsed, 500*time.Millisecond, "should exit quickly on cancel")
}

func TestRetry_MaxRetriesExceeded(t *testing.T) {
    var attempts int32
    
    server := testutil.NewMockServer(t)
    server.OnMethod("POST", "/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        atomic.AddInt32(&attempts, 1)
        testutil.ReplyServerError(w, 500, "Internal Server Error")
    })
    
    sleeper := &testutil.FakeSleeper{}
    client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRetries(2))
    
    _, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
        ChatID: testutil.TestChatID,
        Text:   "Hello",
    })
    
    require.Error(t, err)
    assert.ErrorIs(t, err, sender.ErrMaxRetries)
    assert.Equal(t, int32(3), atomic.LoadInt32(&attempts)) // 1 initial + 2 retries
}
```

### Definition of Done (PR2)

- [ ] Executor success/error tests
- [ ] Non-JSON response handling
- [ ] Context cancellation
- [ ] Retry with Telegram's `retry_after`
- [ ] Retry on 5xx with exponential backoff
- [ ] No retry on 4xx
- [ ] Context cancel stops retry
- [ ] Max retries limit

---

## PR2.5: internal/httpclient Tests

**Goal:** Test HTTP client factory defaults.  
**Estimated Time:** 2-3 hours

```go
// internal/httpclient/client_test.go
package httpclient_test

import (
    "crypto/tls"
    "net/http"
    "testing"
    "time"

    "github.com/prilive-com/galigo/internal/httpclient"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestNew_ReturnsNonNilClient(t *testing.T) {
    client := httpclient.New()
    require.NotNil(t, client)
}

func TestNew_DefaultTimeout(t *testing.T) {
    client := httpclient.New()
    // Check timeout is set to a reasonable default
    assert.Greater(t, client.Timeout, time.Duration(0))
}

func TestNew_WithTimeout(t *testing.T) {
    client := httpclient.New(httpclient.WithTimeout(42 * time.Second))
    assert.Equal(t, 42*time.Second, client.Timeout)
}

func TestNew_TLSMinVersion(t *testing.T) {
    client := httpclient.New()
    
    transport, ok := client.Transport.(*http.Transport)
    require.True(t, ok, "transport should be *http.Transport")
    require.NotNil(t, transport.TLSClientConfig)
    
    assert.GreaterOrEqual(t, transport.TLSClientConfig.MinVersion, uint16(tls.VersionTLS12))
}

func TestNew_ConnectionPooling(t *testing.T) {
    client := httpclient.New()
    
    transport, ok := client.Transport.(*http.Transport)
    require.True(t, ok)
    
    assert.Greater(t, transport.MaxIdleConns, 0)
    assert.Greater(t, transport.MaxIdleConnsPerHost, 0)
}

func TestNew_HTTP2Enabled(t *testing.T) {
    client := httpclient.New()
    
    transport, ok := client.Transport.(*http.Transport)
    require.True(t, ok)
    
    assert.True(t, transport.ForceAttemptHTTP2)
}
```

### Definition of Done (PR2.5)

- [ ] Factory returns non-nil client
- [ ] Timeout configuration works
- [ ] TLS 1.2 minimum enforced
- [ ] Connection pooling configured
- [ ] HTTP/2 enabled

---

## PR3: Rate Limiting Tests

**Goal:** Test global and per-chat rate limiting.  
**Estimated Time:** 4-6 hours

### 3.1 Rate Limit Tests (`sender/ratelimit_test.go`)

```go
package sender_test

import (
    "context"
    "net/http"
    "sync"
    "testing"
    "time"

    "github.com/prilive-com/galigo/internal/testutil"
    "github.com/prilive-com/galigo/sender"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestRateLimit_GlobalLimiter(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.OnMethod("POST", "/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyMessage(w, 123)
    })
    
    // 2 RPS, burst 1
    client := newTestClient(t, server.BaseURL(), sender.WithRateLimit(2, 1))
    
    start := time.Now()
    
    for i := 0; i < 3; i++ {
        _, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
            ChatID: testutil.TestChatID,
            Text:   "Hello",
        })
        require.NoError(t, err)
    }
    
    elapsed := time.Since(start)
    
    // 3 requests at 2 RPS should take ~1 second
    assert.GreaterOrEqual(t, elapsed, 500*time.Millisecond)
}

func TestRateLimit_PerChatIndependence(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.OnMethod("POST", "/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyMessage(w, 123)
    })
    
    client := newTestClient(t, server.BaseURL(),
        sender.WithRateLimit(100, 100), // High global limit
        sender.WithPerChatRateLimit(1, 1), // 1 RPS per chat
    )
    
    start := time.Now()
    
    var wg sync.WaitGroup
    chatIDs := []int64{111, 222, 333}
    
    for _, chatID := range chatIDs {
        wg.Add(1)
        go func(cid int64) {
            defer wg.Done()
            _, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
                ChatID: cid,
                Text:   "Hello",
            })
            require.NoError(t, err)
        }(chatID)
    }
    
    wg.Wait()
    elapsed := time.Since(start)
    
    // Different chats should process in parallel
    assert.Less(t, elapsed, 500*time.Millisecond)
}

func TestRateLimit_SameChatThrottled(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.OnMethod("POST", "/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyMessage(w, 123)
    })
    
    client := newTestClient(t, server.BaseURL(),
        sender.WithRateLimit(100, 100),
        sender.WithPerChatRateLimit(1, 1), // 1 RPS per chat
    )
    
    start := time.Now()
    
    // 2 messages to same chat
    for i := 0; i < 2; i++ {
        _, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
            ChatID: testutil.TestChatID,
            Text:   "Hello",
        })
        require.NoError(t, err)
    }
    
    elapsed := time.Since(start)
    
    // Same chat should be throttled
    assert.GreaterOrEqual(t, elapsed, time.Second)
}

func TestRateLimit_LimiterCount(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.OnMethod("POST", "/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyMessage(w, 123)
    })
    
    client := newTestClient(t, server.BaseURL())
    
    // Initially no chat limiters
    assert.Equal(t, 0, client.ChatLimiterCount())
    
    // Send to 3 different chats
    for _, chatID := range []int64{111, 222, 333} {
        _, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
            ChatID: chatID,
            Text:   "Hello",
        })
        require.NoError(t, err)
    }
    
    // Should have 3 chat limiters
    assert.Equal(t, 3, client.ChatLimiterCount())
}
```

### 3.2 Limiter Store Tests (`sender/limiter_store_test.go`)

```go
package sender

import (
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
)

func TestChatLimiterStore_Get_CreatesNew(t *testing.T) {
    store := newChatLimiterStore(1.0, 1, 5*time.Minute)
    
    limiter := store.get(123)
    
    assert.NotNil(t, limiter)
    assert.Equal(t, 1, store.count())
}

func TestChatLimiterStore_Get_ReusesExisting(t *testing.T) {
    store := newChatLimiterStore(1.0, 1, 5*time.Minute)
    
    limiter1 := store.get(123)
    limiter2 := store.get(123)
    
    assert.Same(t, limiter1, limiter2)
    assert.Equal(t, 1, store.count())
}

func TestChatLimiterStore_Cleanup_RemovesStale(t *testing.T) {
    store := newChatLimiterStore(1.0, 1, 100*time.Millisecond)
    
    _ = store.get(123)
    assert.Equal(t, 1, store.count())
    
    // Wait for entry to become stale
    time.Sleep(150 * time.Millisecond)
    
    removed := store.cleanup(time.Now())
    
    assert.Equal(t, 1, removed)
    assert.Equal(t, 0, store.count())
}

func TestChatLimiterStore_Cleanup_KeepsRecent(t *testing.T) {
    store := newChatLimiterStore(1.0, 1, 1*time.Second)
    
    _ = store.get(123)
    
    // Cleanup immediately - should keep entry
    removed := store.cleanup(time.Now())
    
    assert.Equal(t, 0, removed)
    assert.Equal(t, 1, store.count())
}

func TestChatLimiterStore_Cleanup_RefreshesOnAccess(t *testing.T) {
    store := newChatLimiterStore(1.0, 1, 100*time.Millisecond)
    
    _ = store.get(123)
    
    // Wait half TTL
    time.Sleep(60 * time.Millisecond)
    
    // Access again (refreshes lastUsed)
    _ = store.get(123)
    
    // Wait another half TTL
    time.Sleep(60 * time.Millisecond)
    
    // Should still exist (was refreshed)
    removed := store.cleanup(time.Now())
    
    assert.Equal(t, 0, removed)
    assert.Equal(t, 1, store.count())
}

func TestChatLimiterStore_Reset(t *testing.T) {
    store := newChatLimiterStore(1.0, 1, 5*time.Minute)
    
    _ = store.get(111)
    _ = store.get(222)
    _ = store.get(333)
    
    assert.Equal(t, 3, store.count())
    
    store.reset()
    
    assert.Equal(t, 0, store.count())
}
```

### Definition of Done (PR3)

- [ ] Global limiter throttles
- [ ] Per-chat independence
- [ ] Same chat throttled
- [ ] Limiter count exposed
- [ ] Store creates/reuses entries
- [ ] Cleanup removes stale
- [ ] Cleanup keeps recent
- [ ] Reset clears all

---

## PR3.5: Circuit Breaker Tests

**Goal:** Test breaker state transitions.  
**Estimated Time:** 3-4 hours

```go
// internal/resilience/breaker_test.go
package resilience_test

import (
    "errors"
    "testing"
    "time"

    "github.com/sony/gobreaker/v2"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestBreaker_StartsInClosedState(t *testing.T) {
    cb := gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
        Name: "test",
    })
    
    assert.Equal(t, gobreaker.StateClosed, cb.State())
}

func TestBreaker_OpensAfterConsecutiveFailures(t *testing.T) {
    cb := gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
        Name: "test",
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures >= 3
        },
    })
    
    testErr := errors.New("test error")
    
    for i := 0; i < 3; i++ {
        _, _ = cb.Execute(func() (any, error) {
            return nil, testErr
        })
    }
    
    assert.Equal(t, gobreaker.StateOpen, cb.State())
}

func TestBreaker_RejectsWhenOpen(t *testing.T) {
    cb := gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
        Name: "test",
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures >= 1
        },
    })
    
    // Trip the breaker
    _, _ = cb.Execute(func() (any, error) {
        return nil, errors.New("fail")
    })
    
    assert.Equal(t, gobreaker.StateOpen, cb.State())
    
    // Should reject
    _, err := cb.Execute(func() (any, error) {
        return "success", nil
    })
    
    assert.ErrorIs(t, err, gobreaker.ErrOpenState)
}

func TestBreaker_TransitionsToHalfOpen(t *testing.T) {
    cb := gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
        Name:        "test",
        Timeout:     10 * time.Millisecond, // Short timeout for test
        MaxRequests: 1,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures >= 1
        },
    })
    
    // Trip
    _, _ = cb.Execute(func() (any, error) {
        return nil, errors.New("fail")
    })
    
    assert.Equal(t, gobreaker.StateOpen, cb.State())
    
    // Wait for timeout
    time.Sleep(20 * time.Millisecond)
    
    // Next request should be allowed (half-open probe)
    result, err := cb.Execute(func() (any, error) {
        return "success", nil
    })
    
    assert.NoError(t, err)
    assert.Equal(t, "success", result)
    assert.Equal(t, gobreaker.StateClosed, cb.State())
}

func TestBreaker_HalfOpenFailureReopens(t *testing.T) {
    cb := gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
        Name:        "test",
        Timeout:     10 * time.Millisecond,
        MaxRequests: 1,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures >= 1
        },
    })
    
    // Trip
    _, _ = cb.Execute(func() (any, error) {
        return nil, errors.New("fail")
    })
    
    // Wait for timeout
    time.Sleep(20 * time.Millisecond)
    
    // Probe fails
    _, _ = cb.Execute(func() (any, error) {
        return nil, errors.New("still failing")
    })
    
    // Should be open again
    assert.Equal(t, gobreaker.StateOpen, cb.State())
}

func TestBreaker_SuccessInClosedResets(t *testing.T) {
    cb := gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
        Name: "test",
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures >= 3
        },
    })
    
    // 2 failures
    for i := 0; i < 2; i++ {
        _, _ = cb.Execute(func() (any, error) {
            return nil, errors.New("fail")
        })
    }
    
    // 1 success resets
    _, _ = cb.Execute(func() (any, error) {
        return "ok", nil
    })
    
    // 2 more failures shouldn't trip (counter was reset)
    for i := 0; i < 2; i++ {
        _, _ = cb.Execute(func() (any, error) {
            return nil, errors.New("fail")
        })
    }
    
    assert.Equal(t, gobreaker.StateClosed, cb.State())
}
```

### Definition of Done (PR3.5)

- [ ] Starts in closed state
- [ ] Opens after consecutive failures
- [ ] Rejects when open
- [ ] Transitions to half-open after timeout
- [ ] Half-open success closes
- [ ] Half-open failure reopens
- [ ] Success resets failure counter

---

## PR4-PR7: Remaining PRs

*(Same as v2.0 plan with the improvements already integrated)*

| PR | Focus | Key Changes from v2.0 |
|----|-------|----------------------|
| **PR4** | Sender Method Tests | No changes needed |
| **PR5** | Receiver Tests | No changes needed |
| **PR6** | tg + Fuzzing | No changes needed |
| **PR7** | Bot Facade + CI | No changes needed |

---

## CI Configuration

### Makefile

```makefile
.PHONY: test test-coverage test-race test-fuzz lint ci

GO_PACKAGES := ./...

test:
	go test -v $(GO_PACKAGES)

test-coverage:
	go test -v -coverpkg=$(GO_PACKAGES) -coverprofile=coverage.out $(GO_PACKAGES)
	go tool cover -func=coverage.out | tail -1
	go tool cover -html=coverage.out -o coverage.html

test-race:
	go test -v -race $(GO_PACKAGES)

test-fuzz:
	go test -fuzz=FuzzDecodeUpdate -fuzztime=30s ./tg/
	go test -fuzz=FuzzChatID -fuzztime=30s ./tg/

lint:
	golangci-lint run $(GO_PACKAGES)
	go vet $(GO_PACKAGES)

ci: lint test-race test-coverage
	@COVERAGE=$$(go tool cover -func=coverage.out | tail -1 | awk '{print $$3}' | tr -d '%'); \
	echo "Coverage: $$COVERAGE%"; \
	if [ $$(echo "$$COVERAGE < 80" | bc) -eq 1 ]; then \
		echo "FAIL: Coverage $$COVERAGE% is below 80% threshold"; \
		exit 1; \
	fi; \
	echo "PASS: Coverage meets threshold"

bench:
	go test -bench=. -benchmem $(GO_PACKAGES)

vuln:
	govulncheck $(GO_PACKAGES)
```

### GitHub Actions

```yaml
name: Tests

on:
  push:
    branches: [main]
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      
      - name: Lint
        uses: golangci/golangci-lint-action@v4
      
      - name: Test with race detector
        run: go test -v -race ./...
      
      - name: Test with coverage
        run: |
          go test -v -coverpkg=./... -coverprofile=coverage.out ./...
          go tool cover -func=coverage.out | tee coverage.txt
      
      - name: Check coverage threshold
        run: |
          COVERAGE=$(tail -1 coverage.txt | awk '{print $3}' | tr -d '%')
          echo "Total coverage: $COVERAGE%"
          if (( $(echo "$COVERAGE < 80" | bc -l) )); then
            echo "::error::Coverage $COVERAGE% is below 80%"
            exit 1
          fi
      
      - name: Upload coverage
        uses: codecov/codecov-action@v4
        with:
          file: ./coverage.out
```

---

## Summary

### Final PR Structure (9 PRs)

| PR | Focus | Hours | Deliverables |
|----|-------|-------|--------------|
| **PR1** | Test Infrastructure | 6-8 | testutil package |
| **PR2** | Executor + Retry | 6-8 | Deterministic retry tests |
| **PR2.5** | httpclient Tests | 2-3 | Factory tests |
| **PR3** | Rate Limiting | 4-6 | Store + cleanup tests |
| **PR3.5** | Circuit Breaker | 3-4 | State transition tests |
| **PR4** | Sender Methods | 6-8 | All API methods |
| **PR5** | Receiver | 6-8 | Polling + webhook |
| **PR6** | tg + Fuzzing | 5-7 | Types + fuzz |
| **PR7** | Bot + CI | 4-5 | Facade + 80% gate |
| **TOTAL** | | **43-57** | **≥80% coverage** |

### Key Decisions Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Go version | 1.21+ | No `WaitGroup.Go()` dependency |
| WaitGroup pattern | `wg.Add(1)/go func()` | Standard, future-proof |
| Error goroutines | `errgroup.Group` | Context-aware |
| Encapsulation | `chatLimiterStore` + `count()` | Clean API |
| Retry testing | `Sleeper` interface | Deterministic |
| Cleanup testing | `cleanup(now)` method | No 5-min waits |
| `fmt` usage | `strconv.Itoa` | Simpler |

### Timeline

- **Week 1:** PR1, PR2, PR2.5 (Infrastructure, Executor)
- **Week 2:** PR3, PR3.5, PR4 (Rate Limits, Breaker, Methods)
- **Week 3:** PR5, PR6 (Receiver, Types)
- **Week 4:** PR7 (Facade, CI, Coverage fixes)

---

*galigo Testing Implementation Plan v3.1 - Final Consolidated*  
*Combines best practices from multiple independent analyses*

---

## Appendix: Quick Reference

### Test Client Selection

```go
// For testing retry behavior (breaker won't interfere)
client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, opts...)

// For testing circuit breaker behavior (trips quickly)
client := testutil.NewBreakerTestClient(t, server.BaseURL(), opts...)

// For general API testing (default settings)
client := testutil.NewTestClient(t, server.BaseURL(), opts...)
```

### Error Assertion Quick Reference

```go
// Context cancellation
assert.True(t, errors.Is(err, context.Canceled))

// Context timeout
assert.True(t, errors.Is(err, context.DeadlineExceeded))

// Telegram 429 (API rate limit)
assert.True(t, errors.Is(err, sender.ErrRateLimited))

// Circuit breaker open
assert.True(t, errors.Is(err, sender.ErrCircuitOpen))

// Max retries exceeded
assert.True(t, errors.Is(err, sender.ErrMaxRetries))

// Any context error
assert.True(t, sender.IsContextError(err))
```

### Key Decisions Summary

| Issue | Solution | Why |
|-------|----------|-----|
| Go version | **1.25** | Latest features |
| WaitGroup pattern | `syncutil.Go()` helper | Future-proof |
| Body capture | Restore with `NopCloser` | Handlers can re-read |
| Circuit breaker | Configurable settings | Clean API, no "disable" escape hatch |
| Context errors | Return directly | Clear semantics, Go conventions |
| Error goroutines | `errgroup.Group` | Context-aware |
| `fmt` usage | `strconv.Itoa` | Simpler |