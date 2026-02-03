# galigo-testbot: Ultimate Combined Implementation Plan

**Sources:** 6 independent consultant analyses combined  
**Current Coverage:** 51/123 methods (41%)  
**Target Coverage:** 100+ methods (80%+)

---

## Executive Summary

This document consolidates recommendations from **6 independent consultants** into the definitive implementation strategy. All sources agree on core principles but each contributed unique insights.

### Consensus Points (All 6 Agreed)

| Principle | Description |
|-----------|-------------|
| **SKIP ≠ FAIL** | Missing prerequisites should skip, not fail |
| **Save/Restore** | All mutations must restore original state |
| **Capability Probing** | Detect chat type and bot permissions at startup |
| **Incremental PRs** | Small, reviewable changes |
| **CI Safety** | Never run destructive tests automatically |

### Unique Contributions by Source

| Source | Key Contribution |
|--------|------------------|
| **Original** | 5-phase method categorization (72 methods mapped) |
| **Consultant 1** | GitHub Environments + Required Reviewers |
| **Consultant 2** | `ErrSkipped` type + `ChatContext` + `RequireX` helpers |
| **Consultant 3** | SBOM generation + `step-security/harden-runner` |
| **Consultant 5** | `t.Cleanup()` Go-idiomatic rollback + **global lock fix** |
| **Consultant 6** | Exact file layout + `SenderClient` interface + `StepSeedMessages` |

---

## Part 1: Framework Improvements (Do First)

### 1.1 SKIP Framework (Consultants 2, 6)

**File:** `cmd/galigo-testbot/engine/errors.go`

```go
package engine

import "errors"

// SkipError indicates a scenario was skipped due to missing prerequisites.
type SkipError struct {
    Reason string
}

func (e SkipError) Error() string {
    return "skipped: " + e.Reason
}

// Skip returns a SkipError with the given reason.
func Skip(reason string) error {
    return SkipError{Reason: reason}
}

// IsSkip returns true if err is a SkipError.
func IsSkip(err error) bool {
    var skipErr SkipError
    return errors.As(err, &skipErr)
}
```

**File:** `cmd/galigo-testbot/engine/runner.go` (update)

```go
// In executeScenario(), handle SkipError:
for _, step := range scenario.Steps() {
    result, err := step.Execute(ctx, rt)
    if err != nil {
        if IsSkip(err) {
            return &ScenarioResult{
                Name:       scenario.Name(),
                Success:    true,  // Skipped is NOT a failure!
                Skipped:    true,
                SkipReason: err.Error(),
            }
        }
        // Handle actual failure...
    }
}
```

---

### 1.2 ChatContext for Capability Probing (Consultants 2, 6)

**File:** `cmd/galigo-testbot/engine/runtime.go`

```go
package engine

import (
    "context"
    "fmt"

    "github.com/prilive-com/galigo/tg"
)

// ChatContext holds probed chat capabilities.
type ChatContext struct {
    ChatID   int64
    ChatType string // "private", "group", "supergroup", "channel"
    IsForum  bool

    // Bot capabilities
    BotIsAdmin         bool
    CanChangeInfo      bool
    CanDeleteMessages  bool
    CanRestrictMembers bool
    CanPinMessages     bool
    CanManageTopics    bool
    CanInviteUsers     bool
}

// ChatPhotoSnapshot stores state for restore (Consultant 6).
type ChatPhotoSnapshot struct {
    HadPhoto bool
    Bytes    []byte // Downloaded photo bytes
    MimeType string
}

// PermissionsSnapshot stores permissions for restore.
type PermissionsSnapshot struct {
    Permissions *tg.ChatPermissions
}

// Runtime holds scenario execution state.
type Runtime struct {
    // Existing fields...
    Sender      SenderClient
    ChatID      int64
    AdminUserID int64
    Token       tg.SecretToken

    // Message tracking
    LastMessage       *tg.Message
    CreatedMessages   []CreatedMessage
    BulkMessageIDs    []int  // For bulk operations (Consultant 6)

    // Probed capabilities (Consultants 2, 6)
    ChatCtx *ChatContext

    // Snapshots for restore (Consultants 2, 5, 6)
    OriginalChatPhoto   *ChatPhotoSnapshot
    OriginalPermissions *PermissionsSnapshot

    // Captured IDs for cleanup
    CapturedFileIDs     map[string]string
    CreatedStickerSets  []string

    // Optional chat IDs
    ForumChatID int64
    TestUserID  int64
}

// ProbeChat discovers chat capabilities (Consultants 2, 6).
func (rt *Runtime) ProbeChat(ctx context.Context) error {
    chat, err := rt.Sender.GetChat(ctx, rt.ChatID)
    if err != nil {
        return fmt.Errorf("probeChat: %w", err)
    }

    rt.ChatCtx = &ChatContext{
        ChatID:   chat.ID,
        ChatType: chat.Type,
        IsForum:  chat.IsForum,
    }

    // Get bot's membership to check permissions
    me, err := rt.Sender.GetMe(ctx)
    if err != nil {
        return fmt.Errorf("probeChat getMe: %w", err)
    }

    member, err := rt.Sender.GetChatMember(ctx, rt.ChatID, me.ID)
    if err != nil {
        // Not a member - that's OK, just not admin
        return nil
    }

    status := member.Status()
    if status == "administrator" || status == "creator" {
        rt.ChatCtx.BotIsAdmin = true

        if admin, ok := member.(*tg.ChatMemberAdministrator); ok {
            rt.ChatCtx.CanChangeInfo = admin.CanChangeInfo
            rt.ChatCtx.CanDeleteMessages = admin.CanDeleteMessages
            rt.ChatCtx.CanRestrictMembers = admin.CanRestrictMembers
            rt.ChatCtx.CanPinMessages = admin.CanPinMessages
            rt.ChatCtx.CanManageTopics = admin.CanManageTopics
            rt.ChatCtx.CanInviteUsers = admin.CanInviteUsers
        } else if status == "creator" {
            // Creator has all permissions
            rt.ChatCtx.CanChangeInfo = true
            rt.ChatCtx.CanDeleteMessages = true
            rt.ChatCtx.CanRestrictMembers = true
            rt.ChatCtx.CanPinMessages = true
            rt.ChatCtx.CanManageTopics = true
            rt.ChatCtx.CanInviteUsers = true
        }
    }

    return nil
}
```

---

### 1.3 Precondition Helpers (Consultants 2, 6)

**File:** `cmd/galigo-testbot/engine/require.go`

```go
package engine

import "context"

// RequireAdmin skips if bot is not admin.
func RequireAdmin(ctx context.Context, rt *Runtime) error {
    if rt.ChatCtx == nil {
        if err := rt.ProbeChat(ctx); err != nil {
            return err
        }
    }
    if !rt.ChatCtx.BotIsAdmin {
        return Skip("bot is not admin in test chat")
    }
    return nil
}

// RequireCanChangeInfo skips if bot can't change chat info.
func RequireCanChangeInfo(ctx context.Context, rt *Runtime) error {
    if err := RequireAdmin(ctx, rt); err != nil {
        return err
    }
    if !rt.ChatCtx.CanChangeInfo {
        return Skip("bot lacks can_change_info permission")
    }
    return nil
}

// RequireCanRestrict skips if bot can't restrict members.
func RequireCanRestrict(ctx context.Context, rt *Runtime) error {
    if err := RequireAdmin(ctx, rt); err != nil {
        return err
    }
    if !rt.ChatCtx.CanRestrictMembers {
        return Skip("bot lacks can_restrict_members permission")
    }
    return nil
}

// RequireCanManageTopics skips if bot can't manage forum topics.
func RequireCanManageTopics(ctx context.Context, rt *Runtime) error {
    if err := RequireAdmin(ctx, rt); err != nil {
        return err
    }
    if !rt.ChatCtx.CanManageTopics {
        return Skip("bot lacks can_manage_topics permission")
    }
    return nil
}

// RequireForum skips if chat is not a forum.
func RequireForum(ctx context.Context, rt *Runtime) error {
    if rt.ChatCtx == nil {
        if err := rt.ProbeChat(ctx); err != nil {
            return err
        }
    }
    if !rt.ChatCtx.IsForum {
        return Skip("chat is not a forum-enabled supergroup")
    }
    return nil
}

// RequireForumChatID skips if TESTBOT_FORUM_CHAT_ID is not set.
func RequireForumChatID(rt *Runtime) error {
    if rt.ForumChatID == 0 {
        return Skip("TESTBOT_FORUM_CHAT_ID not configured")
    }
    return nil
}

// RequireTestUser skips if TESTBOT_TEST_USER_ID is not set.
func RequireTestUser(rt *Runtime) error {
    if rt.TestUserID == 0 {
        return Skip("TESTBOT_TEST_USER_ID not configured")
    }
    return nil
}
```

---

### 1.4 Inline 1x1 PNG (Consultants 1, 5)

**File:** `cmd/galigo-testbot/engine/fixtures.go`

```go
package engine

// MinimalPNG is a valid 1x1 red pixel PNG (67 bytes).
// No external file dependency needed for chat photo tests.
var MinimalPNG = []byte{
    0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
    0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
    0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
    0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
    0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41,
    0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
    0x00, 0x03, 0x01, 0x01, 0x00, 0x18, 0xdd, 0x8d,
    0xb0, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
    0x44, 0xae, 0x42, 0x60, 0x82,
}
```

---

### 1.5 Download File Helper (Consultant 6)

**File:** `cmd/galigo-testbot/engine/helpers.go`

```go
package engine

import (
    "fmt"
    "io"
    "net/http"

    "github.com/prilive-com/galigo/tg"
)

// DownloadFileBytes downloads a file from Telegram servers.
func DownloadFileBytes(token tg.SecretToken, filePath string) ([]byte, error) {
    url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", 
        token.Expose(), filePath)
    
    resp, err := http.Get(url)
    if err != nil {
        return nil, fmt.Errorf("download file: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("download file: status %d", resp.StatusCode)
    }

    return io.ReadAll(resp.Body)
}
```

---

## Part 2: Exact File Layout (Consultant 6)

```
cmd/galigo-testbot/
├── engine/
│   ├── errors.go           # NEW: SkipError
│   ├── require.go          # NEW: RequireX helpers
│   ├── fixtures.go         # NEW: MinimalPNG
│   ├── helpers.go          # NEW: DownloadFileBytes
│   ├── runtime.go          # UPDATE: ChatContext, snapshots
│   ├── runner.go           # UPDATE: Handle SkipError
│   ├── scenario.go         # UPDATE: SenderClient interface
│   ├── steps.go            # EXISTING
│   ├── steps_extended.go   # EXISTING
│   ├── steps_chat_admin.go # EXISTING
│   ├── steps_geo.go        # NEW: S25-S26
│   ├── steps_misc.go       # NEW: S27, S29-S30
│   ├── steps_bulk.go       # NEW: S28
│   └── steps_chat_settings.go # NEW: S31-S32
├── suites/
│   ├── tier1.go            # EXISTING
│   ├── chat_admin.go       # EXISTING
│   └── extras.go           # NEW: S25-S32 + AllExtrasScenarios()
├── registry/
│   └── registry.go         # UPDATE: Add all 123 methods
└── main.go                 # UPDATE: New --run keys
```

---

## Part 3: SenderClient Interface Additions (Consultant 6)

**File:** `cmd/galigo-testbot/engine/scenario.go` (update interface)

```go
type SenderClient interface {
    // === Existing ===
    GetMe(ctx context.Context) (*tg.User, error)
    SendMessage(ctx context.Context, chatID int64, text string, opts ...any) (*tg.Message, error)
    // ... other existing methods ...

    // === NEW: Geo (S25-S26) ===
    SendLocation(ctx context.Context, chatID int64, lat, lon float64, opts ...any) (*tg.Message, error)
    SendVenue(ctx context.Context, chatID int64, lat, lon float64, title, address string, opts ...any) (*tg.Message, error)

    // === NEW: Misc (S27, S29-S30) ===
    SendContact(ctx context.Context, chatID int64, phone, firstName, lastName string, opts ...any) (*tg.Message, error)
    SendDice(ctx context.Context, chatID int64, emoji string, opts ...any) (*tg.Message, error)
    SetMessageReaction(ctx context.Context, chatID int64, messageID int, emoji string, isBig bool) error
    GetUserProfilePhotos(ctx context.Context, userID int64, offset, limit int) (*tg.UserProfilePhotos, error)
    GetUserChatBoosts(ctx context.Context, chatID, userID int64) (*tg.UserChatBoosts, error)

    // === NEW: Bulk (S28) ===
    ForwardMessages(ctx context.Context, chatID, fromChatID int64, messageIDs []int) ([]*tg.MessageID, error)
    CopyMessages(ctx context.Context, chatID, fromChatID int64, messageIDs []int) ([]*tg.MessageID, error)
    DeleteMessages(ctx context.Context, chatID int64, messageIDs []int) error

    // === NEW: Chat Settings (S31-S32) ===
    SetChatPhoto(ctx context.Context, chatID int64, photo io.Reader) error
    DeleteChatPhoto(ctx context.Context, chatID int64) error
    SetChatPermissions(ctx context.Context, chatID int64, perms tg.ChatPermissions) error
}
```

---

## Part 4: Step Implementations

### 4.1 Geo Steps (S25-S26)

**File:** `cmd/galigo-testbot/engine/steps_geo.go`

```go
package engine

import (
    "context"
)

// SendLocationStep sends a GPS location.
type SendLocationStep struct {
    Latitude  float64
    Longitude float64
}

func (s *SendLocationStep) Name() string { return "sendLocation" }

func (s *SendLocationStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    lat := s.Latitude
    if lat == 0 {
        lat = 48.8584 // Eiffel Tower
    }
    lon := s.Longitude
    if lon == 0 {
        lon = 2.2945
    }

    msg, err := rt.Sender.SendLocation(ctx, rt.ChatID, lat, lon)
    if err != nil {
        return nil, err
    }

    rt.LastMessage = msg
    rt.TrackMessage(rt.ChatID, msg.MessageID)

    return &StepResult{
        Method:     "sendLocation",
        MessageIDs: []int{msg.MessageID},
        Evidence: map[string]any{
            "message_id": msg.MessageID,
            "latitude":   lat,
            "longitude":  lon,
        },
    }, nil
}

// SendVenueStep sends a venue (place).
type SendVenueStep struct {
    Latitude  float64
    Longitude float64
    Title     string
    Address   string
}

func (s *SendVenueStep) Name() string { return "sendVenue" }

func (s *SendVenueStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    lat := s.Latitude
    if lat == 0 {
        lat = 48.8584
    }
    lon := s.Longitude
    if lon == 0 {
        lon = 2.2945
    }
    title := s.Title
    if title == "" {
        title = "Test Venue"
    }
    address := s.Address
    if address == "" {
        address = "Paris, France"
    }

    msg, err := rt.Sender.SendVenue(ctx, rt.ChatID, lat, lon, title, address)
    if err != nil {
        return nil, err
    }

    rt.LastMessage = msg
    rt.TrackMessage(rt.ChatID, msg.MessageID)

    return &StepResult{
        Method:     "sendVenue",
        MessageIDs: []int{msg.MessageID},
        Evidence: map[string]any{
            "message_id": msg.MessageID,
            "title":      title,
            "address":    address,
        },
    }, nil
}
```

### 4.2 Misc Steps (S27, S29-S30)

**File:** `cmd/galigo-testbot/engine/steps_misc.go`

```go
package engine

import (
    "context"
    "fmt"
)

// SendContactStep sends a phone contact.
type SendContactStep struct {
    PhoneNumber string
    FirstName   string
    LastName    string
}

func (s *SendContactStep) Name() string { return "sendContact" }

func (s *SendContactStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    phone := s.PhoneNumber
    if phone == "" {
        phone = "+10000000000"
    }
    firstName := s.FirstName
    if firstName == "" {
        firstName = "Galigo"
    }
    lastName := s.LastName
    if lastName == "" {
        lastName = "Test"
    }

    msg, err := rt.Sender.SendContact(ctx, rt.ChatID, phone, firstName, lastName)
    if err != nil {
        return nil, err
    }

    rt.LastMessage = msg
    rt.TrackMessage(rt.ChatID, msg.MessageID)

    return &StepResult{
        Method:     "sendContact",
        MessageIDs: []int{msg.MessageID},
        Evidence: map[string]any{
            "message_id":   msg.MessageID,
            "phone_number": phone,
            "first_name":   firstName,
        },
    }, nil
}

// SendDiceStep sends an animated dice/emoji.
type SendDiceStep struct {
    Emoji string // 🎲 🎯 🏀 ⚽ 🎳 🎰
}

func (s *SendDiceStep) Name() string { return "sendDice" }

func (s *SendDiceStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    emoji := s.Emoji
    if emoji == "" {
        emoji = "🎲"
    }

    msg, err := rt.Sender.SendDice(ctx, rt.ChatID, emoji)
    if err != nil {
        return nil, err
    }

    rt.LastMessage = msg
    rt.TrackMessage(rt.ChatID, msg.MessageID)

    var value int
    if msg.Dice != nil {
        value = msg.Dice.Value
    }

    return &StepResult{
        Method:     "sendDice",
        MessageIDs: []int{msg.MessageID},
        Evidence: map[string]any{
            "message_id": msg.MessageID,
            "emoji":      emoji,
            "value":      value,
        },
    }, nil
}

// SetMessageReactionStep sets emoji reaction on last message.
type SetMessageReactionStep struct {
    Emoji string
    IsBig bool
}

func (s *SetMessageReactionStep) Name() string { return "setMessageReaction" }

func (s *SetMessageReactionStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    if rt.LastMessage == nil {
        return nil, fmt.Errorf("no message to react to")
    }

    emoji := s.Emoji
    if emoji == "" {
        emoji = "👍"
    }

    err := rt.Sender.SetMessageReaction(ctx, rt.ChatID, rt.LastMessage.MessageID, emoji, s.IsBig)
    if err != nil {
        return nil, err
    }

    return &StepResult{
        Method: "setMessageReaction",
        Evidence: map[string]any{
            "message_id": rt.LastMessage.MessageID,
            "emoji":      emoji,
            "is_big":     s.IsBig,
        },
    }, nil
}

// GetUserProfilePhotosStep gets profile photos of a user.
type GetUserProfilePhotosStep struct {
    UserID int64 // If 0, uses AdminUserID
}

func (s *GetUserProfilePhotosStep) Name() string { return "getUserProfilePhotos" }

func (s *GetUserProfilePhotosStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    userID := s.UserID
    if userID == 0 {
        userID = rt.AdminUserID
    }
    if userID == 0 {
        return nil, Skip("no user ID available for getUserProfilePhotos")
    }

    photos, err := rt.Sender.GetUserProfilePhotos(ctx, userID, 0, 10)
    if err != nil {
        return nil, err
    }

    return &StepResult{
        Method: "getUserProfilePhotos",
        Evidence: map[string]any{
            "user_id":     userID,
            "total_count": photos.TotalCount,
            "photo_count": len(photos.Photos),
        },
    }, nil
}

// GetUserChatBoostsStep gets user's boosts in the chat.
type GetUserChatBoostsStep struct{}

func (s *GetUserChatBoostsStep) Name() string { return "getUserChatBoosts" }

func (s *GetUserChatBoostsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    if rt.AdminUserID == 0 {
        return nil, Skip("AdminUserID not set")
    }

    boosts, err := rt.Sender.GetUserChatBoosts(ctx, rt.ChatID, rt.AdminUserID)
    if err != nil {
        return nil, err
    }

    return &StepResult{
        Method: "getUserChatBoosts",
        Evidence: map[string]any{
            "boost_count": len(boosts.Boosts),
        },
    }, nil
}
```

### 4.3 Bulk Operations Steps (S28) - Consultant 6's StepSeedMessages

**File:** `cmd/galigo-testbot/engine/steps_bulk.go`

```go
package engine

import (
    "context"
    "fmt"
)

// SeedMessagesStep sends N messages and stores their IDs (Consultant 6).
type SeedMessagesStep struct {
    Count int // Default: 3
}

func (s *SeedMessagesStep) Name() string { return "seedMessages" }

func (s *SeedMessagesStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    count := s.Count
    if count == 0 {
        count = 3
    }

    // Reset bulk IDs
    rt.BulkMessageIDs = make([]int, 0, count)

    for i := 1; i <= count; i++ {
        msg, err := rt.Sender.SendMessage(ctx, rt.ChatID, 
            fmt.Sprintf("Bulk test message %d/%d", i, count))
        if err != nil {
            return nil, fmt.Errorf("seedMessages %d/%d: %w", i, count, err)
        }
        rt.BulkMessageIDs = append(rt.BulkMessageIDs, msg.MessageID)
        rt.TrackMessage(rt.ChatID, msg.MessageID)
    }

    return &StepResult{
        Method:     "sendMessage",
        MessageIDs: rt.BulkMessageIDs,
        Evidence: map[string]any{
            "seeded_count": count,
            "message_ids":  rt.BulkMessageIDs,
        },
    }, nil
}

// ForwardMessagesStep forwards messages from BulkMessageIDs.
type ForwardMessagesStep struct{}

func (s *ForwardMessagesStep) Name() string { return "forwardMessages" }

func (s *ForwardMessagesStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    if len(rt.BulkMessageIDs) == 0 {
        return nil, fmt.Errorf("no bulk message IDs - run SeedMessagesStep first")
    }

    result, err := rt.Sender.ForwardMessages(ctx, rt.ChatID, rt.ChatID, rt.BulkMessageIDs)
    if err != nil {
        return nil, err
    }

    var newIDs []int
    for _, msgID := range result {
        newIDs = append(newIDs, msgID.MessageID)
        rt.TrackMessage(rt.ChatID, msgID.MessageID)
    }

    return &StepResult{
        Method:     "forwardMessages",
        MessageIDs: newIDs,
        Evidence: map[string]any{
            "original_count":  len(rt.BulkMessageIDs),
            "forwarded_count": len(result),
        },
    }, nil
}

// CopyMessagesStep copies messages from BulkMessageIDs.
type CopyMessagesStep struct{}

func (s *CopyMessagesStep) Name() string { return "copyMessages" }

func (s *CopyMessagesStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    if len(rt.BulkMessageIDs) == 0 {
        return nil, fmt.Errorf("no bulk message IDs - run SeedMessagesStep first")
    }

    result, err := rt.Sender.CopyMessages(ctx, rt.ChatID, rt.ChatID, rt.BulkMessageIDs)
    if err != nil {
        return nil, err
    }

    var newIDs []int
    for _, msgID := range result {
        newIDs = append(newIDs, msgID.MessageID)
        rt.TrackMessage(rt.ChatID, msgID.MessageID)
    }

    return &StepResult{
        Method:     "copyMessages",
        MessageIDs: newIDs,
        Evidence: map[string]any{
            "original_count": len(rt.BulkMessageIDs),
            "copied_count":   len(result),
        },
    }, nil
}

// DeleteMessagesStep deletes all tracked messages at once.
type DeleteMessagesStep struct{}

func (s *DeleteMessagesStep) Name() string { return "deleteMessages" }

func (s *DeleteMessagesStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    // Collect all message IDs for this chat
    var msgIDs []int
    for _, cm := range rt.CreatedMessages {
        if cm.ChatID == rt.ChatID {
            msgIDs = append(msgIDs, cm.MessageID)
        }
    }

    if len(msgIDs) == 0 {
        return nil, fmt.Errorf("no messages to delete")
    }

    err := rt.Sender.DeleteMessages(ctx, rt.ChatID, msgIDs)
    if err != nil {
        return nil, err
    }

    // Clear tracked messages for this chat
    newTracked := make([]CreatedMessage, 0)
    for _, cm := range rt.CreatedMessages {
        if cm.ChatID != rt.ChatID {
            newTracked = append(newTracked, cm)
        }
    }
    rt.CreatedMessages = newTracked
    rt.BulkMessageIDs = nil

    return &StepResult{
        Method: "deleteMessages",
        Evidence: map[string]any{
            "deleted_count": len(msgIDs),
            "message_ids":   msgIDs,
        },
    }, nil
}
```

### 4.4 Chat Settings Steps (S31-S32)

**File:** `cmd/galigo-testbot/engine/steps_chat_settings.go`

```go
package engine

import (
    "bytes"
    "context"
    "fmt"

    "github.com/prilive-com/galigo/tg"
)

// ================= Chat Photo Steps (S31) =================

// SaveChatPhotoStep saves current photo for restore (Consultant 6).
type SaveChatPhotoStep struct{}

func (s *SaveChatPhotoStep) Name() string { return "saveChatPhoto" }

func (s *SaveChatPhotoStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    // Check permission
    if err := RequireCanChangeInfo(ctx, rt); err != nil {
        return nil, err
    }

    chat, err := rt.Sender.GetChat(ctx, rt.ChatID)
    if err != nil {
        return nil, err
    }

    rt.OriginalChatPhoto = &ChatPhotoSnapshot{HadPhoto: false}

    if chat.Photo != nil && chat.Photo.BigFileID != "" {
        // Get file info
        file, err := rt.Sender.GetFile(ctx, chat.Photo.BigFileID)
        if err != nil {
            return nil, fmt.Errorf("getFile for chat photo: %w", err)
        }

        // Download the file
        photoBytes, err := DownloadFileBytes(rt.Token, file.FilePath)
        if err != nil {
            return nil, fmt.Errorf("download chat photo: %w", err)
        }

        rt.OriginalChatPhoto = &ChatPhotoSnapshot{
            HadPhoto: true,
            Bytes:    photoBytes,
            MimeType: "image/jpeg",
        }
    }

    return &StepResult{
        Method: "getChat",
        Evidence: map[string]any{
            "had_photo": rt.OriginalChatPhoto.HadPhoto,
        },
    }, nil
}

// SetChatPhotoStep sets chat photo using inline PNG (Consultants 1, 5).
type SetChatPhotoStep struct {
    UseFixture bool   // If true, use fixtures/chat_photo.jpg
    FixturePath string
}

func (s *SetChatPhotoStep) Name() string { return "setChatPhoto" }

func (s *SetChatPhotoStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    if err := RequireCanChangeInfo(ctx, rt); err != nil {
        return nil, err
    }

    // Use inline MinimalPNG by default (no external dependency!)
    photoData := MinimalPNG

    err := rt.Sender.SetChatPhoto(ctx, rt.ChatID, bytes.NewReader(photoData))
    if err != nil {
        return nil, err
    }

    return &StepResult{
        Method: "setChatPhoto",
        Evidence: map[string]any{
            "photo_size": len(photoData),
        },
    }, nil
}

// RestoreChatPhotoStep restores original photo (Consultant 6).
type RestoreChatPhotoStep struct{}

func (s *RestoreChatPhotoStep) Name() string { return "restoreChatPhoto" }

func (s *RestoreChatPhotoStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    if rt.OriginalChatPhoto == nil {
        return nil, fmt.Errorf("no photo snapshot - run SaveChatPhotoStep first")
    }

    if !rt.OriginalChatPhoto.HadPhoto {
        // Delete the photo we set
        err := rt.Sender.DeleteChatPhoto(ctx, rt.ChatID)
        if err != nil {
            return nil, err
        }
        return &StepResult{
            Method: "deleteChatPhoto",
            Evidence: map[string]any{
                "action": "deleted (no original)",
            },
        }, nil
    }

    // Restore original
    err := rt.Sender.SetChatPhoto(ctx, rt.ChatID, bytes.NewReader(rt.OriginalChatPhoto.Bytes))
    if err != nil {
        return nil, err
    }

    return &StepResult{
        Method: "setChatPhoto",
        Evidence: map[string]any{
            "action": "restored original",
        },
    }, nil
}

// ================= Chat Permissions Steps (S32) =================

// SaveChatPermissionsStep saves current permissions.
type SaveChatPermissionsStep struct{}

func (s *SaveChatPermissionsStep) Name() string { return "saveChatPermissions" }

func (s *SaveChatPermissionsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    if err := RequireCanRestrict(ctx, rt); err != nil {
        return nil, err
    }

    chat, err := rt.Sender.GetChat(ctx, rt.ChatID)
    if err != nil {
        return nil, err
    }

    rt.OriginalPermissions = &PermissionsSnapshot{
        Permissions: chat.Permissions,
    }

    return &StepResult{
        Method: "getChat",
        Evidence: map[string]any{
            "saved_permissions": chat.Permissions != nil,
        },
    }, nil
}

// SetChatPermissionsTemporaryStep toggles a low-risk permission.
type SetChatPermissionsTemporaryStep struct {
    DisableWebPagePreviews bool
}

func (s *SetChatPermissionsTemporaryStep) Name() string { return "setChatPermissions" }

func (s *SetChatPermissionsTemporaryStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    if err := RequireCanRestrict(ctx, rt); err != nil {
        return nil, err
    }

    // Keep most things enabled - only toggle web previews
    perms := tg.ChatPermissions{
        CanSendMessages:       true,
        CanSendMediaMessages:  true,
        CanSendPolls:          true,
        CanSendOtherMessages:  true,
        CanAddWebPagePreviews: !s.DisableWebPagePreviews,
        CanInviteUsers:        true,
    }

    err := rt.Sender.SetChatPermissions(ctx, rt.ChatID, perms)
    if err != nil {
        return nil, err
    }

    return &StepResult{
        Method: "setChatPermissions",
        Evidence: map[string]any{
            "can_add_web_page_previews": perms.CanAddWebPagePreviews,
        },
    }, nil
}

// RestoreChatPermissionsStep restores original permissions (Consultant 5 t.Cleanup pattern).
type RestoreChatPermissionsStep struct{}

func (s *RestoreChatPermissionsStep) Name() string { return "restoreChatPermissions" }

func (s *RestoreChatPermissionsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    if rt.OriginalPermissions == nil || rt.OriginalPermissions.Permissions == nil {
        // Set sensible defaults
        perms := tg.ChatPermissions{
            CanSendMessages:       true,
            CanSendMediaMessages:  true,
            CanSendPolls:          true,
            CanSendOtherMessages:  true,
            CanAddWebPagePreviews: true,
            CanInviteUsers:        true,
        }
        err := rt.Sender.SetChatPermissions(ctx, rt.ChatID, perms)
        if err != nil {
            return nil, err
        }
        return &StepResult{
            Method: "setChatPermissions",
            Evidence: map[string]any{
                "action": "restored defaults",
            },
        }, nil
    }

    err := rt.Sender.SetChatPermissions(ctx, rt.ChatID, *rt.OriginalPermissions.Permissions)
    if err != nil {
        return nil, err
    }

    return &StepResult{
        Method: "setChatPermissions",
        Evidence: map[string]any{
            "action": "restored original",
        },
    }, nil
}
```

---

## Part 5: Scenarios (S25-S32)

**File:** `cmd/galigo-testbot/suites/extras.go`

```go
package suites

import (
    "github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// S25_GeoLocation tests sendLocation.
func S25_GeoLocation() engine.Scenario {
    return engine.NewScenario("S25_GeoLocation",
        "Tests sendLocation",
        []string{"sendLocation"},
        []engine.Step{
            &engine.SendLocationStep{},
            &engine.CleanupStep{},
        },
    )
}

// S26_GeoVenue tests sendVenue.
func S26_GeoVenue() engine.Scenario {
    return engine.NewScenario("S26_GeoVenue",
        "Tests sendVenue",
        []string{"sendVenue"},
        []engine.Step{
            &engine.SendVenueStep{
                Title:   "Eiffel Tower",
                Address: "Paris, France",
            },
            &engine.CleanupStep{},
        },
    )
}

// S27_ContactAndDice tests sendContact and sendDice.
func S27_ContactAndDice() engine.Scenario {
    return engine.NewScenario("S27_ContactAndDice",
        "Tests sendContact and sendDice",
        []string{"sendContact", "sendDice"},
        []engine.Step{
            &engine.SendContactStep{},
            &engine.SendDiceStep{Emoji: "🎲"},
            &engine.SendDiceStep{Emoji: "🎯"},
            &engine.CleanupStep{},
        },
    )
}

// S28_BulkOps tests bulk message operations.
func S28_BulkOps() engine.Scenario {
    return engine.NewScenario("S28_BulkOps",
        "Tests forwardMessages, copyMessages, deleteMessages",
        []string{"forwardMessages", "copyMessages", "deleteMessages"},
        []engine.Step{
            &engine.SeedMessagesStep{Count: 3},
            &engine.ForwardMessagesStep{},
            &engine.CopyMessagesStep{},
            &engine.DeleteMessagesStep{}, // Bulk delete all
        },
    )
}

// S29_Reactions tests setMessageReaction.
func S29_Reactions() engine.Scenario {
    return engine.NewScenario("S29_Reactions",
        "Tests setMessageReaction",
        []string{"setMessageReaction"},
        []engine.Step{
            &engine.SendMessageStep{Text: "React to this!"},
            &engine.SetMessageReactionStep{Emoji: "👍"},
            &engine.SetMessageReactionStep{Emoji: "❤️"},
            &engine.CleanupStep{},
        },
    )
}

// S30_UserPhotos tests user info methods.
func S30_UserPhotos() engine.Scenario {
    return engine.NewScenario("S30_UserPhotos",
        "Tests getUserProfilePhotos, getUserChatBoosts",
        []string{"getUserProfilePhotos", "getUserChatBoosts"},
        []engine.Step{
            &engine.GetUserProfilePhotosStep{},
            &engine.GetUserChatBoostsStep{},
        },
    )
}

// S31_ChatPhotoLifecycle tests setChatPhoto, deleteChatPhoto.
func S31_ChatPhotoLifecycle() engine.Scenario {
    return engine.NewScenario("S31_ChatPhotoLifecycle",
        "Tests chat photo lifecycle (requires admin)",
        []string{"setChatPhoto", "deleteChatPhoto"},
        []engine.Step{
            &engine.SaveChatPhotoStep{},
            &engine.SetChatPhotoStep{},
            &engine.RestoreChatPhotoStep{},
        },
    )
}

// S32_ChatPermissionsLifecycle tests setChatPermissions.
func S32_ChatPermissionsLifecycle() engine.Scenario {
    return engine.NewScenario("S32_ChatPermissionsLifecycle",
        "Tests chat permissions lifecycle (requires admin)",
        []string{"setChatPermissions"},
        []engine.Step{
            &engine.SaveChatPermissionsStep{},
            &engine.SetChatPermissionsTemporaryStep{DisableWebPagePreviews: true},
            &engine.RestoreChatPermissionsStep{},
        },
    )
}

// AllExtrasScenarios returns all extras scenarios.
func AllExtrasScenarios() []engine.Scenario {
    return []engine.Scenario{
        S25_GeoLocation(),
        S26_GeoVenue(),
        S27_ContactAndDice(),
        S28_BulkOps(),
        S29_Reactions(),
        S30_UserPhotos(),
        S31_ChatPhotoLifecycle(),
        S32_ChatPermissionsLifecycle(),
    }
}
```

---

## Part 6: Suite Routing

**File:** `cmd/galigo-testbot/main.go` (update switch)

```go
case "geo":
    scenarios = []engine.Scenario{suites.S25_GeoLocation()}
case "venue":
    scenarios = []engine.Scenario{suites.S26_GeoVenue()}
case "contact-dice":
    scenarios = []engine.Scenario{suites.S27_ContactAndDice()}
case "bulk":
    scenarios = []engine.Scenario{suites.S28_BulkOps()}
case "reactions":
    scenarios = []engine.Scenario{suites.S29_Reactions()}
case "user-photos":
    scenarios = []engine.Scenario{suites.S30_UserPhotos()}
case "chat-photo":
    scenarios = []engine.Scenario{suites.S31_ChatPhotoLifecycle()}
case "chat-permissions":
    scenarios = []engine.Scenario{suites.S32_ChatPermissionsLifecycle()}
case "extras":
    scenarios = suites.AllExtrasScenarios()
case "all":
    scenarios = append(suites.AllPhaseAScenarios(), suites.AllPhaseBScenarios()...)
    scenarios = append(scenarios, suites.AllPhaseCScenarios()...)
    scenarios = append(scenarios, suites.AllChatAdminScenarios()...)
    scenarios = append(scenarios, suites.AllStickerScenarios()...)
    scenarios = append(scenarios, suites.AllStarsScenarios()...)
    scenarios = append(scenarios, suites.AllGiftScenarios()...)
    scenarios = append(scenarios, suites.AllExtrasScenarios()...) // NEW
```

---

## Part 7: PR Breakdown (Final)

| PR | Description | Files | Impact |
|----|-------------|-------|--------|
| **PR1** | SKIP framework + ChatContext + RequireX | errors.go, runtime.go, require.go, runner.go | Foundation |
| **PR2** | Fixtures + Helpers | fixtures.go, helpers.go | Utilities |
| **PR3** | SenderClient interface additions | scenario.go, adapter.go | Interface |
| **PR4** | Geo steps (S25-S26) | steps_geo.go | +2 methods |
| **PR5** | Misc steps (S27, S29-S30) | steps_misc.go | +5 methods |
| **PR6** | Bulk steps (S28) | steps_bulk.go | +3 methods |
| **PR7** | Chat settings steps (S31-S32) | steps_chat_settings.go | +3 methods |
| **PR8** | Suites + routing | suites/extras.go, main.go | Scenarios |

---

## Part 8: Expected Coverage

| After | Coverage | Methods |
|-------|----------|---------|
| Current | 41% | 51/123 |
| PR1-8 | **53%** | 65/123 |
| + Forum topics | **63%** | 77/123 |
| + Moderation | **70%** | 86/123 |

---

## Part 9: Environment Variables

```bash
# Existing (required)
TESTBOT_TOKEN=...
TESTBOT_CHAT_ID=...
TESTBOT_ADMINS=...

# New (optional)
TESTBOT_FORUM_CHAT_ID=...        # For forum topic tests
TESTBOT_TEST_USER_ID=...         # For moderation tests
TESTBOT_ALLOW_DESTRUCTIVE=false  # Enable ban/unban tests
```

---

## Summary: Best From All 6 Sources

| Contribution | Source |
|--------------|--------|
| 5-phase method categorization | Original |
| GitHub Environments emphasis | Consultant 1 |
| `ErrSkipped` + `ChatContext` + `RequireX` | Consultant 2 |
| SBOM + `step-security/harden-runner` | Consultant 3 |
| `t.Cleanup()` pattern + inline 1x1 PNG | Consultant 5 |
| Exact file layout + `SenderClient` interface + `StepSeedMessages` | Consultant 6 |
| `DownloadFileBytes` helper | Consultant 6 |
| `BulkMessageIDs` accumulator | Consultants 5, 6 |
| Scenario IDs S25-S32 | Consultants 2, 6 |

---

## Quick Start

```bash
# 1. Implement PR1 (SKIP framework)
# 2. Implement PR4-8 (steps + suites)
# 3. Run:
go run ./cmd/galigo-testbot --run extras

# 4. Validate JSON report shows PASSED/SKIPPED (not FAILED)
```

**Total implementation time:** ~8-12 hours for full extras suite