# galigo v1.0.0 - Enhanced Consolidated Code Review

**Review Date:** January 2026  
**Go Version Target:** Go 1.25  
**Telegram Bot API Version:** 9.3 (Dec 31, 2025)  
**Consolidated from:** Multiple independent reviews

---

## Executive Summary

This enhanced review consolidates findings from multiple independent analyses. **Several critical bugs were identified that can cause data loss, security vulnerabilities, and incorrect behavior.**

### Severity Distribution

| Priority | Count | Description |
|----------|-------|-------------|
| **P0 - Critical** | 10 | Data loss, security, correctness failures |
| **P1 - High** | 8 | Reliability, performance, operability |
| **P2 - Medium** | 10 | Maintainability, API design |
| **P3 - Low** | 6 | Style, documentation |

---

## 🔴 P0 — CRITICAL ISSUES (Fix Before Any Use)

### P0.1 ⚠️ UPDATE LOSS BUG - Polling Drops Updates When Channel Full

**Location:** `receiver/polling.go:288-299`

```go
for _, update := range updates {
    if update.UpdateID >= c.offset {
        c.offset = update.UpdateID + 1  // ← OFFSET ADVANCED FIRST
    }

    select {
    case c.updates <- update:
        c.logger.Debug("update sent", "update_id", update.UpdateID)
    default:
        c.logger.Warn("updates channel full", "update_id", update.UpdateID)
        // ← UPDATE DROPPED BUT OFFSET ALREADY ADVANCED = PERMANENT LOSS
    }
}
```

**Problem:** The offset is incremented **before** attempting to send to channel. If the channel is full, the update is dropped but offset is already advanced. Telegram considers updates with ID < offset as "confirmed" and **will never send them again**. This causes **permanent data loss**.

**Impact:** CRITICAL - Messages from users can be silently lost forever.

**Fix:**
```go
for _, update := range updates {
    // Block until update is delivered (with context timeout)
    select {
    case c.updates <- update:
        // Only advance offset AFTER successful delivery
        if update.UpdateID >= c.offset {
            c.offset = update.UpdateID + 1
        }
        c.logger.Debug("update sent", "update_id", update.UpdateID)
    case <-ctx.Done():
        return // Don't advance offset - updates will be redelivered
    }
}
```

**Or** provide explicit opt-in for lossy mode:
```go
type PollingOption func(*PollingClient)

// WithDropOnBackpressure allows dropping updates when channel is full (NOT recommended)
func WithDropOnBackpressure(drop bool) PollingOption {
    return func(c *PollingClient) {
        c.dropOnBackpressure = drop
    }
}
```

---

### P0.2 ⚠️ URL INJECTION - `allowed_updates` Not URL-Encoded

**Location:** `receiver/polling.go:319-324`

```go
if len(c.allowedUpdates) > 0 {
    encoded, err := json.Marshal(c.allowedUpdates)
    if err == nil {
        url += "&allowed_updates=" + string(encoded)  // RAW JSON IN URL!
    }
}
```

**Problem:** JSON contains characters that must be URL-encoded (`[`, `]`, `"`, spaces). Raw JSON in URL is malformed and may cause unpredictable behavior.

**Example:** `["message","callback_query"]` should be `%5B%22message%22%2C%22callback_query%22%5D`

**Fix:**
```go
import "net/url"

func (c *PollingClient) fetchUpdates(ctx context.Context) ([]tg.Update, error) {
    params := url.Values{}
    params.Set("timeout", strconv.Itoa(c.timeout))
    params.Set("limit", strconv.Itoa(c.limit))
    params.Set("offset", strconv.Itoa(c.offset))
    
    if len(c.allowedUpdates) > 0 {
        encoded, err := json.Marshal(c.allowedUpdates)
        if err == nil {
            params.Set("allowed_updates", string(encoded))  // url.Values handles encoding
        }
    }
    
    apiURL := fmt.Sprintf("%s%s/getUpdates?%s",
        c.baseURL,
        c.token.Value(),
        params.Encode(),  // Proper encoding
    )
    // ...
}
```

---

### P0.3 ⚠️ WRONG RETRY-AFTER PARSING - Uses HTTP Header Instead of JSON

**Location:** `sender/client.go:466-471`

```go
retryAfter := resp.Header.Get("Retry-After")  // WRONG!
if apiResp.ErrorCode == 429 && retryAfter != "" {
    // ...
}
```

**Problem:** Telegram returns `retry_after` in the **JSON response body** under `parameters`, NOT in HTTP headers. Current code will never find it.

**Telegram API Response Format:**
```json
{
    "ok": false,
    "error_code": 429,
    "description": "Too Many Requests: retry after 35",
    "parameters": {
        "retry_after": 35
    }
}
```

**Fix:**
```go
// Add to apiResponse struct
type apiResponse struct {
    OK          bool               `json:"ok"`
    Result      json.RawMessage    `json:"result,omitempty"`
    ErrorCode   int                `json:"error_code,omitempty"`
    Description string             `json:"description,omitempty"`
    Parameters  *ResponseParameters `json:"parameters,omitempty"`  // ADD THIS
}

type ResponseParameters struct {
    RetryAfter      int   `json:"retry_after,omitempty"`
    MigrateToChatID int64 `json:"migrate_to_chat_id,omitempty"`
}

// Then in doRequest:
if !apiResp.OK {
    var retryAfter time.Duration
    if apiResp.Parameters != nil && apiResp.Parameters.RetryAfter > 0 {
        retryAfter = time.Duration(apiResp.Parameters.RetryAfter) * time.Second
    }
    if retryAfter > 0 {
        return nil, NewAPIErrorWithRetry(method, apiResp.ErrorCode, apiResp.Description, retryAfter)
    }
    return nil, NewAPIError(method, apiResp.ErrorCode, apiResp.Description)
}
```

---

### P0.4 ⚠️ NO TIMEOUT - `http.DefaultClient` Used in API Helpers

**Location:** `receiver/api.go:36-38, 85-87, 123-125`

```go
func SetWebhook(ctx context.Context, client *http.Client, token tg.SecretToken, url, secret string) error {
    if client == nil {
        client = http.DefaultClient  // NO TIMEOUT! Can hang forever
    }
    // ...
}
```

**Problem:** `http.DefaultClient` has **no timeout**. A network issue can hang the goroutine indefinitely, leaking resources.

**Fix:**
```go
var defaultAPIClient = &http.Client{
    Timeout: 30 * time.Second,
    Transport: &http.Transport{
        TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
        // ... other settings
    },
}

func SetWebhook(ctx context.Context, client *http.Client, token tg.SecretToken, url, secret string) error {
    if client == nil {
        client = defaultAPIClient  // Safe default
    }
    // ...
}
```

---

### P0.5 ⚠️ WEBHOOK RETURNS 500 FOR OVERSIZED BODY (Should Be 413)

**Location:** `receiver/webhook.go:134-145`

```go
r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)
n, err := io.ReadFull(r.Body, buffer)
if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
    return nil, &WebhookError{Code: 500, Message: "failed to read body", Err: err}  // WRONG CODE
}
```

**Problem:** When body exceeds `maxBodySize`, the error becomes a 500 Internal Server Error. HTTP semantics require **413 Payload Too Large**.

**Fix:**
```go
r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)
body, err := io.ReadAll(r.Body)
if err != nil {
    var maxBytesErr *http.MaxBytesError
    if errors.As(err, &maxBytesErr) {
        return nil, &WebhookError{Code: 413, Message: "payload too large", Err: err}
    }
    return nil, &WebhookError{Code: 500, Message: "failed to read body", Err: err}
}
```

---

### P0.6 ⚠️ CIRCUIT BREAKER PLACEMENT ENABLES DOS

**Location:** `receiver/webhook.go:109-156`

```go
_, err := h.breaker.Execute(func() (interface{}, error) {
    // Domain validation
    if h.allowedDomain != "" && r.Host != h.allowedDomain {
        return nil, ErrForbidden  // INSIDE BREAKER
    }

    // Secret validation
    if h.webhookSecret != "" {
        secret := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
        if subtle.ConstantTimeCompare(...) != 1 {
            return nil, ErrUnauthorized  // INSIDE BREAKER
        }
    }
    // ...
})
```

**Problem:** Authentication failures are counted as circuit breaker failures. An attacker can send many requests with bad secrets to **trip the breaker and block legitimate Telegram requests**.

**Fix:** Move authentication **outside** the breaker:
```go
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // Rate limit (outside breaker)
    if !h.limiter.Allow() {
        h.fail(w, "rate limit exceeded", http.StatusTooManyRequests)
        return
    }

    // Authentication (outside breaker)
    if h.allowedDomain != "" && r.Host != h.allowedDomain {
        h.fail(w, "forbidden", http.StatusForbidden)
        return
    }
    if h.webhookSecret != "" {
        secret := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
        if subtle.ConstantTimeCompare([]byte(secret), []byte(h.webhookSecret)) != 1 {
            h.fail(w, "unauthorized", http.StatusUnauthorized)
            return
        }
    }
    if r.Method != http.MethodPost {
        h.fail(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }

    // Only downstream processing inside breaker
    _, err := h.breaker.Execute(func() (interface{}, error) {
        return h.processUpdate(r)
    })
    // ...
}
```

---

### P0.7 ⚠️ TOKEN VALIDATION NOT USED

**Location:** `bot.go:124-127`

```go
func New(token string, opts ...Option) (*Bot, error) {
    if token == "" {
        return nil, tg.ErrInvalidToken  // Only checks empty!
    }
    // ...
}
```

**Problem:** You have `internal/validate.Token()` that validates `{bot_id}:{secret}` format, but it's never called. Invalid tokens fail later with confusing errors.

**Fix:**
```go
import "github.com/prilive-com/galigo/internal/validate"

func New(token string, opts ...Option) (*Bot, error) {
    if err := validate.Token(token); err != nil {
        return nil, fmt.Errorf("%w: %v", tg.ErrInvalidToken, err)
    }
    // ...
}
```

---

### P0.8 ⚠️ RESPONSE SIZE CHECK FALSE POSITIVE

**Location:** `sender/client.go:456-458`

```go
if len(body) == maxResponseSize {
    return nil, ErrResponseTooLarge  // FALSE POSITIVE if exactly maxResponseSize
}
```

**Problem:** If Telegram's response is **exactly** 10MB (maxResponseSize), it's treated as "too large" even though it's valid.

**Fix:**
```go
const maxResponseSize = 10 << 20 // 10MB

// Read maxResponseSize + 1 to detect overflow
limitedReader := io.LimitReader(resp.Body, maxResponseSize+1)
body, err := io.ReadAll(limitedReader)
if err != nil {
    return nil, fmt.Errorf("failed to read response: %w", err)
}

if int64(len(body)) > maxResponseSize {
    return nil, ErrResponseTooLarge
}
```

---

### P0.9 ⚠️ MISSING RESPONSE SIZE LIMIT IN POLLING

**Location:** `receiver/polling.go:340-342`

```go
body, err := io.ReadAll(resp.Body)  // NO SIZE LIMIT!
```

**Problem:** Unlike sender which limits to 10MB, polling reads unlimited data. A malicious response could exhaust memory.

**Fix:**
```go
const maxPollResponseSize = 50 << 20 // 50MB for updates

limitedReader := io.LimitReader(resp.Body, maxPollResponseSize+1)
body, err := io.ReadAll(limitedReader)
if err != nil {
    return nil, err
}
if int64(len(body)) > maxPollResponseSize {
    return nil, errors.New("response too large")
}
```

---

### P0.10 ⚠️ FILE PATH UPLOAD CLAIMS WITHOUT MULTIPART

**Location:** `sender/requests.go:22`

```go
Photo string `json:"photo"` // URL, file_id, or file path  ← CLAIMS FILE PATH
```

**Problem:** Documentation claims file path support, but `doRequest` only sends JSON. Local file upload requires **multipart/form-data**.

**Fix (Option A - Remove claim):**
```go
Photo string `json:"photo"` // URL or file_id only
```

**Fix (Option B - Implement multipart):**
```go
func (c *Client) SendPhotoFile(ctx context.Context, chatID tg.ChatID, filePath string, opts ...PhotoOption) (*tg.Message, error) {
    // Validate path (no traversal)
    // Open file
    // Create multipart form
    // Send with Content-Type: multipart/form-data
}
```

---

## 🟠 P1 — HIGH PRIORITY (Fix Soon)

### P1.1 Data Race on Polling Offset

**Location:** `receiver/polling.go`

```go
type PollingClient struct {
    offset int  // Written by pollLoop, read by Offset()
}

func (c *PollingClient) Offset() int {
    return c.offset  // RACE!
}
```

**Fix:** Use `atomic.Int64` or mutex.

---

### P1.2 Unbounded Memory Growth in Chat Limiters

**Location:** `sender/client.go:486-505`

Chat limiters are created but never cleaned up. Long-running bots will leak memory.

**Fix:** Add periodic cleanup or use LRU cache.

---

### P1.3 Polling Cannot Restart After Stop

**Location:** `receiver/polling.go:216-218`

```go
c.closeOnce.Do(func() {
    close(c.stopCh)  // Closed forever
})
```

**Fix:** Either recreate channels on Start, or document single-use.

---

### P1.4 Missing `sync.WaitGroup.Go()` (Go 1.25)

**Location:** `receiver/polling.go:198-199`

```go
c.wg.Add(1)
go c.pollLoop(ctx)
```

**Fix:** Use Go 1.25's safer pattern:
```go
c.wg.Go(func() {
    c.pollLoop(ctx)
})
```

---

### P1.5 Duplicate HTTP Client Creation

**Location:** `sender/client.go`, `receiver/polling.go`, `receiver/api.go`

Same transport configuration duplicated in 4+ places.

**Fix:** Use `internal/httpclient` everywhere.

---

### P1.6 `Close()` Does Nothing

**Location:** `sender/client.go:215-217`

```go
func (c *Client) Close() error {
    return nil
}
```

**Fix:** Close idle connections and cleanup resources.

---

### P1.7 Outdated Dependencies

**Location:** `go.mod`

```
golang.org/x/time v0.5.0  ← Current: v0.14.0+
github.com/sony/gobreaker/v2 v2.0.0  ← Current: v2.4.0
```

**Fix:** `go get -u ./...`

---

### P1.8 INFO-Level Logging for Every Update

**Location:** `receiver/webhook.go:150`

```go
h.logger.Info("update forwarded", "update_id", update.UpdateID)
```

**Fix:** Change to DEBUG for per-update logs.

---

## 🟡 P2 — MEDIUM PRIORITY (Should Fix)

| Issue | Location | Description |
|-------|----------|-------------|
| P2.1 | `receiver/config.go` | Many unused config fields (WebhookURL, TLS paths, etc.) |
| P2.2 | `tg/errors.go`, `sender/errors.go` | Duplicate error types - pick one |
| P2.3 | `tg/types.go:7` | `ChatID = any` is too permissive |
| P2.4 | sender methods | `internal/validate` exists but unused |
| P2.5 | `tg/keyboard.go` | No callback_data length validation (64 byte limit) |
| P2.6 | `internal/*` | Most internal packages unused - dead code |
| P2.7 | `bot.go:65` | `WithWebhook` doesn't set URL, only flips mode |
| P2.8 | `sender/client.go:580` | `extractChatID` returns 0 for string usernames |
| P2.9 | Throughout | TLS 1.3 not explicitly preferred |
| P2.10 | Throughout | Missing GoDoc on many exports |

---

## 🟢 P3 — LOW PRIORITY (Nice to Have)

| Issue | Location | Description |
|-------|----------|-------------|
| P3.1 | `sender/options.go` | `Silent()` ambiguous - rename to `WithSilent()` |
| P3.2 | Throughout | Inconsistent error prefixes (galigo: vs sender:) |
| P3.3 | `bot.go` | `Option` should be `BotOption` to avoid collision |
| P3.4 | Tests | No test files found - 0% coverage |
| P3.5 | `go.mod` | Add `toolchain` directive for reproducible builds |
| P3.6 | README | Claims don't match implementation |

---

## 📊 Architecture Issues Summary

### "Two Implementations" Problem

The codebase has **two sets of infrastructure**:

| Feature | `internal/*` | `sender/receiver` |
|---------|--------------|-------------------|
| HTTP client | `internal/httpclient` | Inline in each file |
| Retry logic | `internal/resilience/retry.go` | Inline in `sender/client.go` |
| Circuit breaker | `internal/resilience/breaker.go` | Inline configuration |
| Validation | `internal/validate/validate.go` | **NOT USED** |

**Decision Needed:**
- **Option A:** Delete `internal/*` and keep inline implementations
- **Option B:** Refactor sender/receiver to use `internal/*` (recommended)

---

## 🔧 Recommended Fix Order

### Week 1 - Critical Bugs
1. ✅ P0.1 - Update loss bug (HIGHEST PRIORITY)
2. ✅ P0.2 - URL encoding
3. ✅ P0.3 - retry_after parsing
4. ✅ P0.4 - http.DefaultClient
5. ✅ P0.6 - Circuit breaker DOS

### Week 2 - Correctness
6. ✅ P0.5 - Webhook 413
7. ✅ P0.7 - Token validation
8. ✅ P0.8 - Response size boundary
9. ✅ P0.9 - Polling size limit
10. ✅ P1.1 - Data race

### Week 3 - Reliability
11. ✅ P1.2 - Chat limiter cleanup
12. ✅ P1.3 - Polling restart
13. ✅ P1.4 - WaitGroup.Go()
14. ✅ P1.5 - HTTP client dedup
15. ✅ P1.7 - Dependencies

### Week 4 - Polish
16. ✅ P2.* issues
17. ✅ Add tests
18. ✅ Update documentation

---

## ✅ What's Good

Despite the issues, several things are well done:

| Feature | Assessment |
|---------|------------|
| `tg.SecretToken` | Excellent - proper redaction in logs/JSON |
| Functional options pattern | Well implemented |
| Rate limiting design | Good per-chat + global approach |
| Keyboard builder | Clean API, Go 1.23+ iterators |
| Separation of concerns | Good package structure |
| Constant-time secret comparison | Correct use of `subtle.ConstantTimeCompare` |

---

## 📚 References Used

- [Telegram Bot API 9.3](https://core.telegram.org/bots/api)
- [Telegram Bots FAQ - Rate Limits](https://core.telegram.org/bots/faq)
- [Go 1.25 Release Notes](https://go.dev/doc/go1.25)
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- [golang.org/x/time versions](https://pkg.go.dev/golang.org/x/time)
- [sony/gobreaker releases](https://pkg.go.dev/github.com/sony/gobreaker/v2)

---

*Enhanced review completed January 2026*
*Consolidated from multiple independent analyses*