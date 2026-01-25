# galigo v1.0.0 - Comprehensive Code Review

**Review Date:** January 2026  
**Go Version Target:** Go 1.25  
**Reviewer:** Claude AI  

---

## Executive Summary

Overall, **galigo v1.0.0** is a well-structured library with good separation of concerns and proper resilience patterns. However, there are several issues ranging from **critical security concerns** to **Go 1.25 feature adoption gaps** that should be addressed before production use.

### Rating by Category

| Category | Rating | Notes |
|----------|--------|-------|
| Architecture | ⭐⭐⭐⭐ | Good separation, clean interfaces |
| Security | ⭐⭐⭐ | Some concerns need addressing |
| Go 1.25 Compliance | ⭐⭐⭐ | Missing new features |
| Code Quality | ⭐⭐⭐⭐ | Clean, but some smells |
| Performance | ⭐⭐⭐⭐ | Good resilience patterns |
| Testing | ⭐⭐ | No tests found in package |

---

## 🔴 CRITICAL ISSUES

### 1. Missing `sync.WaitGroup.Go()` (Go 1.25 Feature)

**Location:** `receiver/polling.go:198-199`

```go
// Current (Go 1.24 style) - ERROR PRONE
c.wg.Add(1)
go c.pollLoop(ctx)
```

**Problem:** Go 1.25 introduced `sync.WaitGroup.Go()` which eliminates Add/Done mismatches - a common source of deadlocks.

**Fix:**
```go
// Go 1.25 style - SAFER
c.wg.Go(func() {
    c.pollLoop(ctx)
})
```

**Impact:** Medium - Current code works but doesn't leverage Go 1.25's safety improvements.

---

### 2. Unbounded Memory Growth in Chat Limiters

**Location:** `sender/client.go:486-505`

```go
func (c *Client) getChatLimiter(chatID int64) *rate.Limiter {
    // ...
    limiter = rate.NewLimiter(rate.Limit(c.config.PerChatRPS), c.config.PerChatBurst)
    c.chatLimiters[chatID] = limiter  // NEVER CLEANED UP!
    return limiter
}
```

**Problem:** Chat limiters are created but **never removed**. In a bot serving millions of users, this causes unbounded memory growth.

**Fix:**
```go
type Client struct {
    // ...
    chatLimiters    *sync.Map  // Use sync.Map for concurrent access
    limiterCleanup  *time.Ticker
}

// Add cleanup goroutine in New()
func (c *Client) startLimiterCleanup() {
    go func() {
        for range c.limiterCleanup.C {
            c.chatLimiters.Range(func(key, value any) bool {
                limiter := value.(*chatLimiterEntry)
                if time.Since(limiter.lastUsed) > 10*time.Minute {
                    c.chatLimiters.Delete(key)
                }
                return true
            })
        }
    }()
}
```

**Impact:** High - Memory leak in production.

---

### 3. Response Body Not Limited Before Parse

**Location:** `receiver/polling.go:340-342`

```go
body, err := io.ReadAll(resp.Body)  // NO SIZE LIMIT!
```

**Problem:** Unlike `sender/client.go:450` which properly limits response size, the polling client reads unlimited data, risking DoS via memory exhaustion.

**Fix:**
```go
const maxPollResponseSize = 50 << 20 // 50MB for updates

limitedReader := io.LimitReader(resp.Body, maxPollResponseSize)
body, err := io.ReadAll(limitedReader)
if err != nil {
    return nil, err
}
if int64(len(body)) == maxPollResponseSize {
    return nil, errors.New("response too large")
}
```

**Impact:** High - DoS vulnerability.

---

### 4. Potential Token Exposure in URL

**Location:** `receiver/polling.go:311-317`

```go
url := fmt.Sprintf("%s%s/getUpdates?timeout=%d&limit=%d&offset=%d",
    telegramAPIBaseURL,
    c.token.Value(),  // Token in URL!
    c.timeout,
    // ...
)
```

**Problem:** Token appears in URLs which may be logged by proxies, CDNs, or debug tools. While Telegram requires this format, the URL should never be logged.

**Recommendation:** Add explicit documentation and ensure URL is never logged:
```go
// Never log this URL - contains bot token
req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
```

---

## 🟠 HIGH PRIORITY ISSUES

### 5. Hardcoded API Base URL

**Location:** `receiver/polling.go:23`

```go
const telegramAPIBaseURL = "https://api.telegram.org/bot"
```

**Problem:** Hardcoded URL prevents testing and custom API endpoints. Config struct has `BaseURL` but polling ignores it.

**Fix:**
```go
func (c *PollingClient) fetchUpdates(ctx context.Context) ([]tg.Update, error) {
    url := fmt.Sprintf("%s%s/getUpdates?...",
        c.baseURL,  // Use configurable URL
        c.token.Value(),
        // ...
    )
}
```

---

### 6. Duplicated HTTP Client Creation

**Location:** `sender/client.go:107-123` and `sender/client.go:174-191`

```go
// In New()
c.httpClient = &http.Client{
    Timeout: c.config.RequestTimeout,
    Transport: &http.Transport{...},
}

// In NewFromConfig() - EXACT SAME CODE
c.httpClient = &http.Client{
    Timeout: c.config.RequestTimeout,
    Transport: &http.Transport{...},
}
```

**Problem:** Code duplication violates DRY principle and increases maintenance burden.

**Fix:**
```go
func createHTTPClient(cfg Config) *http.Client {
    return &http.Client{
        Timeout: cfg.RequestTimeout,
        Transport: &http.Transport{
            DialContext: (&net.Dialer{
                Timeout:   10 * time.Second,
                KeepAlive: cfg.KeepAlive,
            }).DialContext,
            MaxIdleConns:        cfg.MaxIdleConns,
            IdleConnTimeout:     cfg.IdleTimeout,
            TLSHandshakeTimeout: 10 * time.Second,
            ForceAttemptHTTP2:   true,
            TLSClientConfig: &tls.Config{
                MinVersion: tls.VersionTLS12,
            },
        },
    }
}

// Then use:
if c.httpClient == nil {
    c.httpClient = createHTTPClient(c.config)
}
```

---

### 7. Missing TLS 1.3 Preference

**Location:** Multiple files

```go
TLSClientConfig: &tls.Config{
    MinVersion: tls.VersionTLS12,
}
```

**Problem:** While TLS 1.2 is the minimum, we should explicitly prefer TLS 1.3 for better security.

**Fix:**
```go
TLSClientConfig: &tls.Config{
    MinVersion:   tls.VersionTLS12,
    CipherSuites: nil,  // Let Go choose (prefers TLS 1.3 suites)
    CurvePreferences: []tls.CurveID{
        tls.X25519,
        tls.CurveP256,
    },
}
```

---

### 8. Inefficient Error Type Assertion Chain

**Location:** `receiver/webhook.go:159-176`

```go
if err != nil {
    switch {
    case errors.Is(err, ErrForbidden):
        h.fail(w, "forbidden", http.StatusForbidden)
    case errors.Is(err, ErrUnauthorized):
        h.fail(w, "unauthorized", http.StatusUnauthorized)
    // ... more cases
    }
}
```

**Problem:** Multiple `errors.Is()` calls are inefficient. Better to use error types with HTTP status.

**Fix:**
```go
type HTTPError struct {
    Status  int
    Message string
    Err     error
}

func (e *HTTPError) Error() string { return e.Message }
func (e *HTTPError) Unwrap() error { return e.Err }

// Then:
if err != nil {
    var httpErr *HTTPError
    if errors.As(err, &httpErr) {
        h.fail(w, httpErr.Message, httpErr.Status)
        return
    }
    h.fail(w, err.Error(), http.StatusInternalServerError)
}
```

---

### 9. `ChatID` Type Definition is Too Loose

**Location:** `tg/types.go:7-8`

```go
// ChatID represents a Telegram chat identifier.
type ChatID = any  // TOO PERMISSIVE!
```

**Problem:** Using `any` allows invalid types at runtime. Should be more restrictive.

**Fix:**
```go
// ChatID represents a Telegram chat identifier.
// Valid values: int64 (numeric ID) or string (channel username starting with @)
type ChatID interface {
    chatID()  // Marker method - only int64 and string implement it
}

// Wrapper types
type NumericChatID int64
func (NumericChatID) chatID() {}

type UsernameChatID string
func (UsernameChatID) chatID() {}

// Helper constructors
func ChatIDFromInt(id int64) ChatID { return NumericChatID(id) }
func ChatIDFromUsername(username string) ChatID { return UsernameChatID(username) }
```

---

### 10. No Context Timeout in HTTP Requests

**Location:** `sender/client.go:428-444`

```go
func (c *Client) doRequest(ctx context.Context, method string, payload any) (*apiResponse, error) {
    // ...
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
    // Uses passed context but no explicit timeout
}
```

**Problem:** If caller passes `context.Background()`, request may hang indefinitely despite client timeout.

**Fix:**
```go
func (c *Client) doRequest(ctx context.Context, method string, payload any) (*apiResponse, error) {
    // Ensure request has timeout
    if _, ok := ctx.Deadline(); !ok {
        var cancel context.CancelFunc
        ctx, cancel = context.WithTimeout(ctx, c.config.RequestTimeout)
        defer cancel()
    }
    // ...
}
```

---

## 🟡 MEDIUM PRIORITY ISSUES

### 11. Missing `encoding/json/v2` Consideration

**Location:** Throughout

Go 1.25 introduced experimental `encoding/json/v2` with 2-3x faster decoding. While experimental, the codebase should be structured to allow future migration.

**Recommendation:** Create abstraction layer:
```go
// internal/json/json.go
package json

import (
    "encoding/json"
    // Future: "encoding/json/v2"
)

func Marshal(v any) ([]byte, error) { return json.Marshal(v) }
func Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
```

---

### 12. Regex Compiled at Package Init

**Location:** `internal/validate/validate.go:166`

```go
var usernameRegex = regexp.MustCompile(`^@?[a-zA-Z][a-zA-Z0-9_]{4,31}$`)
```

**Problem:** Regex is compiled at package init, which is fine, but the pattern could be more efficient.

**Recommendation:** Use `regexp.MustCompile` with a possessive quantifier note:
```go
// Note: Go regex doesn't support possessive quantifiers.
// Consider manual validation for performance-critical code.
var usernameRegex = regexp.MustCompile(`^@?[a-zA-Z][a-zA-Z0-9_]{4,31}$`)
```

---

### 13. `Close()` Method Does Nothing

**Location:** `sender/client.go:215-217`

```go
func (c *Client) Close() error {
    return nil  // DOES NOTHING!
}
```

**Problem:** Users expect `Close()` to release resources. Should close HTTP client if owned.

**Fix:**
```go
func (c *Client) Close() error {
    // Close idle connections
    if t, ok := c.httpClient.Transport.(*http.Transport); ok {
        t.CloseIdleConnections()
    }
    // Stop limiter cleanup if running
    if c.limiterCleanup != nil {
        c.limiterCleanup.Stop()
    }
    return nil
}
```

---

### 14. Config Loading Ignores Errors Silently

**Location:** `sender/config.go:88-134` and `receiver/config.go:115-214`

```go
if d, err := time.ParseDuration(getEnv("REQUEST_TIMEOUT", "30s")); err == nil {
    cfg.RequestTimeout = d
}
// If parsing fails, silently uses default - no warning!
```

**Problem:** Invalid environment variables are silently ignored. User may think config is applied when it's not.

**Fix:**
```go
func LoadConfig() (*Config, error) {
    cfg := DefaultConfig()
    var warnings []string
    
    if d, err := time.ParseDuration(getEnv("REQUEST_TIMEOUT", "30s")); err == nil {
        cfg.RequestTimeout = d
    } else if val := getEnv("REQUEST_TIMEOUT", ""); val != "" {
        warnings = append(warnings, fmt.Sprintf("invalid REQUEST_TIMEOUT %q, using default", val))
    }
    
    // Log warnings
    for _, w := range warnings {
        slog.Warn("config loading", "warning", w)
    }
    
    return &cfg, nil
}
```

---

### 15. No Validation for Keyboard Builder

**Location:** `tg/keyboard.go`

**Problem:** Keyboard builder allows invalid keyboards:
- Too many rows (Telegram limit: unspecified but practical ~100)
- Too many buttons per row (Telegram limit: 8)
- Empty callback data
- Callback data > 64 bytes

**Fix:**
```go
const (
    MaxButtonsPerRow = 8
    MaxCallbackData  = 64
)

func (k *Keyboard) Row(buttons ...InlineKeyboardButton) *Keyboard {
    if len(buttons) > MaxButtonsPerRow {
        buttons = buttons[:MaxButtonsPerRow]  // Or return error
    }
    if len(buttons) > 0 {
        k.rows = append(k.rows, buttons)
    }
    return k
}

func Btn(text, callbackData string) InlineKeyboardButton {
    if len(callbackData) > MaxCallbackData {
        callbackData = callbackData[:MaxCallbackData]  // Or panic
    }
    return InlineKeyboardButton{Text: text, CallbackData: callbackData}
}
```

---

### 16. Missing `options.go` File Reference in Client

**Location:** `sender/client.go`

The file imports options like `WithLogger`, `WithHTTPClient`, etc., but there's also a separate `sender/options.go` file. This can cause confusion.

**Recommendation:** Either:
1. Keep all options in `options.go` and import them in `client.go`
2. Keep all options inline in `client.go` and remove `options.go`

Don't split them across files without clear reasoning.

---

## 🟢 LOW PRIORITY / STYLE ISSUES

### 17. Inconsistent Error Naming

**Location:** Multiple files

```go
// tg/errors.go
ErrMessageNotFound = errors.New("galigo: message not found")

// sender/errors.go
ErrInvalidToken = errors.New("sender: invalid token")  // Different prefix!
```

**Fix:** Use consistent prefix across all packages:
```go
// All errors should use "galigo:" prefix
ErrInvalidToken = errors.New("galigo: invalid token")
```

---

### 18. Dead Code in `extractChatID`

**Location:** `sender/client.go:580-589`

```go
func extractChatID(chatID tg.ChatID) int64 {
    switch v := chatID.(type) {
    case int64:
        return v
    case int:
        return int64(v)
    default:
        return 0  // String usernames return 0 - potentially wrong!
    }
}
```

**Problem:** String chat IDs (like "@username") return 0, which might cause issues with per-chat rate limiting.

**Fix:**
```go
func extractChatID(chatID tg.ChatID) int64 {
    switch v := chatID.(type) {
    case int64:
        return v
    case int:
        return int64(v)
    case string:
        // For usernames, use a hash as the limiter key
        h := fnv.New64a()
        h.Write([]byte(v))
        return int64(h.Sum64())
    default:
        return 0
    }
}
```

---

### 19. Missing GoDoc Comments

**Location:** Multiple files

Several exported functions lack proper GoDoc comments:

```go
// Missing GoDoc
func DefaultBreakerConfig(name string) BreakerConfig {

// Should be:
// DefaultBreakerConfig returns a BreakerConfig with sensible defaults
// for production use. The name parameter identifies the breaker in logs.
func DefaultBreakerConfig(name string) BreakerConfig {
```

---

### 20. Inconsistent Buffer Pool Usage

**Location:** `receiver/webhook.go:54-61` and `receiver/webhook.go:76-81`

```go
// In option
WithWebhookMaxBodySize(size int64) WebhookOption {
    return func(h *WebhookHandler) {
        h.maxBodySize = size
        h.bufferPool = sync.Pool{...}  // Creates new pool
    }
}

// In constructor
h.bufferPool = sync.Pool{...}  // Also creates pool
```

**Problem:** Option recreates pool after constructor already created one.

**Fix:** Apply options before creating pool, or lazy-init the pool.

---

## 📋 RECOMMENDATIONS SUMMARY

### Must Fix Before Production

1. ✅ Add response size limit to polling client
2. ✅ Implement chat limiter cleanup
3. ✅ Use `sync.WaitGroup.Go()` (Go 1.25)
4. ✅ Fix hardcoded API URL in polling

### Should Fix Soon

5. ⬜ Refactor duplicate HTTP client creation
6. ⬜ Add TLS 1.3 preference
7. ⬜ Add context timeout safety
8. ⬜ Fix ChatID type to be more restrictive
9. ⬜ Make Close() actually close resources

### Nice to Have

10. ⬜ Add keyboard validation
11. ⬜ Add config loading warnings
12. ⬜ Consistent error prefixes
13. ⬜ Better GoDoc coverage

---

## 🔬 MISSING GO 1.25 FEATURES

| Feature | Status | Recommendation |
|---------|--------|----------------|
| `sync.WaitGroup.Go()` | ❌ Not used | Use in polling.go |
| Container GOMAXPROCS | ✅ Automatic | No action needed |
| `testing/synctest` | ❌ No tests | Add time-based tests |
| `encoding/json/v2` | ⚠️ Experimental | Wait for stable |
| `os.Root` | ❌ Not used | Could use for photo path validation |

---

## 📊 CODE METRICS

| Metric | Value | Assessment |
|--------|-------|------------|
| Total Go files | 26 | Reasonable |
| Lines of code | ~2,500 | Moderate |
| External dependencies | 2 | Minimal (good!) |
| Test coverage | 0% | ❌ Critical gap |
| Cyclomatic complexity | Low | Good |

---

## 🧪 TESTING GAPS

**Critical:** No tests were found in the package. For a production library, this is unacceptable.

### Recommended Test Files to Add

```
galigo/
├── bot_test.go
├── tg/
│   ├── types_test.go
│   ├── errors_test.go
│   ├── keyboard_test.go
│   └── secret_test.go
├── sender/
│   ├── client_test.go
│   └── requests_test.go
├── receiver/
│   ├── polling_test.go
│   └── webhook_test.go
└── internal/
    ├── resilience/
    │   ├── retry_test.go
    │   └── breaker_test.go
    └── validate/
        └── validate_test.go
```

### Test Types Needed

1. **Unit tests** - For all pure functions
2. **Integration tests** - With mock Telegram API
3. **Fuzz tests** - For `validate.Token()`, JSON parsing
4. **Benchmark tests** - For hot paths (rate limiting, JSON)
5. **synctest tests** - For backoff timing (Go 1.25)

---

## 📝 FINAL VERDICT

**galigo v1.0.0** is a solid foundation with good architecture, but needs work before production use:

1. **Security:** Fix response size limits, memory leaks
2. **Go 1.25:** Adopt new features like `WaitGroup.Go()`
3. **Testing:** Add comprehensive test suite
4. **Documentation:** Improve GoDoc coverage

The library follows good Go idioms overall and the functional options pattern is well implemented. With the fixes above, this could be an excellent Telegram bot library.

---

*Review completed January 2026*