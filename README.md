# galigo

[![Go Reference](https://pkg.go.dev/badge/github.com/prilive-com/galigo.svg)](https://pkg.go.dev/github.com/prilive-com/galigo)
[![CI](https://github.com/prilive-com/galigo/actions/workflows/ci.yml/badge.svg)](https://github.com/prilive-com/galigo/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/prilive-com/galigo)](https://goreportcard.com/report/github.com/prilive-com/galigo)
[![codecov](https://codecov.io/gh/prilive-com/galigo/graph/badge.svg)](https://codecov.io/gh/prilive-com/galigo)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A production-grade Go library for the Telegram Bot API with built-in resilience.

galigo is designed for high-load environments where reliability matters. Unlike libraries that treat errors as exceptional, galigo treats rate limits, network failures, and API instability as expected conditions — handling them automatically so you can focus on your bot's logic.

## Features

- **Resilient** — Circuit breaker prevents cascading failures; only trips on 5xx/network errors, not user errors
- **Respectful** — Smart rate limiting with per-chat and global limits; auto-handles 429 Retry-After
- **Secure** — Tokens auto-redacted from logs and error messages; TLS 1.2+ enforced
- **Complete** — Full Telegram Bot API coverage including Stars, Gifts, Business, and Forum Topics
- **Flexible** — Use the unified Bot type or import only `sender/` or `receiver/` packages
- **Modern** — Built for Go 1.25+ with generics, iterators, and structured logging

## Installation

```bash
go get github.com/prilive-com/galigo
```

**Requirements:** Go 1.25 or later

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
)

func main() {
    bot, err := galigo.New(os.Getenv("TELEGRAM_BOT_TOKEN"),
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
                "Echo: "+update.Message.Text)
        }
    }
}
```

## Documentation

| Resource | Description |
|----------|-------------|
| [API Reference](https://pkg.go.dev/github.com/prilive-com/galigo) | Full type and method documentation |
| [Configuration Guide](./docs/configuration.md) | Circuit breaker, rate limiting, retry, and options |
| [Error Handling](./docs/errors.md) | Sentinel errors and recommended actions |
| [Architecture](./docs/architecture.md) | Goroutines, channels, and operational model |
| [Examples](./examples/) | Working code for common patterns |
| [Testing Guide](./docs/testing.md) | Integration testing with galigo-testbot |
| [Telegram Bot API](https://core.telegram.org/bots/api) | Official Telegram documentation |

### Examples

- [Echo Bot](./examples/echo/) — Basic message handling
- [Keyboards](./examples/keyboard/) — Inline keyboard interactions
- [Webhooks](./examples/webhook/) — Production webhook setup

## Compatibility

| Component | Version | Notes |
|-----------|---------|-------|
| **Go** | 1.25+ | Uses generics, iter.Seq, log/slog |
| **Telegram Bot API** | 8.0+ | Stars, Gifts, Business, Checklists |
| **Platforms** | Linux, macOS, Windows | Pure Go, no CGO |

### Supported API Methods

galigo implements **150+ methods** covering:

- Messages, media, files, and albums
- Inline keyboards and callback queries
- Chat administration and moderation
- Forum topics and permissions
- Stickers and custom emoji
- Payments, Stars, and Gifts
- Polls, quizzes, and giveaways
- Webhooks and long polling

For the complete method list, see the [API Reference](https://pkg.go.dev/github.com/prilive-com/galigo/sender).

## Thread Safety

| Component | Safe | Notes |
|-----------|------|-------|
| `galigo.Bot` | ✅ | All methods safe for concurrent use |
| `sender.Client` | ✅ | Designed for high-concurrency |
| `receiver.PollingClient` | ✅ | Single goroutine fetches, multiple can consume |
| `tg.Update` | ✅ | Immutable after creation |

## Error Handling

galigo provides typed errors for precise handling:

```go
import (
    "errors"
    "github.com/prilive-com/galigo/tg"
)

_, err := bot.SendMessage(ctx, chatID, text)
if err != nil {
    switch {
    case errors.Is(err, tg.ErrBotBlocked):
        // User blocked the bot — remove from database
    case errors.Is(err, tg.ErrTooManyRequests):
        // Rate limited — already retried, consider backing off
    case errors.Is(err, tg.ErrMessageNotFound):
        // Message was deleted — update local state
    case errors.Is(err, tg.ErrCircuitOpen):
        // Circuit breaker open — Telegram API may be down
    default:
        var apiErr *tg.APIError
        if errors.As(err, &apiErr) {
            log.Printf("API error %d: %s", apiErr.Code, apiErr.Description)
        }
    }
}
```

### Error Reference

| Error | When |
|-------|------|
| `ErrBotBlocked` | User blocked the bot |
| `ErrBotKicked` | Bot removed from group |
| `ErrChatNotFound` | Chat doesn't exist |
| `ErrMessageNotFound` | Message was deleted |
| `ErrMessageNotModified` | Edit had no changes |
| `ErrTooManyRequests` | Rate limited (429) |
| `ErrUnauthorized` | Invalid bot token |
| `ErrForbidden` | Missing permissions |
| `ErrCircuitOpen` | Circuit breaker tripped |
| `ErrMaxRetries` | All retry attempts failed |

## Configuration

### Bot Options

```go
bot, err := galigo.New(token,
    // Receiving mode (choose one)
    galigo.WithPolling(30, 100),          // Long polling
    galigo.WithWebhook(8443, "secret"),   // Webhook

    // Resilience
    galigo.WithRetries(3),                // Max retry attempts
    galigo.WithRateLimit(30.0, 5),        // Global: 30 req/s, burst 5

    // Behavior
    galigo.WithLogger(slog.Default()),    // Custom logger
    galigo.WithAllowedUpdates("message", "callback_query"),
)
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TELEGRAM_BOT_TOKEN` | — | Bot token from @BotFather |

For advanced configuration, see the [sender](https://pkg.go.dev/github.com/prilive-com/galigo/sender#Config) and [receiver](https://pkg.go.dev/github.com/prilive-com/galigo/receiver#Config) package documentation.

## Modular Usage

Use only the packages you need:

```go
// Only sending messages
import "github.com/prilive-com/galigo/sender"

client, _ := sender.New(token, sender.WithRetries(3))
client.SendMessage(ctx, sender.SendMessageRequest{
    ChatID: chatID,
    Text:   "Hello!",
})

// Only receiving updates
import "github.com/prilive-com/galigo/receiver"

updates := make(chan tg.Update, 100)
poller := receiver.NewPollingClient(token, updates, logger, cfg)
poller.Start(ctx)
```

## Resilience

### Circuit Breaker

Prevents cascading failures when Telegram is unavailable:

- Opens after 50% failure rate (minimum 3 requests in 60s window)
- Half-open state after 30s timeout
- Only server errors (5xx) and network errors trip the breaker
- Client errors (4xx) never trip — prevents self-inflicted outages

### Rate Limiting

Respects Telegram's limits automatically:

- **Global:** 30 requests/second (configurable)
- **Per-chat:** 1 request/second per chat (configurable)
- **429 handling:** Reads `retry_after` from response, waits automatically

### Retry Strategy

Transient failures are retried with exponential backoff:

- Base wait: 500ms → 1s → 2s → 4s (capped at 30s)
- Cryptographic jitter prevents thundering herd
- Only retries network errors and 5xx responses

## Testing

```bash
# Unit tests
go test ./...

# With race detector
go test -race ./...

# With coverage
go test -coverprofile=coverage.out ./...
```

### Integration Tests

galigo includes a testbot for validating against the real Telegram API:

```bash
export TESTBOT_TOKEN="your-token"
export TESTBOT_CHAT_ID="your-chat-id"
export TESTBOT_ADMINS="your-user-id"

go run ./cmd/galigo-testbot --run all
```

See [docs/testing.md](./docs/testing.md) for complete testing documentation.

## Dependencies

| Package | Purpose |
|---------|---------|
| [`sony/gobreaker/v2`](https://github.com/sony/gobreaker) | Circuit breaker |
| [`golang.org/x/time`](https://pkg.go.dev/golang.org/x/time/rate) | Rate limiting |

Testing only: `stretchr/testify`

## Contributing

Contributions are welcome! Please:

1. Read the [Contributing Guide](CONTRIBUTING.md)
2. Ensure tests pass: `go test ./...`
3. Run linter: `golangci-lint run`
4. Follow existing code style

## Security

Found a vulnerability? Please report it privately:

- Use [GitHub's private vulnerability reporting](https://github.com/prilive-com/galigo/security)
- See [SECURITY.md](.github/SECURITY.md) for our security policy

**Do not** open public issues for security vulnerabilities.

## License

[MIT License](LICENSE) — use freely in personal and commercial projects.
