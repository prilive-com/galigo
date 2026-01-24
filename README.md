# galigo

A unified Go library for Telegram Bot API with built-in resilience features.

## Features

- **Dual mode**: Webhook or long polling for receiving updates
- **Circuit breaker**: Fault tolerance with sony/gobreaker/v2
- **Rate limiting**: Per-chat and global rate limiting with golang.org/x/time/rate
- **Retry with backoff**: Exponential backoff with cryptographic jitter (crypto/rand)
- **TLS 1.2+**: Secure connections by default
- **Secret token protection**: Auto-redacts tokens in logs (slog.LogValuer)
- **Modern Go**: Built for Go 1.25+ with generics and iter.Seq iterators

## Installation

```bash
go get github.com/prilive-com/galigo
```

## Quick Start

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/prilive-com/galigo"
    "github.com/prilive-com/galigo/tg"
)

func main() {
    token := os.Getenv("TELEGRAM_BOT_TOKEN")

    bot, err := galigo.New(token,
        galigo.WithPolling(30, 100),
        galigo.WithRetries(3),
        galigo.WithRateLimit(30.0, 5),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer bot.Close()

    ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer cancel()

    if err := bot.Start(ctx); err != nil {
        log.Fatal(err)
    }

    for update := range bot.Updates() {
        if update.Message != nil {
            bot.SendMessage(ctx, update.Message.Chat.ID,
                "Echo: "+update.Message.Text,
                galigo.WithReplyTo(update.Message.MessageID))
        }
    }
}
```

## Package Structure

```
galigo/
├── bot.go           # Unified Bot type with options
├── tg/              # Shared Telegram types
│   ├── types.go     # Message, User, Chat, Editable interface
│   ├── update.go    # Update, CallbackQuery
│   ├── keyboard.go  # Fluent inline keyboard builder with generics
│   ├── errors.go    # Error types and sentinels
│   ├── config.go    # Configuration helpers
│   ├── parse_mode.go # ParseMode constants
│   └── secret.go    # SecretToken (auto-redacts in logs)
├── receiver/        # Update receiving (webhook/polling)
│   ├── polling.go   # Long polling client with circuit breaker
│   ├── webhook.go   # Webhook HTTP handler
│   ├── api.go       # Webhook management API (set/delete/get)
│   ├── config.go    # Receiver configuration
│   └── errors.go    # Receiver error types
├── sender/          # Message sending
│   ├── client.go    # Sender client with retry and rate limiting
│   ├── requests.go  # Request types (SendMessage, EditMessage, etc.)
│   ├── options.go   # Functional options for requests
│   ├── config.go    # Sender configuration
│   └── errors.go    # API errors with sentinel detection
└── internal/        # Internal packages
    ├── httpclient/  # HTTP client with TLS 1.2+
    ├── resilience/  # Circuit breaker, rate limiting, retry
    └── validate/    # Validation utilities
```

## Modular Usage

Use only what you need:

```go
// Only receiving updates (long polling)
import "github.com/prilive-com/galigo/receiver"

updates := make(chan tg.Update, 100)
client := receiver.NewPollingClient(token, updates, logger, cfg)
client.Start(ctx)

// Only sending messages
import "github.com/prilive-com/galigo/sender"

client, _ := sender.New(token, sender.WithRetries(3))
client.SendMessage(ctx, sender.SendMessageRequest{
    ChatID: chatID,
    Text:   "Hello!",
})
```

## Inline Keyboards

```go
import "github.com/prilive-com/galigo/tg"

// Fluent builder
kb := tg.NewKeyboard().
    Row(tg.Btn("Yes", "yes"), tg.Btn("No", "no")).
    Row(tg.BtnURL("Help", "https://example.com")).
    Build()

// Quick helpers
confirm := tg.Confirm("yes:123", "no:123")
pagination := tg.Pagination(page, total, "page")

// Grid from slice (uses generics)
grid := tg.Grid(items, 2, func(item Item) tg.InlineKeyboardButton {
    return tg.Btn(item.Name, "select:"+item.ID)
})

// Iterate over keyboard rows (uses iter.Seq)
for row := range kb.Rows() {
    fmt.Println(row)
}
```

## Editable Interface

Edit and delete messages using the `Editable` interface:

```go
// Message implements Editable
msg, _ := bot.SendMessage(ctx, chatID, "Original text")

// Edit using Editable
bot.Edit(ctx, msg, "Updated text", sender.WithEditParseMode(tg.ParseModeHTML))

// Delete using Editable
bot.Delete(ctx, msg)

// Store message reference for later
stored := tg.StoredMessage{MsgID: msg.MessageID, ChatID: msg.Chat.ID}
bot.Edit(ctx, stored, "Edited later")
```

## Error Handling

Errors map to sentinels for easy checking with `errors.Is`:

```go
import "errors"

result, err := bot.SendMessage(ctx, chatID, text)
if err != nil {
    var apiErr *sender.APIError
    if errors.As(err, &apiErr) {
        log.Printf("API error: %s (code=%d)", apiErr.Description, apiErr.Code)
    }

    // Check specific error types
    if errors.Is(err, sender.ErrBotBlocked) {
        // User blocked the bot
    }
    if errors.Is(err, sender.ErrMessageNotFound) {
        // Message was deleted
    }
    if errors.Is(err, sender.ErrRateLimited) {
        // Rate limited, retry later
    }
    if errors.Is(err, sender.ErrCircuitOpen) {
        // Circuit breaker is open
    }
}
```

## Bot Options

```go
bot, err := galigo.New(token,
    // Mode selection
    galigo.WithPolling(30, 100),           // Long polling (timeout, limit)
    galigo.WithWebhook(8443, "secret"),    // Or webhook mode

    // Resilience
    galigo.WithRetries(3),                 // Max retry attempts
    galigo.WithRateLimit(30.0, 5),         // Global RPS and burst
    galigo.WithPollingMaxErrors(10),       // Max consecutive errors

    // Behavior
    galigo.WithDeleteWebhook(true),        // Delete webhook before polling
    galigo.WithAllowedUpdates("message", "callback_query"),
    galigo.WithUpdateBufferSize(100),      // Updates channel buffer

    // Logging
    galigo.WithLogger(customLogger),       // Custom slog.Logger
)
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TELEGRAM_BOT_TOKEN` | - | Bot token (required) |
| `RECEIVER_MODE` | `longpolling` | `webhook` or `longpolling` |
| `POLLING_TIMEOUT` | `30` | Long polling timeout (0-60s) |
| `POLLING_LIMIT` | `100` | Updates per request (1-100) |
| `WEBHOOK_PORT` | `8443` | Webhook HTTPS port |
| `RATE_LIMIT_REQUESTS` | `30` | Global rate limit (req/s) |
| `MAX_RETRIES` | `3` | Max retry attempts |

### Sender Configuration

```go
cfg := sender.Config{
    Token:           tg.SecretToken(token),
    BaseURL:         "https://api.telegram.org",
    RequestTimeout:  30 * time.Second,
    MaxRetries:      3,
    RetryBaseWait:   500 * time.Millisecond,
    RetryMaxWait:    30 * time.Second,
    RetryFactor:     2.0,
    GlobalRPS:       30.0,
    GlobalBurst:     5,
    PerChatRPS:      1.0,
    PerChatBurst:    3,
}
client, _ := sender.NewFromConfig(cfg)
```

## Resilience Features

### Circuit Breaker

Prevents cascading failures when Telegram API is unavailable:

```go
// Default settings (configurable)
// - Opens after 50% failure rate (min 3 requests)
// - Half-open after timeout
// - Logs state changes
```

### Rate Limiting

Respects Telegram's rate limits:

- **Global**: 30 requests/second (configurable)
- **Per-chat**: 1 request/second for same chat (configurable)
- Automatically waits when limits exceeded

### Retry with Backoff

Automatically retries transient failures:

- Exponential backoff: 500ms, 1s, 2s, 4s...
- Cryptographic jitter prevents thundering herd
- Respects `Retry-After` header from Telegram
- Only retries network errors and 5xx responses

## Security

- **TLS 1.2+**: All HTTP clients enforce minimum TLS 1.2
- **Secret token protection**: `tg.SecretToken` type prevents accidental logging
- **Webhook validation**: Constant-time comparison of webhook secrets
- **Input validation**: Request parameters validated before sending

## License

MIT License
