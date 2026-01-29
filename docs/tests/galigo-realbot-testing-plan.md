# galigo Real Bot Testing Plan v2.0

## Tier-1 Complete Acceptance Tests Against Telegram API

**Version:** 2.0 (Consolidated from dual analyses)  
**Scope:** All Tier-1 + Legacy methods  
**Architecture:** Standalone test bot with method registry  
**Go Version:** 1.25.6 (stable)  
**Estimated Effort:** 25-35 hours (6 PRs)

---

## Executive Summary

A separate binary (`cmd/galigo-testbot`) that:

- Runs **acceptance scenarios** against Telegram's real Bot API
- Validates sender + receiver + multipart + file flows
- Produces a **JSON report** (uploadable as document)
- **Guarantees coverage** via method registry (no missing methods)
- Cleans up created messages automatically

This complements the `httptest` unit suite - it's not about coverage %, it's "does it actually work with Telegram".

---

## Architecture Overview

```
cmd/galigo-testbot/
├── main.go                      # Entry point
├── config/
│   └── config.go                # Environment + flags
├── auth/
│   └── admin.go                 # Admin user verification
├── registry/
│   └── registry.go              # Method registry (Tier1 + Legacy)
├── engine/
│   ├── scenario.go              # Scenario definition + Covers()
│   ├── runner.go                # Step executor with rate limiting
│   └── steps.go                 # Step implementations
├── suites/
│   ├── smoke.go                 # Quick sanity (S0)
│   ├── tier1.go                 # Full Tier-1 (S1-S9)
│   ├── legacy.go                # Pre-Tier-1 methods
│   └── webhook.go               # Webhook-specific tests
├── fixtures/                    # go:embed test files
│   ├── photo.jpg
│   ├── doc.txt
│   ├── video.mp4
│   ├── audio.mp3
│   ├── anim.gif
│   ├── voice.ogg
│   ├── videonote.mp4
│   └── sticker.webp
├── evidence/
│   ├── store.go                 # Evidence persistence
│   └── report.go                # Report generation
├── cleanup/
│   └── cleanup.go               # Message/webhook cleanup
└── transport/
    ├── polling.go               # Polling mode
    └── webhook.go               # Webhook mode
```

---

## Part 1: Configuration

### 1.1 Environment Variables

```bash
# Required
TESTBOT_TOKEN=123456:ABC-DEF...           # Bot token from BotFather
TESTBOT_CHAT_ID=123456789                 # Target chat for test messages
TESTBOT_ADMINS=123456789,987654321        # Comma-separated admin user IDs

# Optional - General
TESTBOT_MODE=polling                      # polling or webhook (default: polling)
TESTBOT_STORAGE_DIR=./var                 # Reports and state storage
TESTBOT_LOG_LEVEL=info                    # debug, info, warn, error

# Safety Limits (IMPORTANT)
TESTBOT_MAX_MESSAGES_PER_RUN=40           # Cap messages per run
TESTBOT_SEND_INTERVAL=1200ms              # Flood-safe default (≤1 msg/sec)
TESTBOT_ALLOW_STRESS=false                # Enable stress testing

# Webhook-specific (only for webhook suite)
TESTBOT_WEBHOOK_PUBLIC_URL=https://...    # Public URL for webhook
TESTBOT_WEBHOOK_SECRET_TOKEN=...          # Secret for X-Telegram-Bot-Api-Secret-Token
TESTBOT_LISTEN_ADDR=:8080                 # Local listen address

# Fallbacks (for finicky formats)
TESTBOT_STICKER_FILE_ID=                  # Fallback sticker file_id after first success
```

### 1.2 Config Struct

**File:** `cmd/galigo-testbot/config/config.go`

```go
package config

import (
    "os"
    "strconv"
    "strings"
    "time"
)

type Config struct {
    // Required
    Token   string
    ChatID  int64
    Admins  []int64
    
    // General
    Mode       string        // "polling" or "webhook"
    StorageDir string
    LogLevel   string
    
    // Safety limits
    MaxMessagesPerRun int
    SendInterval      time.Duration
    AllowStress       bool
    
    // Webhook
    WebhookPublicURL  string
    WebhookSecretToken string
    ListenAddr        string
    
    // Fallbacks
    StickerFileID string
    
    // Derived
    Timeout time.Duration
}

func Load() (*Config, error) {
    cfg := &Config{
        Token:             os.Getenv("TESTBOT_TOKEN"),
        Mode:              getEnvDefault("TESTBOT_MODE", "polling"),
        StorageDir:        getEnvDefault("TESTBOT_STORAGE_DIR", "./var"),
        LogLevel:          getEnvDefault("TESTBOT_LOG_LEVEL", "info"),
        MaxMessagesPerRun: getEnvIntDefault("TESTBOT_MAX_MESSAGES_PER_RUN", 40),
        SendInterval:      getEnvDurationDefault("TESTBOT_SEND_INTERVAL", 1200*time.Millisecond),
        AllowStress:       os.Getenv("TESTBOT_ALLOW_STRESS") == "true",
        ListenAddr:        getEnvDefault("TESTBOT_LISTEN_ADDR", ":8080"),
        StickerFileID:     os.Getenv("TESTBOT_STICKER_FILE_ID"),
        Timeout:           30 * time.Second,
    }
    
    if cfg.Token == "" {
        return nil, fmt.Errorf("TESTBOT_TOKEN required")
    }
    
    // Parse chat ID
    chatIDStr := os.Getenv("TESTBOT_CHAT_ID")
    if chatIDStr == "" {
        return nil, fmt.Errorf("TESTBOT_CHAT_ID required")
    }
    chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
    if err != nil {
        return nil, fmt.Errorf("invalid TESTBOT_CHAT_ID: %w", err)
    }
    cfg.ChatID = chatID
    
    // Parse admins
    adminsStr := os.Getenv("TESTBOT_ADMINS")
    if adminsStr == "" {
        return nil, fmt.Errorf("TESTBOT_ADMINS required")
    }
    for _, s := range strings.Split(adminsStr, ",") {
        id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
        if err != nil {
            continue
        }
        cfg.Admins = append(cfg.Admins, id)
    }
    if len(cfg.Admins) == 0 {
        return nil, fmt.Errorf("at least one admin required")
    }
    
    // Webhook config
    cfg.WebhookPublicURL = os.Getenv("TESTBOT_WEBHOOK_PUBLIC_URL")
    cfg.WebhookSecretToken = os.Getenv("TESTBOT_WEBHOOK_SECRET_TOKEN")
    
    return cfg, nil
}
```

---

## Part 2: Method Registry (Guarantees Coverage)

### 2.1 Registry Design

**File:** `cmd/galigo-testbot/registry/registry.go`

```go
package registry

// MethodCategory groups methods by implementation phase
type MethodCategory string

const (
    CategoryTier1  MethodCategory = "tier1"
    CategoryLegacy MethodCategory = "legacy"
)

// Method represents a galigo API method
type Method struct {
    Name     string
    Category MethodCategory
    Notes    string // e.g., "requires webhook infra"
}

// AllMethods is the complete list of galigo API methods
var AllMethods = []Method{
    // === Tier 1 Methods ===
    // Core
    {Name: "getMe", Category: CategoryTier1},
    {Name: "sendMessage", Category: CategoryTier1},
    {Name: "editMessageText", Category: CategoryTier1},
    {Name: "deleteMessage", Category: CategoryTier1},
    
    // Callbacks
    {Name: "answerCallbackQuery", Category: CategoryTier1},
    {Name: "editMessageReplyMarkup", Category: CategoryTier1},
    
    // Forward/Copy
    {Name: "forwardMessage", Category: CategoryTier1},
    {Name: "copyMessage", Category: CategoryTier1},
    
    // Chat action
    {Name: "sendChatAction", Category: CategoryTier1},
    
    // Media uploads (multipart)
    {Name: "sendPhoto", Category: CategoryTier1},
    {Name: "sendDocument", Category: CategoryTier1},
    {Name: "sendVideo", Category: CategoryTier1},
    {Name: "sendAudio", Category: CategoryTier1},
    {Name: "sendAnimation", Category: CategoryTier1},
    {Name: "sendVoice", Category: CategoryTier1},
    {Name: "sendVideoNote", Category: CategoryTier1},
    {Name: "sendSticker", Category: CategoryTier1},
    
    // Albums
    {Name: "sendMediaGroup", Category: CategoryTier1},
    
    // Media edit
    {Name: "editMessageMedia", Category: CategoryTier1},
    {Name: "editMessageCaption", Category: CategoryTier1},
    
    // Files
    {Name: "getFile", Category: CategoryTier1},
    {Name: "downloadFile", Category: CategoryTier1, Notes: "helper, not direct API"},
    
    // === Legacy Methods (pre-Tier-1) ===
    // Webhook management
    {Name: "setWebhook", Category: CategoryLegacy, Notes: "requires webhook infra"},
    {Name: "deleteWebhook", Category: CategoryLegacy},
    {Name: "getWebhookInfo", Category: CategoryLegacy},
    
    // Polling
    {Name: "getUpdates", Category: CategoryLegacy, Notes: "internal to receiver"},
    
    // Add other legacy methods as needed...
}

// MethodNames returns just the names
func MethodNames() []string {
    names := make([]string, len(AllMethods))
    for i, m := range AllMethods {
        names[i] = m.Name
    }
    return names
}

// Tier1Methods returns only Tier-1 methods
func Tier1Methods() []Method {
    var methods []Method
    for _, m := range AllMethods {
        if m.Category == CategoryTier1 {
            methods = append(methods, m)
        }
    }
    return methods
}

// LegacyMethods returns only legacy methods
func LegacyMethods() []Method {
    var methods []Method
    for _, m := range AllMethods {
        if m.Category == CategoryLegacy {
            methods = append(methods, m)
        }
    }
    return methods
}
```

### 2.2 Coverage Checker

**File:** `cmd/galigo-testbot/registry/coverage.go`

```go
package registry

import "slices"

// CoverageReport shows which methods are covered/missing
type CoverageReport struct {
    Covered  []string
    Skipped  []string // With reasons
    Missing  []string
}

// CheckCoverage compares scenarios against method registry
func CheckCoverage(scenarios []Scenario) *CoverageReport {
    allMethods := make(map[string]bool)
    for _, m := range AllMethods {
        allMethods[m.Name] = false
    }
    
    // Mark covered methods
    covered := make(map[string]bool)
    for _, s := range scenarios {
        for _, method := range s.Covers() {
            covered[method] = true
            allMethods[method] = true
        }
    }
    
    report := &CoverageReport{}
    
    for method, isCovered := range allMethods {
        if isCovered {
            report.Covered = append(report.Covered, method)
        } else {
            report.Missing = append(report.Missing, method)
        }
    }
    
    slices.Sort(report.Covered)
    slices.Sort(report.Missing)
    
    return report
}
```

---

## Part 3: Scenario Engine

### 3.1 Scenario Definition with Covers()

**File:** `cmd/galigo-testbot/engine/scenario.go`

```go
package engine

import (
    "context"
    "time"
)

// Scenario is a named sequence of steps that declares method coverage
type Scenario interface {
    Name() string
    Description() string
    Covers() []string        // Methods this scenario exercises
    Steps() []Step
    Timeout() time.Duration
}

// BaseScenario provides common implementation
type BaseScenario struct {
    name        string
    description string
    covers      []string
    steps       []Step
    timeout     time.Duration
}

func (s *BaseScenario) Name() string           { return s.name }
func (s *BaseScenario) Description() string    { return s.description }
func (s *BaseScenario) Covers() []string       { return s.covers }
func (s *BaseScenario) Steps() []Step          { return s.steps }
func (s *BaseScenario) Timeout() time.Duration { return s.timeout }

// Step represents a single test step
type Step interface {
    Name() string
    Execute(ctx context.Context, rt *Runtime) (*StepResult, error)
}

// StepResult captures evidence
type StepResult struct {
    StepName   string        `json:"step_name"`
    Method     string        `json:"method,omitempty"`
    Duration   time.Duration `json:"duration"`
    Success    bool          `json:"success"`
    Error      string        `json:"error,omitempty"`
    MessageIDs []int         `json:"message_ids,omitempty"`
    FileIDs    []string      `json:"file_ids,omitempty"`
    Evidence   any           `json:"evidence,omitempty"`
}

// Runtime provides context for step execution
type Runtime struct {
    Bot       *galigo.Bot
    Config    *config.Config
    Logger    *slog.Logger
    
    // State shared between steps
    CreatedMessages []CreatedMessage
    LastMessage     *tg.Message
    LastCallback    *tg.CallbackQuery
    CapturedFileIDs map[string]string // name -> file_id for reuse
}

// CreatedMessage tracks messages for cleanup
type CreatedMessage struct {
    ChatID    int64
    MessageID int
}
```

### 3.2 Runner with Rate Limiting

**File:** `cmd/galigo-testbot/engine/runner.go`

```go
package engine

import (
    "context"
    "time"
)

// Runner executes scenarios with safety limits
type Runner struct {
    runtime      *Runtime
    sendInterval time.Duration
    maxMessages  int
    messageCount int
    logger       *slog.Logger
}

func NewRunner(rt *Runtime, cfg *config.Config) *Runner {
    return &Runner{
        runtime:      rt,
        sendInterval: cfg.SendInterval,
        maxMessages:  cfg.MaxMessagesPerRun,
        logger:       rt.Logger,
    }
}

// Run executes a scenario
func (r *Runner) Run(ctx context.Context, scenario Scenario) *ScenarioResult {
    result := &ScenarioResult{
        ScenarioName: scenario.Name(),
        Covers:       scenario.Covers(),
        StartTime:    time.Now(),
        Steps:        make([]StepResult, 0),
    }
    
    // Apply timeout
    timeout := scenario.Timeout()
    if timeout == 0 {
        timeout = 5 * time.Minute
    }
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    
    r.logger.Info("starting scenario", 
        "name", scenario.Name(),
        "covers", scenario.Covers())
    
    for _, step := range scenario.Steps() {
        // Check message budget
        if r.messageCount >= r.maxMessages {
            result.Error = fmt.Sprintf("message budget exceeded (%d)", r.maxMessages)
            break
        }
        
        // Execute step
        stepResult, err := r.runStep(ctx, step)
        result.Steps = append(result.Steps, *stepResult)
        
        if err != nil {
            result.Success = false
            result.Error = fmt.Sprintf("step %q failed: %v", step.Name(), err)
            break
        }
        
        // Rate limiting between steps
        select {
        case <-ctx.Done():
            result.Error = ctx.Err().Error()
            break
        case <-time.After(r.sendInterval):
            // Continue
        }
    }
    
    if result.Error == "" {
        result.Success = true
    }
    
    result.EndTime = time.Now()
    result.Duration = result.EndTime.Sub(result.StartTime)
    
    r.logger.Info("scenario completed",
        "name", scenario.Name(),
        "success", result.Success,
        "duration", result.Duration,
        "messages", r.messageCount)
    
    return result
}

func (r *Runner) runStep(ctx context.Context, step Step) (*StepResult, error) {
    start := time.Now()
    
    stepResult, err := step.Execute(ctx, r.runtime)
    if stepResult == nil {
        stepResult = &StepResult{StepName: step.Name()}
    }
    
    stepResult.Duration = time.Since(start)
    
    if err != nil {
        stepResult.Success = false
        stepResult.Error = err.Error()
        r.logger.Error("step failed", "step", step.Name(), "error", err)
        return stepResult, err
    }
    
    stepResult.Success = true
    r.messageCount += len(stepResult.MessageIDs)
    
    r.logger.Info("step completed", 
        "step", step.Name(), 
        "duration", stepResult.Duration)
    
    return stepResult, nil
}
```

---

## Part 4: Test Fixtures (Embedded)

### 4.1 Fixtures with go:embed

**File:** `cmd/galigo-testbot/fixtures/fixtures.go`

```go
package fixtures

import (
    "bytes"
    "embed"
    "io"
)

//go:embed photo.jpg doc.txt video.mp4 audio.mp3 anim.gif voice.ogg videonote.mp4 sticker.webp
var content embed.FS

// Photo returns the test photo
func Photo() io.Reader {
    data, _ := content.ReadFile("photo.jpg")
    return bytes.NewReader(data)
}

// Document returns the test document
func Document() io.Reader {
    data, _ := content.ReadFile("doc.txt")
    return bytes.NewReader(data)
}

// Video returns the test video
func Video() io.Reader {
    data, _ := content.ReadFile("video.mp4")
    return bytes.NewReader(data)
}

// Audio returns the test audio
func Audio() io.Reader {
    data, _ := content.ReadFile("audio.mp3")
    return bytes.NewReader(data)
}

// Animation returns the test animation
func Animation() io.Reader {
    data, _ := content.ReadFile("anim.gif")
    return bytes.NewReader(data)
}

// Voice returns the test voice message
func Voice() io.Reader {
    data, _ := content.ReadFile("voice.ogg")
    return bytes.NewReader(data)
}

// VideoNote returns the test video note
func VideoNote() io.Reader {
    data, _ := content.ReadFile("videonote.mp4")
    return bytes.NewReader(data)
}

// Sticker returns the test sticker
func Sticker() io.Reader {
    data, _ := content.ReadFile("sticker.webp")
    return bytes.NewReader(data)
}

// HasVideoNote returns true if video note fixture exists
func HasVideoNote() bool {
    _, err := content.ReadFile("videonote.mp4")
    return err == nil
}
```

### 4.2 Fixture Requirements

| File | Size | Notes |
|------|------|-------|
| `photo.jpg` | < 100KB | Simple test image |
| `doc.txt` | < 10KB | Plain text document |
| `video.mp4` | < 500KB | Tiny video (5-10 sec) |
| `audio.mp3` | < 500KB | Tiny audio (5-10 sec) |
| `anim.gif` | < 200KB | Simple animation |
| `voice.ogg` | < 100KB | OGG Opus format |
| `videonote.mp4` | < 500KB | Square video, optional |
| `sticker.webp` | < 100KB | Static WebP sticker |

---

## Part 5: Tier-1 Test Suites (S1-S9)

### Suite Overview

| ID | Name | Methods Covered | Est. Time |
|----|------|-----------------|-----------|
| S0 | Smoke | getMe, sendMessage | 10s |
| S1 | Identity | getMe | 5s |
| S2 | Message Lifecycle | sendMessage, editMessageText, deleteMessage | 15s |
| S3 | Callbacks | sendMessage, answerCallbackQuery, editMessageReplyMarkup | 60s+ |
| S4 | Forward/Copy | forwardMessage, copyMessage | 15s |
| S5 | Chat Action | sendChatAction | 5s |
| S6 | Media Uploads | sendPhoto, sendDocument, sendVideo, sendAudio, sendAnimation, sendVoice, sendVideoNote, sendSticker | 60s |
| S7 | Media Groups | sendMediaGroup | 15s |
| S8 | Edit Media | editMessageMedia, editMessageCaption | 15s |
| S9 | Files | getFile, downloadFile | 15s |

### 5.1 S1 - Identity

**File:** `cmd/galigo-testbot/suites/tier1.go`

```go
package suites

func S1_Identity(chatID int64) Scenario {
    return &BaseScenario{
        name:        "S1-Identity",
        description: "Verify bot identity",
        covers:      []string{"getMe"},
        timeout:     30 * time.Second,
        steps: []Step{
            &GetMeStep{},
        },
    }
}

type GetMeStep struct{}

func (s *GetMeStep) Name() string { return "getMe" }

func (s *GetMeStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    user, err := rt.Bot.GetMe(ctx)
    if err != nil {
        return nil, err
    }
    
    if !user.IsBot {
        return nil, fmt.Errorf("expected bot, got user")
    }
    
    rt.Logger.Info("bot identity verified", 
        "username", user.Username, 
        "id", user.ID)
    
    return &StepResult{
        Method:   "getMe",
        Evidence: map[string]any{"username": user.Username, "id": user.ID},
    }, nil
}
```

### 5.2 S2 - Message Lifecycle

```go
func S2_MessageLifecycle(chatID int64) Scenario {
    return &BaseScenario{
        name:        "S2-MessageLifecycle",
        description: "Send, edit, delete message",
        covers:      []string{"sendMessage", "editMessageText", "deleteMessage"},
        timeout:     1 * time.Minute,
        steps: []Step{
            &SendMessageStep{ChatID: chatID, Text: "tier1: message lifecycle"},
            &EditMessageTextStep{Text: "tier1: EDITED"},
            &DeleteLastMessageStep{},
        },
    }
}
```

### 5.3 S3 - Callbacks (Human Interaction Required)

```go
func S3_Callbacks(chatID int64) Scenario {
    return &BaseScenario{
        name:        "S3-Callbacks",
        description: "Inline keyboard + callback handling",
        covers:      []string{"sendMessage", "answerCallbackQuery", "editMessageReplyMarkup"},
        timeout:     2 * time.Minute, // Needs human click
        steps: []Step{
            &SendInlineKeyboardStep{
                ChatID: chatID,
                Text:   "Click ACK to continue test:",
                Buttons: [][]tg.InlineKeyboardButton{
                    {{Text: "✅ ACK", CallbackData: "test_ack"}},
                    {{Text: "❌ FAIL", CallbackData: "test_fail"}},
                },
            },
            &WaitForCallbackStep{
                ExpectedData: "test_ack",
                Timeout:      60 * time.Second,
            },
            &AnswerCallbackStep{Text: "ACK received!"},
            &EditReplyMarkupStep{RemoveKeyboard: true},
            &CleanupStep{},
        },
    }
}
```

### 5.4 S4 - Forward/Copy

```go
func S4_ForwardCopy(chatID int64) Scenario {
    return &BaseScenario{
        name:        "S4-ForwardCopy",
        description: "Forward and copy messages",
        covers:      []string{"sendMessage", "forwardMessage", "copyMessage"},
        timeout:     1 * time.Minute,
        steps: []Step{
            &SendMessageStep{ChatID: chatID, Text: "tier1: source message"},
            &ForwardMessageStep{ToChatID: chatID},
            &CopyMessageStep{ToChatID: chatID},
            &CleanupStep{}, // Deletes all 3 messages
        },
    }
}
```

### 5.5 S5 - Chat Action

```go
func S5_ChatAction(chatID int64) Scenario {
    return &BaseScenario{
        name:        "S5-ChatAction",
        description: "Send chat action",
        covers:      []string{"sendChatAction"},
        timeout:     30 * time.Second,
        steps: []Step{
            &SendChatActionStep{ChatID: chatID, Action: tg.ChatActionTyping},
        },
    }
}
```

### 5.6 S6 - Media Uploads (Multipart)

```go
func S6_MediaUploads(chatID int64, stickerFallbackID string) Scenario {
    return &BaseScenario{
        name:        "S6-MediaUploads",
        description: "All media upload types (multipart)",
        covers: []string{
            "sendPhoto", "sendDocument", "sendVideo", "sendAudio",
            "sendAnimation", "sendVoice", "sendVideoNote", "sendSticker",
        },
        timeout: 3 * time.Minute,
        steps: []Step{
            // Photo
            &SendPhotoStep{
                ChatID:  chatID,
                Photo:   tg.FileFromReader("test.jpg", fixtures.Photo()),
                Caption: "Photo upload test",
            },
            
            // Document
            &SendDocumentStep{
                ChatID:   chatID,
                Document: tg.FileFromReader("test.txt", fixtures.Document()),
                Caption:  "Document upload test",
            },
            
            // Video
            &SendVideoStep{
                ChatID:  chatID,
                Video:   tg.FileFromReader("test.mp4", fixtures.Video()),
                Caption: "Video upload test",
            },
            
            // Audio
            &SendAudioStep{
                ChatID:  chatID,
                Audio:   tg.FileFromReader("test.mp3", fixtures.Audio()),
                Caption: "Audio upload test",
            },
            
            // Animation
            &SendAnimationStep{
                ChatID:    chatID,
                Animation: tg.FileFromReader("test.gif", fixtures.Animation()),
                Caption:   "Animation upload test",
            },
            
            // Voice
            &SendVoiceStep{
                ChatID: chatID,
                Voice:  tg.FileFromReader("test.ogg", fixtures.Voice()),
            },
            
            // Video Note (optional)
            &ConditionalStep{
                Condition: fixtures.HasVideoNote,
                Step: &SendVideoNoteStep{
                    ChatID:    chatID,
                    VideoNote: tg.FileFromReader("test.mp4", fixtures.VideoNote()),
                },
                SkipReason: "video note fixture not available",
            },
            
            // Sticker (with fallback)
            &SendStickerStep{
                ChatID:         chatID,
                Sticker:        tg.FileFromReader("test.webp", fixtures.Sticker()),
                FallbackFileID: stickerFallbackID,
            },
            
            &CleanupStep{},
        },
    }
}
```

### 5.7 S7 - Media Groups (attach://)

```go
func S7_MediaGroups(chatID int64) Scenario {
    return &BaseScenario{
        name:        "S7-MediaGroups",
        description: "Media group with attach:// references",
        covers:      []string{"sendMediaGroup"},
        timeout:     1 * time.Minute,
        steps: []Step{
            &SendMediaGroupStep{
                ChatID: chatID,
                Media: []tg.InputMedia{
                    tg.InputMediaPhoto{
                        Type:    "photo",
                        Media:   tg.FileFromReader("photo1.jpg", fixtures.Photo()),
                        Caption: "Album photo 1",
                    },
                    tg.InputMediaPhoto{
                        Type:  "photo",
                        Media: tg.FileFromReader("photo2.jpg", fixtures.Photo()),
                    },
                },
            },
            &CleanupStep{},
        },
    }
}
```

### 5.8 S8 - Edit Media

```go
func S8_EditMedia(chatID int64) Scenario {
    return &BaseScenario{
        name:        "S8-EditMedia",
        description: "Edit message media and caption",
        covers:      []string{"sendPhoto", "editMessageMedia", "editMessageCaption"},
        timeout:     1 * time.Minute,
        steps: []Step{
            &SendPhotoStep{
                ChatID:  chatID,
                Photo:   tg.FileFromReader("original.jpg", fixtures.Photo()),
                Caption: "Original photo",
            },
            &EditMessageMediaStep{
                NewMedia: tg.InputMediaPhoto{
                    Type:  "photo",
                    Media: tg.FileFromReader("replaced.jpg", fixtures.Photo()),
                },
            },
            &EditMessageCaptionStep{Caption: "Replaced photo"},
            &CleanupStep{},
        },
    }
}
```

### 5.9 S9 - Files (Download)

```go
func S9_Files(chatID int64) Scenario {
    return &BaseScenario{
        name:        "S9-Files",
        description: "Get file info and download",
        covers:      []string{"sendDocument", "getFile", "downloadFile"},
        timeout:     1 * time.Minute,
        steps: []Step{
            &SendDocumentStep{
                ChatID:   chatID,
                Document: tg.FileFromReader("download_test.txt", fixtures.Document()),
                Caption:  "File for download test",
            },
            &GetFileStep{},
            &DownloadFileStep{VerifySHA256: true},
            &CleanupStep{},
        },
    }
}
```

---

## Part 6: Legacy Suite

### 6.1 Webhook API (L1)

```go
func L1_WebhookAPI(webhookURL, secretToken string) Scenario {
    return &BaseScenario{
        name:        "L1-WebhookAPI",
        description: "Webhook management API",
        covers:      []string{"setWebhook", "getWebhookInfo", "deleteWebhook"},
        timeout:     1 * time.Minute,
        steps: []Step{
            &SetWebhookStep{
                URL:         webhookURL,
                SecretToken: secretToken,
            },
            &GetWebhookInfoStep{ExpectedURL: webhookURL},
            &DeleteWebhookStep{},
        },
    }
}
```

### 6.2 Receiver Smoke (L2)

```go
func L2_ReceiverSmoke(chatID int64) Scenario {
    return &BaseScenario{
        name:        "L2-ReceiverSmoke",
        description: "Verify receiver processes updates",
        covers:      []string{"getUpdates"}, // Internal
        timeout:     2 * time.Minute,
        steps: []Step{
            // Run callback scenario - verifies receiver works
            &SendInlineKeyboardStep{
                ChatID:  chatID,
                Text:    "Receiver test - click to verify:",
                Buttons: [][]tg.InlineKeyboardButton{
                    {{Text: "✅ Verify", CallbackData: "receiver_test"}},
                },
            },
            &WaitForCallbackStep{
                ExpectedData: "receiver_test",
                Timeout:      60 * time.Second,
            },
            &AnswerCallbackStep{Text: "Receiver OK!"},
            &CleanupStep{},
        },
    }
}
```

---

## Part 7: Bot Commands

### 7.1 Command Router

**File:** `cmd/galigo-testbot/main.go`

```go
func handleCommand(ctx context.Context, bot *galigo.Bot, runner *Runner, 
                   cfg *config.Config, registry *Registry, 
                   chatID int64, command, args string) {
    
    switch command {
    case "run":
        handleRun(ctx, bot, runner, cfg, chatID, args)
    case "status":
        handleStatus(ctx, bot, registry, chatID)
    case "report":
        handleReport(ctx, bot, cfg, chatID, args)
    case "cleanup":
        handleCleanup(ctx, bot, runner.runtime, chatID)
    case "help":
        handleHelp(ctx, bot, chatID)
    default:
        bot.SendMessage(ctx, tg.ChatIDFromInt64(chatID), 
            "Unknown command. Use /help")
    }
}
```

### 7.2 Available Commands

| Command | Description |
|---------|-------------|
| `/run tier1` | Run full Tier-1 suite (S1-S9) |
| `/run smoke` | Quick sanity check |
| `/run legacy` | Run legacy suite |
| `/run all` | All suites + coverage check |
| `/run S1` | Run specific scenario |
| `/status` | Show method coverage |
| `/report last` | Upload last report as document |
| `/cleanup last` | Delete messages from last run |
| `/help` | Show help |

### 7.3 Status Command (Coverage Check)

```go
func handleStatus(ctx context.Context, bot *galigo.Bot, reg *Registry, chatID int64) {
    scenarios := getAllScenarios()
    report := registry.CheckCoverage(scenarios)
    
    var sb strings.Builder
    sb.WriteString("📊 Method Coverage:\n\n")
    
    sb.WriteString(fmt.Sprintf("✅ Covered: %d\n", len(report.Covered)))
    sb.WriteString(fmt.Sprintf("⚠️ Skipped: %d\n", len(report.Skipped)))
    sb.WriteString(fmt.Sprintf("❌ Missing: %d\n\n", len(report.Missing)))
    
    if len(report.Missing) > 0 {
        sb.WriteString("Missing methods:\n")
        for _, m := range report.Missing {
            sb.WriteString(fmt.Sprintf("  • %s\n", m))
        }
    }
    
    bot.SendMessage(ctx, tg.ChatIDFromInt64(chatID), sb.String())
}
```

---

## Part 8: CI Integration

### 8.1 Manual Workflow

**File:** `.github/workflows/acceptance.yml`

```yaml
name: Acceptance Tests

on:
  workflow_dispatch:
    inputs:
      suite:
        description: 'Test suite'
        required: true
        default: 'tier1'
        type: choice
        options:
          - smoke
          - tier1
          - legacy
          - all

jobs:
  acceptance:
    name: Run Acceptance Tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25.6'
      
      - name: Build testbot
        run: go build -o testbot ./cmd/galigo-testbot
      
      - name: Run acceptance tests
        env:
          TESTBOT_TOKEN: ${{ secrets.TESTBOT_TOKEN }}
          TESTBOT_CHAT_ID: ${{ secrets.TESTBOT_CHAT_ID }}
          TESTBOT_ADMINS: ${{ secrets.TESTBOT_ADMINS }}
        run: |
          # Note: Callback tests require human interaction
          # This workflow runs non-interactive scenarios only
          ./testbot --run=${{ inputs.suite }} --skip-interactive
      
      - name: Upload report
        uses: actions/upload-artifact@v4
        with:
          name: acceptance-report
          path: ./var/reports/
```

---

## Part 9: Delivery Plan

### PR Schedule

| PR | Focus | Hours | Deliverables |
|----|-------|-------|--------------|
| **PR1** | Skeleton + config + auth | 4-5 | main.go, config/, auth/ |
| **PR2** | Registry + engine | 5-6 | registry/, engine/ |
| **PR3** | Fixtures + S1-S5 | 5-6 | fixtures/, smoke + core scenarios |
| **PR4** | S6-S8 (Media) | 5-6 | Media upload + edit scenarios |
| **PR5** | S9 + cleanup + report | 4-5 | Files, cleanup, report commands |
| **PR6** | Legacy + webhook | 4-5 | L1-L2, webhook suite |
| **TOTAL** | | **27-33 hours** | |

### Timeline

- **Week 1:** PR1, PR2 (Foundation)
- **Week 2:** PR3, PR4 (Core + Media)
- **Week 3:** PR5, PR6 (Files + Legacy)
- **Week 4:** Testing, bug fixes, documentation

---

## Part 10: Summary

### Coverage Guarantee

The method registry ensures:
- ✅ All Tier-1 methods covered by S1-S9
- ✅ All Legacy methods covered by L1-L2
- ✅ `/status` shows any missing methods
- ✅ `/run all` includes coverage check

### Safety Features

| Feature | Default |
|---------|---------|
| Send interval | 1200ms (flood-safe) |
| Max messages/run | 40 |
| Stress testing | Disabled by default |
| Admin-only | Required |
| Auto-cleanup | After each scenario |

### Suite Summary

| Suite | Scenarios | Methods | Est. Time |
|-------|-----------|---------|-----------|
| smoke | S0 | 2 | 10s |
| tier1 | S1-S9 | 22 | ~5min |
| legacy | L1-L2 | 4 | ~2min |
| all | S0-S9 + L1-L2 | 26 | ~8min |

---

*galigo Real Bot Testing Plan v2.0 - Tier-1 Complete with Method Registry*