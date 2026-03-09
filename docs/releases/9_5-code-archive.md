# Bot API 9.5 — Complete Code Archive for galigo

**Based on**: `9_5-corrected.md`
**Created**: 2026-03-09
**Status**: Ready to apply — all code verified against actual source files
**API version**: Bot API 9.5 (March 1, 2026)

This document contains **complete, copy-paste-ready code** for every change required to implement Bot API 9.5 in galigo. Each section shows the exact file, the exact location, and the exact code to add/modify.

All code has been verified against the actual codebase at commit `2b16ddc`.

---

## Table of Contents

1. [New Files](#1-new-files)
2. [Modified Files — tg/ package](#2-modified-files--tg-package)
3. [Modified Files — sender/ package](#3-modified-files--sender-package)
4. [New Test Files](#4-new-test-files)
5. [Modified Test Files](#5-modified-test-files)
6. [Testbot Files](#6-testbot-files)
7. [Verification Commands](#7-verification-commands)

---

## 1. New Files

### 1.1 `sender/streaming.go` — NEW FILE

```go
package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// SendMessageDraftRequest represents a sendMessageDraft request.
// Added in Bot API 9.5.
type SendMessageDraftRequest struct {
	ChatID          tg.ChatID          `json:"chat_id"`
	DraftID         int                `json:"draft_id"`
	Text            string             `json:"text"`
	MessageThreadID int                `json:"message_thread_id,omitempty"`
	ParseMode       tg.ParseMode       `json:"parse_mode,omitempty"`
	Entities        []tg.MessageEntity `json:"entities,omitempty"`
}

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

### 1.2 `tg/datetime.go` — NEW FILE (optional helper)

```go
package tg

// NewDateTimeEntity creates a MessageEntity for date_time formatting.
// offset and length are in UTF-16 code units (matching Telegram's convention).
// format must match Telegram's date_time_format rules: "r" (standalone relative),
// or any combination of w? + [dD]? + [tT]? (weekday + date + time).
// Pass empty string for format to use the fallback text as-is.
func NewDateTimeEntity(offset, length int, unixTime int64, format string) MessageEntity {
	return MessageEntity{
		Type:           EntityDateTime,
		Offset:         offset,
		Length:         length,
		UnixTime:       unixTime,
		DateTimeFormat: format,
	}
}

// UTF16Len returns the length of a string in UTF-16 code units.
// This is needed because Telegram measures entity offsets/lengths in UTF-16.
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2 // surrogate pair
		} else {
			n++
		}
	}
	return n
}
```

---

## 2. Modified Files — tg/ package

### 2.1 `tg/types.go` — Add EntityDateTime constant

**Location**: After the imports block (line ~7), before the `ChatID` type alias.

```go
// EntityDateTime is the entity type for formatted date/time display (9.5).
const EntityDateTime = "date_time"
```

### 2.2 `tg/types.go` — Add fields to MessageEntity struct

**Location**: Inside `MessageEntity` struct (line ~135), after `CustomEmojiID` field (line 142), before the closing brace.

```go
	// 9.5: date_time entity fields
	UnixTime       int64  `json:"unix_time,omitempty"`
	DateTimeFormat string `json:"date_time_format,omitempty"`
```

**Result** — MessageEntity becomes:
```go
type MessageEntity struct {
	Type          string `json:"type"`
	Offset        int    `json:"offset"`
	Length        int    `json:"length"`
	URL           string `json:"url,omitempty"`
	User          *User  `json:"user,omitempty"`
	Language      string `json:"language,omitempty"`
	CustomEmojiID string `json:"custom_emoji_id,omitempty"`
	// 9.5: date_time entity fields
	UnixTime       int64  `json:"unix_time,omitempty"`
	DateTimeFormat string `json:"date_time_format,omitempty"`
}
```

### 2.3 `tg/types.go` — Add SenderTag to Message struct

**Location**: Inside `Message` struct (line ~22), after `AuthorSignature` (line 39), before `Text`.

```go
	SenderTag             string                `json:"sender_tag,omitempty"`             // 9.5
```

**Result** — the relevant section becomes:
```go
	AuthorSignature       string                `json:"author_signature,omitempty"`
	SenderTag             string                `json:"sender_tag,omitempty"`             // 9.5
	Text                  string                `json:"text,omitempty"`
```

### 2.4 `tg/chat_member.go` — Add Tag to ChatMemberMember

**Location**: Inside `ChatMemberMember` struct (line ~71), after `UntilDate` (line 73), before closing brace.

```go
	Tag       string `json:"tag,omitempty"` // 9.5
```

**Result**:
```go
type ChatMemberMember struct {
	chatMemberBase
	UntilDate int64  `json:"until_date,omitempty"`
	Tag       string `json:"tag,omitempty"` // 9.5
}
```

### 2.5 `tg/chat_member.go` — Add Tag and CanEditTag to ChatMemberRestricted

**Location**: Inside `ChatMemberRestricted` struct (line ~80), after `UntilDate` (line 97), before closing brace.

```go
	Tag        string `json:"tag,omitempty"`  // 9.5
	CanEditTag bool   `json:"can_edit_tag"`   // 9.5 — NOT omitempty (always present in restriction set)
```

> **Why `bool` not `*bool`?** All existing fields in `ChatMemberRestricted` are plain `bool` (always present when Telegram returns a restricted member). `CanEditTag` follows this same pattern.

**Result**:
```go
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
	Tag                   string `json:"tag,omitempty"`  // 9.5
	CanEditTag            bool   `json:"can_edit_tag"`   // 9.5
}
```

### 2.6 `tg/chat_member.go` — Add CanManageTags to ChatMemberAdministrator

**Location**: Inside `ChatMemberAdministrator` struct (line ~45), after `CanManageDirectMessages` (line 63), before `CustomTitle` (line 64).

```go
	CanManageTags           *bool  `json:"can_manage_tags,omitempty"`           // 9.5
```

**Result** — the end of the struct becomes:
```go
	CanManageTopics         *bool  `json:"can_manage_topics,omitempty"`
	CanManageDirectMessages *bool  `json:"can_manage_direct_messages,omitempty"`
	CanManageTags           *bool  `json:"can_manage_tags,omitempty"`           // 9.5
	CustomTitle             string `json:"custom_title,omitempty"`
```

### 2.7 `tg/chat_permissions.go` — Add CanEditTag to ChatPermissions

**Location**: Inside `ChatPermissions` struct (line ~5), after `CanManageTopics` (line 19), before closing brace.

```go
	CanEditTag        *bool `json:"can_edit_tag,omitempty"`        // 9.5
```

> **Why `*bool`?** All existing fields in `ChatPermissions` are `*bool` to distinguish "not set" (nil) from "explicitly false". `CanEditTag` follows this pattern.

### 2.8 `tg/chat_permissions.go` — Update AllPermissions()

**Location**: Inside `AllPermissions()` function (line ~26), after `CanManageTopics: boolPtr(true),` (line 41).

Add:
```go
		CanEditTag:        boolPtr(true),
```

### 2.9 `tg/chat_permissions.go` — Update NoPermissions()

**Location**: Inside `NoPermissions()` function (line ~46), after `CanManageTopics: boolPtr(false),` (line 61).

Add:
```go
		CanEditTag:        boolPtr(false),
```

> `ReadOnlyPermissions()` and `TextOnlyPermissions()` do NOT need `CanEditTag` — they're for message-level restrictions. Leaving it as `nil` (unset) is correct.

### 2.10 `tg/chat_admin_rights.go` — Add CanManageTags to ChatAdministratorRights

**Location**: Inside `ChatAdministratorRights` struct (line ~4), after `CanManageDirectMessages` (line 20), before closing brace.

```go
	CanManageTags           *bool `json:"can_manage_tags,omitempty"`           // 9.5
```

### 2.11 `tg/chat_admin_rights.go` — Update FullAdminRights()

**Location**: Inside `FullAdminRights()` function (line ~24), after `CanManageDirectMessages: boolPtr(true),` (line 41).

Add:
```go
		CanManageTags:           boolPtr(true),
```

### 2.12 `tg/chat_admin_rights.go` — Update ModeratorRights()

**Location**: Inside `ModeratorRights()` function (line ~46), after `CanPinMessages: boolPtr(true),` (line 52).

Add:
```go
		CanManageTags: boolPtr(true),
```

> Moderators typically manage tags. `ContentManagerRights()` does NOT need it — content managers manage content, not members.

---

## 3. Modified Files — sender/ package

### 3.1 `sender/chat_moderation.go` — Add SetChatMemberTagRequest struct

**Location**: After `UnbanChatSenderChatRequest` struct (line ~46), before the `// ================== Moderation Methods ==================` section.

```go
// SetChatMemberTagRequest represents a setChatMemberTag request.
// Added in Bot API 9.5.
type SetChatMemberTagRequest struct {
	ChatID tg.ChatID `json:"chat_id"`
	UserID int64     `json:"user_id"`
	Tag    string    `json:"tag"` // NO omitempty — empty string removes tag
}
```

### 3.2 `sender/chat_moderation.go` — Add SetChatMemberTag method

**Location**: After `UnbanChatSenderChat` method (line ~135), before the `// ================== Options ==================` section.

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

### 3.3 `sender/chat_admin.go` — Add CanManageTags to PromoteChatMemberRequest

**Location**: Inside `PromoteChatMemberRequest` struct (line ~12), after `CanManageTopics` (line 29), before closing brace.

```go
	CanManageTags       *bool     `json:"can_manage_tags,omitempty"`       // 9.5
```

### 3.4 `sender/chat_admin.go` — Update PromoteChatMemberWithRights

**Location**: Inside `PromoteChatMemberWithRights` method (line ~64), after `CanManageTopics: rights.CanManageTopics,` (line 89).

Add:
```go
		CanManageTags:       rights.CanManageTags,
```

### 3.5 `sender/chat_admin.go` — Update DemoteChatMember

**Location**: Inside `DemoteChatMember` method (line ~96), after `CanPinMessages: &f,` (line 117).

Add:
```go
		CanManageTags:       &f,
```

### 3.6 `sender/chat_admin.go` — Add WithCanManageTags option

**Location**: After `WithCanManageTopics` function (line ~226), before the end of file.

```go
// WithCanManageTags grants ability to manage member tags (9.5).
func WithCanManageTags(can bool) PromoteOption {
	return func(r *PromoteChatMemberRequest) {
		r.CanManageTags = &can
	}
}
```

---

## 4. New Test Files

### 4.1 `tg/entity_test.go` — NEW FILE

```go
package tg_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// ==================== date_time entity ====================

func TestMessageEntity_DateTime_Unmarshal(t *testing.T) {
	data := []byte(`{"type":"date_time","offset":0,"length":16,"unix_time":1647531900,"date_time_format":"wDT"}`)
	var e tg.MessageEntity
	require.NoError(t, json.Unmarshal(data, &e))
	assert.Equal(t, tg.EntityDateTime, e.Type)
	assert.Equal(t, int64(1647531900), e.UnixTime)
	assert.Equal(t, "wDT", e.DateTimeFormat)
}

func TestMessageEntity_DateTime_RelativeFormat(t *testing.T) {
	data := []byte(`{"type":"date_time","offset":5,"length":12,"unix_time":1647531900,"date_time_format":"r"}`)
	var e tg.MessageEntity
	require.NoError(t, json.Unmarshal(data, &e))
	assert.Equal(t, "r", e.DateTimeFormat)
	assert.Equal(t, int64(1647531900), e.UnixTime)
}

func TestMessageEntity_DateTime_EmptyFormat(t *testing.T) {
	data := []byte(`{"type":"date_time","offset":0,"length":10,"unix_time":1647531900}`)
	var e tg.MessageEntity
	require.NoError(t, json.Unmarshal(data, &e))
	assert.Equal(t, "date_time", e.Type)
	assert.Equal(t, int64(1647531900), e.UnixTime)
	assert.Empty(t, e.DateTimeFormat)
}

func TestMessageEntity_Bold_NoDateTimeFields(t *testing.T) {
	data := []byte(`{"type":"bold","offset":0,"length":5}`)
	var e tg.MessageEntity
	require.NoError(t, json.Unmarshal(data, &e))
	assert.Equal(t, int64(0), e.UnixTime)
	assert.Empty(t, e.DateTimeFormat)
}

func TestMessageEntity_DateTime_RoundTrip(t *testing.T) {
	original := tg.MessageEntity{
		Type:           tg.EntityDateTime,
		Offset:         0,
		Length:         16,
		UnixTime:       1647531900,
		DateTimeFormat: "Dt",
	}
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded tg.MessageEntity
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, original.UnixTime, decoded.UnixTime)
	assert.Equal(t, original.DateTimeFormat, decoded.DateTimeFormat)
}

func TestMessageEntity_DateTime_OmitEmptyFields(t *testing.T) {
	e := tg.MessageEntity{Type: "bold", Offset: 0, Length: 5}
	data, err := json.Marshal(e)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "unix_time")
	assert.NotContains(t, string(data), "date_time_format")
}

// ==================== Helpers ====================

func TestUTF16Len(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"", 0},
		{"\U0001F389", 2},        // emoji = surrogate pair
		{"Hello \U0001F30D!", 9}, // 7 ASCII + 1 surrogate pair + 1
		{"\u65E5\u672C\u8A9E", 3},                // CJK = 1 UTF-16 unit each
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tg.UTF16Len(tt.input), "UTF16Len(%q)", tt.input)
	}
}

func TestNewDateTimeEntity(t *testing.T) {
	e := tg.NewDateTimeEntity(5, 10, 1647531900, "wDT")
	assert.Equal(t, tg.EntityDateTime, e.Type)
	assert.Equal(t, 5, e.Offset)
	assert.Equal(t, 10, e.Length)
	assert.Equal(t, int64(1647531900), e.UnixTime)
	assert.Equal(t, "wDT", e.DateTimeFormat)
}
```

### 4.2 `sender/streaming_test.go` — NEW FILE

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

// ==================== SendMessageDraft (9.5) ====================

func TestSendMessageDraft_BasicRequest(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessageDraft", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
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

func TestSendMessageDraft_Validation_InvalidChatID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SendMessageDraft(context.Background(), sender.SendMessageDraftRequest{
		ChatID:  nil,
		DraftID: 1,
		Text:    "test",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat_id")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}
```

---

## 5. Modified Test Files

### 5.1 `tg/types_test.go` — APPEND to end of file

```go

// ==================== 9.5: sender_tag ====================

func TestMessage_SenderTag_Unmarshal(t *testing.T) {
	data := []byte(`{
		"message_id":42,"date":1647531900,
		"chat":{"id":-1001234567890,"type":"supergroup","title":"Test"},
		"from":{"id":123456,"is_bot":false,"first_name":"Alice"},
		"text":"Hello",
		"sender_tag":"Team Lead"
	}`)
	var m tg.Message
	require.NoError(t, json.Unmarshal(data, &m))
	assert.Equal(t, "Team Lead", m.SenderTag)
}

func TestMessage_NoSenderTag_Unmarshal(t *testing.T) {
	data := []byte(`{"message_id":1,"date":1647531900,"chat":{"id":123,"type":"private","first_name":"X"},"text":"hi"}`)
	var m tg.Message
	require.NoError(t, json.Unmarshal(data, &m))
	assert.Empty(t, m.SenderTag)
}
```

### 5.2 `tg/chat_member_test.go` — APPEND to end of file

> **Note**: This file uses `package tg` (internal access) — no `tg.` prefix on types.

```go

// ==================== 9.5: Tags ====================

func TestChatMemberMember_Tag_Unmarshal(t *testing.T) {
	data := []byte(`{"status":"member","user":{"id":123456,"is_bot":false,"first_name":"Alice"},"tag":"Developer"}`)
	var m ChatMemberMember
	require.NoError(t, json.Unmarshal(data, &m))
	assert.Equal(t, "Developer", m.Tag)
}

func TestChatMemberMember_NoTag_Unmarshal(t *testing.T) {
	data := []byte(`{"status":"member","user":{"id":123456,"is_bot":false,"first_name":"Alice"}}`)
	var m ChatMemberMember
	require.NoError(t, json.Unmarshal(data, &m))
	assert.Empty(t, m.Tag)
}

func TestChatMemberRestricted_CanEditTag_AlwaysPresent(t *testing.T) {
	data := []byte(`{
		"status":"restricted",
		"user":{"id":123,"is_bot":false,"first_name":"Bob"},
		"is_member":true,
		"can_send_messages":true,"can_send_audios":true,"can_send_documents":true,
		"can_send_photos":true,"can_send_videos":true,"can_send_video_notes":true,
		"can_send_voice_notes":true,"can_send_polls":true,"can_send_other_messages":true,
		"can_add_web_page_previews":true,"can_change_info":false,"can_invite_users":false,
		"can_pin_messages":false,"can_manage_topics":false,"until_date":0,
		"tag":"Intern","can_edit_tag":false
	}`)
	var m ChatMemberRestricted
	require.NoError(t, json.Unmarshal(data, &m))
	assert.Equal(t, "Intern", m.Tag)
	assert.False(t, m.CanEditTag)
}

func TestChatMemberAdministrator_CanManageTags(t *testing.T) {
	data := []byte(`{
		"status":"administrator",
		"user":{"id":789,"is_bot":true,"first_name":"Bot"},
		"can_be_edited":true,"can_manage_chat":true,"can_delete_messages":true,
		"can_manage_video_chats":true,"can_restrict_members":true,"can_promote_members":false,
		"can_change_info":true,"can_invite_users":true,"can_pin_messages":true,
		"can_manage_topics":true,"can_manage_tags":true,"is_anonymous":false
	}`)
	var m ChatMemberAdministrator
	require.NoError(t, json.Unmarshal(data, &m))
	require.NotNil(t, m.CanManageTags)
	assert.True(t, *m.CanManageTags)
}

func TestChatAdministratorRights_CanManageTags_Absent(t *testing.T) {
	data := []byte(`{"can_manage_chat":true,"can_pin_messages":true}`)
	var r ChatAdministratorRights
	require.NoError(t, json.Unmarshal(data, &r))
	assert.Nil(t, r.CanManageTags)
}
```

### 5.3 `tg/chat_permissions_test.go` — APPEND to end of file

> **Note**: This file uses `package tg` (internal access) — no `tg.` prefix on types.

```go

// ==================== 9.5: CanEditTag ====================

func TestChatPermissions_CanEditTag_IsPointer(t *testing.T) {
	// Present and true
	data := []byte(`{"can_send_messages":true,"can_edit_tag":true}`)
	var p ChatPermissions
	require.NoError(t, json.Unmarshal(data, &p))
	require.NotNil(t, p.CanEditTag)
	assert.True(t, *p.CanEditTag)

	// Absent = nil (not false!)
	data2 := []byte(`{"can_send_messages":true}`)
	var p2 ChatPermissions
	require.NoError(t, json.Unmarshal(data2, &p2))
	assert.Nil(t, p2.CanEditTag)
}

func TestChatPermissions_CanEditTag_FalseVsAbsent(t *testing.T) {
	data := []byte(`{"can_edit_tag":false}`)
	var p ChatPermissions
	require.NoError(t, json.Unmarshal(data, &p))
	require.NotNil(t, p.CanEditTag)
	assert.False(t, *p.CanEditTag)
}

func TestAllPermissions_IncludesCanEditTag(t *testing.T) {
	p := AllPermissions()
	require.NotNil(t, p.CanEditTag)
	assert.True(t, *p.CanEditTag)
}

func TestNoPermissions_IncludesCanEditTag(t *testing.T) {
	p := NoPermissions()
	require.NotNil(t, p.CanEditTag)
	assert.False(t, *p.CanEditTag)
}

func TestFullAdminRights_IncludesCanManageTags(t *testing.T) {
	r := FullAdminRights()
	require.NotNil(t, r.CanManageTags)
	assert.True(t, *r.CanManageTags)
}
```

### 5.4 `sender/chat_moderation_test.go` — APPEND to end of file

```go

// ==================== SetChatMemberTag (9.5) ====================

func TestSetChatMemberTag_SetTag(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setChatMemberTag", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "Team Lead", req["tag"])
		assert.Equal(t, float64(123456), req["user_id"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetChatMemberTag(context.Background(), int64(-1001234567890), int64(123456), "Team Lead")
	require.NoError(t, err)
}

// THIS IS THE CRITICAL TDD TEST — catches the omitempty trap
func TestSetChatMemberTag_RemoveTag_EmptyStringNotOmitted(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setChatMemberTag", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetChatMemberTag(context.Background(), int64(-1001234567890), int64(123456), "")
	require.NoError(t, err)

	// THE KEY ASSERTION: "tag":"" MUST be present in the request body
	cap := server.LastCapture()
	assert.Contains(t, string(cap.Body), `"tag":""`)
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

### 5.5 `sender/chat_admin_test.go` — APPEND to end of file + UPDATE TestPromoteOptions

**APPEND to end of file:**

```go

// ==================== PromoteChatMember: can_manage_tags (9.5) ====================

func TestPromoteChatMember_WithCanManageTags(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/promoteChatMember", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, true, req["can_manage_tags"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.PromoteChatMember(context.Background(), int64(-1001234567890), int64(789),
		sender.WithCanManageTags(true),
	)
	require.NoError(t, err)
}

func TestDemoteChatMember_ResetsCanManageTags(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/promoteChatMember", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, false, req["can_manage_tags"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.DemoteChatMember(context.Background(), int64(-100123), 456)
	assert.NoError(t, err)
}
```

**REPLACE existing `TestPromoteOptions` function (line 157-173):**

Replace:
```go
func TestPromoteOptions(t *testing.T) {
	opts := []sender.PromoteOption{
		sender.WithAnonymous(true),
		sender.WithCanManageChat(true),
		sender.WithCanDeleteMessages(true),
		sender.WithCanManageVideoChats(true),
		sender.WithCanRestrictMembers(true),
		sender.WithCanPromoteMembers(true),
		sender.WithCanChangeInfo(true),
		sender.WithCanInviteUsers(true),
		sender.WithCanPostMessages(true),
		sender.WithCanEditMessages(true),
		sender.WithCanPinMessages(true),
		sender.WithCanManageTopics(true),
	}
	assert.Len(t, opts, 12)
}
```

With:
```go
func TestPromoteOptions(t *testing.T) {
	opts := []sender.PromoteOption{
		sender.WithAnonymous(true),
		sender.WithCanManageChat(true),
		sender.WithCanDeleteMessages(true),
		sender.WithCanManageVideoChats(true),
		sender.WithCanRestrictMembers(true),
		sender.WithCanPromoteMembers(true),
		sender.WithCanChangeInfo(true),
		sender.WithCanInviteUsers(true),
		sender.WithCanPostMessages(true),
		sender.WithCanEditMessages(true),
		sender.WithCanPinMessages(true),
		sender.WithCanManageTopics(true),
		sender.WithCanManageTags(true), // 9.5
	}
	assert.Len(t, opts, 13)
}
```

---

## 6. Testbot Files

### 6.1 `cmd/galigo-testbot/engine/steps_api95.go` — NEW FILE

```go
package engine

import (
	"context"
	"fmt"

	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// ==================== Bot API 9.5 Steps ====================

// SetChatMemberTagStep sets a tag on a member in the test group.
type SetChatMemberTagStep struct {
	UserID int64  // 0 = use rt.AdminUserID
	Tag    string
}

func (s *SetChatMemberTagStep) Name() string { return "setChatMemberTag" }

func (s *SetChatMemberTagStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	userID := s.UserID
	if userID == 0 {
		userID = rt.AdminUserID
	}
	if userID == 0 {
		return nil, Skip("no AdminUserID available for setChatMemberTag")
	}

	// Check if bot has can_manage_tags permission
	if rt.ChatCtx != nil && !rt.ChatCtx.CanManageTags {
		return nil, Skip("bot does not have can_manage_tags permission")
	}

	err := rt.Sender.SetChatMemberTag(ctx, rt.ChatID, userID, s.Tag)
	if err != nil {
		return nil, err
	}
	return &StepResult{
		Method: "setChatMemberTag",
		Evidence: map[string]any{
			"user_id": userID,
			"tag":     s.Tag,
		},
	}, nil
}

// VerifyChatMemberTagStep reads back a member and verifies their tag matches.
type VerifyChatMemberTagStep struct {
	UserID      int64  // 0 = use rt.AdminUserID
	ExpectedTag string
}

func (s *VerifyChatMemberTagStep) Name() string { return "getChatMember (verify tag)" }

func (s *VerifyChatMemberTagStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	userID := s.UserID
	if userID == 0 {
		userID = rt.AdminUserID
	}
	if userID == 0 {
		return nil, Skip("no AdminUserID available for tag verification")
	}

	member, err := rt.Sender.GetChatMember(ctx, rt.ChatID, userID)
	if err != nil {
		return nil, fmt.Errorf("getChatMember for tag readback: %w", err)
	}

	// Extract tag from the member (type depends on member status)
	// NOTE: UnmarshalChatMember returns VALUE types, not pointers
	var actualTag string
	switch m := member.(type) {
	case tg.ChatMemberMember:
		actualTag = m.Tag
	case tg.ChatMemberRestricted:
		actualTag = m.Tag
	default:
		return nil, fmt.Errorf("unexpected member type %T — tags only apply to regular/restricted members", member)
	}

	if actualTag != s.ExpectedTag {
		return nil, fmt.Errorf("tag mismatch: expected %q, got %q", s.ExpectedTag, actualTag)
	}

	return &StepResult{
		Method: "getChatMember",
		Evidence: map[string]any{
			"user_id":      userID,
			"expected_tag": s.ExpectedTag,
			"actual_tag":   actualTag,
			"tags_match":   true,
		},
	}, nil
}

// SendDateTimeMessageStep sends a message with a date_time entity via HTML.
type SendDateTimeMessageStep struct {
	UnixTime int64
	Format   string
}

func (s *SendDateTimeMessageStep) Name() string { return "sendMessage (date_time entity)" }

func (s *SendDateTimeMessageStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	fallbackText := fmt.Sprintf("%d", s.UnixTime)
	html := fmt.Sprintf(`Event at: <tg-time unix="%d" format="%s">%s</tg-time>`,
		s.UnixTime, s.Format, fallbackText)

	msg, err := rt.Sender.SendMessage(ctx, rt.ChatID, html, WithParseMode("HTML"))
	if err != nil {
		return nil, err
	}
	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	// Check if entities contain date_time
	hasDateTime := false
	for _, e := range msg.Entities {
		if e.Type == tg.EntityDateTime {
			hasDateTime = true
		}
	}

	return &StepResult{
		Method:     "sendMessage",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"has_date_time_entity": hasDateTime,
			"entity_count":        len(msg.Entities),
		},
	}, nil
}

// SendMessageDraftStep sends a streaming draft.
type SendMessageDraftStep struct {
	DraftID int
	Text    string
}

func (s *SendMessageDraftStep) Name() string { return "sendMessageDraft" }

func (s *SendMessageDraftStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	err := rt.Sender.SendMessageDraft(ctx, sender.SendMessageDraftRequest{
		ChatID:  rt.ChatID,
		DraftID: s.DraftID,
		Text:    s.Text,
	})
	if err != nil {
		return nil, err
	}
	return &StepResult{
		Method: "sendMessageDraft",
		Evidence: map[string]any{
			"draft_id": s.DraftID,
			"text_len": len(s.Text),
		},
	}, nil
}
```

### 6.2 `cmd/galigo-testbot/suites/api95.go` — NEW FILE

```go
package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// ==================== Bot API 9.5 Scenarios ====================

// S44_DateTimeEntity tests sending messages with date_time formatting.
func S44_DateTimeEntity() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S44-DateTimeEntity",
		ScenarioDescription: "Send message with date_time entity (9.5)",
		CoveredMethods:      []string{"sendMessage"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendDateTimeMessageStep{
				UnixTime: time.Now().Add(24 * time.Hour).Unix(),
				Format:   "wDT",
			},
			&engine.CleanupStep{},
		},
	}
}

// S45_MemberTags tests setting, verifying, and removing member tags.
// Uses set -> readback -> verify -> remove -> readback pattern.
// UserID=0 means "use rt.AdminUserID at runtime".
func S45_MemberTags() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S45-MemberTags",
		ScenarioDescription: "Set, verify, and remove member tags (9.5)",
		CoveredMethods:      []string{"setChatMemberTag", "getChatMember"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			// Step 1: Set tag
			&engine.SetChatMemberTagStep{Tag: "galigo-test"},
			// Step 2: Read back and verify tag was set
			&engine.VerifyChatMemberTagStep{ExpectedTag: "galigo-test"},
			// Step 3: Remove tag (empty string)
			&engine.SetChatMemberTagStep{Tag: ""},
			// Step 4: Read back and verify tag was removed
			&engine.VerifyChatMemberTagStep{ExpectedTag: ""},
		},
	}
}

// S46_MessageStreaming tests sendMessageDraft (available to all bots since 9.5).
func S46_MessageStreaming() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S46-MessageStreaming",
		ScenarioDescription: "Send streaming draft message (9.5)",
		CoveredMethods:      []string{"sendMessageDraft"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendMessageDraftStep{DraftID: 1, Text: "Loading"},
			&engine.SendMessageDraftStep{DraftID: 1, Text: "Loading..."},
			&engine.SendMessageDraftStep{DraftID: 1, Text: "Loading complete!"},
		},
	}
}

// AllAPI95Scenarios returns all Bot API 9.5 test scenarios.
func AllAPI95Scenarios() []engine.Scenario {
	return []engine.Scenario{
		S44_DateTimeEntity(),
		S45_MemberTags(),
		S46_MessageStreaming(),
	}
}
```

### 6.3 `cmd/galigo-testbot/engine/scenario.go` — MODIFICATIONS

**6.3.1 Add `CanManageTags` to `ChatContext` struct (after `CanInviteUsers` field, line 87):**

```go
	CanManageTags      bool
```

**6.3.2 Add `CanManageTags` probing to `ProbeChat` (after `CanManageTopics` block, line 189):**

```go
			if admin.CanManageTags != nil {
				rt.ChatCtx.CanManageTags = *admin.CanManageTags
			}
```

**6.3.3 Add methods to `SenderClient` interface (before `// Webhook management methods` comment, line 320):**

```go
	// 9.5: Tags & Streaming
	SetChatMemberTag(ctx context.Context, chatID int64, userID int64, tag string) error
	SendMessageDraft(ctx context.Context, req sender.SendMessageDraftRequest) error
```

### 6.4 `cmd/galigo-testbot/engine/adapter.go` — APPEND before `var _ SenderClient` line (line 733)

```go
// ================= 9.5: Tags & Streaming =================

// SetChatMemberTag sets a member tag.
func (a *SenderAdapter) SetChatMemberTag(ctx context.Context, chatID int64, userID int64, tag string) error {
	return a.client.SetChatMemberTag(ctx, chatID, userID, tag)
}

// SendMessageDraft sends a streaming draft.
func (a *SenderAdapter) SendMessageDraft(ctx context.Context, req sender.SendMessageDraftRequest) error {
	return a.client.SendMessageDraft(ctx, req)
}
```

### 6.5 `cmd/galigo-testbot/registry/registry.go` — Add methods

**After `getUserProfilePhotos` entry (line 77):**

```go
	{Name: "getUserProfileAudios", Category: CategoryMessaging}, // 9.4
```

**After `setChatPermissions` entry (line 91):**

```go
	{Name: "setChatMemberTag", Category: CategoryChatAdmin},    // 9.5
```

**After `editMessageChecklist` entry (line 129):**

```go
	// === 9.5: Streaming ===
	{Name: "sendMessageDraft", Category: CategoryMessaging},
```

### 6.6 `cmd/galigo-testbot/main.go` — MODIFICATIONS

**6.6.1 Add to `runSuiteCommand` switch (after the api94 cases, line ~251):**

```go
	// Bot API 9.5 (S44-S46)
	case "api95", "api-95", "9.5":
		scenarios = suites.AllAPI95Scenarios()
	case "datetime-entity":
		scenarios = []engine.Scenario{suites.S44_DateTimeEntity()}
	case "member-tags":
		scenarios = []engine.Scenario{suites.S45_MemberTags()}
	case "message-streaming":
		scenarios = []engine.Scenario{suites.S46_MessageStreaming()}
```

**6.6.2 Add to `"all"` case in `runSuiteCommand` (after `AllAPI94Scenarios`, line ~261):**

```go
		scenarios = append(scenarios, suites.AllAPI95Scenarios()...)
```

**6.6.3 Add to `showCoverageStatus` (after `AllAPI94Scenarios`, line ~96):**

```go
	scenarios = append(scenarios, suites.AllAPI95Scenarios()...)
```

**6.6.4 Add to `handleRun` switch (after the api94 cases, line ~628):**

```go
	// Bot API 9.5 (S44-S46)
	case "api95", "api-95", "9.5":
		scenarios = suites.AllAPI95Scenarios()
	case "datetime-entity":
		scenarios = []engine.Scenario{suites.S44_DateTimeEntity()}
	case "member-tags":
		scenarios = []engine.Scenario{suites.S45_MemberTags()}
	case "message-streaming":
		scenarios = []engine.Scenario{suites.S46_MessageStreaming()}
```

**6.6.5 Add to `"all"` case in `handleRun` (after `AllAPI94Scenarios`, line ~638):**

```go
		scenarios = append(scenarios, suites.AllAPI95Scenarios()...)
```

**6.6.6 Add to `handleStatus` (after `AllAPI94Scenarios`, line ~682):**

```go
	scenarios = append(scenarios, suites.AllAPI95Scenarios()...)
```

**6.6.7 Update the help string in `handleHelp` (after "Bot API 9.4" section):**

```
Bot API 9.5 (S44-S46):
  api95              - All 9.5 tests
  datetime-entity    - date_time entity (S44)
  member-tags        - Member tag CRUD (S45)
  message-streaming  - sendMessageDraft (S46)
```

**6.6.8 Update the available suites in the error message (line ~265) to include:**

```
api95, datetime-entity, member-tags, message-streaming
```

---

## 7. Verification Commands

```bash
# Phase 1: tg/ package tests
go test ./tg/... -v -run "DateTime|SenderTag|Tag|ManageTags|Permissions|UTF16|FullAdmin|Moderator"

# Phase 2: sender/ package tests
go test ./sender/... -v -run "SetChatMemberTag|SendMessageDraft|PromoteChatMember.*ManageTags|DemoteChatMember.*ManageTags|PromoteOptions"

# Full suite with race detector
go test ./... -race -cover

# Lint
go vet ./...

# Testbot compile check
go build ./cmd/galigo-testbot/...
```

### Critical Assertions Checklist

| # | What | Test | Catches |
|---|------|------|---------|
| 1 | `SetChatMemberTagRequest.Tag` has NO `omitempty` | `TestSetChatMemberTag_RemoveTag_EmptyStringNotOmitted` | Empty string must be sent to remove tags |
| 2 | `ChatMemberRestricted.CanEditTag` is `bool` not `*bool` | `TestChatMemberRestricted_CanEditTag_AlwaysPresent` | Always-present restriction field |
| 3 | `ChatPermissions.CanEditTag` is `*bool` not `bool` | `TestChatPermissions_CanEditTag_IsPointer` | Same name, different type than Restricted |
| 4 | `MessageEntity.UnixTime` is `int64` | `TestMessageEntity_DateTime_Unmarshal` | Timestamp precision |
| 5 | date_time fields omitted for non-datetime entities | `TestMessageEntity_Bold_NoDateTimeFields` | No pollution of existing entities |
| 6 | `SendMessageDraft` rejects `draft_id=0` | `TestSendMessageDraft_DraftIDZero_ValidationError` | Telegram requires non-zero draft_id |
| 7 | `SetChatMemberTag` rejects tags >16 chars | `TestSetChatMemberTag_TagTooLong_ValidationError` | API constraint: 0-16 characters |
| 8 | `SetChatMemberTag` validates chat_id and user_id | `TestSetChatMemberTag_Validation_*` | Matches all moderation methods |
| 9 | `SendMessageDraft` validates chat_id | `TestSendMessageDraft_Validation_InvalidChatID` | Catches nil/missing chat_id |
| 10 | `DemoteChatMember` resets `can_manage_tags` | `TestDemoteChatMember_ResetsCanManageTags` | Admin doesn't keep tag rights after demotion |
| 11 | `AllPermissions()` includes `CanEditTag` | `TestAllPermissions_IncludesCanEditTag` | Preset completeness |
| 12 | `FullAdminRights()` includes `CanManageTags` | `TestFullAdminRights_IncludesCanManageTags` | Preset completeness |
| 13 | S45 tag readback matches after set | `VerifyChatMemberTagStep` in testbot | Proves Telegram persisted the tag |
| 14 | S45 UserID fallback to AdminUserID | `SetChatMemberTagStep.Execute` | Prevents UserID=0 runtime failure |

---

## Change Manifest

| Change Type | Count | Files |
|---|:---:|---|
| New files | 4 | `sender/streaming.go`, `tg/datetime.go`, `tg/entity_test.go`, `sender/streaming_test.go` |
| New testbot files | 2 | `engine/steps_api95.go`, `suites/api95.go` |
| New fields on existing types | 8 | `tg/types.go` (3), `tg/chat_member.go` (3), `tg/chat_permissions.go` (1), `tg/chat_admin_rights.go` (1) |
| New constant | 1 | `tg/types.go` |
| Updated helper functions | 4 | `tg/chat_permissions.go` (2), `tg/chat_admin_rights.go` (2) |
| New request structs | 2 | `sender/chat_moderation.go` (1), `sender/streaming.go` (1) |
| New sender methods | 2 | `SetChatMemberTag`, `SendMessageDraft` |
| New promote option | 1 | `sender/chat_admin.go` |
| Updated existing methods | 2 | `PromoteChatMemberWithRights`, `DemoteChatMember` |
| New unit tests | ~28 | `tg/*_test.go`, `sender/*_test.go` |
| New testbot scenarios | 3 | `suites/api95.go` |
| New testbot steps | 4 | `engine/steps_api95.go` |
| Updated testbot infra | 4 | `scenario.go`, `adapter.go`, `registry.go`, `main.go` |
| Modified test count | 1 | `TestPromoteOptions`: 12→13 |

**Total new/modified files**: ~16

---

## Deferred: Out of Scope

| Item | Reason |
|---|---|
| `BottomButton.IconCustomEmojiID` (WebApps) | WebApps support is not yet in galigo. Track for future. |
