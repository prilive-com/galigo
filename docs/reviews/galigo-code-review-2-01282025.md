# galigo - Combined Code Review

**Review Date:** January 28, 2026  
**Go Version Target:** 1.25  
**Commit:** 3df6223 (2026-01-27)  
**Sources:** Two independent static analyses combined

---

## Executive Summary

galigo is a well-architected Go library for the Telegram Bot API with clear separation of concerns, sensible resilience defaults, and good test coverage. However, this combined review identifies **3 critical security/reliability issues** that require immediate fixes before production deployment.

### Overall Assessment: 7.0/10

| Category | Score | Notes |
|----------|-------|-------|
| Architecture | 9/10 | Clean separation: sender/receiver/tg |
| Security | 6/10 | Token leakage risk, needs hardening |
| Reliability | 6/10 | Circuit breaker self-DOS risk |
| Test Coverage | 8/10 | 98.6% tg, 90.6% receiver, 79.8% sender |
| Go Best Practices | 8/10 | Modern patterns, uses Go 1.25 features |

---

## Key Strengths ✅

1. **Clear Architecture** - `sender` (outbound), `receiver` (inbound), `tg` (types)
2. **Sensible Resilience Defaults** - Timeouts, retries + backoff + jitter, rate limiting, circuit breakers
3. **Secure Secret Handling** - Webhook validates secret using `crypto/subtle.ConstantTimeCompare`
4. **Body-Size Limiting** - `http.MaxBytesReader` + proper `*http.MaxBytesError` handling
5. **Good Error Design** - Sentinel errors with `errors.Is()`/`errors.As()` support
6. **Modern Go** - Uses `sync.WaitGroup.Go()` (Go 1.25), iterators in keyboard builder
7. **TLS 1.2+** - Enforced in HTTP transport

---

## CRITICAL ISSUES (P0) - Must Fix Before Production

### 1. 🔴 Bot Token Leakage via Error Messages

**Severity:** CRITICAL (Credential Compromise Risk)  
**Locations:** `sender/client.go:596,640`, `receiver/polling.go:513-519`

**Problem:**
```go
// sender/client.go:596
url := fmt.Sprintf("%s/bot%s/%s", c.config.BaseURL, c.config.Token.Value(), method)

// sender/client.go:640
resp, err := c.httpClient.Do(req)
if err != nil {
    return nil, fmt.Errorf("request failed: %w", err)  // err may contain URL with token!
}
```

Standard `http.Client` errors include the request URL:
```
Get "https://api.telegram.org/bot123456:ABC-DEF/sendMessage": dial tcp: no such host
```

If the caller logs errors (common in bots), **the token ends up in logs**.

**Impact:**
- Tokens in CloudWatch, Datadog, Splunk, etc.
- Credential compromise
- Bot takeover risk

**Fix - Add URL Sanitization:**
```go
// Add at package level
import "regexp"

var tokenRedactor = regexp.MustCompile(`bot\d+:[A-Za-z0-9_-]+`)

func sanitizeError(err error) error {
    if err == nil {
        return nil
    }
    return &sanitizedError{cause: err}
}

type sanitizedError struct{ cause error }

func (e *sanitizedError) Error() string {
    return tokenRedactor.ReplaceAllString(e.cause.Error(), "bot<REDACTED>")
}

func (e *sanitizedError) Unwrap() error { return e.cause }

// Usage in doRequest:
resp, err := c.httpClient.Do(req)
if err != nil {
    return nil, fmt.Errorf("request failed: %w", sanitizeError(err))
}
```

**Testing Required:**
```go
func TestNoTokenInErrors(t *testing.T) {
    token := "123456:ABC-DEF-secret"
    // Trigger various error conditions
    // assert.NotContains(t, err.Error(), token)
    // assert.NotContains(t, err.Error(), "ABC-DEF")
}
```

---

### 2. 🔴 Multipart Uploads Fully Buffered in Memory

**Severity:** CRITICAL (OOM Risk Under Load)  
**Location:** `sender/client.go:606-621`, `sender/multipart.go:73`

**Problem:**
```go
// sender/client.go:606-621
if multipartReq.HasUploads() {
    var body bytes.Buffer  // ← ENTIRE FILE BUFFERED HERE
    encoder := NewMultipartEncoder(&body)
    if err := encoder.Encode(multipartReq); err != nil {
        return nil, fmt.Errorf("failed to encode multipart request: %w", err)
    }
    // ...
}
```

Despite comment "Stream directly - no buffering" in `multipart.go:73`, the entire file ends up in `bytes.Buffer`.

**Impact:**
- 50MB video = 50MB+ RAM per concurrent request
- Under load: 10 concurrent uploads = 500MB+ spikes
- Potential OOM kills

**Fix - Use io.Pipe for True Streaming:**
```go
func (c *Client) doRequestWithFiles(ctx context.Context, method string, multipartReq MultipartRequest) (*apiResponse, error) {
    pr, pw := io.Pipe()
    mpWriter := multipart.NewWriter(pw)
    
    errCh := make(chan error, 1)
    go func() {
        defer pw.Close()
        
        // Write files
        for _, file := range multipartReq.Files {
            part, err := mpWriter.CreateFormFile(file.FieldName, file.FileName)
            if err != nil {
                pw.CloseWithError(err)
                errCh <- err
                return
            }
            if _, err := io.Copy(part, file.Reader); err != nil {
                pw.CloseWithError(err)
                errCh <- err
                return
            }
        }
        
        // Write params
        for name, value := range multipartReq.Params {
            if err := mpWriter.WriteField(name, value); err != nil {
                pw.CloseWithError(err)
                errCh <- err
                return
            }
        }
        
        errCh <- mpWriter.Close()
    }()
    
    url := c.buildURL(method)  // Use helper that doesn't leak token
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
    if err != nil {
        return nil, err
    }
    req.Header.Set("Content-Type", mpWriter.FormDataContentType())
    
    // ... rest of request handling
}
```

---

### 3. 🔴 Circuit Breaker Counts ALL Errors as Failures (Self-DOS Risk)

**Severity:** CRITICAL (Reliability Risk)  
**Locations:** `sender/client.go:244-257`, `receiver/polling.go:178-197`, `receiver/webhook.go:86`

**Problem:**
```go
c.breaker = gobreaker.NewCircuitBreaker[*apiResponse](gobreaker.Settings{
    Name:        "galigo-sender",
    MaxRequests: c.breakerSettings.MaxRequests,
    Interval:    c.breakerSettings.Interval,
    Timeout:     c.breakerSettings.Timeout,
    ReadyToTrip: c.breakerSettings.ReadyToTrip,
    // MISSING: IsSuccessful callback!
})
```

By default, `gobreaker` treats ANY `err != nil` as a failure.

**Impact:**
- Repeated 400 Bad Request (user error) → breaker opens
- Repeated 403 Forbidden (permissions) → breaker opens
- Once open, **ALL requests fail** for 30 seconds
- Self-inflicted denial of service

**Fix - Add IsSuccessful Callback:**
```go
c.breaker = gobreaker.NewCircuitBreaker[*apiResponse](gobreaker.Settings{
    Name:        "galigo-sender",
    MaxRequests: c.breakerSettings.MaxRequests,
    Interval:    c.breakerSettings.Interval,
    Timeout:     c.breakerSettings.Timeout,
    ReadyToTrip: c.breakerSettings.ReadyToTrip,
    IsSuccessful: func(err error) bool {
        if err == nil {
            return true
        }
        
        // 4xx errors (except 429) are client mistakes, not service failures
        var apiErr *tg.APIError
        if errors.As(err, &apiErr) {
            if apiErr.Code >= 400 && apiErr.Code < 500 && apiErr.Code != 429 {
                return true  // Don't count as breaker failure
            }
            return false  // 429 and 5xx are service issues
        }
        
        // Context cancellation is not a service failure
        if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
            return true
        }
        
        // Network errors count as failures
        return false
    },
})
```

---

## HIGH PRIORITY (P1) - Fix Within 1-2 Weeks

### 4. Per-Chat Rate Limiter Ignores String ChatIDs

**Location:** `sender/client.go:792-801`

**Problem:**
```go
func extractChatID(chatID tg.ChatID) int64 {
    switch v := chatID.(type) {
    case int64:
        return v
    case int:
        return int64(v)
    default:
        return 0  // @channelusername becomes 0!
    }
}
```

All channel usernames (`@mychannel`, `@newsbot`) map to key `0`, sharing a single limiter.

**Fix - Key by Normalized String:**
```go
// Change map type
chatLimiters map[string]*chatLimiterEntry

func extractChatKey(chatID tg.ChatID) string {
    switch v := chatID.(type) {
    case int64:
        return strconv.FormatInt(v, 10)
    case int:
        return strconv.Itoa(v)
    case string:
        return strings.ToLower(v)  // Normalize @Username → @username
    default:
        return fmt.Sprintf("%v", v)
    }
}
```

---

### 5. Inconsistent Retry/Rate-Limit Across Methods

**Location:** `sender/client.go`, `sender/methods.go`

**Problem:**
```go
// Full resilience (rate limit + retry):
func (c *Client) SendMessage(ctx context.Context, req SendMessageRequest) (*tg.Message, error) {
    return withRetry(c, ctx, req.ChatID, func() (*tg.Message, error) {
        return c.sendMessageOnce(ctx, req)
    })
}

// NO retry, NO per-chat rate limit:
func (c *Client) EditMessageText(ctx context.Context, req EditMessageTextRequest) (*tg.Message, error) {
    resp, err := c.executeRequest(ctx, "editMessageText", req)  // Direct call!
    // ...
}
```

**Impact:** `EditMessageText`, `DeleteMessage`, `ForwardMessage`, etc. bypass retry and per-chat limiting.

**Fix:** Make all methods follow consistent resilience path.

---

### 6. Dead/Unused Internal Packages

**Location:** `internal/httpclient/`, `internal/resilience/`, `internal/syncutil/`

**Evidence:**
```bash
grep -r "internal/httpclient" --include="*.go"  # Empty
grep -r "internal/resilience" --include="*.go"  # Empty  
grep -r "internal/syncutil" --include="*.go"    # Only in own test
```

**Findings:**
- `internal/httpclient` has BETTER HTTP config than sender/receiver use
- `internal/resilience` duplicates sender functionality
- `internal/syncutil` is obsolete (Go 1.25 has `WaitGroup.Go`)

**Decision Required:**
- **Option A (Lean):** Delete all three packages
- **Option B (Integrate):** Use `internal/httpclient` everywhere, delete rest

---

### 7. Missing ResponseHeaderTimeout

**Location:** `sender/client.go:180-197`

**Problem:** HTTP client lacks `ResponseHeaderTimeout` - vulnerable to slow-drip attacks.

**Fix:**
```go
Transport: &http.Transport{
    ResponseHeaderTimeout: 10 * time.Second,  // ADD
    ExpectContinueTimeout: 1 * time.Second,   // ADD
    // ... existing config
}
```

---

### 8. Webhook allowedDomain Check is Fragile

**Location:** `receiver/webhook.go:112-115`

**Problem:**
```go
if h.allowedDomain != "" && r.Host != h.allowedDomain {
    h.fail(w, "forbidden", http.StatusForbidden)
    return
}
```

- `r.Host` can include port (`example.com:443`)
- Behind reverse proxies, Host varies
- Host header isn't strong security anyway

**Fix:**
```go
func normalizeHost(host string) string {
    h, _, err := net.SplitHostPort(host)
    if err != nil {
        return strings.ToLower(host)
    }
    return strings.ToLower(h)
}
```

Also document: "allowedDomain is a coarse filter; the secret token is the real auth."

---

### 9. Logger Nil-Safety

**Location:** `receiver/NewPollingClient()`, `receiver/NewWebhookHandler()`

**Problem:** Accept `logger *slog.Logger` but don't default to `slog.Default()` if nil.

**Fix:**
```go
if logger == nil { 
    logger = slog.Default() 
}
c.logger = logger  // Use c.logger everywhere
```

---

### 10. Updates Channel Never Closed

**Location:** `bot.go:215-219`

**Problem:** `Bot.Close()` doesn't close `b.updates` channel - goroutines blocked forever.

**Fix:**
```go
func (b *Bot) Close() error {
    b.Stop()
    close(b.updates)  // ADD THIS
    return b.sender.Close()
}
```

---

## MEDIUM PRIORITY (P2) - Fix Within 1 Month

### 11. ChatID Type is `any` - No Type Safety

**Location:** `tg/types.go:7`

```go
type ChatID = any  // Zero compile-time safety
```

**Recommendation:** Create wrapper type with custom marshal.

---

### 12. Webhook Buffer Pool Inefficient

**Location:** `receiver/webhook.go:153-177`

**Problem:** `io.ReadAll` allocates new memory, then copies to pooled buffer.

**Fix:** Either remove pool or use `io.ReadFull` directly into buffer.

---

### 13. Overkill Crypto Random for Jitter

**Location:** `sender/client.go:781-787`, `receiver/polling.go:578-584`

**Problem:** Using `crypto/rand` for backoff jitter - no security requirement.

**Fix:** Use `math/rand/v2` instead.

---

### 14. Missing Response Size Limit in Sender

**Location:** `sender/client.go:645-653`

**Current:** Reads full response then checks size.

**Better:** Use `io.LimitReader` from the start to cap reads.

---

### 15. Unused Config Fields

**Location:** `sender/config.go`

Fields present but never used:
- `AllowedPhotoDirs`
- `MaxFileSize`
- `LogFilePath`

**Fix:** Wire them in or remove them.

---

## Telegram API Compliance ✅

### Rate Limits (Verified Against Official Docs)

| Limit Type | Official | galigo Default | Status |
|------------|----------|----------------|--------|
| Global broadcast | 30 msg/sec | 30 RPS | ✅ Correct |
| Per-chat (private) | 1 msg/sec | 1 RPS | ✅ Correct |
| Per-group | 20 msg/min | 1 RPS (stricter) | ✅ Safe |
| Paid broadcast | 1000 msg/sec | Not implemented | ⚠️ Optional |

### Bot API 8.0 Enhancements (Nice-to-have)

1. Parse `X-RateLimit-*` headers for proactive throttling
2. Support `adaptive_retry` field in 429 responses (milliseconds)
3. Group-specific limits (20 msg/min for groups)

---

## Dependency Status

| Package | Current | Latest | Status |
|---------|---------|--------|--------|
| sony/gobreaker/v2 | v2.4.0 | v2.4.0 | ✅ Current |
| golang.org/x/time | v0.14.0 | v0.14.0 | ✅ Current |
| stretchr/testify | v1.8.4 | v1.9.x+ | ⚠️ Update recommended |

---

## Implementation Plan

### Phase 1: Security Critical (Week 1)

| PR | Issue | Files | Effort | Risk |
|----|-------|-------|--------|------|
| PR1 | Token-safe error messages | `sender/client.go`, `receiver/polling.go` | Medium | High |
| PR2 | Streaming multipart uploads | `sender/client.go`, `sender/multipart.go` | High | Medium |
| PR3 | Circuit breaker IsSuccessful | `sender/client.go`, `receiver/*.go` | Low | Medium |

**Estimated time:** 3-5 days

### Phase 2: High Priority (Week 2-3)

| PR | Issue | Files | Effort |
|----|-------|-------|--------|
| PR4 | Fix per-chat limiter keying | `sender/client.go` | Low |
| PR5 | Consistent retry all methods | `sender/methods.go` | Medium |
| PR6 | Remove/integrate dead packages | `internal/*` | Low |
| PR7 | Add ResponseHeaderTimeout | `sender/client.go` | Low |
| PR8 | Fix webhook allowedDomain | `receiver/webhook.go` | Low |
| PR9 | Logger nil defaults | `receiver/*.go` | Low |
| PR10 | Close updates channel | `bot.go` | Low |

**Estimated time:** 3-5 days

### Phase 3: Medium Priority (Week 4+)

| PR | Issue | Files | Effort |
|----|-------|-------|--------|
| PR11 | Fix buffer pool or remove | `receiver/webhook.go` | Low |
| PR12 | math/rand/v2 for jitter | `sender/client.go`, `receiver/polling.go` | Low |
| PR13 | Type-safe ChatID | `tg/types.go` | High |
| PR14 | Remove unused config fields | `sender/config.go` | Low |

**Estimated time:** 3-5 days

---

## Testing Recommendations

### 1. Security Tests (NEW - CRITICAL)
```go
func TestNoTokenInErrors(t *testing.T) {
    token := "123456:ABC-DEF-secret"
    // Trigger: network errors, DNS failures, TLS errors
    // Assert: err.Error() does NOT contain token substring
}
```

### 2. Memory Tests
```go
func BenchmarkMultipartUpload(b *testing.B) {
    // Upload 50MB file
    // Monitor memory: runtime.MemStats
    // Before fix: ~50MB per upload
    // After fix: ~constant memory
}
```

### 3. Resilience Tests
```go
func TestBreakerNotOpenOn400(t *testing.T) {
    // Send 100 requests that return 400
    // Assert: breaker still closed
}

func TestBreakerOpensOn500(t *testing.T) {
    // Send requests that return 500
    // Assert: breaker opens after threshold
}
```

### 4. Use `testing/synctest` (Go 1.25)
```go
import "testing/synctest"

func TestRetryWithFakeClock(t *testing.T) {
    synctest.Run(func() {
        // Virtual time, no real delays
        result, err := withRetry(client, ctx, chatID, fn)
        synctest.Wait()
    })
}
```

---

## Conclusion

This combined review identifies **3 critical issues** requiring immediate attention:

| Priority | Issue | Risk |
|----------|-------|------|
| 🔴 P0 | Token leakage via errors | Credential compromise |
| 🔴 P0 | Memory buffering uploads | OOM under load |
| 🔴 P0 | Circuit breaker self-DOS | Service unavailability |

The codebase has strong fundamentals - good architecture, proper error types, and solid test coverage. After addressing P0 and P1 issues (~2 weeks of work), galigo would be production-ready.

**Post-fix Assessment:** 8.5/10 (projected)

---

## Quick Reference: Files to Modify

```
P0 (Critical):
├── sender/client.go      (token redaction, streaming, breaker)
├── sender/multipart.go   (update comment or streaming)
├── receiver/polling.go   (token redaction, breaker)
└── receiver/webhook.go   (breaker)

P1 (High):
├── sender/client.go      (limiter keying, ResponseHeaderTimeout)
├── sender/methods.go     (consistent retry)
├── receiver/webhook.go   (allowedDomain)
├── receiver/*.go         (logger nil-safety)
├── bot.go                (close updates channel)
└── internal/*            (delete or integrate)

P2 (Medium):
├── tg/types.go           (ChatID type)
├── receiver/webhook.go   (buffer pool)
└── sender/config.go      (unused fields)
```