# galigo Configuration Reference

This document covers the configuration options, built-in resilience features, and operational parameters for galigo.

## Bot Options

| Option | Example | Notes |
|--------|---------|-------|
| `WithPolling(timeout, limit)` | `WithPolling(30, 100)` | 30s long-poll timeout, 100 updates per batch |
| `WithWebhook(port, secret)` | `WithWebhook(8443, "secret")` | Requires TLS termination + ingress |
| `WithRetries(n)` | `WithRetries(3)` | Exponential backoff with jitter |
| `WithRateLimit(rps, burst)` | `WithRateLimit(30.0, 5)` | Global rate limiter |
| `WithPerChatRateLimit(rps, burst)` | `WithPerChatRateLimit(1.0, 3)` | Per-chat rate limiter |
| `WithLogger(logger)` | `WithLogger(slog.Default())` | Structured logging with token redaction |
| `WithAllowedUpdates(types...)` | `WithAllowedUpdates("message", "callback_query")` | Only receive specified update types |

## Circuit Breaker

galigo uses [sony/gobreaker](https://github.com/sony/gobreaker) for circuit breaking.

| Parameter | Value |
|-----------|-------|
| Failure threshold | 50% of requests in 60s window (minimum 3 requests) |
| Half-open state | Allows 3 probe requests |
| Recovery timeout | 30 seconds |
| Trips on | 5xx responses + network errors |
| Does NOT trip on | 4xx client errors (prevents self-inflicted outages) |

**Why 4xx errors don't trip the breaker:** A 400 Bad Request or 403 Forbidden is a client error — your request was wrong, not the server. Tripping the breaker on 4xx would cause a self-inflicted outage when sending to blocked users or invalid chats.

## Rate Limiter

galigo enforces Telegram's rate limits automatically.

| Limit | Default | Telegram's Limit |
|-------|---------|------------------|
| Global | 30 req/s, burst 5 | ~30 req/s |
| Per-chat (private) | 1 req/s, burst 3 | ~1 req/s (60/min) |
| Per-chat (group) | 0.33 req/s, burst 2 | ~20 req/min |
| 429 handling | Auto-reads `retry_after`, waits, retries | — |

**Per-chat limiters** are created on-demand and cleaned up after 10 minutes of inactivity to prevent memory leaks.

## Retry Strategy

| Parameter | Value |
|-----------|-------|
| Base wait | 500ms |
| Backoff factor | 2x (exponential) |
| Sequence | 500ms → 1s → 2s → 4s → ... |
| Maximum wait | 30 seconds |
| Jitter | Cryptographic random (prevents thundering herd) |
| Retries on | Network errors, 5xx responses, 429 (after retry_after) |
| Does NOT retry | 4xx client errors (except 429) |

**retry_after parsing:** When Telegram returns 429 Too Many Requests, galigo reads `retry_after` from:
1. JSON response body `parameters.retry_after` (primary)
2. HTTP `Retry-After` header (fallback)

## Thread Safety

| Component | Thread-safe | Notes |
|-----------|-------------|-------|
| `galigo.Bot` | Yes | All methods safe for concurrent use |
| `sender.Client` | Yes | Designed for high-concurrency workloads |
| `receiver.PollingClient` | Yes | Single goroutine fetches; multiple can consume |
| `receiver.WebhookHandler` | Yes | Safe for concurrent HTTP requests |
| `tg.Update` | Yes | Immutable after creation |

## Modular Usage

galigo can be used as a unified bot or as separate sender/receiver components.

| Mode | When to Use | Import |
|------|-------------|--------|
| **Unified Bot** | Most applications | `github.com/prilive-com/galigo` |
| **Sender only** | Notification/alert services | `github.com/prilive-com/galigo/sender` |
| **Receiver only** | Update processor services | `github.com/prilive-com/galigo/receiver` |

### Sender-Only Example

```go
import "github.com/prilive-com/galigo/sender"

client, err := sender.New(token,
    sender.WithRetries(3),
    sender.WithRateLimit(30.0, 5),
)
if err != nil {
    log.Fatal(err)
}
defer client.Close()

msg, err := client.SendMessage(ctx, sender.SendMessageRequest{
    ChatID: chatID,
    Text:   "Hello!",
})
```

### File Upload Example

```go
import "github.com/prilive-com/galigo/sender"

// From file ID (already on Telegram servers)
photo := sender.FromFileID("AgACAgIAAxkBAAI...")

// From URL (Telegram downloads it)
photo := sender.FromURL("https://example.com/image.jpg")

// From bytes (retry-safe)
data, _ := os.ReadFile("photo.jpg")
photo := sender.FromBytes(data, "photo.jpg")

// From io.Reader (single-use, NOT retry-safe)
file, _ := os.Open("photo.jpg")
defer file.Close()
photo := sender.FromReader(file, "photo.jpg")

// Send
msg, err := client.SendPhoto(ctx, sender.SendPhotoRequest{
    ChatID:  chatID,
    Photo:   photo,
    Caption: "Hello!",
})
```

**Note:** Use `FromBytes` for retry-safe uploads. `FromReader` is single-use — if a retry occurs, the reader is at EOF.

### Receiver-Only Example

```go
import "github.com/prilive-com/galigo/receiver"

poller, err := receiver.NewPollingClient(token,
    receiver.WithPollingTimeout(30),
    receiver.WithPollingLimit(100),
)
if err != nil {
    log.Fatal(err)
}
defer poller.Stop()

if err := poller.Start(ctx); err != nil {
    log.Fatal(err)
}

for update := range poller.Updates() {
    // Process update
}
```

## Security

| Feature | Details |
|---------|---------|
| Token redaction | Bot token automatically redacted from logs and error messages |
| TLS enforcement | TLS 1.2+ required for all API connections |
| Webhook validation | Constant-time secret comparison for webhook requests |
| HTTP timeouts | Response header timeout prevents hung connections |

## Dependencies

galigo has only 2 runtime dependencies:

| Dependency | Purpose |
|------------|---------|
| `github.com/sony/gobreaker/v2` | Circuit breaker |
| `golang.org/x/time` | Rate limiting |

Test dependencies (`stretchr/testify`) are not included in production builds.
