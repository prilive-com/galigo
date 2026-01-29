# Tier 2 Implementation Plan - Chat Administration Methods

**Version:** 2.0 (Revised after developer feedback)  
**Target:** galigo Telegram Bot API Library  
**Go Version:** 1.25  
**Total Methods:** 46  
**Estimated Duration:** 3-4 weeks

---

## Revision Summary

Based on code review and developer feedback, this plan:
1. Adds **PR-1** (prerequisite) to fix compile blockers and add unified call helper
2. Fixes incorrect assumptions (`executor.Call()`, `ChatID.IsZero()`)
3. Uses typed options instead of `func(any)` for bulk operations
4. Removes dangerous `KickUser` helper (or makes it explicitly risky)
5. Organizes PRs into parallel batches after PR0

---

## PR Dependency Graph

```
PR-1 (Foundation Fixes)
    ↓
PR0 (Types)
    ↓
┌───┴───┬───────┬───────┐
↓       ↓       ↓       ↓
Batch A Batch B Batch C Batch D
(PR1-3) (PR4-5) (PR6)   (PR7-8)
    ↓       ↓       ↓       ↓
    └───────┴───────┴───────┘
                ↓
            PR9 (Facade + Docs)
```

---

## PR-1: Foundation Fixes (PREREQUISITE)

**Goal:** Fix compile blockers and add unified call helper before implementing any new methods.

**Priority:** MUST be merged before PR0

### Files Changed

#### 1. `sender/call.go` (NEW FILE)

```go
package sender

import (
    "context"
    "encoding/json"
    "fmt"
)

// callJSON is the unified internal helper for all API calls.
// It wraps executeRequest() and provides consistent JSON decoding.
//
// Usage:
//   var result tg.ChatFullInfo
//   if err := c.callJSON(ctx, "getChat", req, &result); err != nil {
//       return nil, err
//   }
//   return &result, nil
func (c *Client) callJSON(ctx context.Context, method string, payload any, out any) error {
    resp, err := c.executeRequest(ctx, method, payload)
    if err != nil {
        return err
    }
    if out == nil {
        return nil // For methods that return bool/void
    }
    if err := json.Unmarshal(resp.Result, out); err != nil {
        return fmt.Errorf("galigo: %s: failed to parse response: %w", method, err)
    }
    return nil
}

// callJSONResult is a generic version for cleaner call sites.
// Requires Go 1.18+ generics.
func callJSONResult[T any](c *Client, ctx context.Context, method string, payload any) (T, error) {
    var result T
    if err := c.callJSON(ctx, method, payload, &result); err != nil {
        var zero T
        return zero, err
    }
    return result, nil
}
```

#### 2. `sender/validate.go` (NEW FILE)

```go
package sender

import (
    "fmt"
    
    "github.com/prilive-com/galigo/tg"
)

// validateChatID validates a ChatID value.
// Returns nil if valid, error if invalid.
func validateChatID(id tg.ChatID) error {
    if id == nil {
        return fmt.Errorf("galigo: chat_id is required")
    }
    switch v := id.(type) {
    case int64:
        if v == 0 {
            return fmt.Errorf("galigo: chat_id cannot be zero")
        }
        return nil
    case int:
        if v == 0 {
            return fmt.Errorf("galigo: chat_id cannot be zero")
        }
        return nil
    case string:
        if v == "" {
            return fmt.Errorf("galigo: chat_id cannot be empty string")
        }
        return nil
    default:
        return fmt.Errorf("galigo: chat_id must be int64 or string, got %T", id)
    }
}

// validateUserID validates a user ID.
func validateUserID(id int64) error {
    if id <= 0 {
        return fmt.Errorf("galigo: user_id must be positive, got %d", id)
    }
    return nil
}

// validateMessageID validates a message ID.
func validateMessageID(id int) error {
    if id <= 0 {
        return fmt.Errorf("galigo: message_id must be positive, got %d", id)
    }
    return nil
}
```

#### 3. `sender/client.go` - Add ResponseHeaderTimeout

```go
// In createHTTPClient function, add:
Transport: &http.Transport{
    DialContext: (&net.Dialer{
        Timeout:   10 * time.Second,
        KeepAlive: cfg.KeepAlive,
    }).DialContext,
    MaxIdleConns:          cfg.MaxIdleConns,
    IdleConnTimeout:       cfg.IdleTimeout,
    TLSHandshakeTimeout:   10 * time.Second,
    ResponseHeaderTimeout: 10 * time.Second,  // ADD THIS
    ExpectContinueTimeout: 1 * time.Second,   // ADD THIS
    ForceAttemptHTTP2:     true,
    TLSClientConfig: &tls.Config{
        MinVersion: tls.VersionTLS12,
    },
},
```

#### 4. `sender/client.go` - Fix Circuit Breaker IsSuccessful

```go
// In New() or wherever breaker is created, add IsSuccessful:
c.breaker = gobreaker.NewCircuitBreaker[*apiResponse](gobreaker.Settings{
    Name:        "galigo-sender",
    MaxRequests: c.breakerSettings.MaxRequests,
    Interval:    c.breakerSettings.Interval,
    Timeout:     c.breakerSettings.Timeout,
    ReadyToTrip: c.breakerSettings.ReadyToTrip,
    IsSuccessful: func(err error) bool {
        if err == nil {
            return true
        }
        // 4xx (except 429) are client errors, not service failures
        var apiErr *tg.APIError
        if errors.As(err, &apiErr) {
            if apiErr.Code >= 400 && apiErr.Code < 500 && apiErr.Code != 429 {
                return true // Don't count as breaker failure
            }
            return false // 429 and 5xx count as failures
        }
        // Context cancellation is not a service failure
        if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
            return true
        }
        // Network errors count as failures
        return false
    },
})
```

#### 5. `sender/client.go` - Token Sanitization in Errors

```go
// Add at package level:
import "regexp"

var tokenRedactor = regexp.MustCompile(`bot\d+:[A-Za-z0-9_-]+`)

func sanitizeTokenFromError(err error) error {
    if err == nil {
        return nil
    }
    return &sanitizedError{cause: err}
}

type sanitizedError struct{ cause error }

func (e *sanitizedError) Error() string {
    return tokenRedactor.ReplaceAllString(e.cause.Error(), "bot<REDACTED>")
}

func (e *sanitizedError) Unwrap() error { return e.cause }

// Update doRequest error handling:
resp, err := c.httpClient.Do(req)
if err != nil {
    return nil, fmt.Errorf("request failed: %w", sanitizeTokenFromError(err))
}
```

### Tests for PR-1

#### `sender/call_test.go`

```go
package sender

import (
    "context"
    "testing"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestCallJSON_Success(t *testing.T) {
    // Setup mock server returning valid JSON
    // Call c.callJSON()
    // Assert result is correctly decoded
}

func TestCallJSON_APIError(t *testing.T) {
    // Setup mock server returning API error
    // Assert error is properly typed as *tg.APIError
}

func TestCallJSON_NilOutput(t *testing.T) {
    // For void methods, out=nil should not panic
}
```

#### `sender/validate_test.go`

```go
package sender

import (
    "testing"
    
    "github.com/stretchr/testify/assert"
)

func TestValidateChatID(t *testing.T) {
    tests := []struct {
        name    string
        input   any
        wantErr bool
    }{
        {"valid int64", int64(123456), false},
        {"valid int", int(123456), false},
        {"valid username", "@testchannel", false},
        {"zero int64", int64(0), true},
        {"zero int", int(0), true},
        {"empty string", "", true},
        {"nil", nil, true},
        {"invalid type float", 123.456, true},
        {"invalid type struct", struct{}{}, true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateChatID(tt.input)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

#### `sender/security_test.go`

```go
package sender

import (
    "strings"
    "testing"
    
    "github.com/stretchr/testify/assert"
)

func TestNoTokenInErrors(t *testing.T) {
    token := "123456789:ABCdefGHIjklMNOpqrSTUvwxYZ"
    
    // Create client with this token
    // Trigger various error conditions (network, DNS, TLS)
    // Assert error messages do NOT contain the token
    
    errMsg := "Get \"https://api.telegram.org/bot123456789:ABCdefGHIjklMNOpqrSTUvwxYZ/getMe\": dial tcp: no such host"
    sanitized := sanitizeTokenFromError(fmt.Errorf(errMsg))
    
    assert.NotContains(t, sanitized.Error(), token)
    assert.NotContains(t, sanitized.Error(), "ABCdef")
    assert.Contains(t, sanitized.Error(), "bot<REDACTED>")
}

func TestBreakerNotOpenOn400(t *testing.T) {
    // Send 100 requests that return 400 Bad Request
    // Assert circuit breaker is still CLOSED
}

func TestBreakerOpensOn500(t *testing.T) {
    // Send requests that return 500 Internal Server Error
    // Assert circuit breaker OPENS after threshold
}
```

### Acceptance Criteria for PR-1

- [ ] `callJSON()` helper works for all existing methods (refactor 2-3 as proof)
- [ ] `validateChatID()` correctly validates int64, int, string; rejects nil and invalid types
- [ ] `ResponseHeaderTimeout` is set to 10 seconds
- [ ] Circuit breaker does NOT open on repeated 400 errors
- [ ] Circuit breaker DOES open on repeated 500 errors
- [ ] No bot token appears in any error message (test with DNS failure, TLS error)
- [ ] All existing tests pass
- [ ] New tests have >90% coverage for new code

---

## PR0: Types Foundation

**Goal:** Add all missing Telegram Bot API types needed for Tier 2 methods.

**Dependencies:** PR-1 merged

### Files Changed

#### 1. `tg/chat_member.go` (NEW FILE)

```go
package tg

import (
    "encoding/json"
    "fmt"
)

// ChatMember represents a member of a chat.
// This is a sealed interface - the concrete types are:
//   - ChatMemberOwner
//   - ChatMemberAdministrator
//   - ChatMemberMember
//   - ChatMemberRestricted
//   - ChatMemberLeft
//   - ChatMemberBanned
type ChatMember interface {
    // chatMember is a marker method to seal the interface.
    chatMember()
    
    // Status returns the member's status string.
    Status() string
    
    // GetUser returns the user information.
    GetUser() *User
}

// chatMemberBase contains fields common to all ChatMember types.
type chatMemberBase struct {
    User *User `json:"user"`
}

func (b chatMemberBase) GetUser() *User { return b.User }

// ChatMemberOwner represents a chat owner.
type ChatMemberOwner struct {
    chatMemberBase
    IsAnonymous bool   `json:"is_anonymous"`
    CustomTitle string `json:"custom_title,omitempty"`
}

func (ChatMemberOwner) chatMember()     {}
func (ChatMemberOwner) Status() string  { return "creator" }

// ChatMemberAdministrator represents a chat administrator.
type ChatMemberAdministrator struct {
    chatMemberBase
    CanBeEdited           bool   `json:"can_be_edited"`
    IsAnonymous           bool   `json:"is_anonymous"`
    CanManageChat         bool   `json:"can_manage_chat"`
    CanDeleteMessages     bool   `json:"can_delete_messages"`
    CanManageVideoChats   bool   `json:"can_manage_video_chats"`
    CanRestrictMembers    bool   `json:"can_restrict_members"`
    CanPromoteMembers     bool   `json:"can_promote_members"`
    CanChangeInfo         bool   `json:"can_change_info"`
    CanInviteUsers        bool   `json:"can_invite_users"`
    CanPostMessages       *bool  `json:"can_post_messages,omitempty"`       // Channels only
    CanEditMessages       *bool  `json:"can_edit_messages,omitempty"`       // Channels only
    CanPinMessages        *bool  `json:"can_pin_messages,omitempty"`        // Groups/supergroups only
    CanPostStories        *bool  `json:"can_post_stories,omitempty"`        // Channels only
    CanEditStories        *bool  `json:"can_edit_stories,omitempty"`        // Channels only
    CanDeleteStories      *bool  `json:"can_delete_stories,omitempty"`      // Channels only
    CanManageTopics       *bool  `json:"can_manage_topics,omitempty"`       // Supergroups only
    CanManageDirectMessages *bool `json:"can_manage_direct_messages,omitempty"` // Bot API 9.2+
    CustomTitle           string `json:"custom_title,omitempty"`
}

func (ChatMemberAdministrator) chatMember()    {}
func (ChatMemberAdministrator) Status() string { return "administrator" }

// ChatMemberMember represents a regular chat member.
type ChatMemberMember struct {
    chatMemberBase
    UntilDate int64 `json:"until_date,omitempty"` // Bot API 8.1+: membership expiration
}

func (ChatMemberMember) chatMember()    {}
func (ChatMemberMember) Status() string { return "member" }

// ChatMemberRestricted represents a restricted user.
type ChatMemberRestricted struct {
    chatMemberBase
    IsMember              bool  `json:"is_member"`
    CanSendMessages       bool  `json:"can_send_messages"`
    CanSendAudios         bool  `json:"can_send_audios"`
    CanSendDocuments      bool  `json:"can_send_documents"`
    CanSendPhotos         bool  `json:"can_send_photos"`
    CanSendVideos         bool  `json:"can_send_videos"`
    CanSendVideoNotes     bool  `json:"can_send_video_notes"`
    CanSendVoiceNotes     bool  `json:"can_send_voice_notes"`
    CanSendPolls          bool  `json:"can_send_polls"`
    CanSendOtherMessages  bool  `json:"can_send_other_messages"`
    CanAddWebPagePreviews bool  `json:"can_add_web_page_previews"`
    CanChangeInfo         bool  `json:"can_change_info"`
    CanInviteUsers        bool  `json:"can_invite_users"`
    CanPinMessages        bool  `json:"can_pin_messages"`
    CanManageTopics       bool  `json:"can_manage_topics"`
    UntilDate             int64 `json:"until_date"`
}

func (ChatMemberRestricted) chatMember()    {}
func (ChatMemberRestricted) Status() string { return "restricted" }

// ChatMemberLeft represents a user who left the chat.
type ChatMemberLeft struct {
    chatMemberBase
}

func (ChatMemberLeft) chatMember()    {}
func (ChatMemberLeft) Status() string { return "left" }

// ChatMemberBanned represents a banned user.
type ChatMemberBanned struct {
    chatMemberBase
    UntilDate int64 `json:"until_date"`
}

func (ChatMemberBanned) chatMember()    {}
func (ChatMemberBanned) Status() string { return "kicked" }

// UnmarshalJSON implements custom unmarshaling for ChatMember union type.
func UnmarshalChatMember(data []byte) (ChatMember, error) {
    // First, extract the status field
    var probe struct {
        Status string `json:"status"`
    }
    if err := json.Unmarshal(data, &probe); err != nil {
        return nil, fmt.Errorf("failed to probe chat member status: %w", err)
    }
    
    var result ChatMember
    var err error
    
    switch probe.Status {
    case "creator":
        var m ChatMemberOwner
        err = json.Unmarshal(data, &m)
        result = m
    case "administrator":
        var m ChatMemberAdministrator
        err = json.Unmarshal(data, &m)
        result = m
    case "member":
        var m ChatMemberMember
        err = json.Unmarshal(data, &m)
        result = m
    case "restricted":
        var m ChatMemberRestricted
        err = json.Unmarshal(data, &m)
        result = m
    case "left":
        var m ChatMemberLeft
        err = json.Unmarshal(data, &m)
        result = m
    case "kicked":
        var m ChatMemberBanned
        err = json.Unmarshal(data, &m)
        result = m
    default:
        return nil, fmt.Errorf("unknown chat member status: %q", probe.Status)
    }
    
    if err != nil {
        return nil, fmt.Errorf("failed to unmarshal chat member (%s): %w", probe.Status, err)
    }
    return result, nil
}

// Helper functions for type checking

// IsOwner returns true if the member is the chat owner.
func IsOwner(m ChatMember) bool {
    _, ok := m.(ChatMemberOwner)
    return ok
}

// IsAdmin returns true if the member is an administrator (including owner).
func IsAdmin(m ChatMember) bool {
    switch m.(type) {
    case ChatMemberOwner, ChatMemberAdministrator:
        return true
    default:
        return false
    }
}

// IsMember returns true if the member is a regular member.
func IsMember(m ChatMember) bool {
    _, ok := m.(ChatMemberMember)
    return ok
}

// IsRestricted returns true if the member is restricted.
func IsRestricted(m ChatMember) bool {
    _, ok := m.(ChatMemberRestricted)
    return ok
}

// IsBanned returns true if the member is banned.
func IsBanned(m ChatMember) bool {
    _, ok := m.(ChatMemberBanned)
    return ok
}

// HasLeft returns true if the member left the chat.
func HasLeft(m ChatMember) bool {
    _, ok := m.(ChatMemberLeft)
    return ok
}
```

#### 2. `tg/chat_permissions.go` (NEW FILE)

```go
package tg

// ChatPermissions describes actions that a non-administrator user is allowed to take in a chat.
type ChatPermissions struct {
    CanSendMessages       *bool `json:"can_send_messages,omitempty"`
    CanSendAudios         *bool `json:"can_send_audios,omitempty"`
    CanSendDocuments      *bool `json:"can_send_documents,omitempty"`
    CanSendPhotos         *bool `json:"can_send_photos,omitempty"`
    CanSendVideos         *bool `json:"can_send_videos,omitempty"`
    CanSendVideoNotes     *bool `json:"can_send_video_notes,omitempty"`
    CanSendVoiceNotes     *bool `json:"can_send_voice_notes,omitempty"`
    CanSendPolls          *bool `json:"can_send_polls,omitempty"`
    CanSendOtherMessages  *bool `json:"can_send_other_messages,omitempty"`
    CanAddWebPagePreviews *bool `json:"can_add_web_page_previews,omitempty"`
    CanChangeInfo         *bool `json:"can_change_info,omitempty"`
    CanInviteUsers        *bool `json:"can_invite_users,omitempty"`
    CanPinMessages        *bool `json:"can_pin_messages,omitempty"`
    CanManageTopics       *bool `json:"can_manage_topics,omitempty"`
}

// Preset permission constructors

// AllPermissions returns ChatPermissions with all permissions enabled.
func AllPermissions() ChatPermissions {
    t := true
    return ChatPermissions{
        CanSendMessages:       &t,
        CanSendAudios:         &t,
        CanSendDocuments:      &t,
        CanSendPhotos:         &t,
        CanSendVideos:         &t,
        CanSendVideoNotes:     &t,
        CanSendVoiceNotes:     &t,
        CanSendPolls:          &t,
        CanSendOtherMessages:  &t,
        CanAddWebPagePreviews: &t,
        CanChangeInfo:         &t,
        CanInviteUsers:        &t,
        CanPinMessages:        &t,
        CanManageTopics:       &t,
    }
}

// NoPermissions returns ChatPermissions with all permissions disabled.
func NoPermissions() ChatPermissions {
    f := false
    return ChatPermissions{
        CanSendMessages:       &f,
        CanSendAudios:         &f,
        CanSendDocuments:      &f,
        CanSendPhotos:         &f,
        CanSendVideos:         &f,
        CanSendVideoNotes:     &f,
        CanSendVoiceNotes:     &f,
        CanSendPolls:          &f,
        CanSendOtherMessages:  &f,
        CanAddWebPagePreviews: &f,
        CanChangeInfo:         &f,
        CanInviteUsers:        &f,
        CanPinMessages:        &f,
        CanManageTopics:       &f,
    }
}

// ReadOnlyPermissions returns permissions for read-only access (no sending).
func ReadOnlyPermissions() ChatPermissions {
    f := false
    return ChatPermissions{
        CanSendMessages:       &f,
        CanSendAudios:         &f,
        CanSendDocuments:      &f,
        CanSendPhotos:         &f,
        CanSendVideos:         &f,
        CanSendVideoNotes:     &f,
        CanSendVoiceNotes:     &f,
        CanSendPolls:          &f,
        CanSendOtherMessages:  &f,
        CanAddWebPagePreviews: &f,
    }
}

// TextOnlyPermissions returns permissions for text-only messaging.
func TextOnlyPermissions() ChatPermissions {
    t, f := true, false
    return ChatPermissions{
        CanSendMessages:       &t,
        CanSendAudios:         &f,
        CanSendDocuments:      &f,
        CanSendPhotos:         &f,
        CanSendVideos:         &f,
        CanSendVideoNotes:     &f,
        CanSendVoiceNotes:     &f,
        CanSendPolls:          &f,
        CanSendOtherMessages:  &f,
        CanAddWebPagePreviews: &f,
    }
}
```

#### 3. `tg/chat_admin_rights.go` (NEW FILE)

```go
package tg

// ChatAdministratorRights represents the rights of an administrator.
type ChatAdministratorRights struct {
    IsAnonymous           bool  `json:"is_anonymous"`
    CanManageChat         bool  `json:"can_manage_chat"`
    CanDeleteMessages     bool  `json:"can_delete_messages"`
    CanManageVideoChats   bool  `json:"can_manage_video_chats"`
    CanRestrictMembers    bool  `json:"can_restrict_members"`
    CanPromoteMembers     bool  `json:"can_promote_members"`
    CanChangeInfo         bool  `json:"can_change_info"`
    CanInviteUsers        bool  `json:"can_invite_users"`
    CanPostMessages       *bool `json:"can_post_messages,omitempty"`
    CanEditMessages       *bool `json:"can_edit_messages,omitempty"`
    CanPinMessages        *bool `json:"can_pin_messages,omitempty"`
    CanPostStories        *bool `json:"can_post_stories,omitempty"`
    CanEditStories        *bool `json:"can_edit_stories,omitempty"`
    CanDeleteStories      *bool `json:"can_delete_stories,omitempty"`
    CanManageTopics       *bool `json:"can_manage_topics,omitempty"`
    CanManageDirectMessages *bool `json:"can_manage_direct_messages,omitempty"` // Bot API 9.2+
}

// Preset admin rights constructors

// FullAdminRights returns administrator rights with all permissions.
func FullAdminRights() ChatAdministratorRights {
    t := true
    return ChatAdministratorRights{
        IsAnonymous:           false,
        CanManageChat:         true,
        CanDeleteMessages:     true,
        CanManageVideoChats:   true,
        CanRestrictMembers:    true,
        CanPromoteMembers:     true,
        CanChangeInfo:         true,
        CanInviteUsers:        true,
        CanPostMessages:       &t,
        CanEditMessages:       &t,
        CanPinMessages:        &t,
        CanPostStories:        &t,
        CanEditStories:        &t,
        CanDeleteStories:      &t,
        CanManageTopics:       &t,
        CanManageDirectMessages: &t,
    }
}

// ModeratorRights returns typical moderator permissions (no promote, no change info).
func ModeratorRights() ChatAdministratorRights {
    t := true
    return ChatAdministratorRights{
        IsAnonymous:         false,
        CanManageChat:       true,
        CanDeleteMessages:   true,
        CanManageVideoChats: false,
        CanRestrictMembers:  true,
        CanPromoteMembers:   false,
        CanChangeInfo:       false,
        CanInviteUsers:      true,
        CanPinMessages:      &t,
    }
}

// ContentManagerRights returns permissions for content management only.
func ContentManagerRights() ChatAdministratorRights {
    t := true
    return ChatAdministratorRights{
        IsAnonymous:       false,
        CanManageChat:     true,
        CanDeleteMessages: true,
        CanChangeInfo:     true,
        CanPostMessages:   &t,
        CanEditMessages:   &t,
        CanPinMessages:    &t,
    }
}
```

#### 4. `tg/chat_full_info.go` (NEW FILE)

```go
package tg

// ChatFullInfo contains full information about a chat.
// Returned by getChat method.
type ChatFullInfo struct {
    // Basic info (always present)
    ID        int64  `json:"id"`
    Type      string `json:"type"` // "private", "group", "supergroup", "channel"
    Title     string `json:"title,omitempty"`
    Username  string `json:"username,omitempty"`
    FirstName string `json:"first_name,omitempty"`
    LastName  string `json:"last_name,omitempty"`
    
    // Optional fields
    IsForum                        bool              `json:"is_forum,omitempty"`
    AccentColorID                  int               `json:"accent_color_id,omitempty"`
    MaxReactionCount               int               `json:"max_reaction_count,omitempty"`
    Photo                          *ChatPhoto        `json:"photo,omitempty"`
    ActiveUsernames                []string          `json:"active_usernames,omitempty"`
    Birthdate                      *Birthdate        `json:"birthdate,omitempty"`
    BusinessIntro                  *BusinessIntro    `json:"business_intro,omitempty"`
    BusinessLocation               *BusinessLocation `json:"business_location,omitempty"`
    BusinessOpeningHours           *BusinessHours    `json:"business_opening_hours,omitempty"`
    PersonalChat                   *Chat             `json:"personal_chat,omitempty"`
    AvailableReactions             []ReactionType    `json:"available_reactions,omitempty"`
    BackgroundCustomEmojiID        string            `json:"background_custom_emoji_id,omitempty"`
    ProfileAccentColorID           *int              `json:"profile_accent_color_id,omitempty"`
    ProfileBackgroundCustomEmojiID string            `json:"profile_background_custom_emoji_id,omitempty"`
    EmojiStatusCustomEmojiID       string            `json:"emoji_status_custom_emoji_id,omitempty"`
    EmojiStatusExpirationDate      int64             `json:"emoji_status_expiration_date,omitempty"`
    Bio                            string            `json:"bio,omitempty"`
    HasPrivateForwards             bool              `json:"has_private_forwards,omitempty"`
    HasRestrictedVoiceAndVideoMessages bool          `json:"has_restricted_voice_and_video_messages,omitempty"`
    JoinToSendMessages             bool              `json:"join_to_send_messages,omitempty"`
    JoinByRequest                  bool              `json:"join_by_request,omitempty"`
    Description                    string            `json:"description,omitempty"`
    InviteLink                     string            `json:"invite_link,omitempty"`
    PinnedMessage                  *Message          `json:"pinned_message,omitempty"`
    Permissions                    *ChatPermissions  `json:"permissions,omitempty"`
    CanSendPaidMedia               bool              `json:"can_send_paid_media,omitempty"`
    SlowModeDelay                  int               `json:"slow_mode_delay,omitempty"`
    UnrestrictBoostCount           int               `json:"unrestrict_boost_count,omitempty"`
    MessageAutoDeleteTime          int               `json:"message_auto_delete_time,omitempty"`
    HasAggressiveAntiSpamEnabled   bool              `json:"has_aggressive_anti_spam_enabled,omitempty"`
    HasHiddenMembers               bool              `json:"has_hidden_members,omitempty"`
    HasProtectedContent            bool              `json:"has_protected_content,omitempty"`
    HasVisibleHistory              bool              `json:"has_visible_history,omitempty"`
    StickerSetName                 string            `json:"sticker_set_name,omitempty"`
    CanSetStickerSet               bool              `json:"can_set_sticker_set,omitempty"`
    CustomEmojiStickerSetName      string            `json:"custom_emoji_sticker_set_name,omitempty"`
    LinkedChatID                   int64             `json:"linked_chat_id,omitempty"`
    Location                       *ChatLocation     `json:"location,omitempty"`
}

// ChatLocation represents a location to which a chat is connected.
type ChatLocation struct {
    Location *Location `json:"location"`
    Address  string    `json:"address"`
}

// ChatPhoto represents a chat photo.
type ChatPhoto struct {
    SmallFileID       string `json:"small_file_id"`
    SmallFileUniqueID string `json:"small_file_unique_id"`
    BigFileID         string `json:"big_file_id"`
    BigFileUniqueID   string `json:"big_file_unique_id"`
}

// Birthdate represents a user's birthdate.
type Birthdate struct {
    Day   int  `json:"day"`
    Month int  `json:"month"`
    Year  *int `json:"year,omitempty"`
}

// BusinessIntro represents a business intro.
type BusinessIntro struct {
    Title   string   `json:"title,omitempty"`
    Message string   `json:"message,omitempty"`
    Sticker *Sticker `json:"sticker,omitempty"`
}

// BusinessLocation represents a business location.
type BusinessLocation struct {
    Address  string    `json:"address"`
    Location *Location `json:"location,omitempty"`
}

// BusinessHours represents business opening hours.
type BusinessHours struct {
    TimeZoneName string                   `json:"time_zone_name"`
    OpeningHours []BusinessOpeningHoursInterval `json:"opening_hours"`
}

// BusinessOpeningHoursInterval represents a time interval.
type BusinessOpeningHoursInterval struct {
    OpeningMinute int `json:"opening_minute"`
    ClosingMinute int `json:"closing_minute"`
}

// ReactionType represents a reaction type.
type ReactionType struct {
    Type        string `json:"type"` // "emoji" or "custom_emoji"
    Emoji       string `json:"emoji,omitempty"`
    CustomEmoji string `json:"custom_emoji_id,omitempty"`
}
```

#### 5. `tg/forum.go` (NEW FILE)

```go
package tg

// ForumTopic represents a forum topic.
type ForumTopic struct {
    MessageThreadID   int    `json:"message_thread_id"`
    Name              string `json:"name"`
    IconColor         int    `json:"icon_color"`
    IconCustomEmojiID string `json:"icon_custom_emoji_id,omitempty"`
}

// Forum topic icon colors (official Telegram colors)
const (
    ForumColorBlue   = 0x6FB9F0 // 7322096
    ForumColorYellow = 0xFFD67E // 16766590
    ForumColorViolet = 0xCB86DB // 13338331
    ForumColorGreen  = 0x8EEE98 // 9367192
    ForumColorRose   = 0xFF93B2 // 16749490
    ForumColorRed    = 0xFB6F5F // 16478047
)

// ForumTopicCreated represents service message about a new forum topic.
type ForumTopicCreated struct {
    Name              string `json:"name"`
    IconColor         int    `json:"icon_color"`
    IconCustomEmojiID string `json:"icon_custom_emoji_id,omitempty"`
}

// ForumTopicEdited represents service message about an edited topic.
type ForumTopicEdited struct {
    Name              string `json:"name,omitempty"`
    IconCustomEmojiID string `json:"icon_custom_emoji_id,omitempty"`
}

// ForumTopicClosed represents service message about a closed topic.
type ForumTopicClosed struct{}

// ForumTopicReopened represents service message about a reopened topic.
type ForumTopicReopened struct{}

// GeneralForumTopicHidden represents service message about hidden General topic.
type GeneralForumTopicHidden struct{}

// GeneralForumTopicUnhidden represents service message about unhidden General topic.
type GeneralForumTopicUnhidden struct{}
```

### Acceptance Criteria for PR0

- [ ] `ChatMember` sealed interface with 6 concrete types
- [ ] `UnmarshalChatMember()` correctly deserializes all status types
- [ ] Helper functions (`IsOwner`, `IsAdmin`, etc.) work correctly
- [ ] `ChatPermissions` with pointer fields for unset vs false distinction
- [ ] Preset constructors (`AllPermissions()`, `NoPermissions()`, etc.) work
- [ ] `ChatAdministratorRights` with all Bot API 9.3 fields
- [ ] `ChatFullInfo` matches Telegram's getChat response
- [ ] `ChatLocation`, `ChatPhoto`, `Birthdate` types defined
- [ ] Forum types with color constants
- [ ] All types have correct JSON tags with omitempty where appropriate
- [ ] Unit tests for JSON unmarshaling round-trips
- [ ] No breaking changes to existing types

---

## Batch A: Admin & Membership (PR1-PR3)

### PR1: Chat Information Methods

**Methods:** `GetChat`, `GetChatAdministrators`, `GetChatMemberCount`, `GetChatMember`

#### `sender/chat_info.go` (NEW FILE)

```go
package sender

import (
    "context"
    "encoding/json"
    "fmt"
    
    "github.com/prilive-com/galigo/tg"
)

// ================== Chat Information Requests ==================

// GetChatRequest represents a getChat request.
type GetChatRequest struct {
    ChatID tg.ChatID `json:"chat_id"`
}

// GetChatMemberRequest represents a getChatMember request.
type GetChatMemberRequest struct {
    ChatID tg.ChatID `json:"chat_id"`
    UserID int64     `json:"user_id"`
}

// ================== Chat Information Methods ==================

// GetChat returns full information about a chat.
func (c *Client) GetChat(ctx context.Context, chatID tg.ChatID) (*tg.ChatFullInfo, error) {
    if err := validateChatID(chatID); err != nil {
        return nil, err
    }
    
    var result tg.ChatFullInfo
    if err := c.callJSON(ctx, "getChat", GetChatRequest{ChatID: chatID}, &result); err != nil {
        return nil, err
    }
    return &result, nil
}

// GetChatAdministrators returns a list of administrators in a chat.
// Returns only non-bot administrators.
func (c *Client) GetChatAdministrators(ctx context.Context, chatID tg.ChatID) ([]tg.ChatMember, error) {
    if err := validateChatID(chatID); err != nil {
        return nil, err
    }
    
    resp, err := c.executeRequest(ctx, "getChatAdministrators", GetChatRequest{ChatID: chatID})
    if err != nil {
        return nil, err
    }
    
    // Need custom unmarshaling for ChatMember union type
    var rawMembers []json.RawMessage
    if err := json.Unmarshal(resp.Result, &rawMembers); err != nil {
        return nil, fmt.Errorf("galigo: getChatAdministrators: failed to parse response: %w", err)
    }
    
    members := make([]tg.ChatMember, 0, len(rawMembers))
    for _, raw := range rawMembers {
        member, err := tg.UnmarshalChatMember(raw)
        if err != nil {
            return nil, fmt.Errorf("galigo: getChatAdministrators: %w", err)
        }
        members = append(members, member)
    }
    
    return members, nil
}

// GetChatMemberCount returns the number of members in a chat.
func (c *Client) GetChatMemberCount(ctx context.Context, chatID tg.ChatID) (int, error) {
    if err := validateChatID(chatID); err != nil {
        return 0, err
    }
    
    var result int
    if err := c.callJSON(ctx, "getChatMemberCount", GetChatRequest{ChatID: chatID}, &result); err != nil {
        return 0, err
    }
    return result, nil
}

// GetChatMember returns information about a member of a chat.
func (c *Client) GetChatMember(ctx context.Context, chatID tg.ChatID, userID int64) (tg.ChatMember, error) {
    if err := validateChatID(chatID); err != nil {
        return nil, err
    }
    if err := validateUserID(userID); err != nil {
        return nil, err
    }
    
    resp, err := c.executeRequest(ctx, "getChatMember", GetChatMemberRequest{
        ChatID: chatID,
        UserID: userID,
    })
    if err != nil {
        return nil, err
    }
    
    return tg.UnmarshalChatMember(resp.Result)
}
```

#### Tests: `sender/chat_info_test.go`

```go
package sender

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    
    "github.com/prilive-com/galigo/tg"
)

func TestGetChat(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "/bot123:token/getChat", r.URL.Path)
        
        json.NewEncoder(w).Encode(map[string]any{
            "ok": true,
            "result": map[string]any{
                "id":    int64(-1001234567890),
                "type":  "supergroup",
                "title": "Test Group",
            },
        })
    }))
    defer server.Close()
    
    client := newTestClient(t, server.URL, "123:token")
    
    chat, err := client.GetChat(context.Background(), int64(-1001234567890))
    require.NoError(t, err)
    assert.Equal(t, int64(-1001234567890), chat.ID)
    assert.Equal(t, "supergroup", chat.Type)
    assert.Equal(t, "Test Group", chat.Title)
}

func TestGetChat_InvalidChatID(t *testing.T) {
    client := newTestClient(t, "http://unused", "token")
    
    _, err := client.GetChat(context.Background(), nil)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "chat_id is required")
    
    _, err = client.GetChat(context.Background(), int64(0))
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "chat_id cannot be zero")
}

func TestGetChatAdministrators(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(map[string]any{
            "ok": true,
            "result": []map[string]any{
                {
                    "status": "creator",
                    "user":   map[string]any{"id": 123, "first_name": "Owner"},
                },
                {
                    "status":           "administrator",
                    "user":             map[string]any{"id": 456, "first_name": "Admin"},
                    "can_delete_messages": true,
                },
            },
        })
    }))
    defer server.Close()
    
    client := newTestClient(t, server.URL, "token")
    
    admins, err := client.GetChatAdministrators(context.Background(), int64(-100123))
    require.NoError(t, err)
    require.Len(t, admins, 2)
    
    assert.True(t, tg.IsOwner(admins[0]))
    assert.True(t, tg.IsAdmin(admins[1]))
    assert.False(t, tg.IsOwner(admins[1]))
}

func TestGetChatMember_AllStatuses(t *testing.T) {
    statuses := []string{"creator", "administrator", "member", "restricted", "left", "kicked"}
    
    for _, status := range statuses {
        t.Run(status, func(t *testing.T) {
            server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                json.NewEncoder(w).Encode(map[string]any{
                    "ok": true,
                    "result": map[string]any{
                        "status": status,
                        "user":   map[string]any{"id": 123, "first_name": "Test"},
                    },
                })
            }))
            defer server.Close()
            
            client := newTestClient(t, server.URL, "token")
            member, err := client.GetChatMember(context.Background(), int64(-100123), 123)
            
            require.NoError(t, err)
            assert.Equal(t, status, member.Status())
        })
    }
}
```

### PR2: Ban/Unban/Restrict Methods

**Methods:** `BanChatMember`, `UnbanChatMember`, `RestrictChatMember`, `BanChatSenderChat`, `UnbanChatSenderChat`

**Note:** NO `KickUser` helper - it's dangerous.

#### `sender/chat_moderation.go` (NEW FILE)

```go
package sender

import (
    "context"
    "time"
    
    "github.com/prilive-com/galigo/tg"
)

// ================== Moderation Requests ==================

// BanChatMemberRequest represents a banChatMember request.
type BanChatMemberRequest struct {
    ChatID         tg.ChatID `json:"chat_id"`
    UserID         int64     `json:"user_id"`
    UntilDate      int64     `json:"until_date,omitempty"`
    RevokeMessages bool      `json:"revoke_messages,omitempty"`
}

// UnbanChatMemberRequest represents an unbanChatMember request.
type UnbanChatMemberRequest struct {
    ChatID       tg.ChatID `json:"chat_id"`
    UserID       int64     `json:"user_id"`
    OnlyIfBanned bool      `json:"only_if_banned,omitempty"`
}

// RestrictChatMemberRequest represents a restrictChatMember request.
type RestrictChatMemberRequest struct {
    ChatID                        tg.ChatID          `json:"chat_id"`
    UserID                        int64              `json:"user_id"`
    Permissions                   tg.ChatPermissions `json:"permissions"`
    UseIndependentChatPermissions bool               `json:"use_independent_chat_permissions,omitempty"`
    UntilDate                     int64              `json:"until_date,omitempty"`
}

// BanChatSenderChatRequest represents a banChatSenderChat request.
type BanChatSenderChatRequest struct {
    ChatID       tg.ChatID `json:"chat_id"`
    SenderChatID int64     `json:"sender_chat_id"`
}

// UnbanChatSenderChatRequest represents an unbanChatSenderChat request.
type UnbanChatSenderChatRequest struct {
    ChatID       tg.ChatID `json:"chat_id"`
    SenderChatID int64     `json:"sender_chat_id"`
}

// ================== Moderation Methods ==================

// BanChatMember bans a user in a group, supergroup, or channel.
// The user will not be able to return to the chat on their own using invite links.
// In supergroups and channels, the user will not be able to return until unbanned.
func (c *Client) BanChatMember(ctx context.Context, chatID tg.ChatID, userID int64, opts ...BanOption) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    if err := validateUserID(userID); err != nil {
        return err
    }
    
    req := BanChatMemberRequest{
        ChatID: chatID,
        UserID: userID,
    }
    for _, opt := range opts {
        opt(&req)
    }
    
    return c.callJSON(ctx, "banChatMember", req, nil)
}

// UnbanChatMember unbans a previously banned user in a supergroup or channel.
// The user will NOT be able to join the chat via invite link until banned again.
// Use OnlyIfBanned option if you don't want to unban a user who was never banned.
func (c *Client) UnbanChatMember(ctx context.Context, chatID tg.ChatID, userID int64, opts ...UnbanOption) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    if err := validateUserID(userID); err != nil {
        return err
    }
    
    req := UnbanChatMemberRequest{
        ChatID: chatID,
        UserID: userID,
    }
    for _, opt := range opts {
        opt(&req)
    }
    
    return c.callJSON(ctx, "unbanChatMember", req, nil)
}

// RestrictChatMember restricts a user in a supergroup.
// The bot must be an administrator with can_restrict_members rights.
// Pass nil permissions to lift all restrictions.
func (c *Client) RestrictChatMember(ctx context.Context, chatID tg.ChatID, userID int64, permissions tg.ChatPermissions, opts ...RestrictOption) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    if err := validateUserID(userID); err != nil {
        return err
    }
    
    req := RestrictChatMemberRequest{
        ChatID:      chatID,
        UserID:      userID,
        Permissions: permissions,
    }
    for _, opt := range opts {
        opt(&req)
    }
    
    return c.callJSON(ctx, "restrictChatMember", req, nil)
}

// BanChatSenderChat bans a channel chat in a supergroup or channel.
// The owner of the banned channel will not be able to send messages on behalf
// of any of their channels until unbanned.
func (c *Client) BanChatSenderChat(ctx context.Context, chatID tg.ChatID, senderChatID int64) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    return c.callJSON(ctx, "banChatSenderChat", BanChatSenderChatRequest{
        ChatID:       chatID,
        SenderChatID: senderChatID,
    }, nil)
}

// UnbanChatSenderChat unbans a previously banned channel chat.
func (c *Client) UnbanChatSenderChat(ctx context.Context, chatID tg.ChatID, senderChatID int64) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    return c.callJSON(ctx, "unbanChatSenderChat", UnbanChatSenderChatRequest{
        ChatID:       chatID,
        SenderChatID: senderChatID,
    }, nil)
}

// ================== Options ==================

// BanOption configures BanChatMember.
type BanOption func(*BanChatMemberRequest)

// WithBanUntil sets the ban expiration time.
// Users banned for less than 366 days will be automatically unbanned.
// If set to 0 or more than 366 days, the user is banned forever.
func WithBanUntil(until time.Time) BanOption {
    return func(r *BanChatMemberRequest) {
        r.UntilDate = until.Unix()
    }
}

// WithBanDuration sets the ban duration from now.
func WithBanDuration(d time.Duration) BanOption {
    return func(r *BanChatMemberRequest) {
        r.UntilDate = time.Now().Add(d).Unix()
    }
}

// WithRevokeMessages revokes all messages from the user in the chat.
func WithRevokeMessages() BanOption {
    return func(r *BanChatMemberRequest) {
        r.RevokeMessages = true
    }
}

// UnbanOption configures UnbanChatMember.
type UnbanOption func(*UnbanChatMemberRequest)

// WithOnlyIfBanned only unbans the user if they are currently banned.
// This prevents accidentally removing a user from the chat if they're not banned.
func WithOnlyIfBanned() UnbanOption {
    return func(r *UnbanChatMemberRequest) {
        r.OnlyIfBanned = true
    }
}

// RestrictOption configures RestrictChatMember.
type RestrictOption func(*RestrictChatMemberRequest)

// WithRestrictUntil sets the restriction expiration time.
func WithRestrictUntil(until time.Time) RestrictOption {
    return func(r *RestrictChatMemberRequest) {
        r.UntilDate = until.Unix()
    }
}

// WithRestrictDuration sets the restriction duration from now.
func WithRestrictDuration(d time.Duration) RestrictOption {
    return func(r *RestrictChatMemberRequest) {
        r.UntilDate = time.Now().Add(d).Unix()
    }
}

// WithIndependentPermissions uses independent chat permissions.
// When true, the bot can grant permissions that are restricted by default.
func WithIndependentPermissions() RestrictOption {
    return func(r *RestrictChatMemberRequest) {
        r.UseIndependentChatPermissions = true
    }
}
```

### PR3: Promote/Title Methods

**Methods:** `PromoteChatMember`, `SetChatAdministratorCustomTitle`

#### `sender/chat_admin.go` (NEW FILE)

```go
package sender

import (
    "context"
    
    "github.com/prilive-com/galigo/tg"
)

// ================== Admin Promotion Requests ==================

// PromoteChatMemberRequest represents a promoteChatMember request.
type PromoteChatMemberRequest struct {
    ChatID               tg.ChatID `json:"chat_id"`
    UserID               int64     `json:"user_id"`
    IsAnonymous          *bool     `json:"is_anonymous,omitempty"`
    CanManageChat        *bool     `json:"can_manage_chat,omitempty"`
    CanDeleteMessages    *bool     `json:"can_delete_messages,omitempty"`
    CanManageVideoChats  *bool     `json:"can_manage_video_chats,omitempty"`
    CanRestrictMembers   *bool     `json:"can_restrict_members,omitempty"`
    CanPromoteMembers    *bool     `json:"can_promote_members,omitempty"`
    CanChangeInfo        *bool     `json:"can_change_info,omitempty"`
    CanInviteUsers       *bool     `json:"can_invite_users,omitempty"`
    CanPostMessages      *bool     `json:"can_post_messages,omitempty"`
    CanEditMessages      *bool     `json:"can_edit_messages,omitempty"`
    CanPinMessages       *bool     `json:"can_pin_messages,omitempty"`
    CanPostStories       *bool     `json:"can_post_stories,omitempty"`
    CanEditStories       *bool     `json:"can_edit_stories,omitempty"`
    CanDeleteStories     *bool     `json:"can_delete_stories,omitempty"`
    CanManageTopics      *bool     `json:"can_manage_topics,omitempty"`
}

// SetChatAdministratorCustomTitleRequest represents a setChatAdministratorCustomTitle request.
type SetChatAdministratorCustomTitleRequest struct {
    ChatID      tg.ChatID `json:"chat_id"`
    UserID      int64     `json:"user_id"`
    CustomTitle string    `json:"custom_title"`
}

// ================== Admin Methods ==================

// PromoteChatMember promotes or demotes a user in a supergroup or channel.
// Pass all boolean parameters as false to demote a user.
func (c *Client) PromoteChatMember(ctx context.Context, chatID tg.ChatID, userID int64, opts ...PromoteOption) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    if err := validateUserID(userID); err != nil {
        return err
    }
    
    req := PromoteChatMemberRequest{
        ChatID: chatID,
        UserID: userID,
    }
    for _, opt := range opts {
        opt(&req)
    }
    
    return c.callJSON(ctx, "promoteChatMember", req, nil)
}

// PromoteChatMemberWithRights promotes a user with the given rights.
// This is a convenience method that applies ChatAdministratorRights.
func (c *Client) PromoteChatMemberWithRights(ctx context.Context, chatID tg.ChatID, userID int64, rights tg.ChatAdministratorRights) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    if err := validateUserID(userID); err != nil {
        return err
    }
    
    req := PromoteChatMemberRequest{
        ChatID:              chatID,
        UserID:              userID,
        IsAnonymous:         &rights.IsAnonymous,
        CanManageChat:       &rights.CanManageChat,
        CanDeleteMessages:   &rights.CanDeleteMessages,
        CanManageVideoChats: &rights.CanManageVideoChats,
        CanRestrictMembers:  &rights.CanRestrictMembers,
        CanPromoteMembers:   &rights.CanPromoteMembers,
        CanChangeInfo:       &rights.CanChangeInfo,
        CanInviteUsers:      &rights.CanInviteUsers,
        CanPostMessages:     rights.CanPostMessages,
        CanEditMessages:     rights.CanEditMessages,
        CanPinMessages:      rights.CanPinMessages,
        CanPostStories:      rights.CanPostStories,
        CanEditStories:      rights.CanEditStories,
        CanDeleteStories:    rights.CanDeleteStories,
        CanManageTopics:     rights.CanManageTopics,
    }
    
    return c.callJSON(ctx, "promoteChatMember", req, nil)
}

// DemoteChatMember removes all admin privileges from a user.
func (c *Client) DemoteChatMember(ctx context.Context, chatID tg.ChatID, userID int64) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    if err := validateUserID(userID); err != nil {
        return err
    }
    
    f := false
    req := PromoteChatMemberRequest{
        ChatID:              chatID,
        UserID:              userID,
        CanManageChat:       &f,
        CanDeleteMessages:   &f,
        CanManageVideoChats: &f,
        CanRestrictMembers:  &f,
        CanPromoteMembers:   &f,
        CanChangeInfo:       &f,
        CanInviteUsers:      &f,
        CanPostMessages:     &f,
        CanEditMessages:     &f,
        CanPinMessages:      &f,
    }
    
    return c.callJSON(ctx, "promoteChatMember", req, nil)
}

// SetChatAdministratorCustomTitle sets a custom title for an administrator.
// Custom titles are shown in the group members list.
// Max length: 16 characters, emoji are not allowed.
func (c *Client) SetChatAdministratorCustomTitle(ctx context.Context, chatID tg.ChatID, userID int64, customTitle string) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    if err := validateUserID(userID); err != nil {
        return err
    }
    if len(customTitle) > 16 {
        return tg.NewValidationError("custom_title", "must be at most 16 characters")
    }
    
    return c.callJSON(ctx, "setChatAdministratorCustomTitle", SetChatAdministratorCustomTitleRequest{
        ChatID:      chatID,
        UserID:      userID,
        CustomTitle: customTitle,
    }, nil)
}

// ================== Options ==================

// PromoteOption configures PromoteChatMember.
type PromoteOption func(*PromoteChatMemberRequest)

// WithAnonymous sets whether the admin's presence is hidden.
func WithAnonymous(anonymous bool) PromoteOption {
    return func(r *PromoteChatMemberRequest) {
        r.IsAnonymous = &anonymous
    }
}

// WithCanManageChat grants ability to access chat settings.
func WithCanManageChat(can bool) PromoteOption {
    return func(r *PromoteChatMemberRequest) {
        r.CanManageChat = &can
    }
}

// WithCanDeleteMessages grants ability to delete messages.
func WithCanDeleteMessages(can bool) PromoteOption {
    return func(r *PromoteChatMemberRequest) {
        r.CanDeleteMessages = &can
    }
}

// WithCanManageVideoChats grants ability to manage video chats.
func WithCanManageVideoChats(can bool) PromoteOption {
    return func(r *PromoteChatMemberRequest) {
        r.CanManageVideoChats = &can
    }
}

// WithCanRestrictMembers grants ability to restrict members.
func WithCanRestrictMembers(can bool) PromoteOption {
    return func(r *PromoteChatMemberRequest) {
        r.CanRestrictMembers = &can
    }
}

// WithCanPromoteMembers grants ability to add new admins.
func WithCanPromoteMembers(can bool) PromoteOption {
    return func(r *PromoteChatMemberRequest) {
        r.CanPromoteMembers = &can
    }
}

// WithCanChangeInfo grants ability to change chat info.
func WithCanChangeInfo(can bool) PromoteOption {
    return func(r *PromoteChatMemberRequest) {
        r.CanChangeInfo = &can
    }
}

// WithCanInviteUsers grants ability to invite users.
func WithCanInviteUsers(can bool) PromoteOption {
    return func(r *PromoteChatMemberRequest) {
        r.CanInviteUsers = &can
    }
}

// WithCanPostMessages grants ability to post in channels.
func WithCanPostMessages(can bool) PromoteOption {
    return func(r *PromoteChatMemberRequest) {
        r.CanPostMessages = &can
    }
}

// WithCanEditMessages grants ability to edit messages in channels.
func WithCanEditMessages(can bool) PromoteOption {
    return func(r *PromoteChatMemberRequest) {
        r.CanEditMessages = &can
    }
}

// WithCanPinMessages grants ability to pin messages.
func WithCanPinMessages(can bool) PromoteOption {
    return func(r *PromoteChatMemberRequest) {
        r.CanPinMessages = &can
    }
}

// WithCanManageTopics grants ability to manage forum topics.
func WithCanManageTopics(can bool) PromoteOption {
    return func(r *PromoteChatMemberRequest) {
        r.CanManageTopics = &can
    }
}
```

---

## Batch B: Chat Settings (PR4-PR5)

### PR4: Chat Settings Methods

**Methods:** `SetChatPermissions`, `SetChatPhoto`, `DeleteChatPhoto`, `SetChatTitle`, `SetChatDescription`

#### `sender/chat_settings.go` (NEW FILE)

```go
package sender

import (
    "context"
    
    "github.com/prilive-com/galigo/tg"
)

// ================== Chat Settings Requests ==================

// SetChatPermissionsRequest represents a setChatPermissions request.
type SetChatPermissionsRequest struct {
    ChatID                        tg.ChatID          `json:"chat_id"`
    Permissions                   tg.ChatPermissions `json:"permissions"`
    UseIndependentChatPermissions bool               `json:"use_independent_chat_permissions,omitempty"`
}

// SetChatPhotoRequest represents a setChatPhoto request.
type SetChatPhotoRequest struct {
    ChatID tg.ChatID `json:"chat_id"`
    Photo  InputFile `json:"photo"`
}

// DeleteChatPhotoRequest represents a deleteChatPhoto request.
type DeleteChatPhotoRequest struct {
    ChatID tg.ChatID `json:"chat_id"`
}

// SetChatTitleRequest represents a setChatTitle request.
type SetChatTitleRequest struct {
    ChatID tg.ChatID `json:"chat_id"`
    Title  string    `json:"title"`
}

// SetChatDescriptionRequest represents a setChatDescription request.
type SetChatDescriptionRequest struct {
    ChatID      tg.ChatID `json:"chat_id"`
    Description string    `json:"description,omitempty"`
}

// ================== Chat Settings Methods ==================

// SetChatPermissions sets default chat permissions for all members.
// The bot must be an administrator with can_restrict_members rights.
func (c *Client) SetChatPermissions(ctx context.Context, chatID tg.ChatID, permissions tg.ChatPermissions, opts ...SetPermissionsOption) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    req := SetChatPermissionsRequest{
        ChatID:      chatID,
        Permissions: permissions,
    }
    for _, opt := range opts {
        opt(&req)
    }
    
    return c.callJSON(ctx, "setChatPermissions", req, nil)
}

// SetChatPhoto sets a new chat photo.
// The bot must be an administrator with can_change_info rights.
func (c *Client) SetChatPhoto(ctx context.Context, chatID tg.ChatID, photo InputFile) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    return c.callJSON(ctx, "setChatPhoto", SetChatPhotoRequest{
        ChatID: chatID,
        Photo:  photo,
    }, nil)
}

// DeleteChatPhoto deletes the chat photo.
// The bot must be an administrator with can_change_info rights.
func (c *Client) DeleteChatPhoto(ctx context.Context, chatID tg.ChatID) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    return c.callJSON(ctx, "deleteChatPhoto", DeleteChatPhotoRequest{
        ChatID: chatID,
    }, nil)
}

// SetChatTitle changes the title of a chat.
// The bot must be an administrator with can_change_info rights.
// Title length: 1-128 characters.
func (c *Client) SetChatTitle(ctx context.Context, chatID tg.ChatID, title string) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    if title == "" {
        return tg.NewValidationError("title", "cannot be empty")
    }
    if len(title) > 128 {
        return tg.NewValidationError("title", "must be at most 128 characters")
    }
    
    return c.callJSON(ctx, "setChatTitle", SetChatTitleRequest{
        ChatID: chatID,
        Title:  title,
    }, nil)
}

// SetChatDescription changes the description of a chat.
// The bot must be an administrator with can_change_info rights.
// Description length: 0-255 characters.
func (c *Client) SetChatDescription(ctx context.Context, chatID tg.ChatID, description string) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    if len(description) > 255 {
        return tg.NewValidationError("description", "must be at most 255 characters")
    }
    
    return c.callJSON(ctx, "setChatDescription", SetChatDescriptionRequest{
        ChatID:      chatID,
        Description: description,
    }, nil)
}

// ================== Options ==================

// SetPermissionsOption configures SetChatPermissions.
type SetPermissionsOption func(*SetChatPermissionsRequest)

// WithIndependentPermissionsForChat uses independent chat permissions.
func WithIndependentPermissionsForChat() SetPermissionsOption {
    return func(r *SetChatPermissionsRequest) {
        r.UseIndependentChatPermissions = true
    }
}
```

### PR5: Pin/Unpin/Leave Methods

**Methods:** `PinChatMessage`, `UnpinChatMessage`, `UnpinAllChatMessages`, `LeaveChat`

#### `sender/chat_pin.go` (NEW FILE)

```go
package sender

import (
    "context"
    
    "github.com/prilive-com/galigo/tg"
)

// ================== Pin Requests ==================

// PinChatMessageRequest represents a pinChatMessage request.
type PinChatMessageRequest struct {
    ChatID              tg.ChatID `json:"chat_id"`
    MessageID           int       `json:"message_id"`
    DisableNotification bool      `json:"disable_notification,omitempty"`
}

// UnpinChatMessageRequest represents an unpinChatMessage request.
type UnpinChatMessageRequest struct {
    ChatID    tg.ChatID `json:"chat_id"`
    MessageID *int      `json:"message_id,omitempty"` // nil = unpin most recent
}

// UnpinAllChatMessagesRequest represents an unpinAllChatMessages request.
type UnpinAllChatMessagesRequest struct {
    ChatID tg.ChatID `json:"chat_id"`
}

// LeaveChatRequest represents a leaveChat request.
type LeaveChatRequest struct {
    ChatID tg.ChatID `json:"chat_id"`
}

// ================== Pin Methods ==================

// PinChatMessage pins a message in a chat.
// The bot must be an administrator with can_pin_messages rights.
func (c *Client) PinChatMessage(ctx context.Context, chatID tg.ChatID, messageID int, opts ...PinOption) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    if err := validateMessageID(messageID); err != nil {
        return err
    }
    
    req := PinChatMessageRequest{
        ChatID:    chatID,
        MessageID: messageID,
    }
    for _, opt := range opts {
        opt(&req)
    }
    
    return c.callJSON(ctx, "pinChatMessage", req, nil)
}

// UnpinChatMessage unpins a message in a chat.
// If messageID is 0, unpins the most recent pinned message.
func (c *Client) UnpinChatMessage(ctx context.Context, chatID tg.ChatID, messageID int) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    req := UnpinChatMessageRequest{ChatID: chatID}
    if messageID > 0 {
        req.MessageID = &messageID
    }
    
    return c.callJSON(ctx, "unpinChatMessage", req, nil)
}

// UnpinAllChatMessages unpins all pinned messages in a chat.
func (c *Client) UnpinAllChatMessages(ctx context.Context, chatID tg.ChatID) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    return c.callJSON(ctx, "unpinAllChatMessages", UnpinAllChatMessagesRequest{
        ChatID: chatID,
    }, nil)
}

// LeaveChat makes the bot leave a group, supergroup, or channel.
func (c *Client) LeaveChat(ctx context.Context, chatID tg.ChatID) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    return c.callJSON(ctx, "leaveChat", LeaveChatRequest{
        ChatID: chatID,
    }, nil)
}

// ================== Options ==================

// PinOption configures PinChatMessage.
type PinOption func(*PinChatMessageRequest)

// WithSilentPin pins the message without notification.
func WithSilentPin() PinOption {
    return func(r *PinChatMessageRequest) {
        r.DisableNotification = true
    }
}
```

---

## Batch C: Bulk Operations (PR6)

### PR6: Bulk Message Operations

**Methods:** `ForwardMessages`, `CopyMessages` with **TYPED** options

Note: These methods already exist in `methods.go`, but need typed options.

#### `sender/bulk_options.go` (NEW FILE)

```go
package sender

// ================== Typed Bulk Options ==================

// ForwardMessagesOption configures ForwardMessages.
// This is a typed option - it only works with ForwardMessagesRequest.
type ForwardMessagesOption func(*ForwardMessagesRequest)

// WithForwardSilent forwards messages without notification.
func WithForwardSilent() ForwardMessagesOption {
    return func(r *ForwardMessagesRequest) {
        r.DisableNotification = true
    }
}

// WithForwardProtected protects forwarded messages from further forwarding.
func WithForwardProtected() ForwardMessagesOption {
    return func(r *ForwardMessagesRequest) {
        r.ProtectContent = true
    }
}

// CopyMessagesOption configures CopyMessages.
// This is a typed option - it only works with CopyMessagesRequest.
type CopyMessagesOption func(*CopyMessagesRequest)

// WithCopySilent copies messages without notification.
func WithCopySilent() CopyMessagesOption {
    return func(r *CopyMessagesRequest) {
        r.DisableNotification = true
    }
}

// WithCopyProtected protects copied messages from further forwarding.
func WithCopyProtected() CopyMessagesOption {
    return func(r *CopyMessagesRequest) {
        r.ProtectContent = true
    }
}

// WithRemoveCaption removes captions from copied messages.
func WithRemoveCaption() CopyMessagesOption {
    return func(r *CopyMessagesRequest) {
        r.RemoveCaption = true
    }
}
```

#### Update `sender/methods.go` - ForwardMessages and CopyMessages

```go
// ForwardMessages forwards multiple messages at once.
func (c *Client) ForwardMessages(ctx context.Context, chatID, fromChatID tg.ChatID, messageIDs []int, opts ...ForwardMessagesOption) ([]tg.MessageID, error) {
    if err := validateChatID(chatID); err != nil {
        return nil, err
    }
    if err := validateChatID(fromChatID); err != nil {
        return nil, err
    }
    if len(messageIDs) == 0 {
        return nil, tg.NewValidationError("message_ids", "cannot be empty")
    }
    if len(messageIDs) > 100 {
        return nil, tg.NewValidationError("message_ids", "cannot exceed 100 messages")
    }
    
    req := ForwardMessagesRequest{
        ChatID:     chatID,
        FromChatID: fromChatID,
        MessageIDs: messageIDs,
    }
    for _, opt := range opts {
        opt(&req)
    }
    
    var ids []tg.MessageID
    if err := c.callJSON(ctx, "forwardMessages", req, &ids); err != nil {
        return nil, err
    }
    return ids, nil
}

// CopyMessages copies multiple messages at once.
func (c *Client) CopyMessages(ctx context.Context, chatID, fromChatID tg.ChatID, messageIDs []int, opts ...CopyMessagesOption) ([]tg.MessageID, error) {
    if err := validateChatID(chatID); err != nil {
        return nil, err
    }
    if err := validateChatID(fromChatID); err != nil {
        return nil, err
    }
    if len(messageIDs) == 0 {
        return nil, tg.NewValidationError("message_ids", "cannot be empty")
    }
    if len(messageIDs) > 100 {
        return nil, tg.NewValidationError("message_ids", "cannot exceed 100 messages")
    }
    
    req := CopyMessagesRequest{
        ChatID:     chatID,
        FromChatID: fromChatID,
        MessageIDs: messageIDs,
    }
    for _, opt := range opts {
        opt(&req)
    }
    
    var ids []tg.MessageID
    if err := c.callJSON(ctx, "copyMessages", req, &ids); err != nil {
        return nil, err
    }
    return ids, nil
}
```

---

## Batch D: Polls & Forums (PR7-PR8)

### PR7: Poll Methods

**Methods:** `SendPoll` (enhanced), `StopPoll`

#### `sender/polls.go` (NEW FILE)

```go
package sender

import (
    "context"
    
    "github.com/prilive-com/galigo/tg"
)

// ================== Poll Requests ==================

// SendPollRequest represents a sendPoll request (enhanced version).
type SendPollRequest struct {
    ChatID                tg.ChatID       `json:"chat_id"`
    Question              string          `json:"question"`
    Options               []InputPollOption `json:"options"`
    IsAnonymous           *bool           `json:"is_anonymous,omitempty"`
    Type                  string          `json:"type,omitempty"` // "regular" or "quiz"
    AllowsMultipleAnswers bool            `json:"allows_multiple_answers,omitempty"`
    CorrectOptionID       *int            `json:"correct_option_id,omitempty"` // For quiz
    Explanation           string          `json:"explanation,omitempty"`
    ExplanationParseMode  tg.ParseMode    `json:"explanation_parse_mode,omitempty"`
    OpenPeriod            int             `json:"open_period,omitempty"`   // 5-600 seconds
    CloseDate             int64           `json:"close_date,omitempty"`
    IsClosed              bool            `json:"is_closed,omitempty"`
    DisableNotification   bool            `json:"disable_notification,omitempty"`
    ProtectContent        bool            `json:"protect_content,omitempty"`
    ReplyToMessageID      int             `json:"reply_to_message_id,omitempty"`
    ReplyMarkup           any             `json:"reply_markup,omitempty"`
}

// InputPollOption represents a poll option.
type InputPollOption struct {
    Text     string           `json:"text"`
    TextParseMode tg.ParseMode `json:"text_parse_mode,omitempty"`
    TextEntities []tg.MessageEntity `json:"text_entities,omitempty"`
}

// StopPollRequest represents a stopPoll request.
type StopPollRequest struct {
    ChatID      tg.ChatID `json:"chat_id"`
    MessageID   int       `json:"message_id"`
    ReplyMarkup any       `json:"reply_markup,omitempty"`
}

// ================== Poll Methods ==================

// SendPollSimple sends a simple regular poll.
func (c *Client) SendPollSimple(ctx context.Context, chatID tg.ChatID, question string, options []string, opts ...PollOption) (*tg.Message, error) {
    if err := validateChatID(chatID); err != nil {
        return nil, err
    }
    if question == "" {
        return nil, tg.NewValidationError("question", "cannot be empty")
    }
    if len(options) < 2 {
        return nil, tg.NewValidationError("options", "must have at least 2 options")
    }
    if len(options) > 10 {
        return nil, tg.NewValidationError("options", "cannot exceed 10 options")
    }
    
    inputOptions := make([]InputPollOption, len(options))
    for i, opt := range options {
        inputOptions[i] = InputPollOption{Text: opt}
    }
    
    req := SendPollRequest{
        ChatID:   chatID,
        Question: question,
        Options:  inputOptions,
    }
    for _, opt := range opts {
        opt(&req)
    }
    
    var msg tg.Message
    if err := c.callJSON(ctx, "sendPoll", req, &msg); err != nil {
        return nil, err
    }
    return &msg, nil
}

// SendQuiz sends a quiz poll with a correct answer.
func (c *Client) SendQuiz(ctx context.Context, chatID tg.ChatID, question string, options []string, correctOptionIndex int, opts ...PollOption) (*tg.Message, error) {
    if err := validateChatID(chatID); err != nil {
        return nil, err
    }
    if correctOptionIndex < 0 || correctOptionIndex >= len(options) {
        return nil, tg.NewValidationError("correct_option_id", "must be valid index within options")
    }
    
    inputOptions := make([]InputPollOption, len(options))
    for i, opt := range options {
        inputOptions[i] = InputPollOption{Text: opt}
    }
    
    req := SendPollRequest{
        ChatID:          chatID,
        Question:        question,
        Options:         inputOptions,
        Type:            "quiz",
        CorrectOptionID: &correctOptionIndex,
    }
    for _, opt := range opts {
        opt(&req)
    }
    
    var msg tg.Message
    if err := c.callJSON(ctx, "sendPoll", req, &msg); err != nil {
        return nil, err
    }
    return &msg, nil
}

// StopPoll stops a poll and returns the final results.
func (c *Client) StopPoll(ctx context.Context, chatID tg.ChatID, messageID int, opts ...StopPollOption) (*tg.Poll, error) {
    if err := validateChatID(chatID); err != nil {
        return nil, err
    }
    if err := validateMessageID(messageID); err != nil {
        return nil, err
    }
    
    req := StopPollRequest{
        ChatID:    chatID,
        MessageID: messageID,
    }
    for _, opt := range opts {
        opt(&req)
    }
    
    var poll tg.Poll
    if err := c.callJSON(ctx, "stopPoll", req, &poll); err != nil {
        return nil, err
    }
    return &poll, nil
}

// ================== Options ==================

// PollOption configures SendPoll.
type PollOption func(*SendPollRequest)

// WithPollAnonymous sets whether the poll is anonymous.
func WithPollAnonymous(anonymous bool) PollOption {
    return func(r *SendPollRequest) {
        r.IsAnonymous = &anonymous
    }
}

// WithMultipleAnswers allows multiple answers in regular polls.
func WithMultipleAnswers() PollOption {
    return func(r *SendPollRequest) {
        r.AllowsMultipleAnswers = true
    }
}

// WithQuizExplanation sets explanation shown after answering quiz.
func WithQuizExplanation(explanation string, parseMode tg.ParseMode) PollOption {
    return func(r *SendPollRequest) {
        r.Explanation = explanation
        r.ExplanationParseMode = parseMode
    }
}

// WithPollOpenPeriod sets how long the poll is active (5-600 seconds).
func WithPollOpenPeriod(seconds int) PollOption {
    return func(r *SendPollRequest) {
        r.OpenPeriod = seconds
    }
}

// StopPollOption configures StopPoll.
type StopPollOption func(*StopPollRequest)

// WithStopPollReplyMarkup sets inline keyboard for stopped poll.
func WithStopPollReplyMarkup(markup any) StopPollOption {
    return func(r *StopPollRequest) {
        r.ReplyMarkup = markup
    }
}
```

### PR8: Forum Topic Methods

**Methods:** `CreateForumTopic`, `EditForumTopic`, `CloseForumTopic`, `ReopenForumTopic`, `DeleteForumTopic`, `UnpinAllForumTopicMessages`, etc.

#### `sender/forum.go` (NEW FILE)

```go
package sender

import (
    "context"
    
    "github.com/prilive-com/galigo/tg"
)

// ================== Forum Requests ==================

// CreateForumTopicRequest represents a createForumTopic request.
type CreateForumTopicRequest struct {
    ChatID            tg.ChatID `json:"chat_id"`
    Name              string    `json:"name"`
    IconColor         *int      `json:"icon_color,omitempty"`
    IconCustomEmojiID string    `json:"icon_custom_emoji_id,omitempty"`
}

// EditForumTopicRequest represents an editForumTopic request.
type EditForumTopicRequest struct {
    ChatID            tg.ChatID `json:"chat_id"`
    MessageThreadID   int       `json:"message_thread_id"`
    Name              *string   `json:"name,omitempty"`
    IconCustomEmojiID *string   `json:"icon_custom_emoji_id,omitempty"`
}

// ForumTopicRequest represents a request that operates on a forum topic.
type ForumTopicRequest struct {
    ChatID          tg.ChatID `json:"chat_id"`
    MessageThreadID int       `json:"message_thread_id"`
}

// ================== Forum Methods ==================

// CreateForumTopic creates a topic in a forum supergroup chat.
// The bot must be an administrator with can_manage_topics rights.
func (c *Client) CreateForumTopic(ctx context.Context, chatID tg.ChatID, name string, opts ...CreateTopicOption) (*tg.ForumTopic, error) {
    if err := validateChatID(chatID); err != nil {
        return nil, err
    }
    if name == "" {
        return nil, tg.NewValidationError("name", "cannot be empty")
    }
    if len(name) > 128 {
        return nil, tg.NewValidationError("name", "must be at most 128 characters")
    }
    
    req := CreateForumTopicRequest{
        ChatID: chatID,
        Name:   name,
    }
    for _, opt := range opts {
        opt(&req)
    }
    
    var topic tg.ForumTopic
    if err := c.callJSON(ctx, "createForumTopic", req, &topic); err != nil {
        return nil, err
    }
    return &topic, nil
}

// EditForumTopic edits name and icon of a topic.
func (c *Client) EditForumTopic(ctx context.Context, chatID tg.ChatID, messageThreadID int, opts ...EditTopicOption) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    req := EditForumTopicRequest{
        ChatID:          chatID,
        MessageThreadID: messageThreadID,
    }
    for _, opt := range opts {
        opt(&req)
    }
    
    return c.callJSON(ctx, "editForumTopic", req, nil)
}

// CloseForumTopic closes an open topic.
func (c *Client) CloseForumTopic(ctx context.Context, chatID tg.ChatID, messageThreadID int) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    return c.callJSON(ctx, "closeForumTopic", ForumTopicRequest{
        ChatID:          chatID,
        MessageThreadID: messageThreadID,
    }, nil)
}

// ReopenForumTopic reopens a closed topic.
func (c *Client) ReopenForumTopic(ctx context.Context, chatID tg.ChatID, messageThreadID int) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    return c.callJSON(ctx, "reopenForumTopic", ForumTopicRequest{
        ChatID:          chatID,
        MessageThreadID: messageThreadID,
    }, nil)
}

// DeleteForumTopic deletes a topic along with all its messages.
func (c *Client) DeleteForumTopic(ctx context.Context, chatID tg.ChatID, messageThreadID int) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    return c.callJSON(ctx, "deleteForumTopic", ForumTopicRequest{
        ChatID:          chatID,
        MessageThreadID: messageThreadID,
    }, nil)
}

// UnpinAllForumTopicMessages unpins all messages in a topic.
func (c *Client) UnpinAllForumTopicMessages(ctx context.Context, chatID tg.ChatID, messageThreadID int) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    return c.callJSON(ctx, "unpinAllForumTopicMessages", ForumTopicRequest{
        ChatID:          chatID,
        MessageThreadID: messageThreadID,
    }, nil)
}

// EditGeneralForumTopic edits the name of the General topic.
func (c *Client) EditGeneralForumTopic(ctx context.Context, chatID tg.ChatID, name string) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    return c.callJSON(ctx, "editGeneralForumTopic", struct {
        ChatID tg.ChatID `json:"chat_id"`
        Name   string    `json:"name"`
    }{ChatID: chatID, Name: name}, nil)
}

// CloseGeneralForumTopic closes the General topic.
func (c *Client) CloseGeneralForumTopic(ctx context.Context, chatID tg.ChatID) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    return c.callJSON(ctx, "closeGeneralForumTopic", struct {
        ChatID tg.ChatID `json:"chat_id"`
    }{ChatID: chatID}, nil)
}

// ReopenGeneralForumTopic reopens the General topic.
func (c *Client) ReopenGeneralForumTopic(ctx context.Context, chatID tg.ChatID) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    return c.callJSON(ctx, "reopenGeneralForumTopic", struct {
        ChatID tg.ChatID `json:"chat_id"`
    }{ChatID: chatID}, nil)
}

// HideGeneralForumTopic hides the General topic.
func (c *Client) HideGeneralForumTopic(ctx context.Context, chatID tg.ChatID) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    return c.callJSON(ctx, "hideGeneralForumTopic", struct {
        ChatID tg.ChatID `json:"chat_id"`
    }{ChatID: chatID}, nil)
}

// UnhideGeneralForumTopic unhides the General topic.
func (c *Client) UnhideGeneralForumTopic(ctx context.Context, chatID tg.ChatID) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    return c.callJSON(ctx, "unhideGeneralForumTopic", struct {
        ChatID tg.ChatID `json:"chat_id"`
    }{ChatID: chatID}, nil)
}

// UnpinAllGeneralForumTopicMessages unpins all messages in the General topic.
func (c *Client) UnpinAllGeneralForumTopicMessages(ctx context.Context, chatID tg.ChatID) error {
    if err := validateChatID(chatID); err != nil {
        return err
    }
    
    return c.callJSON(ctx, "unpinAllGeneralForumTopicMessages", struct {
        ChatID tg.ChatID `json:"chat_id"`
    }{ChatID: chatID}, nil)
}

// GetForumTopicIconStickers returns custom emoji stickers usable as topic icons.
func (c *Client) GetForumTopicIconStickers(ctx context.Context) ([]*tg.Sticker, error) {
    var stickers []*tg.Sticker
    if err := c.callJSON(ctx, "getForumTopicIconStickers", struct{}{}, &stickers); err != nil {
        return nil, err
    }
    return stickers, nil
}

// ================== Options ==================

// CreateTopicOption configures CreateForumTopic.
type CreateTopicOption func(*CreateForumTopicRequest)

// WithTopicColor sets the icon color (use tg.ForumColor* constants).
func WithTopicColor(color int) CreateTopicOption {
    return func(r *CreateForumTopicRequest) {
        r.IconColor = &color
    }
}

// WithTopicEmoji sets a custom emoji for the topic icon.
func WithTopicEmoji(emojiID string) CreateTopicOption {
    return func(r *CreateForumTopicRequest) {
        r.IconCustomEmojiID = emojiID
    }
}

// EditTopicOption configures EditForumTopic.
type EditTopicOption func(*EditForumTopicRequest)

// WithEditTopicName changes the topic name.
func WithEditTopicName(name string) EditTopicOption {
    return func(r *EditForumTopicRequest) {
        r.Name = &name
    }
}

// WithEditTopicEmoji changes the topic icon emoji.
func WithEditTopicEmoji(emojiID string) EditTopicOption {
    return func(r *EditForumTopicRequest) {
        r.IconCustomEmojiID = &emojiID
    }
}
```

---

## PR9: Facade + Documentation

**Goal:** Add high-level Bot convenience methods and comprehensive documentation.

### `bot.go` Additions

```go
// ================== Admin Convenience Methods ==================

// BanUser bans a user from a chat.
// This is a convenience method that delegates to sender.BanChatMember.
func (b *Bot) BanUser(ctx context.Context, chatID tg.ChatID, userID int64, opts ...sender.BanOption) error {
    return b.sender.BanChatMember(ctx, chatID, userID, opts...)
}

// UnbanUser unbans a user from a chat.
func (b *Bot) UnbanUser(ctx context.Context, chatID tg.ChatID, userID int64, opts ...sender.UnbanOption) error {
    return b.sender.UnbanChatMember(ctx, chatID, userID, opts...)
}

// MuteUser restricts a user to read-only (no sending).
func (b *Bot) MuteUser(ctx context.Context, chatID tg.ChatID, userID int64, opts ...sender.RestrictOption) error {
    return b.sender.RestrictChatMember(ctx, chatID, userID, tg.ReadOnlyPermissions(), opts...)
}

// UnmuteUser removes all restrictions from a user.
func (b *Bot) UnmuteUser(ctx context.Context, chatID tg.ChatID, userID int64) error {
    return b.sender.RestrictChatMember(ctx, chatID, userID, tg.AllPermissions())
}

// PromoteToModerator promotes a user with typical moderator rights.
func (b *Bot) PromoteToModerator(ctx context.Context, chatID tg.ChatID, userID int64) error {
    return b.sender.PromoteChatMemberWithRights(ctx, chatID, userID, tg.ModeratorRights())
}

// PromoteToAdmin promotes a user with full admin rights.
func (b *Bot) PromoteToAdmin(ctx context.Context, chatID tg.ChatID, userID int64) error {
    return b.sender.PromoteChatMemberWithRights(ctx, chatID, userID, tg.FullAdminRights())
}

// Demote removes all admin privileges from a user.
func (b *Bot) Demote(ctx context.Context, chatID tg.ChatID, userID int64) error {
    return b.sender.DemoteChatMember(ctx, chatID, userID)
}
```

### Documentation Updates

1. **Update README.md** with Tier 2 examples
2. **Add MIGRATION.md** for breaking changes (if any)
3. **Update godoc** for all new types and methods
4. **Add examples/** directory with:
   - `examples/admin_bot/main.go` - Admin bot example
   - `examples/forum_bot/main.go` - Forum management example
   - `examples/poll_bot/main.go` - Poll creation example

---

## Summary: File Change List by PR

### PR-1: Foundation Fixes
```
sender/call.go           (NEW)
sender/validate.go       (NEW)
sender/security_test.go  (NEW)
sender/call_test.go      (NEW)
sender/validate_test.go  (NEW)
sender/client.go         (MODIFY: breaker, HTTP transport, error sanitization)
```

### PR0: Types
```
tg/chat_member.go        (NEW)
tg/chat_permissions.go   (NEW)
tg/chat_admin_rights.go  (NEW)
tg/chat_full_info.go     (NEW)
tg/forum.go              (NEW)
tg/chat_member_test.go   (NEW)
```

### PR1-PR3: Admin & Membership (Batch A)
```
sender/chat_info.go        (NEW)
sender/chat_moderation.go  (NEW)
sender/chat_admin.go       (NEW)
sender/chat_info_test.go   (NEW)
sender/chat_moderation_test.go (NEW)
sender/chat_admin_test.go  (NEW)
```

### PR4-PR5: Chat Settings (Batch B)
```
sender/chat_settings.go    (NEW)
sender/chat_pin.go         (NEW)
sender/chat_settings_test.go (NEW)
sender/chat_pin_test.go    (NEW)
```

### PR6: Bulk Operations (Batch C)
```
sender/bulk_options.go     (NEW)
sender/methods.go          (MODIFY: update ForwardMessages, CopyMessages)
sender/bulk_test.go        (NEW)
```

### PR7-PR8: Polls & Forums (Batch D)
```
sender/polls.go            (NEW)
sender/forum.go            (NEW)
sender/polls_test.go       (NEW)
sender/forum_test.go       (NEW)
```

### PR9: Facade + Docs
```
bot.go                     (MODIFY: add convenience methods)
README.md                  (MODIFY)
examples/admin_bot/main.go (NEW)
examples/forum_bot/main.go (NEW)
examples/poll_bot/main.go  (NEW)
```

---

## Timeline

| Week | PRs | Description |
|------|-----|-------------|
| 1 | PR-1, PR0 | Foundation fixes + types |
| 2 | PR1, PR2, PR3 | Admin & membership (Batch A) |
| 2 | PR4, PR5 | Chat settings (Batch B) - parallel |
| 3 | PR6 | Bulk operations (Batch C) |
| 3 | PR7, PR8 | Polls & forums (Batch D) - parallel |
| 4 | PR9 | Facade + docs + polish |

**Total: ~3-4 weeks** (with parallel execution of independent batches)

---

## Acceptance Criteria Summary

Each PR must meet:
1. [ ] All new code has tests with >80% coverage
2. [ ] No breaking changes to existing API (unless documented)
3. [ ] All validation uses `validateChatID()` / `validateUserID()` helpers
4. [ ] All methods use `callJSON()` for consistency
5. [ ] All options are typed (no `func(any)`)
6. [ ] No bot token appears in any error message
7. [ ] JSON tags match Telegram Bot API exactly
8. [ ] Godoc comments on all exported types/functions
9. [ ] CI passes (lint, test, build)