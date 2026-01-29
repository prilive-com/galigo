# galigo Testbot Coverage Completion Plan v2.0

## From 76% (19/25) to 100% (25/25) Method Coverage

**Version:** 2.0 (Consolidated from dual analyses)  
**Current State:** 19/25 methods covered (76%)  
**Target:** 25/25 methods covered (100%)  
**Go Version:** 1.25.6 (stable)  
**Estimated Effort:** 15-20 hours (5 PRs)

---

## Executive Summary

This plan completes testbot coverage with these key principles:

1. **Don't fake coverage** - Use instrumentation for receiver-side methods, not risky wrappers
2. **Suite separation** - polling / webhook / interactive are mutually exclusive runs
3. **Safety rails** - Store/restore webhook state, breaker isolation, deterministic fixtures
4. **Telegram API compliance** - Respect getUpdates/webhook mutual exclusivity

---

## Current State Analysis

### ✅ Already Covered (19 methods)

| Category | Methods |
|----------|---------|
| Core | getMe, sendMessage, editMessageText, deleteMessage |
| Media | sendPhoto, sendDocument, sendVideo, sendAudio, sendAnimation, sendVoice, sendVideoNote, sendSticker |
| Albums | sendMediaGroup |
| Files | getFile, downloadFile |
| Forward/Copy | forwardMessage, copyMessage |
| Actions | sendChatAction |
| Markup | editMessageReplyMarkup |

### ❌ Missing (6 methods)

| Method | Category | Challenge | Solution |
|--------|----------|-----------|----------|
| **editMessageMedia** | Tier1 | Not in sender | Add to sender + S10 scenario |
| **answerCallbackQuery** | Tier1 | Needs real callback_query_id | S11 interactive scenario |
| **getUpdates** | Legacy | Internal to receiver | **Instrumentation** (not wrapper) |
| **setWebhook** | Legacy | Mutually exclusive with polling | S12 webhook suite |
| **deleteWebhook** | Legacy | Part of webhook lifecycle | S12 webhook suite |
| **getWebhookInfo** | Legacy | Part of webhook lifecycle | S12 webhook suite |

---

## Architecture: Three Suites

```
┌─────────────────────────────────────────────────────────────┐
│  Suite: polling (default)                                    │
│  Runs: S1-S10 (all sender methods + getUpdates via tracer)  │
│  Mode: Normal polling receiver                               │
│  Coverage: 21/25 methods                                     │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  Suite: interactive (opt-in, requires human)                 │
│  Runs: S11_CallbackQuery                                     │
│  Mode: Polling + human clicks inline button                  │
│  Coverage: +1 (answerCallbackQuery)                          │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  Suite: webhook (separate run, mutually exclusive)           │
│  Runs: S12_WebhookLifecycle                                  │
│  Mode: Temporarily sets webhook, then restores               │
│  Coverage: +3 (setWebhook, getWebhookInfo, deleteWebhook)    │
└─────────────────────────────────────────────────────────────┘
```

**Important:** Per Telegram docs, getUpdates and webhooks are mutually exclusive. Never run webhook suite while polling is active.

---

## PR1: Coverage Instrumentation (No Behavior Changes)

**Goal:** Track receiver-side methods without adding risky sender wrappers  
**Complexity:** Low  
**Estimated Time:** 2-3 hours

### Why Not Add sender.GetUpdates()?

Adding a public `GetUpdates()` to sender would:
- Race with the receiver's internal offset tracking
- Potentially mess up update delivery
- Create confusion about which component owns offsets

**Better approach:** Instrument the receiver's internal calls.

### 1.1 Method Tracer Interface

**File:** `cmd/galigo-testbot/coverage/tracer.go`

```go
package coverage

import "sync"

// MethodTracer tracks which API methods have been invoked
type MethodTracer struct {
    mu      sync.RWMutex
    methods map[string]int // method -> call count
}

func NewMethodTracer() *MethodTracer {
    return &MethodTracer{
        methods: make(map[string]int),
    }
}

// Hit records a method invocation
func (t *MethodTracer) Hit(method string) {
    t.mu.Lock()
    t.methods[method]++
    t.mu.Unlock()
}

// Hits returns call count for a method
func (t *MethodTracer) Hits(method string) int {
    t.mu.RLock()
    defer t.mu.RUnlock()
    return t.methods[method]
}

// HitMethods returns all methods that were invoked
func (t *MethodTracer) HitMethods() []string {
    t.mu.RLock()
    defer t.mu.RUnlock()
    
    var methods []string
    for m := range t.methods {
        methods = append(methods, m)
    }
    return methods
}

// Reset clears all hits
func (t *MethodTracer) Reset() {
    t.mu.Lock()
    t.methods = make(map[string]int)
    t.mu.Unlock()
}
```

### 1.2 Wire Tracer into Sender

**File:** `sender/client.go` (modification)

```go
// Add to Client struct
type Client struct {
    // ... existing fields
    tracer MethodTracer // Optional tracer for coverage
}

// Add option
func WithMethodTracer(tracer MethodTracer) Option {
    return func(c *clientConfig) {
        c.tracer = tracer
    }
}

// In doRequest, after determining method name
func (c *Client) doRequest(ctx context.Context, method string, payload any) (*apiResponse, error) {
    // Track method hit
    if c.tracer != nil {
        c.tracer.Hit(method)
    }
    
    // ... rest of implementation
}
```

### 1.3 Wire Tracer into Receiver (for getUpdates)

**File:** `receiver/polling.go` (modification)

```go
// Add tracer to Receiver
type Receiver struct {
    // ... existing fields
    tracer MethodTracer
}

// In the polling loop where getUpdates is called
func (r *Receiver) poll(ctx context.Context) error {
    // Track getUpdates hit
    if r.tracer != nil {
        r.tracer.Hit("getUpdates")
    }
    
    // ... existing getUpdates call
}
```

### 1.4 Update Coverage Report

**File:** `cmd/galigo-testbot/registry/coverage.go`

```go
func CheckCoverage(scenarios []Scenario, tracer *coverage.MethodTracer) *CoverageReport {
    report := &CoverageReport{}
    
    // Collect from scenarios
    scenarioCovered := make(map[string]bool)
    for _, s := range scenarios {
        for _, method := range s.Covers() {
            scenarioCovered[method] = true
        }
    }
    
    // Collect from tracer (receiver-side methods)
    tracerCovered := make(map[string]bool)
    if tracer != nil {
        for _, method := range tracer.HitMethods() {
            tracerCovered[method] = true
        }
    }
    
    // Merge coverage
    for _, m := range AllMethods {
        if m.Skip {
            report.Skipped = append(report.Skipped, m.Name)
            continue
        }
        
        if scenarioCovered[m.Name] || tracerCovered[m.Name] {
            report.Covered = append(report.Covered, m.Name)
        } else {
            report.Missing = append(report.Missing, m.Name)
        }
    }
    
    return report
}
```

### Acceptance Criteria

- [ ] Running `--run polling` shows `getUpdates` as covered (via tracer)
- [ ] No new public sender methods for getUpdates
- [ ] Tracer is optional (doesn't break non-testbot usage)

---

## PR2: editMessageMedia (Sender + Scenario)

**Goal:** Reach 20/25 (80%) - biggest practical win  
**Complexity:** Medium  
**Estimated Time:** 3-4 hours

### Telegram API Requirements

From Bot API docs:
- `editMessageMedia` edits animation/audio/document/photo/video
- Can upload new file **only for non-inline messages**
- Inline messages can only use URL/file_id (no upload)

### 2.1 Add editMessageMedia to Sender

**File:** `sender/edit_media.go`

```go
package sender

import (
    "context"
    "fmt"
    
    "github.com/prilive-com/galigo/tg"
)

// EditMessageMediaRequest represents a request to edit message media
type EditMessageMediaRequest struct {
    // Required: either (ChatID + MessageID) or InlineMessageID
    ChatID          tg.ChatID `json:"chat_id,omitempty"`
    MessageID       int       `json:"message_id,omitempty"`
    InlineMessageID string    `json:"inline_message_id,omitempty"`
    
    // Required: new media content
    Media tg.InputMedia `json:"media"`
    
    // Optional
    ReplyMarkup *tg.InlineKeyboardMarkup `json:"reply_markup,omitempty"`
    
    // Business features (Bot API 9.x)
    BusinessConnectionID string `json:"business_connection_id,omitempty"`
}

// EditMessageMedia edits the media content of a message
func (c *Client) EditMessageMedia(ctx context.Context, req EditMessageMediaRequest) (*tg.Message, error) {
    // Validation
    hasInline := req.InlineMessageID != ""
    hasChat := !req.ChatID.IsZero() && req.MessageID != 0
    
    if !hasInline && !hasChat {
        return nil, fmt.Errorf("either (chat_id + message_id) or inline_message_id required")
    }
    if hasInline && hasChat {
        return nil, fmt.Errorf("cannot specify both inline_message_id and chat_id")
    }
    
    var msg tg.Message
    if err := c.doRequestResult(ctx, "editMessageMedia", req, &msg); err != nil {
        return nil, fmt.Errorf("editMessageMedia failed: %w", err)
    }
    return &msg, nil
}
```

### 2.2 Handle InputMedia in Multipart Builder

**File:** `sender/multipart.go` (addition)

```go
// handleEditMessageMediaRequest handles multipart encoding for editMessageMedia
func (b *MultipartRequestBuilder) handleEditMessageMediaRequest(req *EditMessageMediaRequest) error {
    // Standard fields
    if !req.ChatID.IsZero() {
        b.AddField("chat_id", req.ChatID.String())
    }
    if req.MessageID != 0 {
        b.AddField("message_id", strconv.Itoa(req.MessageID))
    }
    if req.InlineMessageID != "" {
        b.AddField("inline_message_id", req.InlineMessageID)
    }
    if req.BusinessConnectionID != "" {
        b.AddField("business_connection_id", req.BusinessConnectionID)
    }
    
    // Handle InputMedia with potential upload
    mediaJSON, attachments, err := b.prepareInputMedia(req.Media, "media0")
    if err != nil {
        return err
    }
    
    b.AddField("media", mediaJSON)
    
    for name, file := range attachments {
        b.AddFile(name, file.Filename, file.Reader)
        b.hasUploads = true
    }
    
    // Reply markup (JSON)
    if req.ReplyMarkup != nil {
        markupJSON, err := json.Marshal(req.ReplyMarkup)
        if err != nil {
            return err
        }
        b.AddField("reply_markup", string(markupJSON))
    }
    
    return nil
}

// prepareInputMedia converts InputMedia to JSON and extracts file attachments
func (b *MultipartRequestBuilder) prepareInputMedia(media tg.InputMedia, attachPrefix string) (string, map[string]FileAttachment, error) {
    attachments := make(map[string]FileAttachment)
    
    // Get the media InputFile
    mediaFile := media.GetMedia()
    
    if mediaFile.IsUpload() {
        // Need attach:// reference for upload
        attachName := attachPrefix
        attachments[attachName] = FileAttachment{
            Filename: mediaFile.Filename(),
            Reader:   mediaFile.Reader(),
        }
        
        // Modify media to use attach:// reference
        modifiedMedia := media.WithMedia(tg.FileAttach(attachName))
        mediaJSON, err := json.Marshal(modifiedMedia)
        return string(mediaJSON), attachments, err
    }
    
    // No upload - just JSON
    mediaJSON, err := json.Marshal(media)
    return string(mediaJSON), attachments, err
}
```

### 2.3 Unit Tests (3-Test Pattern)

**File:** `sender/edit_media_test.go`

```go
func TestEditMessageMedia_FileID_MinimalSuccess(t *testing.T) {
    server := testutil.NewMockServer(t, func(w http.ResponseWriter, r *http.Request) {
        // Should be JSON (no upload)
        assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
        testutil.ReplyMessage(w, 42)
    })
    
    client := testutil.NewTestClient(t, server.URL)
    
    msg, err := client.EditMessageMedia(context.Background(), sender.EditMessageMediaRequest{
        ChatID:    tg.ChatIDFromInt64(123),
        MessageID: 42,
        Media: tg.InputMediaPhoto{
            Type:  "photo",
            Media: tg.FileID("AgACAgIAAxk..."),
        },
    })
    
    require.NoError(t, err)
    assert.Equal(t, 42, msg.MessageID)
}

func TestEditMessageMedia_Upload_MinimalSuccess(t *testing.T) {
    server := testutil.NewMockServer(t, func(w http.ResponseWriter, r *http.Request) {
        // Should be multipart
        assert.True(t, strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data"))
        
        mp := testutil.ParseMultipart(t, r)
        mp.AssertFormFieldExists(t, "media")
        mp.AssertFilePart(t, "media0", "new_photo.jpg")
        
        // Verify media JSON contains attach://
        assert.Contains(t, mp.Fields["media"], "attach://media0")
        
        testutil.ReplyMessage(w, 42)
    })
    
    client := testutil.NewTestClient(t, server.URL)
    
    msg, err := client.EditMessageMedia(context.Background(), sender.EditMessageMediaRequest{
        ChatID:    tg.ChatIDFromInt64(123),
        MessageID: 42,
        Media: tg.InputMediaPhoto{
            Type:  "photo",
            Media: tg.FileFromReader("new_photo.jpg", strings.NewReader("photo bytes")),
        },
    })
    
    require.NoError(t, err)
    assert.Equal(t, 42, msg.MessageID)
}

func TestEditMessageMedia_TelegramError(t *testing.T) {
    server := testutil.NewMockServer(t, func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyError(w, 400, "Bad Request: message to edit not found")
    })
    
    client := testutil.NewTestClient(t, server.URL)
    
    _, err := client.EditMessageMedia(context.Background(), sender.EditMessageMediaRequest{
        ChatID:    tg.ChatIDFromInt64(123),
        MessageID: 999,
        Media: tg.InputMediaPhoto{
            Type:  "photo",
            Media: tg.FileID("AgACAgIAAxk..."),
        },
    })
    
    require.Error(t, err)
    var apiErr *tg.APIError
    require.ErrorAs(t, err, &apiErr)
    assert.Equal(t, 400, apiErr.Code)
}

func TestEditMessageMedia_Validation_NeitherChatNorInline(t *testing.T) {
    client := testutil.NewTestClient(t, "http://unused")
    
    _, err := client.EditMessageMedia(context.Background(), sender.EditMessageMediaRequest{
        Media: tg.InputMediaPhoto{Type: "photo", Media: tg.FileID("x")},
    })
    
    require.Error(t, err)
    assert.Contains(t, err.Error(), "either (chat_id + message_id) or inline_message_id required")
}
```

### 2.4 Testbot Scenario S10_EditMessageMedia

**File:** `cmd/galigo-testbot/suites/tier1.go`

```go
// S10_EditMessageMedia tests editing message media
func S10_EditMessageMedia(chatID int64) engine.Scenario {
    return &engine.BaseScenario{
        ScenarioName:    "S10_EditMessageMedia",
        ScenarioDesc:    "Test editing message media content (non-inline upload)",
        CoveredMethods:  []string{"sendDocument", "editMessageMedia"},
        ScenarioTimeout: 1 * time.Minute,
        ScenarioSteps: []engine.Step{
            // 1. Send initial document
            &engine.SendDocumentStep{
                ChatID:   chatID,
                Document: tg.FileFromReader("original.txt", fixtures.Document()),
                Caption:  "Original document - will be replaced",
            },
            
            // 2. Wait for Telegram to process
            &engine.SleepStep{Duration: time.Second},
            
            // 3. Edit media to different document (upload allowed for non-inline)
            &engine.EditMessageMediaStep{
                Media: tg.InputMediaDocument{
                    Type:    "document",
                    Media:   tg.FileFromReader("replaced.txt", fixtures.Document2()),
                    Caption: "Replaced via editMessageMedia",
                },
            },
            
            // 4. Verify and cleanup
            &engine.SleepStep{Duration: time.Second},
            &engine.CleanupStep{},
        },
    }
}
```

### Acceptance Criteria

- [ ] `editMessageMedia` added to sender with proper multipart handling
- [ ] Unit tests pass (3-test pattern + validation test)
- [ ] S10 scenario passes in testbot
- [ ] Coverage: 20/25 (80%)

---

## PR3: Interactive Callback Scenario (answerCallbackQuery)

**Goal:** Cover the interactive-only method cleanly  
**Complexity:** High (bridges sender and receiver)  
**Estimated Time:** 4-5 hours

### Design Principles

- **Opt-in only** - Excluded from `--run all`, requires `--run interactive`
- **Clear user prompts** - Tell admin exactly what to do
- **Timeout handling** - Don't hang forever waiting for human

### 3.1 Add CallbackChan to Runtime

**File:** `cmd/galigo-testbot/engine/scenario.go`

```go
// Runtime provides context for step execution
type Runtime struct {
    Bot       *galigo.Bot
    Config    *config.Config
    Logger    *slog.Logger
    Tracer    *coverage.MethodTracer
    
    // Receiver integration for interactive scenarios
    CallbackChan chan *tg.CallbackQuery
    
    // State shared between steps
    CreatedMessages []CreatedMessage
    LastMessage     *tg.Message
    LastCallback    *tg.CallbackQuery
    CapturedFileIDs map[string]string
}
```

### 3.2 Forward Callbacks in Update Handler

**File:** `cmd/galigo-testbot/main.go`

```go
func handleUpdate(rt *engine.Runtime, update *tg.Update) {
    // Forward callback queries to waiting scenarios
    if update.CallbackQuery != nil {
        select {
        case rt.CallbackChan <- update.CallbackQuery:
            rt.Logger.Debug("callback forwarded to scenario",
                "callback_id", update.CallbackQuery.ID,
                "data", update.CallbackQuery.Data)
        default:
            rt.Logger.Debug("callback channel full or no listener")
        }
        return // Don't process callback as command
    }
    
    // ... rest of update handling (commands, etc.)
}
```

### 3.3 Implement Steps

**File:** `cmd/galigo-testbot/engine/steps.go`

```go
// SendInlineKeyboardStep sends a message with inline keyboard
type SendInlineKeyboardStep struct {
    ChatID  int64
    Text    string
    Buttons [][]tg.InlineKeyboardButton
}

func (s *SendInlineKeyboardStep) Name() string { return "sendInlineKeyboard" }

func (s *SendInlineKeyboardStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    keyboard := tg.InlineKeyboardMarkup{InlineKeyboard: s.Buttons}
    
    msg, err := rt.Bot.SendMessage(ctx, tg.ChatIDFromInt64(s.ChatID), s.Text,
        sender.WithReplyMarkup(keyboard))
    if err != nil {
        return nil, err
    }
    
    rt.LastMessage = msg
    rt.CreatedMessages = append(rt.CreatedMessages, CreatedMessage{
        ChatID:    s.ChatID,
        MessageID: msg.MessageID,
    })
    
    return &StepResult{
        Method:     "sendMessage",
        MessageIDs: []int{msg.MessageID},
    }, nil
}

// WaitForCallbackStep waits for a callback query from admin
type WaitForCallbackStep struct {
    ExpectedData string        // Optional: specific callback_data to match
    Timeout      time.Duration // Default: 60s
}

func (s *WaitForCallbackStep) Name() string { return "waitForCallback" }

func (s *WaitForCallbackStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    timeout := s.Timeout
    if timeout == 0 {
        timeout = 60 * time.Second
    }
    
    rt.Logger.Info("⏳ Waiting for callback query",
        "expected_data", s.ExpectedData,
        "timeout", timeout)
    
    // Prompt admin
    promptMsg, _ := rt.Bot.SendMessage(ctx, tg.ChatIDFromInt64(rt.Config.ChatID),
        fmt.Sprintf("👆 Please click a button above within %v", timeout))
    if promptMsg != nil {
        rt.CreatedMessages = append(rt.CreatedMessages, CreatedMessage{
            ChatID:    rt.Config.ChatID,
            MessageID: promptMsg.MessageID,
        })
    }
    
    select {
    case callback := <-rt.CallbackChan:
        if s.ExpectedData != "" && callback.Data != s.ExpectedData {
            return nil, fmt.Errorf("callback data mismatch: got %q, want %q",
                callback.Data, s.ExpectedData)
        }
        
        rt.LastCallback = callback
        rt.Logger.Info("✅ Callback received",
            "callback_id", callback.ID,
            "data", callback.Data)
        
        return &StepResult{
            Method: "receiver", // Internal tracking
            Evidence: map[string]any{
                "callback_id": callback.ID,
                "data":        callback.Data,
            },
        }, nil
        
    case <-time.After(timeout):
        return nil, fmt.Errorf("timeout waiting for callback (%v) - please click the button", timeout)
        
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}

// AnswerCallbackQueryStep answers the last received callback
type AnswerCallbackQueryStep struct {
    Text      string
    ShowAlert bool
}

func (s *AnswerCallbackQueryStep) Name() string { return "answerCallbackQuery" }

func (s *AnswerCallbackQueryStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    if rt.LastCallback == nil {
        return nil, fmt.Errorf("no callback query to answer")
    }
    
    err := rt.Bot.AnswerCallbackQuery(ctx, rt.LastCallback.ID,
        sender.WithCallbackText(s.Text),
        sender.WithCallbackShowAlert(s.ShowAlert))
    if err != nil {
        return nil, err
    }
    
    rt.Logger.Info("✅ Callback answered", "text", s.Text)
    
    return &StepResult{
        Method: "answerCallbackQuery",
        Evidence: map[string]any{
            "callback_id": rt.LastCallback.ID,
            "text":        s.Text,
        },
    }, nil
}
```

### 3.4 Create S11_CallbackQuery Scenario

**File:** `cmd/galigo-testbot/suites/interactive.go`

```go
package suites

// S11_CallbackQuery tests callback query handling
// INTERACTIVE: Requires human to click button
func S11_CallbackQuery(chatID int64) engine.Scenario {
    return &engine.BaseScenario{
        ScenarioName:    "S11_CallbackQuery",
        ScenarioDesc:    "Test inline keyboard + callback query (INTERACTIVE)",
        CoveredMethods:  []string{"sendMessage", "answerCallbackQuery"},
        ScenarioTimeout: 2 * time.Minute,
        Interactive:     true, // Mark as interactive
        ScenarioSteps: []engine.Step{
            // 1. Send message with inline keyboard
            &engine.SendInlineKeyboardStep{
                ChatID: chatID,
                Text:   "🧪 **galigo Callback Test**\n\nPlease click a button to continue the test:",
                Buttons: [][]tg.InlineKeyboardButton{
                    {
                        {Text: "✅ Confirm", CallbackData: "test_confirm"},
                        {Text: "❌ Cancel", CallbackData: "test_cancel"},
                    },
                    {
                        {Text: "ℹ️ Info", CallbackData: "test_info"},
                    },
                },
            },
            
            // 2. Wait for admin to click (any button)
            &engine.WaitForCallbackStep{
                Timeout: 60 * time.Second,
            },
            
            // 3. Answer the callback
            &engine.AnswerCallbackQueryStep{
                Text:      "✅ Test passed! Callback received and answered.",
                ShowAlert: false,
            },
            
            // 4. Edit message to show completion
            &engine.EditMessageTextStep{
                Text: "✅ Callback test completed successfully!\n\nYou clicked: {{.CallbackData}}",
            },
            
            // 5. Cleanup
            &engine.CleanupStep{},
        },
    }
}

// InteractiveScenarios returns scenarios that require human interaction
func InteractiveScenarios(chatID int64) []engine.Scenario {
    return []engine.Scenario{
        S11_CallbackQuery(chatID),
    }
}
```

### 3.5 Register Interactive Suite

**File:** `cmd/galigo-testbot/main.go`

```go
func handleRun(ctx context.Context, args string) {
    switch args {
    case "polling", "tier1", "":
        // Default: all non-interactive scenarios
        scenarios := suites.PollingScenarios(cfg.ChatID)
        runScenarios(ctx, scenarios)
        
    case "interactive":
        // Interactive scenarios (opt-in)
        scenarios := suites.InteractiveScenarios(cfg.ChatID)
        fmt.Println("⚠️  Interactive mode: You will need to click buttons!")
        runScenarios(ctx, scenarios)
        
    case "webhook":
        // Webhook scenarios (separate run)
        // ...
        
    case "all":
        // All NON-interactive scenarios
        scenarios := suites.AllNonInteractiveScenarios(cfg.ChatID)
        runScenarios(ctx, scenarios)
        fmt.Println("\n💡 Tip: Run --run interactive separately to test answerCallbackQuery")
    }
}
```

### Acceptance Criteria

- [ ] S11 scenario works when human clicks button
- [ ] Timeout is clear and actionable
- [ ] Interactive scenarios excluded from `--run all`
- [ ] Coverage: 21/25 (with answerCallbackQuery)

---

## PR4: Webhook Suite with Safety Rails

**Goal:** Cover webhook management methods safely  
**Complexity:** Medium  
**Estimated Time:** 3-4 hours

### Design Principles

1. **Mutual exclusivity** - Never run while polling is active
2. **Store/restore** - Save previous webhook state, restore on exit
3. **Safety switch** - Require `--allow-webhook-mutations` flag
4. **Infra flexibility** - Support both public HTTPS and local Bot API server

### 4.1 Webhook Configuration

**File:** `cmd/galigo-testbot/config/config.go`

```go
type Config struct {
    // ... existing fields
    
    // Webhook suite config
    WebhookURL              string // Required for webhook suite
    WebhookSecretToken      string // For X-Telegram-Bot-Api-Secret-Token
    WebhookListenAddr       string // Local server address (e.g., ":8443")
    AllowWebhookMutations   bool   // Safety switch
    
    // Infra mode
    UseLocalBotAPI          bool   // If true, use local telegram-bot-api server
    LocalBotAPIURL          string // e.g., "http://localhost:8081"
}
```

### 4.2 Add Webhook Methods to Sender

**File:** `sender/webhook.go`

```go
package sender

// SetWebhookRequest matches Telegram Bot API
type SetWebhookRequest struct {
    URL                string      `json:"url"`
    Certificate        tg.InputFile `json:"certificate,omitempty"`
    IPAddress          string      `json:"ip_address,omitempty"`
    MaxConnections     int         `json:"max_connections,omitempty"`
    AllowedUpdates     []string    `json:"allowed_updates,omitempty"`
    DropPendingUpdates bool        `json:"drop_pending_updates,omitempty"`
    SecretToken        string      `json:"secret_token,omitempty"`
}

func (c *Client) SetWebhook(ctx context.Context, req SetWebhookRequest) error {
    var result bool
    if err := c.doRequestResult(ctx, "setWebhook", req, &result); err != nil {
        return fmt.Errorf("setWebhook failed: %w", err)
    }
    if !result {
        return fmt.Errorf("setWebhook returned false")
    }
    return nil
}

func (c *Client) DeleteWebhook(ctx context.Context, dropPending bool) error {
    req := struct {
        DropPendingUpdates bool `json:"drop_pending_updates,omitempty"`
    }{DropPendingUpdates: dropPending}
    
    var result bool
    if err := c.doRequestResult(ctx, "deleteWebhook", req, &result); err != nil {
        return fmt.Errorf("deleteWebhook failed: %w", err)
    }
    return nil
}

type WebhookInfo struct {
    URL                          string   `json:"url"`
    HasCustomCertificate         bool     `json:"has_custom_certificate"`
    PendingUpdateCount           int      `json:"pending_update_count"`
    IPAddress                    string   `json:"ip_address,omitempty"`
    LastErrorDate                int64    `json:"last_error_date,omitempty"`
    LastErrorMessage             string   `json:"last_error_message,omitempty"`
    LastSynchronizationErrorDate int64    `json:"last_synchronization_error_date,omitempty"`
    MaxConnections               int      `json:"max_connections,omitempty"`
    AllowedUpdates               []string `json:"allowed_updates,omitempty"`
}

func (c *Client) GetWebhookInfo(ctx context.Context) (*WebhookInfo, error) {
    var info WebhookInfo
    if err := c.doRequestResult(ctx, "getWebhookInfo", struct{}{}, &info); err != nil {
        return nil, fmt.Errorf("getWebhookInfo failed: %w", err)
    }
    return &info, nil
}
```

### 4.3 Webhook State Manager (Store/Restore)

**File:** `cmd/galigo-testbot/webhook/state.go`

```go
package webhook

import "context"

// StateManager handles webhook state backup/restore
type StateManager struct {
    bot           *galigo.Bot
    previousURL   string
    previousSet   bool
    logger        *slog.Logger
}

func NewStateManager(bot *galigo.Bot, logger *slog.Logger) *StateManager {
    return &StateManager{bot: bot, logger: logger}
}

// Backup saves current webhook state
func (m *StateManager) Backup(ctx context.Context) error {
    info, err := m.bot.GetWebhookInfo(ctx)
    if err != nil {
        return fmt.Errorf("failed to get webhook info: %w", err)
    }
    
    m.previousURL = info.URL
    m.previousSet = info.URL != ""
    
    m.logger.Info("webhook state backed up",
        "previous_url", m.previousURL,
        "was_set", m.previousSet)
    
    return nil
}

// Restore restores previous webhook state
func (m *StateManager) Restore(ctx context.Context) error {
    if !m.previousSet {
        // Was not set before - delete webhook
        m.logger.Info("restoring: deleting webhook (was not set)")
        return m.bot.DeleteWebhook(ctx, false)
    }
    
    // Was set - restore previous URL
    // Note: We can't restore secret_token (not returned by getWebhookInfo)
    m.logger.Info("restoring: setting webhook to previous URL", "url", m.previousURL)
    return m.bot.SetWebhook(ctx, sender.SetWebhookRequest{
        URL: m.previousURL,
    })
}
```

### 4.4 Create S12_WebhookLifecycle Scenario

**File:** `cmd/galigo-testbot/suites/webhook.go`

```go
package suites

// S12_WebhookLifecycle tests webhook management
// WARNING: Mutually exclusive with polling!
func S12_WebhookLifecycle(webhookURL, secretToken string) engine.Scenario {
    return &engine.BaseScenario{
        ScenarioName:    "S12_WebhookLifecycle",
        ScenarioDesc:    "Test webhook set/get/delete lifecycle",
        CoveredMethods:  []string{"setWebhook", "getWebhookInfo", "deleteWebhook"},
        ScenarioTimeout: 2 * time.Minute,
        RequiresWebhook: true,
        ScenarioSteps: []engine.Step{
            // 1. Start clean - delete any existing webhook
            &engine.DeleteWebhookStep{DropPending: true},
            
            // 2. Verify webhook is cleared
            &engine.GetWebhookInfoStep{
                ExpectedURL: "",
                Description: "verify webhook cleared",
            },
            
            // 3. Set new webhook
            &engine.SetWebhookStep{
                URL:         webhookURL,
                SecretToken: secretToken,
            },
            
            // 4. Verify webhook is set
            &engine.GetWebhookInfoStep{
                ExpectedURL: webhookURL,
                Description: "verify webhook set",
            },
            
            // 5. Optional: Send a message to generate update and verify delivery
            // (Only if we have a webhook server running)
            &engine.ConditionalStep{
                Condition: func(rt *engine.Runtime) bool {
                    return rt.Config.WebhookListenAddr != ""
                },
                Step: &engine.WebhookDeliveryTestStep{
                    SecretToken: secretToken,
                },
            },
            
            // 6. Delete webhook (cleanup)
            &engine.DeleteWebhookStep{DropPending: true},
            
            // 7. Verify cleanup
            &engine.GetWebhookInfoStep{
                ExpectedURL: "",
                Description: "verify webhook deleted",
            },
        },
    }
}
```

### 4.5 Webhook Suite Runner with Safety

**File:** `cmd/galigo-testbot/main.go`

```go
func runWebhookSuite(ctx context.Context, cfg *config.Config, bot *galigo.Bot) error {
    // Safety check
    if !cfg.AllowWebhookMutations {
        return fmt.Errorf("webhook mutations not allowed; set --allow-webhook-mutations=true")
    }
    
    if cfg.WebhookURL == "" {
        return fmt.Errorf("--webhook-url required for webhook suite")
    }
    
    // Create state manager
    stateMgr := webhook.NewStateManager(bot, slog.Default())
    
    // Backup current state
    if err := stateMgr.Backup(ctx); err != nil {
        return fmt.Errorf("failed to backup webhook state: %w", err)
    }
    
    // Ensure restore on exit
    defer func() {
        restoreCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        
        if err := stateMgr.Restore(restoreCtx); err != nil {
            slog.Error("failed to restore webhook state", "error", err)
        }
    }()
    
    // Run webhook scenarios
    scenarios := suites.WebhookScenarios(cfg.WebhookURL, cfg.WebhookSecretToken)
    return runScenarios(ctx, scenarios)
}
```

### Acceptance Criteria

- [ ] Webhook methods added to sender
- [ ] State backup/restore works
- [ ] S12 scenario passes with proper infra
- [ ] Safety switch prevents accidental mutations
- [ ] Coverage: 24/25 (all except internal getUpdates)

---

## PR5: Hardening (Non-Flaky Tests)

**Goal:** Make tests reliable in real-world conditions  
**Complexity:** Low  
**Estimated Time:** 2-3 hours

### 5.1 Breaker Isolation

**File:** `cmd/galigo-testbot/engine/runner.go`

```go
// Option 1: Disable breaker entirely in testbot
func createTestbotClient(token string) (*sender.Client, error) {
    return sender.New(token,
        sender.WithCircuitBreakerDisabled(),
        // OR: sender.WithCircuitBreakerSettings that never trips
    )
}

// Option 2: Fresh client per scenario
func (r *Runner) Run(ctx context.Context, scenario Scenario) *ScenarioResult {
    // Create fresh client to avoid breaker poisoning
    client, err := createTestbotClient(r.config.Token)
    if err != nil {
        return &ScenarioResult{Error: err.Error()}
    }
    
    r.runtime.Bot = galigo.NewWithSender(client)
    
    // ... run scenario
}
```

### 5.2 Deterministic Fixtures

**File:** `cmd/galigo-testbot/fixtures/fixtures.go`

```go
package fixtures

import (
    "bytes"
    "embed"
)

//go:embed photo.jpg document.txt document2.txt video.mp4 audio.mp3
var content embed.FS

// Use real files, not base64 strings
func Photo() *bytes.Reader {
    data, _ := content.ReadFile("photo.jpg")
    return bytes.NewReader(data)
}

func Document() *bytes.Reader {
    data, _ := content.ReadFile("document.txt")
    return bytes.NewReader(data)
}

func Document2() *bytes.Reader {
    data, _ := content.ReadFile("document2.txt")
    return bytes.NewReader(data)
}

// Stable captions for assertions
const (
    PhotoCaption    = "galigo test photo"
    DocumentCaption = "galigo test document"
)
```

### 5.3 Per-Scenario Timeouts and Retries

**File:** `cmd/galigo-testbot/engine/runner.go`

```go
func (r *Runner) runStep(ctx context.Context, step Step) (*StepResult, error) {
    // Per-step timeout
    stepTimeout := 30 * time.Second
    if ts, ok := step.(TimeoutStep); ok {
        stepTimeout = ts.Timeout()
    }
    
    stepCtx, cancel := context.WithTimeout(ctx, stepTimeout)
    defer cancel()
    
    // Retry transient errors (network, 5xx) but NOT logic bugs (4xx)
    var result *StepResult
    var err error
    
    for attempt := 0; attempt < 3; attempt++ {
        result, err = step.Execute(stepCtx, r.runtime)
        
        if err == nil {
            return result, nil
        }
        
        // Don't retry 4xx errors (logic bugs)
        if is4xxError(err) {
            return result, err
        }
        
        // Retry transient errors
        if isTransientError(err) && attempt < 2 {
            r.logger.Warn("transient error, retrying",
                "step", step.Name(),
                "attempt", attempt+1,
                "error", err)
            time.Sleep(time.Duration(attempt+1) * time.Second)
            continue
        }
        
        break
    }
    
    return result, err
}

func isTransientError(err error) bool {
    // Network errors, 5xx, 429
    var apiErr *tg.APIError
    if errors.As(err, &apiErr) {
        return apiErr.Code >= 500 || apiErr.Code == 429
    }
    return errors.Is(err, context.DeadlineExceeded) ||
           strings.Contains(err.Error(), "connection")
}

func is4xxError(err error) bool {
    var apiErr *tg.APIError
    if errors.As(err, &apiErr) {
        return apiErr.Code >= 400 && apiErr.Code < 500 && apiErr.Code != 429
    }
    return false
}
```

### Acceptance Criteria

- [ ] Breaker doesn't poison subsequent scenarios
- [ ] Fixtures are real embedded files
- [ ] Transient errors retry, logic bugs don't
- [ ] Tests pass reliably in CI

---

## Summary

### Final Coverage by Suite

| Suite | Mode | Methods Covered |
|-------|------|-----------------|
| **polling** | Default | 20 (Tier1 + getUpdates via tracer) |
| **interactive** | Opt-in | +1 (answerCallbackQuery) |
| **webhook** | Separate | +3 (setWebhook, getWebhookInfo, deleteWebhook) |
| **Total** | | **24/25** (getUpdates counted via tracer) |

### PR Timeline

| PR | Focus | Hours | Coverage After |
|----|-------|-------|----------------|
| PR1 | Instrumentation | 2-3 | 20/25 (getUpdates via tracer) |
| PR2 | editMessageMedia | 3-4 | 21/25 |
| PR3 | S11_CallbackQuery | 4-5 | 22/25 |
| PR4 | Webhook suite | 3-4 | 24/25 |
| PR5 | Hardening | 2-3 | 24/25 (stable) |
| **Total** | | **15-19 hours** | **24/25 (96%)** |

### Commands After Implementation

```bash
# Default suite (all non-interactive)
./testbot --run polling

# Interactive callback test
./testbot --run interactive

# Webhook suite (requires --allow-webhook-mutations)
./testbot --run webhook \
  --webhook-url=https://example.com/webhook \
  --webhook-secret=my_secret \
  --allow-webhook-mutations

# Check coverage
./testbot --status
# ✅ Covered: 24
# ⏭️ Internal: 1 (getUpdates - tracked via tracer)
# ❌ Missing: 0
```

### Infra Options for Webhook Testing

| Option | Pros | Cons |
|--------|------|------|
| **Public HTTPS** | Production-like | Needs domain, cert, ports 443/80/88/8443 |
| **Local Bot API Server** | HTTP allowed, any port | Extra setup, needs API credentials |
| **ngrok/cloudflare tunnel** | Quick setup | Dependency, URL changes |

---

*galigo Testbot Coverage Completion Plan v2.0*  
*Consolidated from dual analyses*