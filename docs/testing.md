# galigo Testing Guide

This guide covers how to write tests for galigo and use the testing infrastructure.

## Quick Start

```go
package sender_test

import (
    "context"
    "net/http"
    "testing"

    "github.com/prilive-com/galigo/internal/testutil"
    "github.com/prilive-com/galigo/sender"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestSendMessage_Success(t *testing.T) {
    // Create mock server
    server := testutil.NewMockServer(t)

    // Register handler
    server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyMessage(w, 123)
    })

    // Create client using test helper (automatically cleaned up)
    client := testutil.NewTestClient(t, server.BaseURL())

    // Test
    msg, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
        ChatID: testutil.TestChatID,
        Text:   "Hello",
    })

    // Assert
    require.NoError(t, err)
    assert.Equal(t, 123, msg.MessageID)

    // Verify request
    cap := server.LastCapture()
    cap.AssertMethod(t, "POST")
    cap.AssertJSONField(t, "chat_id", float64(testutil.TestChatID))
    cap.AssertJSONField(t, "text", "Hello")
}
```

## Running Tests

```bash
# Run all tests
make test

# Run with verbose output
go test -v ./...

# Run specific package
go test -v ./sender/...

# Run specific test
go test -v -run TestSendMessage ./sender/...

# Run with race detector
make test-race

# Run with coverage
make test-coverage

# Run short tests only (skip slow tests)
make test-short
```

## Test Infrastructure

### MockTelegramServer

Creates a mock HTTP server that simulates the Telegram Bot API.

```go
// Create server (automatically cleaned up after test)
server := testutil.NewMockServer(t)

// Register handlers
server.On("/bot"+token+"/sendMessage", handler)           // POST (most common)
server.OnMethod("GET", "/bot"+token+"/getMe", handler)    // Specific method

// Get base URL for client configuration
baseURL := server.BaseURL()  // e.g., "http://127.0.0.1:12345"
botURL := server.BotURL(token)  // e.g., "http://127.0.0.1:12345/bot123:ABC"
```

#### Request Capture

All requests are automatically captured:

```go
// Get captures
captures := server.Captures()      // All captures
cap := server.LastCapture()        // Most recent
cap := server.CaptureAt(0)         // By index
count := server.CaptureCount()     // Total count

// Time between requests (for rate limit testing)
duration := server.TimeBetweenCaptures(0, 1)

// Reset
server.Reset()          // Clear captures and handlers
server.ResetCaptures()  // Clear captures only
```

#### Capture Assertions

```go
cap := server.LastCapture()

// Basic assertions
cap.AssertMethod(t, "POST")
cap.AssertPath(t, "/bot123:ABC/sendMessage")
cap.AssertContentType(t, "application/json")
cap.AssertHeader(t, "Authorization", "Bearer token")

// Query parameters
cap.AssertQuery(t, "offset", "100")
cap.HasQuery("timeout")  // bool
cap.GetQuery("limit")    // string

// JSON body assertions
cap.AssertJSONField(t, "chat_id", float64(123))
cap.AssertJSONFieldExists(t, "text")
cap.AssertJSONFieldAbsent(t, "parse_mode")

// Get body as map or decode to struct
body := cap.BodyMap(t)
cap.BodyJSON(t, &myStruct)
raw := cap.BodyString()
```

### Telegram Reply Helpers

Pre-built response functions for common Telegram API responses:

```go
// Success responses
testutil.ReplyOK(w, result)                    // Generic success
testutil.ReplyMessage(w, messageID)            // Message sent
testutil.ReplyMessageWithChat(w, msgID, chatID)
testutil.ReplyBool(w, true)                    // For deleteMessage, etc.
testutil.ReplyMessageID(w, messageID)          // For copyMessage
testutil.ReplyUser(w)                          // For getMe
testutil.ReplyUpdates(w, updates)              // For getUpdates
testutil.ReplyEmptyUpdates(w)
testutil.ReplyWebhookInfo(w, url, pendingCount)

// Error responses
testutil.ReplyError(w, code, description, params)
testutil.ReplyBadRequest(w, "chat not found")
testutil.ReplyForbidden(w, "bot was blocked")
testutil.ReplyNotFound(w, "message not found")
testutil.ReplyServerError(w, 502, "Bad Gateway")
testutil.ReplyRateLimit(w, retryAfterSeconds)  // 429 with retry_after
```

### FakeSleeper

For deterministic retry testing without actual delays:

```go
// Create fake sleeper
sleeper := &testutil.FakeSleeper{}

// Pass to client (requires WithSleeper option)
client, _ := sender.New(token,
    sender.WithBaseURL(server.BaseURL()),
    sender.WithSleeper(sleeper),
)

// After test, verify sleep calls
assert.Equal(t, 2, sleeper.CallCount())
assert.Equal(t, 5*time.Second, sleeper.LastCall())
assert.Equal(t, 2*time.Second, sleeper.CallAt(0))
assert.Equal(t, 7*time.Second, sleeper.TotalDuration())

// Get all calls
calls := sleeper.Calls()  // []time.Duration

// Reset for next test
sleeper.Reset()
```

#### RealSleeper

For integration tests that need actual delays:

```go
sleeper := testutil.RealSleeper{}
err := sleeper.Sleep(ctx, 100*time.Millisecond)
```

### Test Client Helpers

Pre-configured test clients for different testing scenarios:

```go
// Standard test client (no retries, default circuit breaker)
client := testutil.NewTestClient(t, server.BaseURL())

// Retry test client (circuit breaker never trips, use with FakeSleeper)
sleeper := &testutil.FakeSleeper{}
client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper,
    sender.WithRetries(3),
)

// Circuit breaker test client (aggressive tripping, no retries)
client := testutil.NewBreakerTestClient(t, server.BaseURL())
```

All test clients are automatically cleaned up when the test completes.

#### Circuit Breaker Settings

For tests that need custom circuit breaker behavior:

```go
// Never trip - for testing retry logic without breaker interference
settings := testutil.CircuitBreakerNeverTrip()

// Aggressive trip - for testing circuit breaker behavior
settings := testutil.CircuitBreakerAggressiveTrip()

// Use with manual client creation
client, _ := sender.New(testutil.TestToken,
    sender.WithBaseURL(server.BaseURL()),
    sender.WithCircuitBreakerSettings(settings),
)
```

### Test Fixtures

Pre-built test data for consistent tests:

```go
// Constants
testutil.TestToken      // "123456789:ABCdefGHIjklMNOpqrsTUVwxyz"
testutil.TestChatID     // int64(123456789)
testutil.TestUserID     // int64(987654321)
testutil.TestBotID      // int64(123456789)
testutil.TestUsername   // "testuser"
testutil.TestBotUsername // "testbot"

// User fixtures
user := testutil.TestUser()   // Regular user
bot := testutil.TestBot()     // Bot user

// Chat fixtures
chat := testutil.TestChat()                           // Private chat
group := testutil.TestGroupChat(id, "Group Name")
supergroup := testutil.TestSuperGroupChat(id, "Title", "username")
channel := testutil.TestChannelChat(id, "Title", "username")

// Message fixtures
msg := testutil.TestMessage(messageID, "text")
msg := testutil.TestMessageInChat(messageID, chatID, "text")

// Update fixtures
update := testutil.TestUpdate(updateID, "text")
update := testutil.TestUpdateWithMessage(updateID, msg)
update := testutil.TestUpdateWithCallback(updateID, "cb_id", "data")

// Callback query fixtures
cb := testutil.TestCallbackQuery("id", "data")
cb := testutil.TestCallbackQueryWithMessage("id", "data", msg)

// Keyboard fixtures
kb := testutil.TestInlineKeyboard(
    []tg.InlineKeyboardButton{
        testutil.TestInlineButton("Click", "callback_data"),
        testutil.TestURLButton("Open", "https://example.com"),
    },
)
```

## Test Patterns

### Testing Retry Logic

```go
func TestRetry_429WithRetryAfter(t *testing.T) {
    var attempts atomic.Int32

    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        if attempts.Add(1) == 1 {
            testutil.ReplyRateLimit(w, 2)  // First: rate limit
            return
        }
        testutil.ReplyMessage(w, 123)  // Second: success
    })

    sleeper := &testutil.FakeSleeper{}
    client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper,
        sender.WithRetries(3),
    )

    msg, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
        ChatID: testutil.TestChatID,
        Text:   "Hello",
    })

    require.NoError(t, err)
    assert.Equal(t, int32(2), attempts.Load())
    assert.Equal(t, 2*time.Second, sleeper.LastCall())  // Used retry_after
}
```

### Testing Circuit Breaker

```go
func TestCircuitBreaker_OpensOnFailures(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyServerError(w, 500, "Internal Server Error")
    })

    // Use breaker test client (trips after 2 consecutive failures)
    client := testutil.NewBreakerTestClient(t, server.BaseURL())

    // Make requests to trip breaker
    for i := 0; i < 3; i++ {
        client.SendMessage(context.Background(), sender.SendMessageRequest{
            ChatID: testutil.TestChatID,
            Text:   "Hello",
        })
    }

    // Next request should fail with circuit open
    _, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
        ChatID: testutil.TestChatID,
        Text:   "Hello",
    })

    assert.ErrorIs(t, err, sender.ErrCircuitOpen)
}
```

### Testing Error Handling

```go
func TestSendMessage_BotBlocked(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyForbidden(w, "bot was blocked by the user")
    })

    client := testutil.NewTestClient(t, server.BaseURL())

    _, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
        ChatID: testutil.TestChatID,
        Text:   "Hello",
    })

    require.Error(t, err)
    assert.ErrorIs(t, err, sender.ErrBotBlocked)
}
```

### Testing Context Cancellation

```go
func TestSendMessage_ContextCancel(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(5 * time.Second)  // Slow response
        testutil.ReplyMessage(w, 123)
    })

    client := testutil.NewTestClient(t, server.BaseURL())

    ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
    defer cancel()

    _, err := client.SendMessage(ctx, sender.SendMessageRequest{
        ChatID: testutil.TestChatID,
        Text:   "Hello",
    })

    // Context errors are returned directly (not wrapped)
    assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled))
}
```

### Table-Driven Tests

```go
func TestSendMessage(t *testing.T) {
    tests := []struct {
        name     string
        request  sender.SendMessageRequest
        response func(http.ResponseWriter, *http.Request)
        wantErr  bool
        errCode  int
    }{
        {
            name:    "success",
            request: sender.SendMessageRequest{ChatID: 123, Text: "Hi"},
            response: func(w http.ResponseWriter, r *http.Request) {
                testutil.ReplyMessage(w, 1)
            },
            wantErr: false,
        },
        {
            name:    "chat not found",
            request: sender.SendMessageRequest{ChatID: 123, Text: "Hi"},
            response: func(w http.ResponseWriter, r *http.Request) {
                testutil.ReplyBadRequest(w, "chat not found")
            },
            wantErr: true,
            errCode: 400,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            server := testutil.NewMockServer(t)
            server.On("/bot"+testutil.TestToken+"/sendMessage", tt.response)

            client := testutil.NewTestClient(t, server.BaseURL())
            _, err := client.SendMessage(context.Background(), tt.request)

            if tt.wantErr {
                require.Error(t, err)
                var apiErr *sender.APIError
                if errors.As(err, &apiErr) {
                    assert.Equal(t, tt.errCode, apiErr.Code)
                }
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

## Coverage

### Running Coverage

```bash
# Generate coverage report
make test-coverage

# View HTML report
open coverage.html

# Check coverage threshold (80%)
make check-coverage
```

### Coverage Targets

| Package | Target | Priority |
|---------|--------|----------|
| `sender/` | 85% | Critical |
| `receiver/` | 85% | Critical |
| `tg/` | 90% | High |
| `internal/validate/` | 100% | High |
| `bot.go` | 80% | Medium |

### Cross-Package Coverage

Use `-coverpkg` to include coverage from other packages:

```bash
go test -coverpkg=./... -coverprofile=coverage.out ./...
```

## CI Integration

Tests run automatically on push/PR via GitHub Actions:

```yaml
# .github/workflows/test.yml
- go test -v -race ./...           # Race detector
- go test -coverpkg=./... ...      # Coverage
- govulncheck ./...                # Vulnerability scan
```

### Coverage Gate

CI fails if coverage drops below 80%:

```bash
COVERAGE=$(go tool cover -func=coverage.out | tail -1 | awk '{print $3}' | tr -d '%')
if (( $(echo "$COVERAGE < 80" | bc -l) )); then
    exit 1
fi
```

## Error Semantics

Understanding which errors are returned in different scenarios:

| Scenario | Error | How to Check |
|----------|-------|--------------|
| Context cancelled during any wait | `context.Canceled` | `errors.Is(err, context.Canceled)` |
| Context timeout elapsed | `context.DeadlineExceeded` | `errors.Is(err, context.DeadlineExceeded)` |
| Telegram returned 429 | `sender.ErrTooManyRequests` | `errors.Is(err, sender.ErrTooManyRequests)` |
| Circuit breaker is open | `sender.ErrCircuitOpen` | `errors.Is(err, sender.ErrCircuitOpen)` |
| Max retries exhausted | `sender.ErrMaxRetries` | `errors.Is(err, sender.ErrMaxRetries)` |
| Bot blocked by user | `sender.ErrBotBlocked` | `errors.Is(err, sender.ErrBotBlocked)` |
| Chat not found | `sender.ErrChatNotFound` | `errors.Is(err, sender.ErrChatNotFound)` |

Note: Context errors (`context.Canceled`, `context.DeadlineExceeded`) are returned directly without wrapping, making them easy to check with `errors.Is()`.

## Best Practices

1. **Use table-driven tests** for testing multiple scenarios
2. **Always clean up** - use `t.Cleanup()` or `defer`
3. **Test error paths** - not just happy paths
4. **Use FakeSleeper** for retry tests to avoid slow tests
5. **Use NewRetryTestClient** for retry tests to prevent circuit breaker interference
6. **Verify request content** with capture assertions
7. **Use meaningful test names** that describe the scenario
8. **Keep tests independent** - each test should set up its own server
9. **Test edge cases** - empty responses, large payloads, timeouts

## Fuzzing

Fuzz tests are in `tg/fuzz_test.go`:

```bash
# Run fuzz tests
make test-fuzz

# Or manually
go test -fuzz=FuzzDecodeUpdate -fuzztime=30s ./tg/
```

## Troubleshooting

### Test Timeout

If a test hangs, it's usually due to:
- Missing mock handler (default handler returns success but may not match expected response)
- Infinite retry loop
- Deadlock in concurrent code

Add `-timeout` flag:
```bash
go test -timeout 30s ./...
```

### Race Conditions

Run with race detector:
```bash
go test -race ./...
```

### Flaky Tests

- Use `FakeSleeper` instead of real time
- Use atomic operations for counters in handlers
- Avoid `time.Sleep` in tests when possible

## Acceptance Tests (galigo-testbot)

The testbot validates API methods against the real Telegram Bot API. It sends actual messages, uploads files, and verifies responses. All created messages are cleaned up after each scenario.

### Prerequisites

1. A Telegram bot token (from [@BotFather](https://t.me/BotFather))
2. A chat ID where the bot can send messages (send `/start` to your bot first)

### Configuration

Set environment variables or create a `.env` file in the project root:

```bash
# Required
TESTBOT_TOKEN=123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11
TESTBOT_CHAT_ID=123456789
TESTBOT_ADMINS=123456789          # Comma-separated human user IDs (required for sticker tests)

# Optional
TESTBOT_MODE=polling              # polling (default) or webhook
TESTBOT_SEND_INTERVAL=1s          # Delay between API calls (default: 1s)
TESTBOT_MAX_MESSAGES=100          # Max messages per run (default: 100)
TESTBOT_STORAGE_DIR=var/reports   # Report output directory
TESTBOT_LOG_LEVEL=info            # info or debug
```

### Running Acceptance Tests

```bash
# Run all scenarios (Phase A + B + C + D + E, excludes interactive/webhook/checklists)
go run ./cmd/galigo-testbot --run all

# Run by phase
go run ./cmd/galigo-testbot --run core        # Phase A: core messaging (S0-S5)
go run ./cmd/galigo-testbot --run media       # Phase B: media uploads (S6-S11)
go run ./cmd/galigo-testbot --run keyboards   # Phase C: keyboards (S10)
go run ./cmd/galigo-testbot --run chat-admin  # Phase D: chat administration (S15-S19)
go run ./cmd/galigo-testbot --run stickers    # Phase E: sticker lifecycle (S20)
go run ./cmd/galigo-testbot --run stars       # Phase E: star balance + transactions (S21-S22)
go run ./cmd/galigo-testbot --run gifts       # Phase E: gift catalog (S23)
go run ./cmd/galigo-testbot --run checklists  # Phase E: checklist lifecycle (S24, requires Premium)

# Run individual scenarios
go run ./cmd/galigo-testbot --run smoke            # S0: Quick sanity check
go run ./cmd/galigo-testbot --run identity         # S1: Bot identity
go run ./cmd/galigo-testbot --run messages         # S2: Send, edit, delete
go run ./cmd/galigo-testbot --run forward          # S4: Forward and copy
go run ./cmd/galigo-testbot --run actions          # S5: Chat actions
go run ./cmd/galigo-testbot --run media-uploads    # S6: Photo, document, animation, audio, voice
go run ./cmd/galigo-testbot --run media-groups     # S7: Albums
go run ./cmd/galigo-testbot --run edit-media       # S8: Edit captions
go run ./cmd/galigo-testbot --run get-file             # S9: File download info
go run ./cmd/galigo-testbot --run edit-message-media   # S11: Edit message media
go run ./cmd/galigo-testbot --run inline-keyboard      # S10: Inline keyboard + edit markup
go run ./cmd/galigo-testbot --run chat-info            # S15: Chat info
go run ./cmd/galigo-testbot --run chat-settings        # S16: Chat title/description
go run ./cmd/galigo-testbot --run pin-messages         # S17: Pin/unpin
go run ./cmd/galigo-testbot --run polls                # S18: Polls
go run ./cmd/galigo-testbot --run forum-stickers       # S19: Forum stickers
go run ./cmd/galigo-testbot --run sticker-lifecycle    # S20: Full sticker lifecycle
go run ./cmd/galigo-testbot --run star-balance         # S21: Star balance
go run ./cmd/galigo-testbot --run invoice              # S22: Invoice

# Interactive scenarios (requires user interaction, excluded from "all")
go run ./cmd/galigo-testbot --run interactive           # S12: Callback query (click button)

# Webhook scenarios (excluded from "all", may disrupt active webhooks)
go run ./cmd/galigo-testbot --run webhook               # S13+S14: All webhook tests
go run ./cmd/galigo-testbot --run webhook-lifecycle      # S13: Set, verify, delete webhook
go run ./cmd/galigo-testbot --run get-updates            # S14: Non-blocking getUpdates

# Extras scenarios (S25-S32)
go run ./cmd/galigo-testbot --run extras                 # All extras tests
go run ./cmd/galigo-testbot --run geo                    # S25: Location
go run ./cmd/galigo-testbot --run venue                  # S26: Venue
go run ./cmd/galigo-testbot --run contact-dice           # S27: Contact + Dice
go run ./cmd/galigo-testbot --run bulk                   # S28: Bulk operations
go run ./cmd/galigo-testbot --run reactions              # S29: Reactions
go run ./cmd/galigo-testbot --run user-info              # S30: User profile photos + boosts
go run ./cmd/galigo-testbot --run chat-photo             # S31: Chat photo lifecycle
go run ./cmd/galigo-testbot --run chat-permissions       # S32: Chat permissions lifecycle

# Show method coverage
go run ./cmd/galigo-testbot --status
```

### Interactive Mode

Run without `--run` to start interactive mode. The bot listens for Telegram commands:

```
/run <suite>  - Run a test suite
/status       - Show method coverage
/help         - Show available commands
```

### Test Phases

#### Phase A: Core Messaging (S0-S5)

| Scenario | Methods | Description |
|----------|---------|-------------|
| S0-Smoke | getMe, sendMessage, deleteMessage | Quick sanity check |
| S1-Identity | getMe | Verify bot identity |
| S2-MessageLifecycle | sendMessage, editMessageText, deleteMessage | Full message lifecycle |
| S4-ForwardCopy | sendMessage, forwardMessage, copyMessage | Forward and copy operations |
| S5-ChatAction | sendChatAction | Typing indicators |

#### Phase B: Media (S6-S9)

| Scenario | Methods | Description |
|----------|---------|-------------|
| S6-MediaUploads | sendPhoto, sendDocument, sendAnimation, sendAudio, sendVoice, sendVideo, sendSticker, sendVideoNote | File upload via multipart |
| S7-MediaGroups | sendMediaGroup | Album with multiple documents |
| S8-EditMedia | sendPhoto, editMessageCaption | Caption editing |
| S9-GetFile | sendDocument, getFile | File metadata retrieval |
| S11-EditMessageMedia | sendPhoto, editMessageMedia | Replace media content (photo → document) |

#### Phase C: Keyboards (S10+)

| Scenario | Methods | Description |
|----------|---------|-------------|
| S10-InlineKeyboard | sendMessage (with markup), editMessageReplyMarkup | Send, edit, and remove inline keyboard |

#### Phase D: Chat Administration (S15-S19)

| Scenario | Methods | Description |
|----------|---------|-------------|
| S15-ChatInfo | getChat, getChatAdministrators, getChatMemberCount, getChatMember | Chat info retrieval |
| S16-ChatSettings | setChatTitle, setChatDescription | Set and restore chat title/description |
| S17-PinMessages | pinChatMessage, unpinChatMessage, unpinAllChatMessages | Pin/unpin operations |
| S18-Polls | sendPoll, stopPoll | Simple poll, quiz poll, stop poll |
| S19-ForumStickers | getForumTopicIconStickers | Forum topic icon stickers |

Requires a supergroup chat. S16 uses `isNotModifiedErr` to handle idempotent "not modified" 400 errors.

#### Phase E: Extended (S20-S24)

| Scenario | Methods | Description |
|----------|---------|-------------|
| S20-StickerLifecycle | createNewStickerSet, getStickerSet, addStickerToSet, setStickerPositionInSet, setStickerEmojiList, setStickerSetTitle, deleteStickerFromSet, deleteStickerSet | Full sticker set lifecycle |
| S21-StarBalance | getMyStarBalance, getStarTransactions | Star balance and transactions |
| S22-Invoice | sendInvoice | Send a star invoice |
| S23-Gifts | getAvailableGifts | Gift catalog |
| S24-Checklists | sendChecklist, editChecklist | Checklist lifecycle (requires Premium) |

S20 requires `TESTBOT_ADMINS` (human user_id for `createNewStickerSet`). S24 requires Telegram Premium and is excluded from `--run all`.

#### Phase F: Extras (S25-S32)

| Scenario | Methods | Description |
|----------|---------|-------------|
| S25-GeoLocation | sendLocation | GPS location |
| S26-GeoVenue | sendVenue | Venue with title and address |
| S27-ContactAndDice | sendContact, sendDice | Phone contact and animated dice |
| S28-BulkOps | forwardMessages, copyMessages, deleteMessages | Bulk message operations |
| S29-Reactions | setMessageReaction | Emoji reactions on messages |
| S30-UserInfo | getUserProfilePhotos, getUserChatBoosts | User profile and boost info |
| S31-ChatPhotoLifecycle | setChatPhoto, deleteChatPhoto | Chat photo save/restore with FromFileID |
| S32-ChatPermissionsLifecycle | setChatPermissions | Permissions save/restore using tg.AllPermissions() |

S31-S32 require admin with `can_change_info` / `can_restrict_members` permissions. Uses `SkipError` framework for graceful prerequisite handling.

#### Interactive (opt-in, excluded from `--run all`)

| Scenario | Methods | Description |
|----------|---------|-------------|
| S12-CallbackQuery | sendMessage, answerCallbackQuery | Send inline keyboard, wait for user click, answer callback |

Interactive scenarios require a human to click buttons in the chat. They are excluded from `--run all` to keep CI pipelines non-interactive. Run explicitly with `--run interactive`.

#### Webhook (opt-in, excluded from `--run all`)

| Scenario | Methods | Description |
|----------|---------|-------------|
| S13-WebhookLifecycle | setWebhook, getWebhookInfo, deleteWebhook | Backup → set → verify → delete → verify → restore |
| S14-GetUpdates | getUpdates | Non-blocking getUpdates call (timeout=0) |

Webhook scenarios are excluded from `--run all` to avoid disrupting production webhooks. Run explicitly with `--run webhook`.

### Method Coverage

Current registry: **64 target methods** across 4 categories (messaging, chat-admin, extended, legacy).

All 64 methods have acceptance test coverage. Checklists (S24) require Premium and are excluded from `--run all` but available via `--run checklists`.

### Test Fixtures

All media fixtures are embedded via `go:embed` in `cmd/galigo-testbot/fixtures/`. Generated with pure Go standard library (no external dependencies like ffmpeg):

| File | Format | Size | Description |
|------|--------|------|-------------|
| `photo.jpg` | JPEG | 791B | 100x100 red square (`image/jpeg`) |
| `animation.gif` | GIF | 317B | 100x100 2-frame red/blue (`image/gif`) |
| `sticker.png` | PNG | 1.9KB | 512x512 color gradient (`image/png`) |
| `audio.mp3` | MP3 | 2.1KB | 5 silent MPEG1 Layer3 frames (raw bytes) |
| `voice.ogg` | OGG Opus | 124B | 3 OGG pages with 1 silent Opus frame (raw bytes) |
| `video.mp4` | MP4/H.264 | 663B | 320x240 single black frame (minimal ftyp+moov+mdat) |
| `videonote.mp4` | MP4/H.264 | 663B | 240x240 square single black frame (for video notes) |

### Evidence Reports

Each test run generates a JSON report in `var/reports/`:

```json
{
  "run_id": "20260127-153819",
  "start_time": "2026-01-27T15:38:19Z",
  "success": true,
  "scenarios": [
    {
      "scenario_name": "S0-Smoke",
      "covers": ["getMe", "sendMessage", "deleteMessage"],
      "success": true,
      "duration": "3.755s",
      "steps": [
        {
          "step_name": "getMe",
          "method": "getMe",
          "success": true,
          "duration": "83ms",
          "evidence": {"username": "my_bot", "id": 123456789}
        }
      ]
    }
  ]
}
```

### Testbot Architecture

```
cmd/galigo-testbot/
├── main.go         # CLI entry, flag parsing, suite dispatch
├── engine/
│   ├── scenario.go      # Scenario, Step, Runtime (AdminUserID, ChatCtx), SenderClient interface
│   ├── steps.go         # Core step implementations (GetMeStep, SendPhotoStep, etc.)
│   ├── steps_chat_admin.go # Chat admin steps (GetChat, SetChatTitle, Pin, Polls, Forum)
│   ├── steps_extended.go   # Extended steps (Stickers, Stars, Gifts, Checklists)
│   ├── steps_geo.go        # Geo steps (SendLocation, SendVenue, SendContact)
│   ├── steps_misc.go       # Misc steps (SendDice, SetMessageReaction, GetUserProfilePhotos, GetUserChatBoosts)
│   ├── steps_bulk.go       # Bulk steps (SeedMessages, ForwardMessages, CopyMessages, DeleteMessages)
│   ├── steps_chat_settings.go # Chat settings (SetChatPhoto, SetChatPermissions with save/restore)
│   ├── errors.go        # SkipError for graceful prerequisite handling
│   ├── require.go       # RequireAdmin, RequireCanChangeInfo, RequireCanRestrict, etc.
│   ├── fixtures.go      # MinimalPNG inline fixture for chat photo tests
│   ├── runner.go        # Scenario executor with timing, error handling, skip support
│   └── adapter.go       # SenderAdapter: wraps sender.Client to SenderClient interface
├── suites/
│   ├── tier1.go       # Phase A scenarios (S0-S5)
│   ├── media.go       # Phase B scenarios (S6-S11)
│   ├── keyboards.go   # Phase C scenarios (S10+)
│   ├── chat_admin.go  # Phase D scenarios (S15-S19)
│   ├── stickers.go    # Phase E: Sticker lifecycle (S20)
│   ├── stars.go       # Phase E: Stars + Invoice (S21-S22)
│   ├── gifts.go       # Phase E: Gifts (S23)
│   ├── checklists.go  # Phase E: Checklists (S24, Premium)
│   ├── extras.go      # Phase F: Extras (S25-S32: geo, bulk, reactions, user info, chat settings)
│   ├── interactive.go # Interactive scenarios (S12, opt-in)
│   └── webhook.go     # Webhook scenarios (S13-S14, opt-in)
├── fixtures/
│   ├── fixtures.go # go:embed declarations and accessor functions
│   ├── photo.jpg, animation.gif, sticker.png, audio.mp3, voice.ogg
├── config/         # Environment variable loading + .env parser
├── evidence/       # JSON report generation and formatting
├── registry/       # Target method list and coverage checking (64 methods)
└── cleanup/        # Message cleanup utilities
```

### Adding a New Acceptance Test Scenario

1. **Add step** in `engine/steps.go`:
   ```go
   type SendLocationStep struct {
       Latitude, Longitude float64
   }
   func (s *SendLocationStep) Name() string { return "sendLocation" }
   func (s *SendLocationStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
       // Call rt.Sender, track message, return evidence
   }
   ```

2. **Add interface method** in `engine/scenario.go` `SenderClient` interface

3. **Add adapter** in `engine/adapter.go`

4. **Create scenario** in `suites/`:
   ```go
   func S10_Location() engine.Scenario {
       return &engine.BaseScenario{
           ScenarioName:   "S10_Location",
           CoveredMethods: []string{"sendLocation"},
           ScenarioSteps:  []engine.Step{&engine.SendLocationStep{...}, &engine.CleanupStep{}},
       }
   }
   ```

5. **Register suite** in `main.go` switch cases and help text

6. **Add target method** in `registry/registry.go` if not already listed

### Adding a New Test Fixture

For formats with Go stdlib support (image/jpeg, image/gif, image/png):
1. Write a generator script (see project history for examples)
2. Run it to generate the binary file
3. Place in `cmd/galigo-testbot/fixtures/`
4. Add `//go:embed` directive and accessor functions in `fixtures.go`

For binary formats without Go stdlib support (MP3, OGG), construct minimal valid files using raw byte sequences by studying the format specification to find the smallest valid structure.
