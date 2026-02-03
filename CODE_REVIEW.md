# galigo — Code Review Request

## Overview

**galigo** is a unified Go library for the Telegram Bot API with built-in resilience features. It combines update receiving (webhook + long polling) and message sending into a single module with circuit breaker, rate limiting, retry with backoff, and streaming file uploads.

- **Module**: `github.com/prilive-com/galigo`
- **Language**: Go 1.25+
- **Source files**: 93 (+ 47 test files)
- **Lines of code**: ~27,500
- **Commits**: 24
- **Latest commit**: `a7644cb` (2026-02-02)
- **License**: MIT

## Repository Structure

```
galigo/
├── bot.go, doc.go          # Unified Bot type with functional options
├── tg/                     # Shared Telegram types (28 files)
│   ├── types.go            # Message, User, Chat, File, Editable interface
│   ├── update.go           # Update, CallbackQuery
│   ├── keyboard.go         # Fluent inline keyboard builder with generics
│   ├── errors.go           # Canonical error types and sentinels
│   ├── stickers.go         # Sticker, StickerSet, InputSticker
│   ├── payments.go         # Invoice, LabeledPrice, StarAmount
│   ├── gifts.go            # Gift, Gifts, OwnedGifts
│   ├── checklists.go       # InputChecklist, InputChecklistTask
│   ├── business.go         # BusinessConnection, InputStoryContent
│   ├── games.go            # Game, GameHighScore
│   ├── inline.go           # InlineQuery, InlineQueryResult types
│   └── ...                 # chat_member, chat_permissions, forum, boosts, passport
├── receiver/               # Update receiving (webhook + long polling)
│   ├── polling.go          # Long polling with circuit breaker + delivery policies
│   ├── webhook.go          # Webhook HTTP handler
│   └── api.go              # Webhook management API
├── sender/                 # Message sending (34 files)
│   ├── client.go           # Core client with retry, rate limiting, circuit breaker
│   ├── inputfile.go        # InputFile (FileID, URL, Reader, FromBytes)
│   ├── multipart.go        # Multipart encoder for file uploads
│   ├── methods.go          # Core methods (send, edit, delete, forward, copy)
│   ├── stickers.go         # Sticker set CRUD operations
│   ├── payments.go         # Invoices, star transactions, refunds
│   ├── gifts.go            # Gift operations
│   ├── inline.go           # Inline queries, checklists
│   ├── business.go         # Business account methods
│   ├── games.go            # Game methods
│   ├── verification.go     # User/chat verification
│   ├── chat_info.go        # getChat, getChatMember, etc.
│   ├── chat_settings.go    # setChatTitle, setChatPhoto, etc.
│   ├── chat_admin.go       # Promote, demote, custom title
│   ├── chat_moderation.go  # Ban, unban, restrict
│   ├── chat_pin.go         # Pin/unpin messages
│   ├── polls.go            # Polls and quizzes
│   ├── forum.go            # Forum topic management
│   └── ...                 # config, options, validate, errors
├── internal/
│   ├── testutil/           # Mock server, fixtures, helpers
│   └── validate/           # Token and input validation
├── cmd/galigo-testbot/     # Acceptance test bot
│   ├── main.go             # CLI entry point
│   ├── engine/             # Scenario runner, steps, adapter, Runtime
│   ├── suites/             # 24 test scenarios (S0-S24)
│   ├── fixtures/           # Embedded test media (JPEG, GIF, PNG, MP3, OGG, MP4)
│   ├── registry/           # 48 target method coverage tracking
│   ├── evidence/           # JSON report generation
│   └── config/, cleanup/   # Env config, message cleanup
└── examples/echo/          # Echo bot example
```

## Key Design Decisions

### 1. Resilience Stack
Every outgoing request passes through: **rate limiter** (global + per-chat) → **circuit breaker** (5xx/network errors only, 4xx excluded) → **retry with exponential backoff + cryptographic jitter** → **retry_after from JSON body or HTTP header**.

### 2. File Upload Abstraction
`InputFile` supports four modes: `FromFileID`, `FromURL`, `FromReader` (single-use), `FromBytes` (retry-safe via `Source` factory). The multipart encoder auto-detects whether a request needs `multipart/form-data` or JSON.

### 3. Error Taxonomy
Canonical sentinel errors in `tg/errors.go` with `DetectSentinel()` mapping API response codes/descriptions. `APIError.Unwrap()` returns the sentinel, enabling `errors.Is()` checks. Backward-compatible aliases in `sender/errors.go`.

### 4. Acceptance Testing
Built-in testbot with 24 scenarios covering 48 API methods. Scenarios are declarative (step sequences), automatically clean up created messages, and generate JSON evidence reports. Sticker tests require a human `user_id` via `TESTBOT_ADMINS`. Checklists require Premium.

### 5. Type Safety
- `tg.ChatID` type alias for chat identification
- `tg.SecretToken` auto-redacts in logs (`slog.LogValuer`, `fmt.Stringer`, `encoding.TextMarshaler`)
- `tg.Editable` interface for edit/delete operations
- Fluent keyboard builder with generics and `iter.Seq`
- Polymorphic `ChatMember` unmarshal based on `status` field

## Areas of Focus for Review

### Architecture & API Design
- Is the package split (`tg/`, `sender/`, `receiver/`, root `Bot`) appropriate?
- Are the functional options patterns consistent and ergonomic?
- Is the `SenderClient` interface in the testbot engine well-designed for testability?

### Resilience & Correctness
- Circuit breaker configuration: 50% failure threshold, 4xx excluded — correct trade-off?
- Rate limiter cleanup (10-minute stale TTL) — appropriate for production?
- Retry logic: exponential backoff with crypto jitter, `retry_after` parsing priority (JSON body > HTTP header)
- `isNotModifiedErr` pattern for idempotent 400 errors in testbot

### Error Handling
- Sentinel error detection via description matching then HTTP code — correct priority?
- Token scrubbing in HTTP error messages
- Context error propagation (unwrapped, not wrapped)

### Security
- TLS 1.2+ enforcement, `ResponseHeaderTimeout`, `MaxBytesReader`
- Constant-time comparison for webhook secrets
- Token scrubbing preserving error chain via `Unwrap()`

### Testing
- Unit test coverage patterns (mock server, capture assertions, FakeSleeper)
- Acceptance test architecture (engine/steps/suites/adapter pattern)
- Test fixture generation (pure Go, no external dependencies)

## How to Build & Test

```bash
# Build
go build ./...

# Unit tests
go test -race ./...

# Acceptance tests (requires Telegram bot token)
export TESTBOT_TOKEN="your-token"
export TESTBOT_CHAT_ID="your-supergroup-id"
export TESTBOT_ADMINS="your-user-id"
go run ./cmd/galigo-testbot --run all
go run ./cmd/galigo-testbot --status
```

## Dependencies

- `github.com/sony/gobreaker/v2` — Circuit breaker
- `golang.org/x/time` — Rate limiting
- `github.com/stretchr/testify` — Testing assertions (test-only)

No other external dependencies. Standard library used for HTTP, TLS, crypto, logging, and all media fixture generation.
