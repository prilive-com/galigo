# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with the galigo library.

## Repository Overview

Unified Go library for Telegram Bot API combining receiving and sending functionality with built-in resilience:

- **galigo** (root): Unified `Bot` type with functional options
- **tg/**: Shared Telegram types, `Editable` interface, `SecretToken`, keyboard builders, canonical errors
- **receiver/**: Dual-mode update receiving (webhook + long polling) with circuit breaker and delivery policies
- **sender/**: Resilient message sending with rate limiting, retries, circuit breaker, and file uploads
- **internal/**: HTTP client, resilience utilities, sync utilities, validation, test utilities
- **cmd/galigo-testbot/**: Acceptance test bot for real Telegram API validation

## Package Structure

```
galigo/
├── bot.go              # Unified Bot type with options
├── doc.go              # Package documentation
├── tg/                 # Shared Telegram types
│   ├── types.go        # Message, User, Chat, File, Editable interface
│   ├── update.go       # Update, CallbackQuery
│   ├── keyboard.go     # Fluent inline keyboard builder with generics
│   ├── errors.go       # Canonical error types and sentinels
│   ├── config.go       # Configuration helpers
│   ├── parse_mode.go   # ParseMode constants
│   └── secret.go       # SecretToken (auto-redacts in logs)
├── receiver/           # Update receiving (webhook/polling)
│   ├── polling.go      # Long polling with circuit breaker + delivery policies
│   ├── webhook.go      # Webhook HTTP handler
│   ├── api.go          # Webhook management API (set/delete/get)
│   ├── config.go       # Receiver configuration + delivery policy
│   └── errors.go       # Receiver error types
├── sender/             # Message sending
│   ├── client.go       # Sender client with retry, rate limiting, multipart detection
│   ├── methods.go      # API methods (GetMe, SendDocument, SendVideo, etc.)
│   ├── requests.go     # Request types (SendMessage, SendDocument, etc.)
│   ├── inputfile.go    # InputFile for file uploads (FileID, URL, Reader)
│   ├── multipart.go    # Multipart encoder for file uploads
│   ├── options.go      # Functional options for requests
│   ├── config.go       # Sender configuration
│   └── errors.go       # Error aliases (backward compatible with tg.Err*)
├── internal/           # Internal packages
│   ├── httpclient/     # HTTP client with TLS 1.2+
│   ├── resilience/     # Circuit breaker, rate limiting, retry
│   ├── syncutil/       # WaitGroup utilities
│   ├── testutil/       # Test utilities, mock server, fixtures
│   └── validate/       # Token and input validation
├── cmd/
│   └── galigo-testbot/ # Acceptance test bot
│       ├── main.go     # CLI entry point (--run, --status flags)
│       ├── engine/     # Scenario runner, steps, SenderClient interface, adapter
│       ├── suites/     # Test scenario definitions
│       │   ├── tier1.go     # Phase A: core scenarios (S0-S5)
│       │   ├── media.go     # Phase B: media scenarios (S6-S9)
│       │   └── keyboards.go # Phase C: keyboard scenarios (S10+)
│       ├── fixtures/   # Embedded test media files (go:embed)
│       │   ├── photo.jpg      # 100x100 JPEG
│       │   ├── animation.gif  # 100x100 2-frame GIF
│       │   ├── sticker.png    # 512x512 PNG
│       │   ├── audio.mp3      # Minimal MP3 (5 silent frames)
│       │   └── voice.ogg      # Minimal OGG Opus (1 silent frame)
│       ├── config/     # Environment config + .env loader
│       ├── evidence/   # JSON report generation
│       ├── registry/   # Method coverage tracking (25 target methods)
│       └── cleanup/    # Message cleanup utilities
└── examples/
    └── echo/           # Echo bot example
```

## Build and Test Commands

```bash
# Build all packages
go build ./...

# Run unit tests
go test ./...

# Run with verbose output
go test -v ./...

# Run with race detector
go test -race ./...

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

# Format and vet
go fmt ./...
go vet ./...

# Run acceptance tests (requires TELEGRAM_BOT_TOKEN and TESTBOT_CHAT_ID)
go run ./cmd/galigo-testbot --run all
go run ./cmd/galigo-testbot --run core     # Phase A only
go run ./cmd/galigo-testbot --run media    # Phase B only
go run ./cmd/galigo-testbot --status       # Show method coverage
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
       │   └── Delivery policies               │
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
           └── File uploads (streaming multipart)
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

// InputFile for file uploads (sender/inputfile.go)
type InputFile struct {
    FileID, URL string
    Reader      io.Reader
    FileName    string
    MediaType   string  // "photo", "video", "document", etc.
    Caption     string
    ParseMode   string
}
func FromReader(r io.Reader, filename string) InputFile
func FromFileID(fileID string) InputFile
func FromURL(url string) InputFile
func (f InputFile) MarshalJSON() ([]byte, error)  // Returns Value() for JSON (FileID or URL)
```

### Multipart Upload Detection

`sender.Client.doRequest()` automatically detects whether a request contains file uploads (InputFile with Reader set). If uploads are found, it uses `multipart/form-data` encoding; otherwise, it uses standard JSON encoding. This is transparent to the caller.

For single-file methods (sendPhoto, sendDocument, etc.), the file is placed directly in the multipart field named after the parameter (e.g., `photo`, `document`). The `attach://` reference syntax is only used for `sendMediaGroup`.

### Concurrency Patterns

- `sync/atomic` for thread-safe state (running, consecutive errors)
- `sync.Once` for safe resource cleanup
- `sync.RWMutex` for per-chat rate limiter map
- Buffered channels for update processing
- Context cancellation for graceful shutdown

## Error Handling

Errors are defined canonically in `tg` package with backward-compatible aliases in `sender`:

```go
// Canonical errors (tg/errors.go)
var ErrBotBlocked = errors.New("galigo: bot blocked by user")
var ErrTooManyRequests = errors.New("galigo: too many requests")
var ErrCircuitOpen = errors.New("galigo: circuit breaker open")

// APIError with sentinel unwrapping
type APIError struct {
    Code        int
    Description string
    RetryAfter  time.Duration
    Method      string
    cause       error  // sentinel error
}

func (e *APIError) Unwrap() error { return e.cause }
func (e *APIError) IsRetryable() bool  // Check if error can be retried

// DetectSentinel maps API errors to sentinels
// Priority: description matching first (more specific), then HTTP code (more generic)
func DetectSentinel(code int, description string) error

// Usage - both tg.Err* and sender.Err* work (aliases)
if errors.Is(err, tg.ErrBotBlocked) {
    // Handle blocked bot
}
if errors.Is(err, sender.ErrTooManyRequests) {
    // Rate limited - will be retried automatically
}
```

### Available Sentinel Errors

| Error | Description |
|-------|-------------|
| `ErrUnauthorized` | Invalid bot token |
| `ErrForbidden` | Bot lacks permissions |
| `ErrNotFound` | Resource not found |
| `ErrTooManyRequests` | Rate limited (429) |
| `ErrBotBlocked` | Bot blocked by user |
| `ErrBotKicked` | Bot kicked from chat |
| `ErrChatNotFound` | Chat doesn't exist |
| `ErrMessageNotFound` | Message to edit/delete not found |
| `ErrMessageNotModified` | Message content unchanged |
| `ErrCircuitOpen` | Circuit breaker is open |
| `ErrMaxRetries` | Max retries exceeded |
| `ErrRateLimited` | Local rate limit exceeded |

## Adding New Features

### Adding a New Bot Method

1. Add request type in `sender/requests.go`:
```go
type SendDocumentRequest struct {
    ChatID   tg.ChatID `json:"chat_id"`
    Document InputFile `json:"document"`
    Caption  string    `json:"caption,omitempty"`
    // ...
}
```

2. Add method to sender client in `sender/methods.go`:
```go
func (c *Client) SendDocument(ctx context.Context, req SendDocumentRequest) (*tg.Message, error) {
    resp, err := c.executeRequest(ctx, "sendDocument", req)
    if err != nil {
        return nil, err
    }
    return parseMessage(resp)
}
```

3. Add convenience method to Bot in `bot.go`:
```go
func (b *Bot) SendDocument(ctx context.Context, chatID tg.ChatID, doc InputFile, opts ...DocumentOption) (*tg.Message, error) {
    req := sender.SendDocumentRequest{ChatID: chatID, Document: doc}
    for _, opt := range opts {
        opt(&req)
    }
    return b.sender.SendDocument(ctx, req)
}
```

4. Add acceptance test coverage in `cmd/galigo-testbot/`:
   - Add step in `engine/steps.go`
   - Add adapter method in `engine/adapter.go`
   - Add to scenario in `suites/media.go`
   - Register in `registry/registry.go` target methods

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

1. Add sentinel in `tg/errors.go` (canonical location):
```go
var ErrDocumentTooLarge = errors.New("galigo: document too large")
```

2. Add detection in `DetectSentinel()`:
```go
case strings.Contains(descLower, "file is too big"):
    return ErrDocumentTooLarge
```

3. Add alias in `sender/errors.go` (backward compatibility):
```go
var ErrDocumentTooLarge = tg.ErrDocumentTooLarge
```

## Acceptance Testing (galigo-testbot)

The testbot validates API methods against real Telegram servers. It runs scenarios that exercise each method and generates JSON evidence reports.

### Test Phases

| Phase | Scenarios | Methods Covered |
|-------|-----------|-----------------|
| A (Core) | S0-S5: Smoke, Identity, Messages, Forward, Actions | getMe, sendMessage, editMessageText, deleteMessage, forwardMessage, copyMessage, sendChatAction |
| B (Media) | S6-S9: Media Uploads, Media Groups, Edit Media, Get File | sendPhoto, sendDocument, sendAnimation, sendAudio, sendVoice, sendMediaGroup, editMessageCaption, getFile |
| C (Keyboards) | S10: Inline Keyboard | sendMessage (with markup), editMessageReplyMarkup |

### Running Tests

```bash
# Set environment
export TELEGRAM_BOT_TOKEN="your-token"
export TESTBOT_CHAT_ID="your-chat-id"

# Run all tests
go run ./cmd/galigo-testbot --run all

# Run specific phase or scenario
go run ./cmd/galigo-testbot --run core
go run ./cmd/galigo-testbot --run media
go run ./cmd/galigo-testbot --run keyboards
go run ./cmd/galigo-testbot --run media-uploads

# Check coverage
go run ./cmd/galigo-testbot --status
```

### Available Suites

CLI `--run` values: `smoke`, `identity`, `messages`, `forward`, `actions`, `core`, `media`, `media-uploads`, `media-groups`, `edit-media`, `get-file`, `keyboards`, `inline-keyboard`, `all`

### Test Fixtures

All media fixtures are embedded via `go:embed` in `cmd/galigo-testbot/fixtures/`. Generated with pure Go (no external tools like ffmpeg):

- `photo.jpg` — 100x100 red JPEG (image/jpeg)
- `animation.gif` — 100x100 2-frame red/blue GIF (image/gif)
- `sticker.png` — 512x512 gradient PNG (image/png)
- `audio.mp3` — 5 silent MPEG1 Layer3 frames
- `voice.ogg` — Minimal OGG Opus with 1 silent frame

### Adding a New Test Scenario

1. Add step type in `engine/steps.go`:
```go
type SendVoiceStep struct {
    Voice   MediaInput
    Caption string
}
func (s *SendVoiceStep) Name() string { return "sendVoice" }
func (s *SendVoiceStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) { ... }
```

2. Add adapter method in `engine/adapter.go`
3. Add to `SenderClient` interface in `engine/scenario.go`
4. Create or update scenario in `suites/media.go`
5. Register method in `registry/registry.go`

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

// Per-chat limiter (created on demand, cleaned up after 10 minutes idle)
chatLimiter := rate.NewLimiter(rate.Limit(1.0), 3)
```

### Retry with Backoff (sender/client.go)

```go
// Exponential backoff with cryptographic jitter
backoff := baseWait * math.Pow(factor, attempt-1)
jitter := crypto/rand.Int(backoff * 0.2)  // +/- 20%

// retry_after parsing priority:
// 1. JSON response body parameters.retry_after (primary)
// 2. HTTP Retry-After header (fallback)
```

### Update Delivery Policy (receiver/config.go)

```go
type UpdateDeliveryPolicy int

const (
    DeliveryPolicyBlock      UpdateDeliveryPolicy = iota  // Block with timeout (default)
    DeliveryPolicyDropNewest                               // Drop new updates when full
    DeliveryPolicyDropOldest                               // Drop oldest updates when full
)

cfg := receiver.DefaultConfig()
cfg.UpdateDeliveryPolicy = receiver.DeliveryPolicyBlock
cfg.UpdateDeliveryTimeout = 5 * time.Second
cfg.OnUpdateDropped = func(updateID int, reason string) {
    metrics.IncrCounter("updates_dropped", 1, "reason", reason)
}
```

## File Uploads

```go
import "github.com/prilive-com/galigo/sender"

// From file ID (already on Telegram servers)
doc := sender.FromFileID("AgACAgIAAxkBAAI...")

// From URL (Telegram will download)
doc := sender.FromURL("https://example.com/file.pdf")

// From io.Reader (streamed, no memory buffering)
file, _ := os.Open("document.pdf")
defer file.Close()
doc := sender.FromReader(file, "document.pdf")

// Send document
client.SendDocument(ctx, sender.SendDocumentRequest{
    ChatID:   chatID,
    Document: doc,
    Caption:  "Here's your document",
})
```

## Supported API Methods

### Bot Identity
- `GetMe` - Get bot information
- `LogOut` - Log out from cloud Bot API
- `CloseBot` - Close bot instance

### Messages
- `SendMessage` - Send text messages
- `SendPhoto` - Send photos (InputFile: file upload, URL, or file_id)
- `SendDocument` - Send documents
- `SendVideo` - Send videos
- `SendAudio` - Send audio files
- `SendVoice` - Send voice messages
- `SendAnimation` - Send GIFs/animations
- `SendVideoNote` - Send video notes
- `SendSticker` - Send stickers
- `SendMediaGroup` - Send albums

### Utilities
- `GetFile` - Get file info for download
- `SendChatAction` - Send typing indicator, etc.
- `GetUserProfilePhotos` - Get user's profile photos

### Location & Contact
- `SendLocation` - Send location
- `SendVenue` - Send venue
- `SendContact` - Send phone contact
- `SendPoll` - Send native polls
- `SendDice` - Send animated dice

### Message Operations
- `EditMessageText` - Edit message text
- `EditMessageCaption` - Edit caption
- `EditMessageReplyMarkup` - Edit reply markup
- `DeleteMessage` - Delete a message
- `ForwardMessage` - Forward a message
- `CopyMessage` - Copy a message

### Callback Queries
- `AnswerCallbackQuery` - Answer callback queries
- `Answer` - Answer with options (convenience)
- `Acknowledge` - Silently acknowledge

### Bulk Operations
- `ForwardMessages` - Forward multiple messages
- `CopyMessages` - Copy multiple messages
- `DeleteMessages` - Delete multiple messages
- `SetMessageReaction` - Set message reaction

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
// github.com/prilive-com/galigo/internal/syncutil
// github.com/prilive-com/galigo/internal/testutil
// github.com/prilive-com/galigo/internal/validate
```

## Dependencies

```go
require (
    github.com/sony/gobreaker/v2 v2.4.0   // Circuit breaker
    golang.org/x/time v0.14.0              // Rate limiting
    github.com/stretchr/testify v1.8.4     // Testing
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
- Test fixtures embedded via `go:embed`, no external tool dependencies

## Security Considerations

- Never log raw tokens - use `tg.SecretToken`
- Validate webhook secrets with constant-time comparison
- Enforce TLS 1.2+ minimum
- Limit response body sizes to prevent memory exhaustion
- Use `http.MaxBytesReader` for request body limits
