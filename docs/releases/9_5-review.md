# Bot API 9.5 Implementation Plan — Code Review

**Reviewed by**: galigo maintainer
**Plan version**: v2 (`9_5-implementation.md`)
**Review date**: 2026-03-08
**Verdict**: **Reject — 21 issues found, 6 will not compile, 5 break codebase conventions**

The plan's TDD approach and feature coverage are solid (verified against the [official changelog](https://core.telegram.org/bots/api-changelog)). But it has critical errors from incorrect function signatures, wrong file paths, and API patterns that don't match the existing codebase. All 21 issues are documented below with corrected code.

---

## Table of Contents

- [Category C: Will Not Compile (6 issues)](#category-c-will-not-compile)
- [Category D: Breaks Codebase Conventions (5 issues)](#category-d-breaks-codebase-conventions)
- [Category M: Missing Items (7 issues)](#category-m-missing-items)
- [Category N: Minor Nits (3 issues)](#category-n-minor-nits)
- [Summary of Correct Patterns](#summary-of-correct-patterns)

---

## Category C: Will Not Compile

### C1. Wrong file paths — 3 files don't exist

| Plan says | Actual location |
|---|---|
| `tg/entity.go` | `tg/types.go` (MessageEntity at line 135) |
| `tg/message.go` | `tg/types.go` (Message at line 22) |
| `tg/admin_rights.go` | `tg/chat_admin_rights.go` |

The plan references `tg/entity.go` for Phase 1A/1B, `tg/message.go` for Phase 1C, and `tg/admin_rights.go` for Phase 1H. None of these files exist. There is no `entity.go` file — all entity types are in `tg/types.go`. There is no separate `message.go` — `Message` struct is also in `tg/types.go`. The admin rights file is named `tg/chat_admin_rights.go`.

**Note**: Phase 3E proposes creating `tg/datetime.go` as a new file — that's fine. But the existing types must be modified in their actual locations.

---

### C2. `doJSON[bool]` doesn't exist

**Plan (Phase 3B, line 691)**:
```go
_, err := doJSON[bool](ctx, c, "setChatMemberTag", req)
```

**Actual codebase pattern** (`sender/call.go:19`):
```go
func (c *Client) callJSON(ctx context.Context, method string, payload any, out any, chatIDs ...string) error
```

There is no generic `doJSON` function anywhere in the sender package. The codebase uses `c.callJSON(ctx, method, req, nil)` for methods that return `bool` (pass `nil` for `out`).

**Corrected code**:
```go
return c.callJSON(ctx, "setChatMemberTag", req, nil, extractChatID(chatID))
```

This applies to **both** `SetChatMemberTag` (Phase 3B) and `SendMessageDraft` (Phase 3C).

---

### C3. `testutil.ReadBody` doesn't exist

**Plan (Phase 2E, line 500)**:
```go
body := testutil.ReadBody(t, r)
assert.Contains(t, body, `"tag":"Team Lead"`)
```

This function is not in `internal/testutil/`. No `ReadBody` helper exists. The existing test pattern (see `sender/chat_moderation_test.go:35`, `sender/chat_admin_test.go:24`) is:

```go
var req map[string]any
json.NewDecoder(r.Body).Decode(&req)
assert.Equal(t, "Team Lead", req["tag"])
```

This affects **all 5 test functions** in Phase 2E and 2F that use `ReadBody`.

---

### C4. `testutil.ReplyOK(w)` — wrong arity

**Plan (Phase 2E, line 503)**:
```go
testutil.ReplyOK(w)
```

**Actual signature** (`internal/testutil/replies.go:25`):
```go
func ReplyOK(w http.ResponseWriter, result any)
```

`ReplyOK` requires a second argument. For methods returning `bool`, the existing pattern is:

```go
testutil.ReplyBool(w, true)
```

This affects **all test handlers** in Phase 2E, 2F, and 2G that use `ReplyOK(w)`.

---

### C5. `testutil.NewTestClient(t, server)` — wrong type

**Plan (Phase 2E, line 505)**:
```go
client := testutil.NewTestClient(t, server)
```

**Actual signature** (`internal/testutil/client.go:79`):
```go
func NewTestClient(t *testing.T, baseURL string, opts ...sender.Option) *sender.Client
```

The second argument is `string` (the base URL), not `*MockTelegramServer`. The existing pattern across all test files is:

```go
client := testutil.NewTestClient(t, server.BaseURL())
```

This affects **every test function** in Phase 2E, 2F, and 2G.

---

### C6. Wrong package qualifiers in test code

The plan's Phase 2B/2C/2H tests use `tg.` prefix but append to existing files that use `package tg` (not `package tg_test`):

| Existing file | Package | Plan uses |
|---|---|---|
| `tg/chat_member_test.go` | `package tg` | `tg.ChatMemberMember` (wrong) |
| `tg/chat_permissions_test.go` | `package tg` | `tg.ChatPermissions` (wrong) |

**Example (Phase 2B, line 385)**:
```go
// WRONG — this file is package tg, not package tg_test
var m tg.ChatMemberMember
```

**Corrected**:
```go
// Correct — no tg. prefix needed since we're inside package tg
var m ChatMemberMember
```

The new `tg/entity_test.go` file (Phase 2A) is fine IF it declares `package tg_test` and imports `tg`. But the tests for `chat_member_test.go` and `chat_permissions_test.go` must match their existing `package tg` declaration — meaning no `tg.` prefix on types and no `tg` import.

---

## Category D: Breaks Codebase Conventions

### D1. `SetChatMemberTag` passes raw request struct — breaks the moderation API pattern

**Plan (Phase 3B)**:
```go
func (c *Client) SetChatMemberTag(ctx context.Context, req SetChatMemberTagRequest) error
```

**Existing pattern** — every moderation method uses decomposed parameters:
```go
// sender/chat_moderation.go
func (c *Client) BanChatMember(ctx context.Context, chatID tg.ChatID, userID int64, opts ...BanOption) error
func (c *Client) UnbanChatMember(ctx context.Context, chatID tg.ChatID, userID int64, opts ...UnbanOption) error
func (c *Client) RestrictChatMember(ctx context.Context, chatID tg.ChatID, userID int64, perms tg.ChatPermissions, opts ...RestrictOption) error

// sender/chat_admin.go
func (c *Client) PromoteChatMember(ctx context.Context, chatID tg.ChatID, userID int64, opts ...PromoteOption) error
func (c *Client) SetChatAdministratorCustomTitle(ctx context.Context, chatID tg.ChatID, userID int64, customTitle string) error
```

The tag is a single string value with no optional parameters, so this is even simpler than the others — no options pattern needed.

**Corrected signature**:
```go
func (c *Client) SetChatMemberTag(ctx context.Context, chatID tg.ChatID, userID int64, tag string) error
```

**Corrected implementation**:
```go
// SetChatMemberTag sets or removes a custom tag for a chat member.
// Pass an empty string to remove the tag.
// The bot must be an administrator with can_manage_tags right.
// Tag must be 0-16 characters.
func (c *Client) SetChatMemberTag(ctx context.Context, chatID tg.ChatID, userID int64, tag string) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    if err := validateUserID(userID); err != nil {
        return err
    }
    if len(tag) > 16 {
        return tg.NewValidationError("tag", "must be at most 16 characters")
    }

    return c.callJSON(ctx, "setChatMemberTag", SetChatMemberTagRequest{
        ChatID: chatID,
        UserID: userID,
        Tag:    tag,
    }, nil, extractChatID(chatID))
}
```

**Corrected request struct**:
```go
type SetChatMemberTagRequest struct {
    ChatID tg.ChatID `json:"chat_id"`
    UserID int64     `json:"user_id"`
    Tag    string    `json:"tag"` // NO omitempty — empty string removes tag
}
```

**Corrected test** (matching `sender/chat_admin_test.go` conventions):
```go
func TestSetChatMemberTag_SetTag(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/setChatMemberTag", func(w http.ResponseWriter, r *http.Request) {
        var req map[string]any
        json.NewDecoder(r.Body).Decode(&req)
        assert.Equal(t, "Team Lead", req["tag"])
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.SetChatMemberTag(context.Background(), int64(-1001234567890), int64(123456), "Team Lead")
    require.NoError(t, err)
}

func TestSetChatMemberTag_RemoveTag_EmptyStringNotOmitted(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/setChatMemberTag", func(w http.ResponseWriter, r *http.Request) {
        // Read raw body to verify "tag":"" is present (not omitted)
        cap := server.LastCapture()
        assert.Contains(t, string(cap.Body), `"tag":""`)
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.SetChatMemberTag(context.Background(), int64(-1001234567890), int64(123456), "")
    require.NoError(t, err)
}

func TestSetChatMemberTag_TagTooLong_ValidationError(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    err := client.SetChatMemberTag(context.Background(), int64(-1001234567890), int64(123456), "This Is Way Too Long")
    require.Error(t, err)
    assert.Contains(t, err.Error(), "16 characters")
    assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestSetChatMemberTag_Validation_InvalidChatID(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    err := client.SetChatMemberTag(context.Background(), nil, int64(123456), "test")
    require.Error(t, err)
    assert.Contains(t, err.Error(), "chat_id")
    assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestSetChatMemberTag_Validation_InvalidUserID(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    err := client.SetChatMemberTag(context.Background(), int64(-1001234567890), 0, "test")
    require.Error(t, err)
    assert.Contains(t, err.Error(), "user_id")
    assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}
```

---

### D2. `SendMessageDraft` API design

The plan proposes passing a raw request struct:
```go
func (c *Client) SendMessageDraft(ctx context.Context, req SendMessageDraftRequest) error
```

For `sendMessageDraft`, passing a struct is more defensible than for `SetChatMemberTag` because the method has multiple optional fields (parse_mode, entities, message_thread_id). The existing `SendMessage` also uses a request struct in `sender/requests.go`. However, two issues remain:

**Issue 1: `ChatID int64` should be `tg.ChatID`** — Every request struct in the codebase uses `tg.ChatID`:
```go
// sender/requests.go — all use tg.ChatID
type SendMessageRequest struct {
    ChatID tg.ChatID `json:"chat_id"`
type SendPhotoRequest struct {
    ChatID tg.ChatID `json:"chat_id"`
type ForwardMessageRequest struct {
    ChatID tg.ChatID `json:"chat_id"`
```

If `sendMessageDraft` truly only accepts integer chat IDs (no usernames), add client-side validation for that — don't change the type.

**Issue 2: `ParseMode string` should be `tg.ParseMode`** — Every request struct uses the named type.

**Corrected request struct**:
```go
// SendMessageDraftRequest represents a sendMessageDraft request.
// Added in Bot API 9.5.
type SendMessageDraftRequest struct {
    ChatID          tg.ChatID          `json:"chat_id"`
    DraftID         int                `json:"draft_id"`           // Must be non-zero
    Text            string             `json:"text"`
    MessageThreadID int                `json:"message_thread_id,omitempty"`
    ParseMode       tg.ParseMode       `json:"parse_mode,omitempty"`
    Entities        []tg.MessageEntity `json:"entities,omitempty"`
}
```

**Corrected implementation**:
```go
// SendMessageDraft sends a draft message for streaming.
// Call repeatedly with the same DraftID to update the draft text.
// When complete, call SendMessage to finalize.
// DraftID must be non-zero. Text is required.
func (c *Client) SendMessageDraft(ctx context.Context, req SendMessageDraftRequest) error {
    if err := validateChatID(req.ChatID); err != nil {
        return err
    }
    if req.DraftID == 0 {
        return tg.NewValidationError("draft_id", "must be non-zero")
    }
    if req.Text == "" {
        return tg.NewValidationError("text", "is required")
    }

    return c.callJSON(ctx, "sendMessageDraft", req, nil, extractChatID(req.ChatID))
}
```

**Corrected tests**:
```go
func TestSendMessageDraft_BasicRequest(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/sendMessageDraft", func(w http.ResponseWriter, r *http.Request) {
        var req map[string]any
        json.NewDecoder(r.Body).Decode(&req)
        // chat_id should be numeric (not quoted string)
        assert.Equal(t, float64(123456789), req["chat_id"])
        assert.Equal(t, float64(42), req["draft_id"])
        assert.Equal(t, "Generating...", req["text"])
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.SendMessageDraft(context.Background(), sender.SendMessageDraftRequest{
        ChatID:  int64(123456789),
        DraftID: 42,
        Text:    "Generating...",
    })
    require.NoError(t, err)
}

func TestSendMessageDraft_WithParseMode(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/sendMessageDraft", func(w http.ResponseWriter, r *http.Request) {
        var req map[string]any
        json.NewDecoder(r.Body).Decode(&req)
        assert.Equal(t, "HTML", req["parse_mode"])
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.SendMessageDraft(context.Background(), sender.SendMessageDraftRequest{
        ChatID:    int64(123456789),
        DraftID:   1,
        Text:      "<b>Bold</b>",
        ParseMode: tg.ParseModeHTML,
    })
    require.NoError(t, err)
}

func TestSendMessageDraft_DraftIDZero_ValidationError(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    err := client.SendMessageDraft(context.Background(), sender.SendMessageDraftRequest{
        ChatID:  int64(123456789),
        DraftID: 0,
        Text:    "test",
    })
    require.Error(t, err)
    assert.Contains(t, err.Error(), "draft_id")
    assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestSendMessageDraft_EmptyText_ValidationError(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    err := client.SendMessageDraft(context.Background(), sender.SendMessageDraftRequest{
        ChatID:  int64(123456789),
        DraftID: 1,
        Text:    "",
    })
    require.Error(t, err)
    assert.Contains(t, err.Error(), "text")
    assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}
```

---

### D3. Missing `validateChatID` / `validateUserID` calls

Every moderation method validates inputs before making HTTP calls:
```go
// sender/chat_moderation.go:53-58 (BanChatMember)
if err := validateChatID(chatID); err != nil {
    return err
}
if err := validateUserID(userID); err != nil {
    return err
}
```

The plan's `SetChatMemberTag` implementation (Phase 3B) skips both validators. The corrected code in D1 above includes them.

---

### D4. Missing `extractChatID` for per-chat rate limiting

Every `callJSON` call in the codebase passes `extractChatID(chatID)` as the last variadic argument:
```go
// sender/chat_moderation.go:68
return c.callJSON(ctx, "banChatMember", req, nil, extractChatID(chatID))

// sender/chat_admin.go:59
return c.callJSON(ctx, "promoteChatMember", req, nil, extractChatID(chatID))
```

The plan's implementations use `doJSON` which doesn't exist, and even if corrected to `callJSON`, they don't pass the chat ID for rate limiting. The corrected code in D1 and D2 includes this.

---

### D5. `WithCanManageTags` placed in wrong file

**Plan says**: `sender/options.go`

**Should be**: `sender/chat_admin.go` — where all other `PromoteOption` functions are defined (lines 143-230):
```go
// sender/chat_admin.go:146
type PromoteOption func(*PromoteChatMemberRequest)

// All WithCan* functions follow at lines 149-230:
func WithAnonymous(anonymous bool) PromoteOption { ... }
func WithCanManageChat(can bool) PromoteOption { ... }
func WithCanDeleteMessages(can bool) PromoteOption { ... }
// ... etc
```

`sender/options.go` contains `EditOption` and `ForwardOption` — unrelated option types.

---

## Category M: Missing Items

### M1. `AllPermissions()` / `NoPermissions()` not updated

Adding `CanEditTag *bool` to `ChatPermissions` without updating the 4 helper functions means these presets will silently omit the new permission.

**Files to update**: `tg/chat_permissions.go`

```go
// AllPermissions — add:
CanEditTag: boolPtr(true),

// NoPermissions — add:
CanEditTag: boolPtr(false),

// ReadOnlyPermissions — no change needed (read-only shouldn't include tag editing)
// TextOnlyPermissions — no change needed
```

---

### M2. `FullAdminRights()` not updated

Adding `CanManageTags` to `ChatAdministratorRights` without updating helper functions:

**File**: `tg/chat_admin_rights.go`

```go
// FullAdminRights — add:
CanManageTags: boolPtr(true),

// ModeratorRights — consider adding:
CanManageTags: boolPtr(true), // Moderators typically manage tags
```

---

### M3. `PromoteChatMemberWithRights` mapping incomplete

This method (`sender/chat_admin.go:64-93`) maps `ChatAdministratorRights` fields to `PromoteChatMemberRequest` fields one-by-one. Without adding the new field, promoting with `FullAdminRights()` won't grant `can_manage_tags`.

**Add to `sender/chat_admin.go:89`** (after the `CanManageTopics` line):
```go
CanManageTags:   rights.CanManageTags,
```

---

### M4. `DemoteChatMember` doesn't reset `CanManageTags`

`DemoteChatMember` (`sender/chat_admin.go:96-121`) explicitly sets all admin rights to `false`. Without including `CanManageTags`, an admin who had tag management rights won't lose them on demotion.

**Add to `sender/chat_admin.go:118`** (after `CanPinMessages`):
```go
CanManageTags: &f,
```

**Also update the test** `TestDemoteChatMember` (`sender/chat_admin_test.go:94-113`) to assert:
```go
assert.Equal(t, false, req["can_manage_tags"])
```

---

### M5. S45 scenario `UserID: 0` never gets resolved at runtime

**Plan (Phase 4B, line 947-965)**:
```go
ScenarioSteps: []engine.Step{
    &engine.SetChatMemberTagStep{
        UserID: 0, // filled from rt.AdminUserID at runtime  ← NEVER HAPPENS
        Tag:    "galigo-test",
    },
```

The comment says `UserID` is "filled from rt.AdminUserID at runtime" but no code does this. The `Execute` method uses `s.UserID` directly, which will be `0`. This will fail `validateUserID` (which requires positive values) or fail at the Telegram API.

**Two options to fix**:

**Option A** — Fill at execution time (recommended):
```go
func (s *SetChatMemberTagStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    userID := s.UserID
    if userID == 0 {
        userID = rt.AdminUserID // Fallback to the configured admin user
    }
    // ... use userID instead of s.UserID
```

**Option B** — Fill at scenario construction time (simpler but less reusable):
The scenario function would take `adminUserID int64` as a parameter.

---

### M6. `VerifyChatMemberTagStep` uses pointer type assertions on value types

`UnmarshalChatMember` returns value types, not pointers:
```go
// tg/chat_member.go:135-136
var m ChatMemberMember
result = m  // value, NOT pointer
```

**Plan (Phase 4A, line 894)**:
```go
case *tg.ChatMemberMember:    // WRONG — will never match
    actualTag = m.Tag
case *tg.ChatMemberRestricted: // WRONG — will never match
    actualTag = m.Tag
```

**Corrected**:
```go
case tg.ChatMemberMember:      // Correct — value type
    actualTag = m.Tag
case tg.ChatMemberRestricted:  // Correct — value type
    actualTag = m.Tag
```

---

### M7. Missing `ProbeChat` extension for `CanManageTags`

The testbot's `ChatContext` struct (`engine/scenario.go:74-88`) tracks bot capabilities for skip logic. Without `CanManageTags`, the S45 scenario can't check whether the bot has permission to manage tags and will fail ungracefully instead of skipping.

**Add to `ChatContext`**:
```go
CanManageTags bool
```

**Add to `ProbeChat`** (after the `CanManageTopics` block at `engine/scenario.go:188-190`):
```go
if admin.CanManageTags != nil {
    rt.ChatCtx.CanManageTags = *admin.CanManageTags
}
```

---

## Category N: Minor Nits

### N1. `ChatMemberAdministrator.CanManageTags` as `*bool` — correct but verify

The plan proposes `*bool` for this field, which follows the pattern of other optional admin capabilities (`CanManageTopics *bool`, `CanManageDirectMessages *bool`). This is correct since the Telegram API marks `can_manage_tags` as optional.

However, the plan's fixture 0H shows `can_manage_tags` as always-present (like `CanManageChat`), which would suggest `bool`. Verify against the actual API docs which is the canonical source. The `*bool` choice is safer for forward compatibility.

No code change needed — just flagging the discrepancy for awareness.

---

### N2. `SendMessageDraftRequest.ChatID int64` — intent is correct, type is wrong

The plan correctly identifies that `sendMessageDraft` only accepts integer chat IDs (no `@username` support). However, changing the struct field type to `int64` breaks the universal convention where every request struct uses `tg.ChatID` (which is `any`).

**Correct approach**: Keep `tg.ChatID` in the struct, add client-side validation in the method:
```go
func (c *Client) SendMessageDraft(ctx context.Context, req SendMessageDraftRequest) error {
    if err := validateChatID(req.ChatID); err != nil {
        return err
    }
    // Optionally: reject string chat IDs specifically
    if _, ok := req.ChatID.(string); ok {
        return tg.NewValidationError("chat_id", "sendMessageDraft only accepts numeric chat IDs")
    }
    // ...
}
```

---

### N3. `BottomButton.IconCustomEmojiID` omitted

The official Bot API 9.5 changelog lists the addition of `IconCustomEmojiID` to the `BottomButton` class (WebApps). The plan omits this entirely. If WebApps support is out of scope for this release, it should be explicitly documented as deferred.

---

## Summary of Correct Patterns

Quick reference for the consultant — these are the actual codebase patterns to follow:

### Test file setup
```go
package sender_test

import (
    "context"
    "encoding/json"
    "net/http"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/prilive-com/galigo/internal/testutil"
    "github.com/prilive-com/galigo/sender"
    "github.com/prilive-com/galigo/tg"
)
```

### Mock server + client setup
```go
server := testutil.NewMockServer(t)
server.On("/bot"+testutil.TestToken+"/methodName", func(w http.ResponseWriter, r *http.Request) {
    var req map[string]any
    json.NewDecoder(r.Body).Decode(&req)
    assert.Equal(t, expectedValue, req["field"])
    testutil.ReplyBool(w, true) // For bool-returning methods
})

client := testutil.NewTestClient(t, server.BaseURL())
```

### Validation test pattern
```go
func TestMethod_Validation_InvalidChatID(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    err := client.Method(context.Background(), nil, /* other args */)
    require.Error(t, err)
    assert.Contains(t, err.Error(), "chat_id")
    assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}
```

### Method implementation pattern
```go
func (c *Client) Method(ctx context.Context, chatID tg.ChatID, userID int64, ...) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    if err := validateUserID(userID); err != nil {
        return err
    }
    // Business validation
    if someField > limit {
        return tg.NewValidationError("field_name", "must be at most N characters")
    }

    return c.callJSON(ctx, "methodName", RequestStruct{...}, nil, extractChatID(chatID))
}
```

### PromoteOption pattern
```go
// In sender/chat_admin.go (NOT sender/options.go)
func WithCanSomething(can bool) PromoteOption {
    return func(r *PromoteChatMemberRequest) {
        r.CanSomething = &can
    }
}
```

### Pointer vs value types in type assertions
```go
// UnmarshalChatMember returns VALUE types, not pointers:
member, err := client.GetChatMember(ctx, chatID, userID)
switch m := member.(type) {
case tg.ChatMemberMember:       // VALUE — no asterisk
    // m.Tag
case tg.ChatMemberRestricted:   // VALUE — no asterisk
    // m.Tag
case tg.ChatMemberAdministrator: // VALUE — no asterisk
    // m.CanManageTags
}
```

### `tg/` package test files
```
chat_member_test.go     → package tg      → use ChatMemberMember (no prefix)
chat_permissions_test.go → package tg      → use ChatPermissions (no prefix)
types_test.go           → package tg_test  → use tg.Message (with prefix)
gifts_test.go           → package tg_test  → use tg.Gift (with prefix)
```

New test files should use `package tg_test` for black-box testing. But code appended to existing files must match the file's existing package declaration.

---

## Issue Count Summary

| Category | Count | Severity |
|---|:---:|---|
| C — Will Not Compile | 6 | Blocks all progress |
| D — Breaks Conventions | 5 | Code review rejection |
| M — Missing Items | 7 | Silent bugs, incomplete features |
| N — Minor Nits | 3 | Low risk, should document |
| **Total** | **21** | |

---

## What the Plan Gets Right

These decisions are correct and well-reasoned — keep them:

1. `SetChatMemberTagRequest.Tag` with NO `omitempty` — critical for tag removal
2. `ChatMemberRestricted.CanEditTag` as `bool` (matches existing restriction fields pattern)
3. `ChatPermissions.CanEditTag` as `*bool` (matches existing permission fields pattern)
4. `EntityDateTime` constant and `date_time` entity field additions
5. `UTF16Len` and `NewDateTimeEntity` helpers in `tg/datetime.go`
6. Validation for `draft_id=0`, `tag > 16 chars`, empty text
7. S45 readback pattern (set → getChatMember → verify → remove → verify)
8. TDD ordering: fixtures → stubs → tests → implementation
9. Phase 3E being explicitly marked optional
