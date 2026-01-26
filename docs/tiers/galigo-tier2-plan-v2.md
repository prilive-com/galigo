# galigo Tier 2 Implementation Plan v2.0

## Consolidated Technical Specification

**Version:** 2.0 (Enhanced)  
**Target:** galigo v2.1.0  
**Telegram Bot API:** 9.3 (Dec 31, 2025)  
**Go Version:** 1.25  
**Estimated Effort:** 4-5 weeks (10 PRs)  
**Prerequisites:** Tier 1 complete (unified `sender.Call*`, `tg.InputFile`, `tg.ChatID`, correct error parsing)

---

## Executive Summary

This consolidated plan combines multiple independent analyses to deliver the methods most production bots need beyond "toy bot" stage.

### Key Improvements in v2.0

| Area | v1.0 Plan | v2.0 Enhanced Plan |
|------|-----------|-------------------|
| ChatMember | Flat struct | **Union types** (Owner/Admin/Member/Restricted/Left/Banned) |
| Admin Rights | Basic fields | **Includes `can_manage_direct_messages`** (Bot API 9.2) |
| Bulk Ops | Combined | **Separate PR with `direct_messages_topic_id`** |
| Forum Topics | Basic | **"Topics in private chats" compatible** (Bot API 9.3) |
| Pin Methods | Basic | **`business_connection_id` + specific `message_id` unpin** |
| Type Strategy | All fields | **Minimal fields + pointers for optionals** |
| Polls | In media | **Separate PR for completeness** |

---

## Tier 2 Goals

Deliver the methods production bots need:

| Category | Methods | Use Case |
|----------|---------|----------|
| Chat & Membership | 5 | Get chat info, member status |
| Moderation | 7 | Bans, restrictions, promotions |
| Invite Links | 6 | Create/manage invites, join requests |
| Pins & Appearance | 7 | Pin messages, set chat photo/title |
| Commands & Menu | 11 | Bot commands, menu button, profile |
| Bulk Operations | 2 | Copy/forward multiple messages |
| Polls | 2 | Send and stop polls |
| Forum Topics | 6+ | Topic management |
| **TOTAL** | **~46** | |

---

## PR Dependency Graph

```
Tier 1 Complete
      │
      ▼
PR0 (Tier 2 Types) ◄─────────────────────────┐
      │                                       │
      ├───────┬───────┬───────┬───────┐      │
      ▼       ▼       ▼       ▼       ▼      │
    PR1     PR2     PR3     PR4     PR5      │
  (Chat)  (Mod)  (Invite) (Pins) (Commands)  │
      │       │       │       │       │      │
      └───────┴───────┴───────┴───────┘      │
                      │                       │
                      ▼                       │
              PR6 (Bulk Ops) ─────────────────┤
                      │                       │
                      ▼                       │
              PR7 (Polls) ────────────────────┤
                      │                       │
                      ▼                       │
              PR8 (Forum Topics) ─────────────┘
                      │
                      ▼
              PR9 (Facade + Docs + Release)
```

---

## PR0: Tier 2 Type Definitions

**Goal:** Define all types needed by Tier 2 methods with proper union modeling.  
**Estimated Time:** 6-8 hours  
**Breaking Changes:** None (additive)

### Design Principles

1. **Minimal field coverage** - Start with essential fields, leave room for expansion
2. **Pointers for optionals** - Use `*bool`, `*int` for optional fields
3. **Union types** - Model Telegram's "one of" types properly
4. **Future-proof** - Unknown fields can be ignored safely

### 0.1 ChatMember Union Types

**File:** `tg/chat_member.go` (new)

```go
package tg

import (
    "encoding/json"
    "fmt"
)

// ChatMember represents a member of a chat.
// Use the type-specific methods or type assertion to access status-specific fields.
type ChatMember interface {
    chatMember()
    GetStatus() string
    GetUser() *User
}

// chatMemberBase contains fields common to all ChatMember types
type chatMemberBase struct {
    Status string `json:"status"`
    User   *User  `json:"user"`
}

func (b chatMemberBase) GetStatus() string { return b.Status }
func (b chatMemberBase) GetUser() *User    { return b.User }

// ChatMemberOwner represents a chat owner
type ChatMemberOwner struct {
    chatMemberBase
    IsAnonymous bool   `json:"is_anonymous"`
    CustomTitle string `json:"custom_title,omitempty"`
}

func (ChatMemberOwner) chatMember() {}

// ChatMemberAdministrator represents an administrator
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
    CanPostStories        bool   `json:"can_post_stories"`
    CanEditStories        bool   `json:"can_edit_stories"`
    CanDeleteStories      bool   `json:"can_delete_stories"`
    CanPostMessages       *bool  `json:"can_post_messages,omitempty"`       // Channels only
    CanEditMessages       *bool  `json:"can_edit_messages,omitempty"`       // Channels only
    CanPinMessages        *bool  `json:"can_pin_messages,omitempty"`
    CanManageTopics       *bool  `json:"can_manage_topics,omitempty"`
    CanManageDirectMessages *bool `json:"can_manage_direct_messages,omitempty"` // Bot API 9.2
    CustomTitle           string `json:"custom_title,omitempty"`
}

func (ChatMemberAdministrator) chatMember() {}

// ChatMemberMember represents a regular member
type ChatMemberMember struct {
    chatMemberBase
    UntilDate *int64 `json:"until_date,omitempty"` // Bot API 8.1: premium subscription expiry
}

func (ChatMemberMember) chatMember() {}

// ChatMemberRestricted represents a restricted user
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

func (ChatMemberRestricted) chatMember() {}

// ChatMemberLeft represents a user who left or was removed
type ChatMemberLeft struct {
    chatMemberBase
}

func (ChatMemberLeft) chatMember() {}

// ChatMemberBanned represents a banned user
type ChatMemberBanned struct {
    chatMemberBase
    UntilDate int64 `json:"until_date"`
}

func (ChatMemberBanned) chatMember() {}

// UnmarshalChatMember decodes a ChatMember from JSON
func UnmarshalChatMember(data []byte) (ChatMember, error) {
    // First, decode just the status
    var base struct {
        Status string `json:"status"`
    }
    if err := json.Unmarshal(data, &base); err != nil {
        return nil, err
    }
    
    // Then decode into the appropriate type
    switch base.Status {
    case "creator":
        var m ChatMemberOwner
        if err := json.Unmarshal(data, &m); err != nil {
            return nil, err
        }
        return m, nil
    case "administrator":
        var m ChatMemberAdministrator
        if err := json.Unmarshal(data, &m); err != nil {
            return nil, err
        }
        return m, nil
    case "member":
        var m ChatMemberMember
        if err := json.Unmarshal(data, &m); err != nil {
            return nil, err
        }
        return m, nil
    case "restricted":
        var m ChatMemberRestricted
        if err := json.Unmarshal(data, &m); err != nil {
            return nil, err
        }
        return m, nil
    case "left":
        var m ChatMemberLeft
        if err := json.Unmarshal(data, &m); err != nil {
            return nil, err
        }
        return m, nil
    case "kicked":
        var m ChatMemberBanned
        if err := json.Unmarshal(data, &m); err != nil {
            return nil, err
        }
        return m, nil
    default:
        return nil, fmt.Errorf("unknown chat member status: %s", base.Status)
    }
}

// Helper methods for type checking
func IsOwner(m ChatMember) bool         { _, ok := m.(ChatMemberOwner); return ok }
func IsAdministrator(m ChatMember) bool { _, ok := m.(ChatMemberAdministrator); return ok }
func IsMember(m ChatMember) bool        { _, ok := m.(ChatMemberMember); return ok }
func IsRestricted(m ChatMember) bool    { _, ok := m.(ChatMemberRestricted); return ok }
func HasLeft(m ChatMember) bool         { _, ok := m.(ChatMemberLeft); return ok }
func IsBanned(m ChatMember) bool        { _, ok := m.(ChatMemberBanned); return ok }

// IsAdmin returns true if the member is owner or administrator
func IsAdmin(m ChatMember) bool {
    return IsOwner(m) || IsAdministrator(m)
}
```

### 0.2 ChatAdministratorRights (with can_manage_direct_messages)

**File:** `tg/chat_rights.go` (new)

```go
package tg

// ChatAdministratorRights represents the rights of an administrator.
// Includes can_manage_direct_messages added in Bot API 9.2.
type ChatAdministratorRights struct {
    IsAnonymous             bool  `json:"is_anonymous"`
    CanManageChat           bool  `json:"can_manage_chat"`
    CanDeleteMessages       bool  `json:"can_delete_messages"`
    CanManageVideoChats     bool  `json:"can_manage_video_chats"`
    CanRestrictMembers      bool  `json:"can_restrict_members"`
    CanPromoteMembers       bool  `json:"can_promote_members"`
    CanChangeInfo           bool  `json:"can_change_info"`
    CanInviteUsers          bool  `json:"can_invite_users"`
    CanPostStories          bool  `json:"can_post_stories"`
    CanEditStories          bool  `json:"can_edit_stories"`
    CanDeleteStories        bool  `json:"can_delete_stories"`
    CanPostMessages         *bool `json:"can_post_messages,omitempty"`         // Channels only
    CanEditMessages         *bool `json:"can_edit_messages,omitempty"`         // Channels only
    CanPinMessages          *bool `json:"can_pin_messages,omitempty"`
    CanManageTopics         *bool `json:"can_manage_topics,omitempty"`
    CanManageDirectMessages *bool `json:"can_manage_direct_messages,omitempty"` // Bot API 9.2
}

// FullAdminRights returns ChatAdministratorRights with all permissions enabled
func FullAdminRights() ChatAdministratorRights {
    t := true
    return ChatAdministratorRights{
        IsAnonymous:             false,
        CanManageChat:           true,
        CanDeleteMessages:       true,
        CanManageVideoChats:     true,
        CanRestrictMembers:      true,
        CanPromoteMembers:       true,
        CanChangeInfo:           true,
        CanInviteUsers:          true,
        CanPostStories:          true,
        CanEditStories:          true,
        CanDeleteStories:        true,
        CanPostMessages:         &t,
        CanEditMessages:         &t,
        CanPinMessages:          &t,
        CanManageTopics:         &t,
        CanManageDirectMessages: &t,
    }
}

// ModeratorRights returns rights suitable for a moderator (can ban, restrict, delete)
func ModeratorRights() ChatAdministratorRights {
    t := true
    return ChatAdministratorRights{
        CanManageChat:       true,
        CanDeleteMessages:   true,
        CanRestrictMembers:  true,
        CanPinMessages:      &t,
    }
}
```

### 0.3 ChatPermissions

```go
// ChatPermissions describes actions that a non-administrator user is allowed to take
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

// AllPermissions returns ChatPermissions with all permissions enabled
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

// NoPermissions returns ChatPermissions with all permissions disabled
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

// TextOnlyPermissions allows only text messages
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

### 0.4 BotCommandScope Union Types

**File:** `tg/bot_command.go` (new)

```go
package tg

import "encoding/json"

// BotCommand represents a bot command
type BotCommand struct {
    Command     string `json:"command"`     // 1-32 chars
    Description string `json:"description"` // 1-256 chars
}

// BotCommandScope represents the scope of bot commands
type BotCommandScope interface {
    botCommandScope()
    ScopeType() string
}

type botCommandScopeBase struct {
    Type string `json:"type"`
}

func (b botCommandScopeBase) ScopeType() string { return b.Type }

// BotCommandScopeDefault - default scope (all private chats)
type BotCommandScopeDefault struct {
    botCommandScopeBase
}

func (BotCommandScopeDefault) botCommandScope() {}

func ScopeDefault() BotCommandScopeDefault {
    return BotCommandScopeDefault{botCommandScopeBase{Type: "default"}}
}

// BotCommandScopeAllPrivateChats - all private chats
type BotCommandScopeAllPrivateChats struct {
    botCommandScopeBase
}

func (BotCommandScopeAllPrivateChats) botCommandScope() {}

func ScopeAllPrivateChats() BotCommandScopeAllPrivateChats {
    return BotCommandScopeAllPrivateChats{botCommandScopeBase{Type: "all_private_chats"}}
}

// BotCommandScopeAllGroupChats - all group and supergroup chats
type BotCommandScopeAllGroupChats struct {
    botCommandScopeBase
}

func (BotCommandScopeAllGroupChats) botCommandScope() {}

func ScopeAllGroupChats() BotCommandScopeAllGroupChats {
    return BotCommandScopeAllGroupChats{botCommandScopeBase{Type: "all_group_chats"}}
}

// BotCommandScopeAllChatAdministrators - all chat administrators
type BotCommandScopeAllChatAdministrators struct {
    botCommandScopeBase
}

func (BotCommandScopeAllChatAdministrators) botCommandScope() {}

func ScopeAllChatAdministrators() BotCommandScopeAllChatAdministrators {
    return BotCommandScopeAllChatAdministrators{botCommandScopeBase{Type: "all_chat_administrators"}}
}

// BotCommandScopeChat - a specific chat
type BotCommandScopeChat struct {
    botCommandScopeBase
    ChatID ChatID `json:"chat_id"`
}

func (BotCommandScopeChat) botCommandScope() {}

func ScopeChat(chatID ChatID) BotCommandScopeChat {
    return BotCommandScopeChat{
        botCommandScopeBase: botCommandScopeBase{Type: "chat"},
        ChatID:              chatID,
    }
}

// BotCommandScopeChatAdministrators - administrators of a specific chat
type BotCommandScopeChatAdministrators struct {
    botCommandScopeBase
    ChatID ChatID `json:"chat_id"`
}

func (BotCommandScopeChatAdministrators) botCommandScope() {}

func ScopeChatAdministrators(chatID ChatID) BotCommandScopeChatAdministrators {
    return BotCommandScopeChatAdministrators{
        botCommandScopeBase: botCommandScopeBase{Type: "chat_administrators"},
        ChatID:              chatID,
    }
}

// BotCommandScopeChatMember - a specific member of a specific chat
type BotCommandScopeChatMember struct {
    botCommandScopeBase
    ChatID ChatID `json:"chat_id"`
    UserID int64  `json:"user_id"`
}

func (BotCommandScopeChatMember) botCommandScope() {}

func ScopeChatMember(chatID ChatID, userID int64) BotCommandScopeChatMember {
    return BotCommandScopeChatMember{
        botCommandScopeBase: botCommandScopeBase{Type: "chat_member"},
        ChatID:              chatID,
        UserID:              userID,
    }
}
```

### 0.5 MenuButton Union Types

```go
// MenuButton represents a bot menu button
type MenuButton interface {
    menuButton()
    ButtonType() string
}

type menuButtonBase struct {
    Type string `json:"type"`
}

func (b menuButtonBase) ButtonType() string { return b.Type }

// MenuButtonCommands - shows the bot's command list
type MenuButtonCommands struct {
    menuButtonBase
}

func (MenuButtonCommands) menuButton() {}

func ButtonCommands() MenuButtonCommands {
    return MenuButtonCommands{menuButtonBase{Type: "commands"}}
}

// MenuButtonWebApp - launches a Web App
type MenuButtonWebApp struct {
    menuButtonBase
    Text   string      `json:"text"`
    WebApp *WebAppInfo `json:"web_app"`
}

func (MenuButtonWebApp) menuButton() {}

func ButtonWebApp(text string, webAppURL string) MenuButtonWebApp {
    return MenuButtonWebApp{
        menuButtonBase: menuButtonBase{Type: "web_app"},
        Text:           text,
        WebApp:         &WebAppInfo{URL: webAppURL},
    }
}

// MenuButtonDefault - no specific button
type MenuButtonDefault struct {
    menuButtonBase
}

func (MenuButtonDefault) menuButton() {}

func ButtonDefault() MenuButtonDefault {
    return MenuButtonDefault{menuButtonBase{Type: "default"}}
}

// WebAppInfo describes a Web App
type WebAppInfo struct {
    URL string `json:"url"`
}

// UnmarshalMenuButton decodes a MenuButton from JSON
func UnmarshalMenuButton(data []byte) (MenuButton, error) {
    var base struct {
        Type string `json:"type"`
    }
    if err := json.Unmarshal(data, &base); err != nil {
        return nil, err
    }
    
    switch base.Type {
    case "commands":
        return MenuButtonCommands{menuButtonBase{Type: "commands"}}, nil
    case "web_app":
        var btn MenuButtonWebApp
        if err := json.Unmarshal(data, &btn); err != nil {
            return nil, err
        }
        return btn, nil
    case "default":
        return MenuButtonDefault{menuButtonBase{Type: "default"}}, nil
    default:
        return MenuButtonDefault{menuButtonBase{Type: "default"}}, nil
    }
}
```

### 0.6 Other Essential Types

```go
// ChatInviteLink represents an invite link for a chat
type ChatInviteLink struct {
    InviteLink              string `json:"invite_link"`
    Creator                 *User  `json:"creator"`
    CreatesJoinRequest      bool   `json:"creates_join_request"`
    IsPrimary               bool   `json:"is_primary"`
    IsRevoked               bool   `json:"is_revoked"`
    Name                    string `json:"name,omitempty"`
    ExpireDate              *int64 `json:"expire_date,omitempty"`
    MemberLimit             *int   `json:"member_limit,omitempty"`
    PendingJoinRequestCount *int   `json:"pending_join_request_count,omitempty"`
    SubscriptionPeriod      *int   `json:"subscription_period,omitempty"` // Bot API 7.9
    SubscriptionPrice       *int   `json:"subscription_price,omitempty"`  // Bot API 7.9
}

// ForumTopic represents a forum topic
type ForumTopic struct {
    MessageThreadID   int    `json:"message_thread_id"`
    Name              string `json:"name"`
    IconColor         int    `json:"icon_color"`
    IconCustomEmojiID string `json:"icon_custom_emoji_id,omitempty"`
}

// Poll represents a poll
type Poll struct {
    ID                    string          `json:"id"`
    Question              string          `json:"question"`
    QuestionEntities      []MessageEntity `json:"question_entities,omitempty"`
    Options               []PollOption    `json:"options"`
    TotalVoterCount       int             `json:"total_voter_count"`
    IsClosed              bool            `json:"is_closed"`
    IsAnonymous           bool            `json:"is_anonymous"`
    Type                  string          `json:"type"` // "regular" or "quiz"
    AllowsMultipleAnswers bool            `json:"allows_multiple_answers"`
    CorrectOptionID       *int            `json:"correct_option_id,omitempty"`
    Explanation           string          `json:"explanation,omitempty"`
    ExplanationEntities   []MessageEntity `json:"explanation_entities,omitempty"`
    OpenPeriod            *int            `json:"open_period,omitempty"`
    CloseDate             *int64          `json:"close_date,omitempty"`
}

// PollOption contains information about one answer option in a poll
type PollOption struct {
    Text         string          `json:"text"`
    TextEntities []MessageEntity `json:"text_entities,omitempty"`
    VoterCount   int             `json:"voter_count"`
}

// InputPollOption represents a poll option to be sent
type InputPollOption struct {
    Text          string          `json:"text"`
    TextParseMode ParseMode       `json:"text_parse_mode,omitempty"`
    TextEntities  []MessageEntity `json:"text_entities,omitempty"`
}

// ChatFullInfo contains full information about a chat (from getChat)
type ChatFullInfo struct {
    ID                          int64             `json:"id"`
    Type                        string            `json:"type"`
    Title                       string            `json:"title,omitempty"`
    Username                    string            `json:"username,omitempty"`
    FirstName                   string            `json:"first_name,omitempty"`
    LastName                    string            `json:"last_name,omitempty"`
    IsForum                     bool              `json:"is_forum,omitempty"`
    AccentColorID               *int              `json:"accent_color_id,omitempty"`
    MaxReactionCount            *int              `json:"max_reaction_count,omitempty"`
    Photo                       *ChatPhoto        `json:"photo,omitempty"`
    ActiveUsernames             []string          `json:"active_usernames,omitempty"`
    Bio                         string            `json:"bio,omitempty"`
    HasPrivateForwards          *bool             `json:"has_private_forwards,omitempty"`
    Description                 string            `json:"description,omitempty"`
    InviteLink                  string            `json:"invite_link,omitempty"`
    PinnedMessage               *Message          `json:"pinned_message,omitempty"`
    Permissions                 *ChatPermissions  `json:"permissions,omitempty"`
    SlowModeDelay               *int              `json:"slow_mode_delay,omitempty"`
    MessageAutoDeleteTime       *int              `json:"message_auto_delete_time,omitempty"`
    HasProtectedContent         *bool             `json:"has_protected_content,omitempty"`
    HasVisibleHistory           *bool             `json:"has_visible_history,omitempty"`
    StickerSetName              string            `json:"sticker_set_name,omitempty"`
    CanSetStickerSet            *bool             `json:"can_set_sticker_set,omitempty"`
    LinkedChatID                *int64            `json:"linked_chat_id,omitempty"`
    Location                    *ChatLocation     `json:"location,omitempty"`
    // Bot API 9.x fields
    CanSendPaidMedia            *bool             `json:"can_send_paid_media,omitempty"`
    UnrestrictBoostCount        *int              `json:"unrestrict_boost_count,omitempty"`
    HasAggressiveAntiSpamEnabled *bool            `json:"has_aggressive_anti_spam_enabled,omitempty"`
    HasHiddenMembers            *bool             `json:"has_hidden_members,omitempty"`
}
```

### Definition of Done (PR0)

- [ ] ChatMember union types with proper unmarshaling
- [ ] ChatAdministratorRights with `can_manage_direct_messages`
- [ ] ChatPermissions with preset constructors
- [ ] BotCommandScope union types with constructors
- [ ] MenuButton union types with constructors
- [ ] ChatInviteLink, ForumTopic, Poll types
- [ ] ChatFullInfo with Bot API 9.x fields
- [ ] All JSON marshaling/unmarshaling tested
- [ ] No `map[string]any` in exported signatures

---

## PR1: Chat Info & Membership Methods

**Goal:** Get chat info and member status.  
**Estimated Time:** 3-4 hours  
**Breaking Changes:** None (additive)  
**Dependencies:** PR0

### Methods

| Method | Description |
|--------|-------------|
| `getChat` | Get full chat info |
| `getChatAdministrators` | List all administrators |
| `getChatMember` | Get specific member info |
| `getChatMemberCount` | Get member count |
| `leaveChat` | Make bot leave |

### Implementation

**File:** `sender/methods_chat_info.go`

```go
package sender

import (
    "context"
    "encoding/json"
    
    "github.com/example/galigo/tg"
)

// GetChat returns up-to-date information about the chat.
func (c *Client) GetChat(ctx context.Context, chatID tg.ChatID) (*tg.ChatFullInfo, error) {
    if chatID.IsZero() {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    
    params := map[string]any{"chat_id": chatID}
    
    var chat tg.ChatFullInfo
    if err := c.executor.Call(ctx, "getChat", params, &chat); err != nil {
        return nil, err
    }
    return &chat, nil
}

// GetChatAdministrators returns a list of administrators in a chat.
func (c *Client) GetChatAdministrators(ctx context.Context, chatID tg.ChatID) ([]tg.ChatMember, error) {
    if chatID.IsZero() {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    
    params := map[string]any{"chat_id": chatID}
    
    // Get raw JSON array
    var rawMembers []json.RawMessage
    if err := c.executor.Call(ctx, "getChatAdministrators", params, &rawMembers); err != nil {
        return nil, err
    }
    
    // Unmarshal each member using union type decoder
    members := make([]tg.ChatMember, len(rawMembers))
    for i, raw := range rawMembers {
        m, err := tg.UnmarshalChatMember(raw)
        if err != nil {
            return nil, fmt.Errorf("failed to decode member %d: %w", i, err)
        }
        members[i] = m
    }
    
    return members, nil
}

// GetChatMember returns information about a member of a chat.
func (c *Client) GetChatMember(ctx context.Context, chatID tg.ChatID, userID int64) (tg.ChatMember, error) {
    if chatID.IsZero() {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    if userID == 0 {
        return nil, tg.NewValidationError("user_id", "is required")
    }
    
    params := map[string]any{
        "chat_id": chatID,
        "user_id": userID,
    }
    
    var raw json.RawMessage
    if err := c.executor.Call(ctx, "getChatMember", params, &raw); err != nil {
        return nil, err
    }
    
    return tg.UnmarshalChatMember(raw)
}

// GetChatMemberCount returns the number of members in a chat.
func (c *Client) GetChatMemberCount(ctx context.Context, chatID tg.ChatID) (int, error) {
    if chatID.IsZero() {
        return 0, tg.NewValidationError("chat_id", "is required")
    }
    
    params := map[string]any{"chat_id": chatID}
    
    var count int
    if err := c.executor.Call(ctx, "getChatMemberCount", params, &count); err != nil {
        return 0, err
    }
    return count, nil
}

// LeaveChat makes the bot leave a group, supergroup or channel.
func (c *Client) LeaveChat(ctx context.Context, chatID tg.ChatID) error {
    if chatID.IsZero() {
        return tg.NewValidationError("chat_id", "is required")
    }
    
    params := map[string]any{"chat_id": chatID}
    return c.executor.Call(ctx, "leaveChat", params, nil)
}
```

### Tests

```go
func TestGetChatMember_UnmarshalUnionType(t *testing.T) {
    tests := []struct {
        name   string
        json   string
        status string
        check  func(t *testing.T, m tg.ChatMember)
    }{
        {
            name:   "owner",
            json:   `{"status":"creator","user":{"id":123},"is_anonymous":true}`,
            status: "creator",
            check: func(t *testing.T, m tg.ChatMember) {
                owner, ok := m.(tg.ChatMemberOwner)
                require.True(t, ok)
                assert.True(t, owner.IsAnonymous)
            },
        },
        {
            name:   "administrator with can_manage_direct_messages",
            json:   `{"status":"administrator","user":{"id":456},"can_manage_direct_messages":true}`,
            status: "administrator",
            check: func(t *testing.T, m tg.ChatMember) {
                admin, ok := m.(tg.ChatMemberAdministrator)
                require.True(t, ok)
                require.NotNil(t, admin.CanManageDirectMessages)
                assert.True(t, *admin.CanManageDirectMessages)
            },
        },
        {
            name:   "restricted",
            json:   `{"status":"restricted","user":{"id":789},"can_send_messages":false,"until_date":1700000000}`,
            status: "restricted",
            check: func(t *testing.T, m tg.ChatMember) {
                restricted, ok := m.(tg.ChatMemberRestricted)
                require.True(t, ok)
                assert.False(t, restricted.CanSendMessages)
                assert.Equal(t, int64(1700000000), restricted.UntilDate)
            },
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            m, err := tg.UnmarshalChatMember([]byte(tt.json))
            require.NoError(t, err)
            assert.Equal(t, tt.status, m.GetStatus())
            tt.check(t, m)
        })
    }
}
```

### Definition of Done (PR1)

- [ ] All 5 methods implemented
- [ ] ChatMember union decoding works correctly
- [ ] `@username` ChatID serialization tested
- [ ] Response decoding tests for all ChatMember variants
- [ ] Error handling for API errors

---

## PR2: Moderation/Admin Actions

**Goal:** Ban, restrict, promote members, set permissions.  
**Estimated Time:** 5-6 hours  
**Breaking Changes:** None (additive)  
**Dependencies:** PR0

### Methods

| Method | Description |
|--------|-------------|
| `banChatMember` | Ban a user |
| `unbanChatMember` | Unban a user |
| `restrictChatMember` | Restrict user permissions |
| `promoteChatMember` | Promote/demote to admin |
| `setChatAdministratorCustomTitle` | Set admin custom title |
| `setChatPermissions` | Set default chat permissions |

### Implementation

**File:** `sender/methods_moderation.go`

```go
package sender

import (
    "context"
    "time"
    
    "github.com/example/galigo/tg"
)

// BanChatMemberRequest contains parameters for banChatMember
type BanChatMemberRequest struct {
    ChatID         tg.ChatID `json:"chat_id"`
    UserID         int64     `json:"user_id"`
    UntilDate      *int64    `json:"until_date,omitempty"`
    RevokeMessages *bool     `json:"revoke_messages,omitempty"`
}

// BanChatMember bans a user in a group, supergroup or channel.
func (c *Client) BanChatMember(ctx context.Context, chatID tg.ChatID, userID int64, opts ...BanOption) error {
    if chatID.IsZero() {
        return tg.NewValidationError("chat_id", "is required")
    }
    if userID == 0 {
        return tg.NewValidationError("user_id", "is required")
    }
    
    req := BanChatMemberRequest{
        ChatID: chatID,
        UserID: userID,
    }
    for _, opt := range opts {
        opt(&req)
    }
    
    return c.executor.Call(ctx, "banChatMember", req, nil)
}

// BanOption configures a ban request
type BanOption func(*BanChatMemberRequest)

// WithBanUntil sets the ban expiration date
func WithBanUntil(t time.Time) BanOption {
    return func(r *BanChatMemberRequest) {
        unix := t.Unix()
        r.UntilDate = &unix
    }
}

// WithBanDuration sets the ban duration from now
func WithBanDuration(d time.Duration) BanOption {
    return func(r *BanChatMemberRequest) {
        unix := time.Now().Add(d).Unix()
        r.UntilDate = &unix
    }
}

// WithRevokeMessages removes all messages from the banned user
func WithRevokeMessages(revoke bool) BanOption {
    return func(r *BanChatMemberRequest) {
        r.RevokeMessages = &revoke
    }
}

// UnbanChatMember unbans a previously banned user.
func (c *Client) UnbanChatMember(ctx context.Context, chatID tg.ChatID, userID int64, onlyIfBanned bool) error {
    if chatID.IsZero() {
        return tg.NewValidationError("chat_id", "is required")
    }
    if userID == 0 {
        return tg.NewValidationError("user_id", "is required")
    }
    
    params := map[string]any{
        "chat_id":        chatID,
        "user_id":        userID,
        "only_if_banned": onlyIfBanned,
    }
    
    return c.executor.Call(ctx, "unbanChatMember", params, nil)
}

// RestrictChatMemberRequest contains parameters for restrictChatMember
type RestrictChatMemberRequest struct {
    ChatID                        tg.ChatID          `json:"chat_id"`
    UserID                        int64              `json:"user_id"`
    Permissions                   tg.ChatPermissions `json:"permissions"`
    UseIndependentChatPermissions *bool              `json:"use_independent_chat_permissions,omitempty"`
    UntilDate                     *int64             `json:"until_date,omitempty"`
}

// RestrictChatMember restricts a user in a supergroup.
func (c *Client) RestrictChatMember(ctx context.Context, chatID tg.ChatID, userID int64, permissions tg.ChatPermissions, opts ...RestrictOption) error {
    if chatID.IsZero() {
        return tg.NewValidationError("chat_id", "is required")
    }
    if userID == 0 {
        return tg.NewValidationError("user_id", "is required")
    }
    
    req := RestrictChatMemberRequest{
        ChatID:      chatID,
        UserID:      userID,
        Permissions: permissions,
    }
    for _, opt := range opts {
        opt(&req)
    }
    
    return c.executor.Call(ctx, "restrictChatMember", req, nil)
}

// RestrictOption configures a restrict request
type RestrictOption func(*RestrictChatMemberRequest)

// WithRestrictUntil sets the restriction expiration date
func WithRestrictUntil(t time.Time) RestrictOption {
    return func(r *RestrictChatMemberRequest) {
        unix := t.Unix()
        r.UntilDate = &unix
    }
}

// WithIndependentPermissions uses independent chat permissions
func WithIndependentPermissions(use bool) RestrictOption {
    return func(r *RestrictChatMemberRequest) {
        r.UseIndependentChatPermissions = &use
    }
}

// PromoteChatMemberRequest contains parameters for promoteChatMember
// Uses pointers to distinguish between "not set" and "set to false"
type PromoteChatMemberRequest struct {
    ChatID                  tg.ChatID `json:"chat_id"`
    UserID                  int64     `json:"user_id"`
    IsAnonymous             *bool     `json:"is_anonymous,omitempty"`
    CanManageChat           *bool     `json:"can_manage_chat,omitempty"`
    CanDeleteMessages       *bool     `json:"can_delete_messages,omitempty"`
    CanManageVideoChats     *bool     `json:"can_manage_video_chats,omitempty"`
    CanRestrictMembers      *bool     `json:"can_restrict_members,omitempty"`
    CanPromoteMembers       *bool     `json:"can_promote_members,omitempty"`
    CanChangeInfo           *bool     `json:"can_change_info,omitempty"`
    CanInviteUsers          *bool     `json:"can_invite_users,omitempty"`
    CanPostStories          *bool     `json:"can_post_stories,omitempty"`
    CanEditStories          *bool     `json:"can_edit_stories,omitempty"`
    CanDeleteStories        *bool     `json:"can_delete_stories,omitempty"`
    CanPostMessages         *bool     `json:"can_post_messages,omitempty"`         // Channels only
    CanEditMessages         *bool     `json:"can_edit_messages,omitempty"`         // Channels only
    CanPinMessages          *bool     `json:"can_pin_messages,omitempty"`
    CanManageTopics         *bool     `json:"can_manage_topics,omitempty"`
    CanManageDirectMessages *bool     `json:"can_manage_direct_messages,omitempty"` // Bot API 9.2
}

// PromoteChatMember promotes or demotes a user in a supergroup or channel.
func (c *Client) PromoteChatMember(ctx context.Context, req PromoteChatMemberRequest) error {
    if req.ChatID.IsZero() {
        return tg.NewValidationError("chat_id", "is required")
    }
    if req.UserID == 0 {
        return tg.NewValidationError("user_id", "is required")
    }
    
    return c.executor.Call(ctx, "promoteChatMember", req, nil)
}

// SetChatAdministratorCustomTitle sets a custom title for an administrator.
func (c *Client) SetChatAdministratorCustomTitle(ctx context.Context, chatID tg.ChatID, userID int64, customTitle string) error {
    if chatID.IsZero() {
        return tg.NewValidationError("chat_id", "is required")
    }
    if userID == 0 {
        return tg.NewValidationError("user_id", "is required")
    }
    if len(customTitle) > 16 {
        return tg.NewValidationError("custom_title", "must be at most 16 characters")
    }
    
    params := map[string]any{
        "chat_id":      chatID,
        "user_id":      userID,
        "custom_title": customTitle,
    }
    
    return c.executor.Call(ctx, "setChatAdministratorCustomTitle", params, nil)
}

// SetChatPermissions sets default chat permissions for all members.
func (c *Client) SetChatPermissions(ctx context.Context, chatID tg.ChatID, permissions tg.ChatPermissions, useIndependent *bool) error {
    if chatID.IsZero() {
        return tg.NewValidationError("chat_id", "is required")
    }
    
    params := map[string]any{
        "chat_id":     chatID,
        "permissions": permissions,
    }
    if useIndependent != nil {
        params["use_independent_chat_permissions"] = *useIndependent
    }
    
    return c.executor.Call(ctx, "setChatPermissions", params, nil)
}

// Convenience helpers

// MuteUser mutes a user (removes send permissions)
func (c *Client) MuteUser(ctx context.Context, chatID tg.ChatID, userID int64, duration time.Duration) error {
    opts := []RestrictOption{}
    if duration > 0 {
        opts = append(opts, WithRestrictUntil(time.Now().Add(duration)))
    }
    return c.RestrictChatMember(ctx, chatID, userID, tg.NoPermissions(), opts...)
}

// UnmuteUser restores all permissions for a user
func (c *Client) UnmuteUser(ctx context.Context, chatID tg.ChatID, userID int64) error {
    return c.RestrictChatMember(ctx, chatID, userID, tg.AllPermissions())
}

// KickUser kicks a user (ban then immediately unban)
func (c *Client) KickUser(ctx context.Context, chatID tg.ChatID, userID int64) error {
    if err := c.BanChatMember(ctx, chatID, userID); err != nil {
        return err
    }
    return c.UnbanChatMember(ctx, chatID, userID, true)
}

// PromoteToModerator promotes a user with moderator-level rights
func (c *Client) PromoteToModerator(ctx context.Context, chatID tg.ChatID, userID int64) error {
    t := true
    return c.PromoteChatMember(ctx, PromoteChatMemberRequest{
        ChatID:            chatID,
        UserID:            userID,
        CanManageChat:     &t,
        CanDeleteMessages: &t,
        CanRestrictMembers: &t,
        CanPinMessages:    &t,
    })
}

// DemoteAdmin removes all admin rights from a user
func (c *Client) DemoteAdmin(ctx context.Context, chatID tg.ChatID, userID int64) error {
    f := false
    return c.PromoteChatMember(ctx, PromoteChatMemberRequest{
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
        CanManageTopics:     &f,
    })
}
```

### Definition of Done (PR2)

- [ ] All 6 methods implemented
- [ ] `can_manage_direct_messages` supported in promote
- [ ] Convenience helpers: MuteUser, UnmuteUser, KickUser, PromoteToModerator, DemoteAdmin
- [ ] Rights/permissions serialization tested
- [ ] UntilDate validation (reasonable ranges)
- [ ] `go test -race` clean

---

## PR3: Invite Links & Join Requests

**Goal:** Create, edit, revoke invite links; handle join requests.  
**Estimated Time:** 4-5 hours  
**Breaking Changes:** None (additive)  
**Dependencies:** PR0

### Methods

| Method | Description |
|--------|-------------|
| `exportChatInviteLink` | Generate new primary link |
| `createChatInviteLink` | Create additional link |
| `editChatInviteLink` | Edit existing link |
| `revokeChatInviteLink` | Revoke a link |
| `approveChatJoinRequest` | Approve join request |
| `declineChatJoinRequest` | Decline join request |

### Implementation

Similar to v1 plan but with consistent `expire_date` handling using pointers:

```go
// CreateChatInviteLinkRequest contains parameters for createChatInviteLink
type CreateChatInviteLinkRequest struct {
    ChatID             tg.ChatID `json:"chat_id"`
    Name               string    `json:"name,omitempty"`
    ExpireDate         *int64    `json:"expire_date,omitempty"`
    MemberLimit        *int      `json:"member_limit,omitempty"`
    CreatesJoinRequest *bool     `json:"creates_join_request,omitempty"`
}

// Convenience options
func WithInviteExpiry(t time.Time) InviteLinkOption {
    return func(r *CreateChatInviteLinkRequest) {
        unix := t.Unix()
        r.ExpireDate = &unix
    }
}

func WithInviteExpiryDuration(d time.Duration) InviteLinkOption {
    return func(r *CreateChatInviteLinkRequest) {
        unix := time.Now().Add(d).Unix()
        r.ExpireDate = &unix
    }
}
```

### Definition of Done (PR3)

- [ ] All 6 methods implemented
- [ ] Time fields use pointers consistently
- [ ] Serialization tests for optional fields
- [ ] Response decode test for ChatInviteLink
- [ ] Example in README/examples

---

## PR4: Pins & Chat Appearance

**Goal:** Pin messages, set chat photo/title/description.  
**Estimated Time:** 4-5 hours  
**Breaking Changes:** None (additive)  
**Dependencies:** PR0

### Methods

| Method | Description |
|--------|-------------|
| `pinChatMessage` | Pin a message |
| `unpinChatMessage` | Unpin a message (supports specific `message_id`) |
| `unpinAllChatMessages` | Unpin all messages |
| `setChatPhoto` | Set chat photo (multipart) |
| `deleteChatPhoto` | Delete chat photo |
| `setChatTitle` | Set chat title |
| `setChatDescription` | Set chat description |

### Key Implementation Details

**Important:** `pinChatMessage` and `unpinChatMessage` support `business_connection_id`:

```go
// PinChatMessageRequest contains parameters for pinChatMessage
type PinChatMessageRequest struct {
    ChatID               tg.ChatID `json:"chat_id"`
    MessageID            int       `json:"message_id"`
    BusinessConnectionID string    `json:"business_connection_id,omitempty"` // Bot API 7.2+
    DisableNotification  *bool     `json:"disable_notification,omitempty"`
}

// UnpinChatMessageRequest contains parameters for unpinChatMessage
type UnpinChatMessageRequest struct {
    ChatID               tg.ChatID `json:"chat_id"`
    BusinessConnectionID string    `json:"business_connection_id,omitempty"` // Bot API 7.2+
    MessageID            *int      `json:"message_id,omitempty"` // Specific message to unpin
}

// UnpinChatMessage unpins a message in a chat.
// If messageID is 0, unpins the most recent pinned message.
// If messageID is provided, unpins that specific message.
func (c *Client) UnpinChatMessage(ctx context.Context, chatID tg.ChatID, opts ...UnpinOption) error {
    if chatID.IsZero() {
        return tg.NewValidationError("chat_id", "is required")
    }
    
    req := UnpinChatMessageRequest{ChatID: chatID}
    for _, opt := range opts {
        opt(&req)
    }
    
    return c.executor.Call(ctx, "unpinChatMessage", req, nil)
}

type UnpinOption func(*UnpinChatMessageRequest)

// WithUnpinMessageID unpins a specific message
func WithUnpinMessageID(messageID int) UnpinOption {
    return func(r *UnpinChatMessageRequest) {
        r.MessageID = &messageID
    }
}

// WithUnpinBusinessConnection sets the business connection ID
func WithUnpinBusinessConnection(connID string) UnpinOption {
    return func(r *UnpinChatMessageRequest) {
        r.BusinessConnectionID = connID
    }
}
```

### Definition of Done (PR4)

- [ ] All 7 methods implemented
- [ ] `business_connection_id` supported in pin/unpin
- [ ] `message_id` option in unpinChatMessage
- [ ] setChatPhoto multipart upload tested
- [ ] Title/description length validation

---

## PR5: Commands, Menu Button, Bot Profile

**Goal:** Manage bot commands, menu button, and profile settings.  
**Estimated Time:** 5-6 hours  
**Breaking Changes:** None (additive)  
**Dependencies:** PR0

### Methods

| Method | Description |
|--------|-------------|
| `setMyCommands` | Set bot commands |
| `getMyCommands` | Get bot commands |
| `deleteMyCommands` | Delete bot commands |
| `setChatMenuButton` | Set menu button |
| `getChatMenuButton` | Get menu button |
| `setMyName` | Set bot name |
| `getMyName` | Get bot name |
| `setMyDescription` | Set bot description |
| `getMyDescription` | Get bot description |
| `setMyShortDescription` | Set short description |
| `getMyShortDescription` | Get short description |

### Key Implementation Details

**Language code support** - All bot profile methods support `language_code`:

```go
// SetMyCommandsRequest contains parameters for setMyCommands
type SetMyCommandsRequest struct {
    Commands     []tg.BotCommand    `json:"commands"`
    Scope        tg.BotCommandScope `json:"scope,omitempty"`
    LanguageCode string             `json:"language_code,omitempty"`
}

// SetMyCommands sets the bot's commands.
// Max 100 commands per scope/language combination.
func (c *Client) SetMyCommands(ctx context.Context, commands []tg.BotCommand, opts ...CommandOption) error {
    if len(commands) > 100 {
        return tg.NewValidationError("commands", "must have at most 100 commands")
    }
    
    for i, cmd := range commands {
        if len(cmd.Command) < 1 || len(cmd.Command) > 32 {
            return tg.NewValidationError("commands", "command %d: must be 1-32 characters", i)
        }
        if len(cmd.Description) < 1 || len(cmd.Description) > 256 {
            return tg.NewValidationError("commands", "description %d: must be 1-256 characters", i)
        }
    }
    
    req := SetMyCommandsRequest{Commands: commands}
    for _, opt := range opts {
        opt(&req)
    }
    
    return c.executor.Call(ctx, "setMyCommands", req, nil)
}

// CommandOption configures command-related requests
type CommandOption func(*SetMyCommandsRequest)

// WithCommandScope sets the scope for commands
func WithCommandScope(scope tg.BotCommandScope) CommandOption {
    return func(r *SetMyCommandsRequest) {
        r.Scope = scope
    }
}

// WithCommandLanguage sets the language code for commands
func WithCommandLanguage(lang string) CommandOption {
    return func(r *SetMyCommandsRequest) {
        r.LanguageCode = lang
    }
}

// Example usage:
// client.SetMyCommands(ctx, commands,
//     WithCommandScope(tg.ScopeAllGroupChats()),
//     WithCommandLanguage("ru"),
// )
```

**MenuButton union decoding:**

```go
// GetChatMenuButton returns the current menu button.
func (c *Client) GetChatMenuButton(ctx context.Context, chatID *tg.ChatID) (tg.MenuButton, error) {
    params := map[string]any{}
    if chatID != nil && !chatID.IsZero() {
        params["chat_id"] = chatID
    }
    
    var raw json.RawMessage
    if err := c.executor.Call(ctx, "getChatMenuButton", params, &raw); err != nil {
        return nil, err
    }
    
    return tg.UnmarshalMenuButton(raw)
}
```

### Definition of Done (PR5)

- [ ] All 11 methods implemented
- [ ] Command validation (count, length)
- [ ] BotCommandScope serialization tested
- [ ] MenuButton union decoding tested
- [ ] Language code support across all methods
- [ ] Example: "Set commands for default + Russian, menu button to web_app"

---

## PR6: Bulk Operations (copyMessages, forwardMessages)

**Goal:** Copy/forward multiple messages at once.  
**Estimated Time:** 3-4 hours  
**Breaking Changes:** None (additive)  
**Dependencies:** PR0

### Methods

| Method | Description |
|--------|-------------|
| `copyMessages` | Copy multiple messages |
| `forwardMessages` | Forward multiple messages |

### Key Implementation Details

**CRITICAL:** Include `direct_messages_topic_id` (Bot API 9.x "topics in private chats"):

```go
// CopyMessagesRequest contains parameters for copyMessages
type CopyMessagesRequest struct {
    ChatID                tg.ChatID `json:"chat_id"`
    FromChatID            tg.ChatID `json:"from_chat_id"`
    MessageIDs            []int     `json:"message_ids"` // 1-100 messages
    MessageThreadID       *int      `json:"message_thread_id,omitempty"`
    DirectMessagesTopicID *int      `json:"direct_messages_topic_id,omitempty"` // Bot API 9.3
    DisableNotification   *bool     `json:"disable_notification,omitempty"`
    ProtectContent        *bool     `json:"protect_content,omitempty"`
    RemoveCaption         *bool     `json:"remove_caption,omitempty"`
}

// CopyMessages copies messages of any kind.
// Returns an array of MessageId on success.
func (c *Client) CopyMessages(ctx context.Context, req CopyMessagesRequest) ([]tg.MessageID, error) {
    if req.ChatID.IsZero() {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    if req.FromChatID.IsZero() {
        return nil, tg.NewValidationError("from_chat_id", "is required")
    }
    if len(req.MessageIDs) < 1 || len(req.MessageIDs) > 100 {
        return nil, tg.NewValidationError("message_ids", "must have 1-100 messages")
    }
    
    var ids []tg.MessageID
    if err := c.executor.Call(ctx, "copyMessages", req, &ids); err != nil {
        return nil, err
    }
    return ids, nil
}

// ForwardMessagesRequest contains parameters for forwardMessages
type ForwardMessagesRequest struct {
    ChatID                tg.ChatID `json:"chat_id"`
    FromChatID            tg.ChatID `json:"from_chat_id"`
    MessageIDs            []int     `json:"message_ids"` // 1-100 messages
    MessageThreadID       *int      `json:"message_thread_id,omitempty"`
    DirectMessagesTopicID *int      `json:"direct_messages_topic_id,omitempty"` // Bot API 9.3
    DisableNotification   *bool     `json:"disable_notification,omitempty"`
    ProtectContent        *bool     `json:"protect_content,omitempty"`
}

// ForwardMessages forwards multiple messages of any kind.
func (c *Client) ForwardMessages(ctx context.Context, req ForwardMessagesRequest) ([]tg.MessageID, error) {
    // Similar validation...
}

// BulkOption configures bulk operations
type BulkOption func(req any)

// WithDirectMessagesTopic sets the direct messages topic ID (Bot API 9.3)
func WithDirectMessagesTopic(topicID int) BulkOption {
    return func(req any) {
        switch r := req.(type) {
        case *CopyMessagesRequest:
            r.DirectMessagesTopicID = &topicID
        case *ForwardMessagesRequest:
            r.DirectMessagesTopicID = &topicID
        }
    }
}
```

### Tests

```go
func TestCopyMessages_SerializesMessageIDs(t *testing.T) {
    // Verify message_ids is JSON array, not query string
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        body, _ := io.ReadAll(r.Body)
        
        var req map[string]any
        json.Unmarshal(body, &req)
        
        // Verify message_ids is an array
        ids, ok := req["message_ids"].([]any)
        require.True(t, ok, "message_ids should be array")
        assert.Len(t, ids, 3)
        
        json.NewEncoder(w).Encode(tg.APIResponse[[]tg.MessageID]{
            OK:     true,
            Result: []tg.MessageID{{MessageID: 1}, {MessageID: 2}, {MessageID: 3}},
        })
    }))
    defer server.Close()
    
    client := newTestClient(t, server.URL)
    
    _, err := client.CopyMessages(ctx, CopyMessagesRequest{
        ChatID:     tg.ChatIDFromInt64(123),
        FromChatID: tg.ChatIDFromInt64(456),
        MessageIDs: []int{1, 2, 3},
    })
    require.NoError(t, err)
}
```

### Definition of Done (PR6)

- [ ] Both methods implemented
- [ ] `direct_messages_topic_id` supported
- [ ] `message_ids` serializes as JSON array
- [ ] 1-100 message validation
- [ ] Response decoding tested

---

## PR7: Polls

**Goal:** Send and stop polls.  
**Estimated Time:** 3-4 hours  
**Breaking Changes:** None (additive)  
**Dependencies:** PR0

### Methods

| Method | Description |
|--------|-------------|
| `sendPoll` | Send a poll |
| `stopPoll` | Stop a poll |

### Implementation

```go
// SendPollRequest contains parameters for sendPoll
type SendPollRequest struct {
    ChatID                tg.ChatID           `json:"chat_id"`
    Question              string              `json:"question"`
    QuestionParseMode     tg.ParseMode        `json:"question_parse_mode,omitempty"`
    QuestionEntities      []tg.MessageEntity  `json:"question_entities,omitempty"`
    Options               []tg.InputPollOption `json:"options"` // 2-10 options
    IsAnonymous           *bool               `json:"is_anonymous,omitempty"`
    Type                  string              `json:"type,omitempty"` // "quiz" or "regular"
    AllowsMultipleAnswers *bool               `json:"allows_multiple_answers,omitempty"`
    CorrectOptionID       *int                `json:"correct_option_id,omitempty"` // Quiz mode
    Explanation           string              `json:"explanation,omitempty"`
    ExplanationParseMode  tg.ParseMode        `json:"explanation_parse_mode,omitempty"`
    ExplanationEntities   []tg.MessageEntity  `json:"explanation_entities,omitempty"`
    OpenPeriod            *int                `json:"open_period,omitempty"` // 5-600 seconds
    CloseDate             *int64              `json:"close_date,omitempty"`
    IsClosed              *bool               `json:"is_closed,omitempty"`
    // Common message options
    MessageThreadID       *int                `json:"message_thread_id,omitempty"`
    DisableNotification   *bool               `json:"disable_notification,omitempty"`
    ProtectContent        *bool               `json:"protect_content,omitempty"`
    MessageEffectID       string              `json:"message_effect_id,omitempty"`
    ReplyParameters       *tg.ReplyParameters `json:"reply_parameters,omitempty"`
    ReplyMarkup           any                 `json:"reply_markup,omitempty"`
    BusinessConnectionID  string              `json:"business_connection_id,omitempty"`
}

// SendPoll sends a native poll.
func (c *Client) SendPoll(ctx context.Context, req SendPollRequest) (*tg.Message, error) {
    if req.ChatID.IsZero() {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    if req.Question == "" || len(req.Question) > 300 {
        return nil, tg.NewValidationError("question", "must be 1-300 characters")
    }
    if len(req.Options) < 2 || len(req.Options) > 10 {
        return nil, tg.NewValidationError("options", "must have 2-10 options")
    }
    
    var msg tg.Message
    if err := c.executor.Call(ctx, "sendPoll", req, &msg); err != nil {
        return nil, err
    }
    return &msg, nil
}

// StopPoll stops a poll.
func (c *Client) StopPoll(ctx context.Context, chatID tg.ChatID, messageID int, replyMarkup any) (*tg.Poll, error) {
    if chatID.IsZero() {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    if messageID == 0 {
        return nil, tg.NewValidationError("message_id", "is required")
    }
    
    params := map[string]any{
        "chat_id":    chatID,
        "message_id": messageID,
    }
    if replyMarkup != nil {
        params["reply_markup"] = replyMarkup
    }
    
    var poll tg.Poll
    if err := c.executor.Call(ctx, "stopPoll", params, &poll); err != nil {
        return nil, err
    }
    return &poll, nil
}

// Helper constructors

// NewRegularPoll creates a regular poll request
func NewRegularPoll(chatID tg.ChatID, question string, options ...string) SendPollRequest {
    opts := make([]tg.InputPollOption, len(options))
    for i, o := range options {
        opts[i] = tg.InputPollOption{Text: o}
    }
    return SendPollRequest{
        ChatID:   chatID,
        Question: question,
        Options:  opts,
        Type:     "regular",
    }
}

// NewQuizPoll creates a quiz poll request
func NewQuizPoll(chatID tg.ChatID, question string, correctIndex int, options ...string) SendPollRequest {
    opts := make([]tg.InputPollOption, len(options))
    for i, o := range options {
        opts[i] = tg.InputPollOption{Text: o}
    }
    return SendPollRequest{
        ChatID:          chatID,
        Question:        question,
        Options:         opts,
        Type:            "quiz",
        CorrectOptionID: &correctIndex,
    }
}
```

### Definition of Done (PR7)

- [ ] `sendPoll` implemented with full options
- [ ] `stopPoll` implemented
- [ ] Quiz mode fields supported
- [ ] Options array serialization tested
- [ ] Helper constructors for regular/quiz polls

---

## PR8: Forum Topics

**Goal:** Full forum topic management.  
**Estimated Time:** 5-6 hours  
**Breaking Changes:** None (additive)  
**Dependencies:** PR0

### Methods

| Method | Description |
|--------|-------------|
| `createForumTopic` | Create a topic |
| `editForumTopic` | Edit a topic |
| `closeForumTopic` | Close a topic |
| `reopenForumTopic` | Reopen a topic |
| `deleteForumTopic` | Delete a topic |
| `unpinAllForumTopicMessages` | Unpin all in topic |
| (General topic methods) | 6 more methods |
| `getForumTopicIconStickers` | Get icon stickers |

### Bot API 9.3 Compatibility

**"Topics in private chats"** - Forum topic methods should work seamlessly with private chats that have topics enabled. The `message_thread_id` parameter is now valid in private chats per Bot API 9.3.

```go
// CreateForumTopicRequest contains parameters for createForumTopic
type CreateForumTopicRequest struct {
    ChatID            tg.ChatID `json:"chat_id"`
    Name              string    `json:"name"` // 1-128 characters
    IconColor         *int      `json:"icon_color,omitempty"`
    IconCustomEmojiID string    `json:"icon_custom_emoji_id,omitempty"`
}

// CreateForumTopic creates a topic in a forum supergroup chat.
// Works with both supergroups and private chats with topics enabled (Bot API 9.3).
func (c *Client) CreateForumTopic(ctx context.Context, chatID tg.ChatID, name string, opts ...ForumTopicOption) (*tg.ForumTopic, error) {
    if chatID.IsZero() {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    if name == "" || len(name) > 128 {
        return nil, tg.NewValidationError("name", "must be 1-128 characters")
    }
    
    req := CreateForumTopicRequest{
        ChatID: chatID,
        Name:   name,
    }
    for _, opt := range opts {
        opt(&req)
    }
    
    var topic tg.ForumTopic
    if err := c.executor.Call(ctx, "createForumTopic", req, &topic); err != nil {
        return nil, err
    }
    return &topic, nil
}

// Topic icon color constants
const (
    TopicColorBlue   = 0x6FB9F0 // 7322096
    TopicColorYellow = 0xFFD67E // 16766590
    TopicColorViolet = 0xCB86DB // 13338331
    TopicColorGreen  = 0x8EEE98 // 9367192
    TopicColorRose   = 0xFF93B2 // 16749490
    TopicColorRed    = 0xFB6F5F // 16478047
)

type ForumTopicOption func(*CreateForumTopicRequest)

func WithTopicColor(color int) ForumTopicOption {
    return func(r *CreateForumTopicRequest) {
        r.IconColor = &color
    }
}

func WithTopicEmoji(emojiID string) ForumTopicOption {
    return func(r *CreateForumTopicRequest) {
        r.IconCustomEmojiID = emojiID
    }
}
```

### Definition of Done (PR8)

- [ ] All 13 forum topic methods implemented
- [ ] Color constants defined
- [ ] Works with private chats (Bot API 9.3 compatible)
- [ ] ForumTopic response decoding tested
- [ ] Icon stickers method returns Sticker array

---

## PR9: Facade + Documentation + Release

**Goal:** Complete Bot facade, examples, documentation, release v2.1.0.  
**Estimated Time:** 4-6 hours  
**Breaking Changes:** Documented

### 9.1 Bot Facade

Add all Tier 2 methods to `bot.go`:

```go
// --- Chat Info ---
func (b *Bot) GetChat(ctx, chatID) (*tg.ChatFullInfo, error)
func (b *Bot) GetChatAdministrators(ctx, chatID) ([]tg.ChatMember, error)
func (b *Bot) GetChatMember(ctx, chatID, userID) (tg.ChatMember, error)
func (b *Bot) GetChatMemberCount(ctx, chatID) (int, error)
func (b *Bot) LeaveChat(ctx, chatID) error

// --- Moderation ---
func (b *Bot) BanChatMember(ctx, chatID, userID, opts...) error
func (b *Bot) UnbanChatMember(ctx, chatID, userID, onlyIfBanned) error
func (b *Bot) RestrictChatMember(ctx, chatID, userID, permissions, opts...) error
func (b *Bot) PromoteChatMember(ctx, req) error
func (b *Bot) SetChatAdministratorCustomTitle(ctx, chatID, userID, title) error
func (b *Bot) SetChatPermissions(ctx, chatID, permissions, useIndependent) error
func (b *Bot) MuteUser(ctx, chatID, userID, duration) error
func (b *Bot) UnmuteUser(ctx, chatID, userID) error
func (b *Bot) KickUser(ctx, chatID, userID) error

// --- Invite Links ---
func (b *Bot) ExportChatInviteLink(ctx, chatID) (string, error)
func (b *Bot) CreateChatInviteLink(ctx, req) (*tg.ChatInviteLink, error)
func (b *Bot) EditChatInviteLink(ctx, req) (*tg.ChatInviteLink, error)
func (b *Bot) RevokeChatInviteLink(ctx, chatID, inviteLink) (*tg.ChatInviteLink, error)
func (b *Bot) ApproveChatJoinRequest(ctx, chatID, userID) error
func (b *Bot) DeclineChatJoinRequest(ctx, chatID, userID) error

// --- Pins & Appearance ---
func (b *Bot) PinChatMessage(ctx, chatID, messageID, opts...) error
func (b *Bot) UnpinChatMessage(ctx, chatID, opts...) error
func (b *Bot) UnpinAllChatMessages(ctx, chatID) error
func (b *Bot) SetChatPhoto(ctx, chatID, photo) error
func (b *Bot) DeleteChatPhoto(ctx, chatID) error
func (b *Bot) SetChatTitle(ctx, chatID, title) error
func (b *Bot) SetChatDescription(ctx, chatID, description) error

// --- Commands & Profile ---
func (b *Bot) SetMyCommands(ctx, commands, opts...) error
func (b *Bot) GetMyCommands(ctx, scope, languageCode) ([]tg.BotCommand, error)
func (b *Bot) DeleteMyCommands(ctx, scope, languageCode) error
func (b *Bot) SetChatMenuButton(ctx, chatID, button) error
func (b *Bot) GetChatMenuButton(ctx, chatID) (tg.MenuButton, error)
func (b *Bot) SetMyName(ctx, name, languageCode) error
func (b *Bot) GetMyName(ctx, languageCode) (*tg.BotName, error)
func (b *Bot) SetMyDescription(ctx, description, languageCode) error
func (b *Bot) GetMyDescription(ctx, languageCode) (*tg.BotDescription, error)
func (b *Bot) SetMyShortDescription(ctx, shortDescription, languageCode) error
func (b *Bot) GetMyShortDescription(ctx, languageCode) (*tg.BotShortDescription, error)

// --- Bulk Ops ---
func (b *Bot) CopyMessages(ctx, req) ([]tg.MessageID, error)
func (b *Bot) ForwardMessages(ctx, req) ([]tg.MessageID, error)

// --- Polls ---
func (b *Bot) SendPoll(ctx, req) (*tg.Message, error)
func (b *Bot) StopPoll(ctx, chatID, messageID, replyMarkup) (*tg.Poll, error)

// --- Forum Topics ---
func (b *Bot) CreateForumTopic(ctx, chatID, name, opts...) (*tg.ForumTopic, error)
func (b *Bot) EditForumTopic(ctx, chatID, threadID, name, iconEmojiID) error
func (b *Bot) CloseForumTopic(ctx, chatID, threadID) error
func (b *Bot) ReopenForumTopic(ctx, chatID, threadID) error
func (b *Bot) DeleteForumTopic(ctx, chatID, threadID) error
func (b *Bot) UnpinAllForumTopicMessages(ctx, chatID, threadID) error
// ... General topic methods ...
func (b *Bot) GetForumTopicIconStickers(ctx) ([]tg.Sticker, error)
```

### 9.2 Examples

Create example files:

```
examples/
├── admin-moderation/
│   └── main.go          # Ban, mute, promote examples
├── commands-menu/
│   └── main.go          # Set commands for different scopes/languages
├── invites-join-requests/
│   └── main.go          # Create invite links, handle join requests
├── polls/
│   └── main.go          # Create regular and quiz polls
└── forum-topics/
    └── main.go          # Create and manage forum topics
```

### 9.3 Documentation Updates

**Must mention explicitly:**
- `can_manage_direct_messages` in ChatAdministratorRights (Bot API 9.2)
- `direct_messages_topic_id` in bulk operations (Bot API 9.3)
- `business_connection_id` in pin methods
- Topics in private chats compatibility (Bot API 9.3)

### 9.4 CHANGELOG

```markdown
## [2.1.0] - 2026-XX-XX

### Added

#### Types (PR0)
- ChatMember union types (Owner/Administrator/Member/Restricted/Left/Banned)
- ChatAdministratorRights with `can_manage_direct_messages` (Bot API 9.2)
- ChatPermissions with preset constructors
- BotCommandScope union types
- MenuButton union types
- ChatInviteLink, ForumTopic, Poll types
- ChatFullInfo

#### Chat Information (PR1)
- `GetChat()`, `GetChatAdministrators()`, `GetChatMember()`, `GetChatMemberCount()`, `LeaveChat()`

#### Moderation (PR2)
- `BanChatMember()`, `UnbanChatMember()`, `RestrictChatMember()`, `PromoteChatMember()`
- `SetChatAdministratorCustomTitle()`, `SetChatPermissions()`
- Convenience helpers: `MuteUser()`, `UnmuteUser()`, `KickUser()`, `PromoteToModerator()`, `DemoteAdmin()`

#### Invite Links (PR3)
- `ExportChatInviteLink()`, `CreateChatInviteLink()`, `EditChatInviteLink()`, `RevokeChatInviteLink()`
- `ApproveChatJoinRequest()`, `DeclineChatJoinRequest()`

#### Pins & Appearance (PR4)
- `PinChatMessage()`, `UnpinChatMessage()`, `UnpinAllChatMessages()`
- `SetChatPhoto()`, `DeleteChatPhoto()`, `SetChatTitle()`, `SetChatDescription()`
- `business_connection_id` support in pin methods

#### Commands & Profile (PR5)
- `SetMyCommands()`, `GetMyCommands()`, `DeleteMyCommands()`
- `SetChatMenuButton()`, `GetChatMenuButton()`
- `SetMyName()`, `GetMyName()`, `SetMyDescription()`, `GetMyDescription()`
- `SetMyShortDescription()`, `GetMyShortDescription()`
- Full BotCommandScope support with language codes

#### Bulk Operations (PR6)
- `CopyMessages()`, `ForwardMessages()`
- `direct_messages_topic_id` support (Bot API 9.3)

#### Polls (PR7)
- `SendPoll()`, `StopPoll()`
- Quiz mode support

#### Forum Topics (PR8)
- Full topic management (13 methods)
- Compatible with "topics in private chats" (Bot API 9.3)
```

### Definition of Done (PR9)

- [ ] All facade methods added
- [ ] 5 example directories created
- [ ] CHANGELOG complete
- [ ] README updated
- [ ] Documentation mentions Bot API 9.x features
- [ ] Tag v2.1.0

---

## Summary

| PR | Title | Methods | Hours |
|----|-------|---------|-------|
| PR0 | Tier 2 Types | - | 6-8 |
| PR1 | Chat Info & Membership | 5 | 3-4 |
| PR2 | Moderation | 6 | 5-6 |
| PR3 | Invite Links | 6 | 4-5 |
| PR4 | Pins & Appearance | 7 | 4-5 |
| PR5 | Commands & Profile | 11 | 5-6 |
| PR6 | Bulk Operations | 2 | 3-4 |
| PR7 | Polls | 2 | 3-4 |
| PR8 | Forum Topics | 13+ | 5-6 |
| PR9 | Facade + Docs | - | 4-6 |
| **TOTAL** | | **~52** | **43-54** |

---

## Key Insights from Combined Analysis

1. **ChatMember as union types** - More type-safe than flat struct
2. **`can_manage_direct_messages`** - Bot API 9.2 addition, essential for business features
3. **`direct_messages_topic_id`** - Bot API 9.3, critical for private chat topics
4. **Pointer fields for optionals** - Distinguishes "not set" from "set to false"
5. **`business_connection_id`** in pin methods - Easy to miss but important
6. **`message_id` in unpinChatMessage** - Allows unpinning specific messages
7. **Topics in private chats** - Bot API 9.3 feature, future-proof your types

---

## References

- [Telegram Bot API](https://core.telegram.org/bots/api)
- [Bot API Changelog](https://core.telegram.org/bots/api-changelog)
- Bot API 9.2: `can_manage_direct_messages`
- Bot API 9.3: Topics in private chats, `direct_messages_topic_id`

---

*Consolidated Tier 2 Plan v2.0 - January 2026*
*Combines insights from multiple independent analyses*