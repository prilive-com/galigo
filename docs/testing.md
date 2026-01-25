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

    // Create client with mock server URL
    client, err := sender.New(testutil.TestToken, sender.WithBaseURL(server.BaseURL()))
    require.NoError(t, err)
    defer client.Close()

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

### syncutil.Go

Helper for spawning goroutines tracked by WaitGroup:

```go
import "github.com/prilive-com/galigo/internal/syncutil"

var wg sync.WaitGroup

// Instead of:
wg.Add(1)
go func() {
    defer wg.Done()
    doWork()
}()

// Use:
syncutil.Go(&wg, func() {
    doWork()
})

wg.Wait()
```

## Test Patterns

### Testing Retry Logic

```go
func TestRetry_429WithRetryAfter(t *testing.T) {
    var attempts int32

    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        if atomic.AddInt32(&attempts, 1) == 1 {
            testutil.ReplyRateLimit(w, 2)  // First: rate limit
            return
        }
        testutil.ReplyMessage(w, 123)  // Second: success
    })

    sleeper := &testutil.FakeSleeper{}
    client := newTestClient(t, server.BaseURL(), sleeper)

    msg, err := client.SendMessage(ctx, req)

    require.NoError(t, err)
    assert.Equal(t, int32(2), atomic.LoadInt32(&attempts))
    assert.Equal(t, 2*time.Second, sleeper.LastCall())  // Used retry_after
}
```

### Testing Rate Limiting

```go
func TestRateLimit_Throttles(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyMessage(w, 123)
    })

    client := newTestClient(t, server.BaseURL(), sender.WithRateLimit(2, 1))  // 2 RPS

    start := time.Now()
    for i := 0; i < 3; i++ {
        client.SendMessage(ctx, req)
    }
    elapsed := time.Since(start)

    assert.GreaterOrEqual(t, elapsed, 500*time.Millisecond)
}
```

### Testing Error Handling

```go
func TestSendMessage_BotBlocked(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyForbidden(w, "bot was blocked by the user")
    })

    client := newTestClient(t, server.BaseURL())

    _, err := client.SendMessage(ctx, req)

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

    client := newTestClient(t, server.BaseURL())

    ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
    defer cancel()

    _, err := client.SendMessage(ctx, req)

    assert.ErrorIs(t, err, context.DeadlineExceeded)
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

            client := newTestClient(t, server.BaseURL())
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
| `internal/resilience/` | 80% | Medium |
| `internal/httpclient/` | 75% | Medium |
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

## Best Practices

1. **Use table-driven tests** for testing multiple scenarios
2. **Always clean up** - use `t.Cleanup()` or `defer`
3. **Test error paths** - not just happy paths
4. **Use FakeSleeper** for retry tests to avoid slow tests
5. **Verify request content** with capture assertions
6. **Use meaningful test names** that describe the scenario
7. **Keep tests independent** - each test should set up its own server
8. **Test edge cases** - empty responses, large payloads, timeouts

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
