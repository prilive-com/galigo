# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with the galigo library.

## Repository Overview

Unified Go library for Telegram Bot API combining receiving and sending functionality with built-in resilience:

- **galigo** (root): Unified `Bot` type with functional options
- **tg/**: Shared Telegram types, `Editable` interface, `SecretToken`, keyboard builders
- **receiver/**: Dual-mode update receiving (webhook + long polling) with circuit breaker
- **sender/**: Resilient message sending with rate limiting, retries, and circuit breaker
- **internal/**: HTTP client, resilience utilities, validation

## Build and Test Commands

```bash
# Build all packages
go build ./...

# Run tests
go test ./...

# Run with verbose output
go test -v ./...

# Run with race detector
go test -race ./...

# Format and vet
go fmt ./...
go vet ./...

# Tidy dependencies
go mod tidy
```

## Architecture

```
User Application
       │
       ▼
   galigo.Bot ─────────────────────────────────┐
       │                                       │
       ├── receiver.PollingClient              │
       │   └── Circuit breaker                 │
       │   └── Exponential backoff             │
       │                                       │
       ├── receiver.WebhookHandler             │
       │   └── Rate limiting                   │
       │   └── Secret validation               │
       │                                       │
       └── sender.Client ──────────────────────┘
           └── Circuit breaker
           └── Per-chat rate limiting
           └── Global rate limiting
           └── Retry with jitter
                   │
                   ▼
           Telegram Bot API
```

### Key Types

```go
// Unified bot (bot.go)
type Bot struct {
    token    tg.SecretToken
    logger   *slog.Logger
    receiver *receiver.PollingClient
    webhook  *receiver.WebhookHandler
    sender   *sender.Client
    updates  chan tg.Update
}

// Editable interface for edit/delete operations (tg/types.go)
type Editable interface {
    MessageSig() (messageID string, chatID int64)
}

// Secret token with log redaction (tg/secret.go)
type SecretToken string
func (s SecretToken) LogValue() slog.Value  // slog.LogValuer
func (s SecretToken) String() string        // fmt.Stringer
func (s SecretToken) GoString() string      // fmt.GoStringer
func (s SecretToken) MarshalText() ([]byte, error)  // encoding.TextMarshaler
```

### Concurrency Patterns

- `sync/atomic` for thread-safe state (running, consecutive errors)
- `sync.Once` for safe resource cleanup
- `sync.RWMutex` for per-chat rate limiter map
- Buffered channels for update processing
- Context cancellation for graceful shutdown

## Adding New Features

### Adding a New Bot Method

1. Add request type in `sender/requests.go`:
```go
type SendDocumentRequest struct {
    ChatID   tg.ChatID `json:"chat_id"`
    Document string    `json:"document"`
    Caption  string    `json:"caption,omitempty"`
    // ...
}
```

2. Add method to sender client in `sender/client.go`:
```go
func (c *Client) SendDocument(ctx context.Context, req SendDocumentRequest) (*tg.Message, error) {
    return withRetry(c, ctx, req.ChatID, func() (*tg.Message, error) {
        return c.sendDocumentOnce(ctx, req)
    })
}
```

3. Add convenience method to Bot in `bot.go`:
```go
func (b *Bot) SendDocument(ctx context.Context, chatID tg.ChatID, doc string, opts ...DocumentOption) (*tg.Message, error) {
    req := sender.SendDocumentRequest{ChatID: chatID, Document: doc}
    for _, opt := range opts {
        opt(&req)
    }
    return b.sender.SendDocument(ctx, req)
}
```

### Adding New Telegram Types

Add to `tg/types.go`:
```go
type Document struct {
    FileID       string     `json:"file_id"`
    FileUniqueID string     `json:"file_unique_id"`
    Thumbnail    *PhotoSize `json:"thumbnail,omitempty"`
    FileName     string     `json:"file_name,omitempty"`
    MimeType     string     `json:"mime_type,omitempty"`
    FileSize     int64      `json:"file_size,omitempty"`
}
```

### Adding New Sentinel Errors

1. Add sentinel in `sender/errors.go`:
```go
var ErrDocumentTooLarge = errors.New("galigo/sender: document too large")
```

2. Add detection in `detectSentinel()`:
```go
case strings.Contains(descLower, "file is too big"):
    return ErrDocumentTooLarge
```

## Error Handling

```go
// Sentinel errors support errors.Is
var ErrBotBlocked = errors.New("galigo/sender: bot blocked by user")

// APIError wraps sentinels via Unwrap()
type APIError struct {
    Code        int
    Description string
    RetryAfter  time.Duration
    Method      string
    cause       error  // sentinel error
}

func (e *APIError) Unwrap() error { return e.cause }

// Usage
if errors.Is(err, sender.ErrBotBlocked) {
    // Handle blocked bot
}
```

## Resilience Configuration

### Circuit Breaker (sender/client.go, receiver/polling.go)

```go
gobreaker.Settings{
    Name:        "galigo-sender",
    MaxRequests: 3,              // Requests in half-open state
    Interval:    60 * time.Second,
    Timeout:     30 * time.Second,
    ReadyToTrip: func(counts gobreaker.Counts) bool {
        if counts.Requests < 3 {
            return false
        }
        return float64(counts.TotalFailures)/float64(counts.Requests) >= 0.5
    },
}
```

### Rate Limiting (sender/client.go)

```go
// Global limiter
globalLimiter := rate.NewLimiter(rate.Limit(30.0), 5)

// Per-chat limiter (created on demand)
chatLimiter := rate.NewLimiter(rate.Limit(1.0), 3)
```

### Retry with Backoff (sender/client.go)

```go
// Exponential backoff with cryptographic jitter
backoff := baseWait * math.Pow(factor, attempt-1)
jitter := crypto/rand.Int(backoff * 0.2)  // +/- 20%
```

## Module Structure

```go
// Main package
import "github.com/prilive-com/galigo"

// Subpackages
import "github.com/prilive-com/galigo/tg"
import "github.com/prilive-com/galigo/sender"
import "github.com/prilive-com/galigo/receiver"

// Internal (not importable)
// github.com/prilive-com/galigo/internal/httpclient
// github.com/prilive-com/galigo/internal/resilience
// github.com/prilive-com/galigo/internal/validate
```

## Dependencies

```go
require (
    github.com/sony/gobreaker/v2 v2.0.0  // Circuit breaker
    golang.org/x/time v0.5.0              // Rate limiting
)
```

## Style Conventions

- Go 1.25+, use generics and `iter.Seq` where appropriate
- Error wrapping with `%w`, use `errors.Is`/`errors.As`
- Structured logging with `log/slog`
- `SecretToken` type for any sensitive data (auto-redacts)
- Functional options pattern for configuration
- Table-driven tests with testify
- TLS 1.2+ for all HTTP clients
- Constant-time comparison for secrets (`crypto/subtle`)

## Security Considerations

- Never log raw tokens - use `tg.SecretToken`
- Validate webhook secrets with constant-time comparison
- Enforce TLS 1.2+ minimum
- Limit response body sizes to prevent memory exhaustion
- Use `http.MaxBytesReader` for request body limits
