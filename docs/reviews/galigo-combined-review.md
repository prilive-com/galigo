# galigo — Ultimate Consolidated Code Review

**Date:** February 2026  
**Sources:** 5 independent analyses (3 consultants + Gemini + developer response review)  
**Commit:** 3df6223 (2026-01-27)  
**Go Version:** 1.25+

---

## Executive Summary

This document represents the **definitive, consolidated analysis** of the galigo Telegram Bot API library, synthesizing findings from five independent technical reviews. All claims have been verified against source code and official Telegram documentation.

### Final Assessment: **6.5/10 — SOLID FOUNDATION WITH 5 CRITICAL FIXES REQUIRED**

| Category | Score | Key Issues |
|----------|-------|------------|
| Architecture | 7/10 | Good separation, pipeline inconsistency |
| Security | 7/10 | Strong intent, token leak in receiver |
| Performance | 5/10 | **Critical hot path locking bottleneck** |
| Resilience | 6/10 | Incomplete rate limiting, wrong breaker behavior |
| Error Handling | 9/10 | Excellent sentinel pattern |
| Testing | 8/10 | High coverage, minor test smells |
| Code Quality | 7/10 | Clean but gofmt issues |

### Critical Issues At-a-Glance

| Priority | Issue | Impact | Source |
|----------|-------|--------|--------|
| **P0** | Hot path locking | Serializes all concurrent requests | Gemini |
| **P0** | Pipeline inconsistency | 28 methods bypass rate limiting | All consultants |
| **P0** | Token leakage in receiver | Security vulnerability | Consultants |
| **P0** | Webhook 503 retry storms | Duplicate processing under load | Consultants |
| **P1** | 429 opens circuit breaker | Cascading failures | All |
| **P1** | Group rate limits unsafe | Exceeds Telegram's 20/min limit | Consultants |
| **P1** | ResponseHeaderTimeout wrong | Suboptimal timeout handling | Gemini |

---

# Part 1: Critical Findings (P0)

## 1. 🚨 NEW: Hot Path Global Lock Contention

| Aspect | Detail |
|--------|--------|
| **Source** | Gemini (unique finding) |
| **Severity** | **CRITICAL — Performance Killer** |
| **Verified** | ✅ Code confirmed |

### The Problem

In `sender/client.go`, the `getChatLimiter()` function acquires a **global write lock for EVERY request** just to update a timestamp:

```go
// sender/client.go lines 694-725 - CURRENT CODE
func (c *Client) getChatLimiter(chatID string) *rate.Limiter {
    now := time.Now()

    // Fast path: read lock
    c.limiterMu.RLock()
    entry, exists := c.chatLimiters[chatID]
    c.limiterMu.RUnlock()

    if exists {
        // 🚨 PROBLEM: Global WRITE lock for EVERY request!
        c.limiterMu.Lock()
        entry.lastUsed = now  // Just updating a timestamp...
        c.limiterMu.Unlock()
        return entry.limiter
    }
    // ... create new entry with write lock (this is fine)
}
```

### Why This Is Critical

In high-throughput scenarios (1,000+ concurrent goroutines sending messages):

1. **Every request** must acquire the global write lock
2. All goroutines **serialize** at this single point
3. Throughput collapses to essentially single-threaded
4. Latency spikes as goroutines queue up waiting for the lock

**This is a textbook concurrency anti-pattern.**

### Required Fix

Use `atomic.Int64` for the timestamp — lock-free updates:

```go
// sender/client.go - FIXED

type chatLimiterEntry struct {
    limiter  *rate.Limiter
    lastUsed atomic.Int64  // Unix nanoseconds, not time.Time
}

func (c *Client) getChatLimiter(chatID string) *rate.Limiter {
    now := time.Now().UnixNano()

    // Read lock for map access
    c.limiterMu.RLock()
    entry, exists := c.chatLimiters[chatID]
    c.limiterMu.RUnlock()

    if exists {
        // ✅ FIXED: Lock-free atomic update
        entry.lastUsed.Store(now)
        return entry.limiter
    }

    // Write lock only for NEW entries (this is acceptable)
    c.limiterMu.Lock()
    defer c.limiterMu.Unlock()

    // Double-check after acquiring write lock
    if entry, exists = c.chatLimiters[chatID]; exists {
        entry.lastUsed.Store(now)
        return entry.limiter
    }

    entry = &chatLimiterEntry{
        limiter: rate.NewLimiter(rate.Limit(c.getEffectiveRPS(chatID)), c.config.PerChatBurst),
    }
    entry.lastUsed.Store(now)
    c.chatLimiters[chatID] = entry
    return entry.limiter
}

// Update cleanup to use atomic
func (c *Client) cleanupStaleLimiters() {
    threshold := time.Now().Add(-10 * time.Minute).UnixNano()
    
    c.limiterMu.Lock()
    defer c.limiterMu.Unlock()
    
    for chatID, entry := range c.chatLimiters {
        if entry.lastUsed.Load() < threshold {
            delete(c.chatLimiters, chatID)
        }
    }
}
```

---

## 2. 🚨 Sender Pipeline Inconsistency

| Aspect | Detail |
|--------|--------|
| **Source** | All 3 consultants |
| **Severity** | **CRITICAL** |
| **Developer Response** | Accepted (but underestimated complexity) |

### The Problem

**Only 2 of ~30 methods use the resilience pipeline:**

```go
// HAVE resilience (rate limiting + retry):
SendMessage()  → withRetry() → sendMessageOnce() → waitForRateLimit()
SendPhoto()    → withRetry() → sendPhotoOnce()   → waitForRateLimit()

// NO resilience (28+ methods go direct to breaker):
SendDocument(), SendVideo(), SendAudio(), SendVoice(), SendAnimation(),
SendSticker(), SendMediaGroup(), EditMessageText(), EditMessageCaption(),
EditMessageReplyMarkup(), EditMessageMedia(), DeleteMessage(), ForwardMessage(),
CopyMessage(), AnswerCallbackQuery(), SendLocation(), SendVenue(), SendContact(),
SendPoll(), SendDice(), GetMe(), GetFile(), GetUserProfilePhotos(), 
SetMessageReaction(), ForwardMessages(), CopyMessages(), DeleteMessages(), etc.
```

### Impact

1. **Inconsistent behavior**: `SendMessage` handles load gracefully, `SendDocument` returns 429
2. **429 storms**: Unthrottled methods hammer the API
3. **Breaker chaos**: 429s from unthrottled methods can open breaker globally
4. **User confusion**: Library claims "resilient sender" but most methods aren't

### Required Fix

Centralize rate limiting in `executeRequest()`:

```go
// sender/client.go - PROPOSED

func (c *Client) executeRequest(ctx context.Context, method string, payload any) (*apiResponse, error) {
    // 1. Extract chatID for per-chat limiting
    chatID := extractChatIDFromRequest(payload)
    
    // 2. Apply rate limiting (ALL methods)
    if err := c.waitForRateLimit(ctx, chatID); err != nil {
        return nil, err
    }
    
    // 3. Circuit breaker + actual request
    return c.breaker.Execute(func() (*apiResponse, error) {
        return c.doRequest(ctx, method, payload)
    })
}

// Type switch for known request types (fast path)
func extractChatIDFromRequest(req any) string {
    switch r := req.(type) {
    case SendMessageRequest:
        return extractChatID(r.ChatID)
    case SendDocumentRequest:
        return extractChatID(r.ChatID)
    case SendVideoRequest:
        return extractChatID(r.ChatID)
    case EditMessageTextRequest:
        if r.ChatID != nil {
            return extractChatID(r.ChatID)
        }
        return ""  // Inline message edit
    // ... add all request types with ChatID
    default:
        return ""  // Global rate limit only
    }
}
```

**For retry policy**, apply selectively:
- **Message-sending methods**: Wrap in `withRetry()` (respects retry_after)
- **Read-only methods** (GetMe, GetFile): No retry needed
- **Time-sensitive methods** (AnswerCallbackQuery): No retry

---

## 3. 🚨 Token Leakage in receiver/api.go

| Aspect | Detail |
|--------|--------|
| **Source** | All consultants |
| **Severity** | **CRITICAL — Security** |
| **Developer Response** | Accepted |

### The Problem

`sender/client.go` correctly scrubs tokens from errors, but `receiver/api.go` does NOT:

```go
// receiver/api.go - VULNERABLE
func SetWebhook(ctx context.Context, client *http.Client, token tg.SecretToken, url string, ...) error {
    apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook", token.Value())
    // ...
    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("setWebhook request failed: %w", err)  // ⚠️ Token in URL!
    }
}
```

Go's HTTP client includes the full URL in error messages:
```
setWebhook request failed: Post "https://api.telegram.org/bot123456:ABC-DEF/setWebhook": connection refused
```

### Required Fix

Create shared scrubbing helper (prevents drift from duplication):

```go
// internal/scrub/token.go
package scrub

import (
    "strings"
    "github.com/prilive-com/galigo/tg"
)

func TokenFromError(err error, token tg.SecretToken) error {
    if err == nil {
        return nil
    }
    tokenVal := token.Value()
    if tokenVal == "" {
        return err
    }
    msg := err.Error()
    if strings.Contains(msg, tokenVal) {
        return &scrubbedError{
            msg: strings.ReplaceAll(msg, tokenVal, "[REDACTED]"),
            err: err,
        }
    }
    return err
}

type scrubbedError struct {
    msg string
    err error
}

func (e *scrubbedError) Error() string { return e.msg }
func (e *scrubbedError) Unwrap() error { return e.err }
```

Apply in receiver/api.go:
```go
import "github.com/prilive-com/galigo/internal/scrub"

func SetWebhook(...) error {
    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("setWebhook failed: %w", scrub.TokenFromError(err, token))
    }
}
```

---

## 4. 🚨 Webhook 503 Causes Retry Storms

| Aspect | Detail |
|--------|--------|
| **Source** | All 3 consultants |
| **Severity** | **CRITICAL** |
| **Developer Response** | ❌ **REJECTED (INCORRECTLY)** |

### Why Developer Is Wrong

**Developer claimed:** "The receiver/ package already has UpdateDeliveryPolicy... The mechanism already exists."

**The truth (verified by code inspection):**

1. `UpdateDeliveryPolicy` exists in config ✓
2. `PollingClient` uses it via `deliverUpdate()` switch statement ✓
3. **`WebhookHandler` does NOT use it** — simple non-blocking send:

```go
// receiver/webhook.go lines 186-192 - ACTUAL CODE
select {
case h.updates <- update:
    h.logger.Debug("update forwarded", "update_id", update.UpdateID)
default:
    return ErrChannelBlocked  // → Always results in 503!
}
```

**Proof:** Test `TestWebhook_ChannelFull_Returns503` explicitly expects 503.

**Telegram's documented behavior:** Non-2xx responses trigger retries with exponential backoff.

### Impact

1. Channel full → 503 → Telegram retries
2. Channel still full → 503 → More retries
3. **Same update delivered multiple times** when channel clears
4. **Load amplification** during overload (exactly when you can't handle it)

### Required Fix

Apply the same delivery policy system to webhook:

```go
// receiver/webhook.go - FIXED

func (h *WebhookHandler) processUpdate(w http.ResponseWriter, r *http.Request) error {
    // ... parse update ...
    
    // Use delivery policy (same as polling)
    switch h.deliveryPolicy {
    case DeliveryPolicyBlock:
        return h.deliverBlocking(r.Context(), update)
    case DeliveryPolicyDropNewest:
        return h.deliverDropNewest(update)
    case DeliveryPolicyDropOldest:
        return h.deliverDropOldest(update)
    default:
        return h.deliverBlocking(r.Context(), update)
    }
}

func (h *WebhookHandler) deliverBlocking(ctx context.Context, update tg.Update) error {
    deliveryCtx := ctx
    if h.deliveryTimeout > 0 {
        var cancel context.CancelFunc
        deliveryCtx, cancel = context.WithTimeout(ctx, h.deliveryTimeout)
        defer cancel()
    }
    
    select {
    case h.updates <- update:
        return nil
    case <-deliveryCtx.Done():
        // Timeout - ACK 200 anyway to prevent Telegram retry!
        h.logger.Warn("delivery timeout, dropping", "update_id", update.UpdateID)
        if h.onUpdateDropped != nil {
            h.onUpdateDropped(update.UpdateID, "delivery_timeout")
        }
        return nil  // ✅ Return nil = 200 OK
    }
}

func (h *WebhookHandler) deliverDropNewest(update tg.Update) error {
    select {
    case h.updates <- update:
        return nil
    default:
        h.logger.Warn("channel full, dropping", "update_id", update.UpdateID)
        if h.onUpdateDropped != nil {
            h.onUpdateDropped(update.UpdateID, "channel_full")
        }
        return nil  // ✅ ACK 200, prevents retry storm
    }
}
```

**Key principle:** Never return 503 unless you WANT Telegram to retry.

---

# Part 2: High Priority Findings (P1)

## 5. Circuit Breaker Opens on 429

| Aspect | Detail |
|--------|--------|
| **Source** | All reviewers (unanimous) |
| **Severity** | HIGH |
| **Developer Response** | Accepted |

### The Problem

```go
func isBreakerSuccess(err error) bool {
    if apiErr.Code == 429 {
        return false  // WRONG: Counts toward tripping breaker
    }
}
```

**429 is "you're sending too fast" — not service failure.**

### Cascade Failure Scenario

1. Bot hits rate limit → 429
2. Breaker counts as failure
3. After threshold → breaker opens → ALL requests fail
4. Breaker closes → burst of queued requests → 429 → repeat

### Required Fix (One Line)

```go
func isBreakerSuccess(err error) bool {
    if err == nil {
        return true
    }
    
    var apiErr *APIError
    if errors.As(err, &apiErr) {
        // 429 = rate limited = handle via retry, NOT breaker
        if apiErr.Code == 429 {
            return true  // ← CHANGED
        }
        if apiErr.Code >= 400 && apiErr.Code < 500 {
            return true  // Client errors
        }
        return false  // 5xx = actual service failure
    }
    
    if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
        return true
    }
    
    return false  // Network errors = service failure
}
```

---

## 6. Per-Chat Limits Don't Respect Group Limits

| Aspect | Detail |
|--------|--------|
| **Source** | Consultants (2nd consultant strongest) |
| **Severity** | HIGH |
| **Developer Response** | ❌ **REJECTED (INCORRECTLY)** |

### The Facts (Telegram Documentation)

| Chat Type | Telegram Limit | Current Default | Status |
|-----------|---------------|-----------------|--------|
| Private | ~1 msg/sec | 1 msg/sec | ✅ OK |
| **Groups** | **≤20 msg/min** (~0.33/s) | 1 msg/sec | ❌ **3x OVER LIMIT** |
| Channels | Similar to groups | 1 msg/sec | ❌ Over |

### Developer's Objection

> "Auto-detection by ID sign is brittle and wrong"

### Why Developer Is Wrong

1. **Telegram documents negative IDs** for groups/supergroups/channels
2. **Safe by default is correct** — too conservative = slight latency, too aggressive = API blocks
3. The detection doesn't need to be perfect, just safer than current behavior

### Required Fix

```go
func (c *Client) getChatLimiter(chatID string) *rate.Limiter {
    // ... lock handling ...
    
    // Determine appropriate limit based on chat type
    rps := c.config.PerChatRPS
    burst := c.config.PerChatBurst
    
    // Safe default for groups/channels (negative IDs)
    if id, err := strconv.ParseInt(chatID, 10, 64); err == nil && id < 0 {
        // Groups/supergroups/channels: 20/min = 0.33/s
        if rps > 0.33 {
            rps = 0.33
            burst = 2
            c.logger.Debug("using group rate limit", "chat_id", chatID, "rps", rps)
        }
    }
    
    // ... create limiter with rps, burst ...
}
```

**Plus provide override:**
```go
func WithGroupRateLimit(rps float64, burst int) Option
```

---

## 7. NEW: ResponseHeaderTimeout Misconfiguration

| Aspect | Detail |
|--------|--------|
| **Source** | Gemini (unique finding) |
| **Severity** | MEDIUM |

### The Problem

```go
// sender/client.go - createHTTPClient()
Transport: &http.Transport{
    ResponseHeaderTimeout: cfg.RequestTimeout,  // Same as total timeout!
}
```

If `RequestTimeout` = 30s and server hangs on headers for 29s, you only have 1s for the body.

### Required Fix

```go
Transport: &http.Transport{
    ResponseHeaderTimeout: 10 * time.Second,  // Fail fast if no headers
    // ... other settings
}
```

Or make it configurable:
```go
ResponseHeaderTimeout: cfg.HeaderTimeout,  // Default: 10s
```

---

# Part 3: Medium/Low Priority Findings (P2-P3)

## P2 Findings

| # | Finding | Source | Fix |
|---|---------|--------|-----|
| 8 | **Limiter map size limit** | Gemini | Add max entries or LRU eviction |
| 9 | **Webhook domain port check** | Consultants | Use `net.SplitHostPort()` |
| 10 | **Validation consistency** | Consultants | Add `validateChatID()` to SendMessage/SendPhoto |
| 11 | **gofmt compliance** | All | `gofmt -w .` + CI check |
| 12 | **Duplicate scrub helper** | All | Move to `internal/scrub/` |

## P3 Findings (Nice to Have)

| # | Finding | Source | Fix |
|---|---------|--------|-----|
| 13 | **Test time.Sleep** | Gemini | Use channel signaling |
| 14 | **Map cleanup locking** | Gemini | Consider sync.Map or sharding |
| 15 | **withRetry unused chatID param** | 2nd consultant | Remove or use for logging |

---

# Part 4: What's Excellent (Keep These!)

## ✅ Go 1.25 Feature Adoption
- Correct `sync.WaitGroup.Go()` usage
- Proper `iter.Seq` iterators with early termination

## ✅ Secret Token Design
- Comprehensive `SecretToken` with 4 redaction interfaces
- Constant-time comparison for webhook secrets

## ✅ Error Handling
- Full `errors.Is()`/`errors.As()` support via `Unwrap()`
- `DetectSentinel()` for Telegram error mapping
- 12+ well-named sentinel errors

## ✅ Testing Infrastructure
- `MockTelegramServer` with request capture
- `FakeSleeper` for deterministic retry testing
- Fuzz tests for JSON parsing (5 fuzz functions)
- 98.6% coverage in `tg/` package

## ✅ File Upload Design
- Streaming via `io.Pipe` (no memory buffering)
- Retry-safe `Source` factory pattern

## ✅ Fluent APIs
- Keyboard builder with chainable methods
- Generic `Grid[T]` helper
- Functional options throughout

---

# Part 5: Developer Response Scorecard

| Finding | Developer Said | Correct? | Notes |
|---------|---------------|----------|-------|
| P0-1 Pipeline | Accept | ✅ | Underestimated complexity |
| P0-2 Token scrub | Accept | ⚠️ | Should use shared helper |
| **P0-3 Webhook 503** | **REJECT** | ❌ **WRONG** | Webhook doesn't use delivery policy |
| **P1-4 Group limits** | **REJECT** | ❌ **WRONG** | Safe defaults are correct |
| P1-5 429 breaker | Accept | ✅ | |
| P1-6 Domain port | Accept (P2) | ✅ | |
| P1-7 Validation | Accept (partial) | ✅ | |
| P1-8 gofmt | Accept | ✅ | |

**Score: 5/8 correct decisions**

---

# Part 6: Final Implementation Plan

## Recommended Order

| Order | Fix | Effort | Risk if Skipped |
|-------|-----|--------|-----------------|
| 1 | **Hot path locking** | Small | Performance collapse at scale |
| 2 | **Token scrub receiver** | Small | Security vulnerability |
| 3 | **429 breaker fix** | Tiny | Cascading failures |
| 4 | **Webhook delivery policy** | Medium | Duplicate processing |
| 5 | **Group rate limits** | Small | API blocks |
| 6 | **Pipeline unification** | Large | Inconsistent behavior |
| 7 | **ResponseHeaderTimeout** | Tiny | Suboptimal timeouts |
| 8 | Validation consistency | Small | Subtle bugs |
| 9 | Domain port check | Tiny | Edge case failures |
| 10 | gofmt + CI | Small | Code quality |

## PR Strategy

```
PR #1: Performance (Items 1, 7)
  - Fix hot path locking (atomic lastUsed)
  - Fix ResponseHeaderTimeout
  
PR #2: Security (Items 2, 3)
  - Token scrubbing in receiver
  - 429 breaker classification
  
PR #3: Reliability (Items 4, 5)
  - Webhook delivery policy
  - Group rate limits
  
PR #4: Pipeline Unification (Item 6)
  - Largest change, do after above stabilizes
  
PR #5: Cleanup (Items 8, 9, 10)
  - Validation, domain check, gofmt
```

---

# Conclusion

## Before These Fixes

- **Cannot handle high concurrency** (hot path lock)
- **Security vulnerability** (token leakage)
- **Unreliable under load** (webhook duplicates, wrong breaker behavior)
- **May violate Telegram ToS** (group rate limits)

## After These Fixes

- **Production-ready** for high-throughput bots
- **Secure by default** (no token leakage)
- **Reliable under load** (proper backpressure handling)
- **Compliant** with Telegram's rate limits

## Final Verdict

| Metric | Current | After Fixes |
|--------|---------|-------------|
| **Score** | 6.5/10 | **9/10** |
| **Production Ready** | ❌ No | ✅ Yes |
| **Recommended** | With reservations | Strongly |

The galigo library has excellent foundations and demonstrates strong Go engineering. Once the 5 critical fixes are implemented, it will be one of the best Telegram Bot API libraries for Go.

---

*Ultimate consolidated analysis from 5 independent sources, February 2026*

---

# Appendix A: All Reviewers Summary

| Reviewer | Key Contribution | Unique Finds |
|----------|-----------------|--------------|
| **Claude (Initial)** | Architecture, Go 1.25 patterns, testing | Fuzz tests, iter.Seq usage |
| **Consultant 1** | Pipeline inconsistency, webhook 503 | First to identify 503 retry storm |
| **Consultant 2** | Developer response validation | Confirmed webhook issue, group limits defense |
| **Gemini** | Performance analysis | Hot path locking (critical), ResponseHeaderTimeout |
| **Developer** | Implementation context | 5/8 correct decisions |

# Appendix B: Code Locations Reference

| Issue | File | Lines |
|-------|------|-------|
| Hot path locking | sender/client.go | 694-725 |
| Pipeline (SendMessage) | sender/client.go | ~400-450 |
| Pipeline (methods.go) | sender/methods.go | All |
| Token scrub missing | receiver/api.go | SetWebhook, DeleteWebhook, GetWebhookInfo |
| Webhook 503 | receiver/webhook.go | 186-192 |
| Breaker 429 | sender/client.go | isBreakerSuccess() |
| ResponseHeaderTimeout | sender/client.go | createHTTPClient() |
| Delivery policy (polling) | receiver/polling.go | 388-399 |
| Delivery policy (config) | receiver/config.go | UpdateDeliveryPolicy |