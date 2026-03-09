# Bot API 9.5 — Corrected TDD Implementation Guide for galigo

**Based on**: `9_5-implementation.md` v2
**Corrected by**: galigo maintainer
**Correction date**: 2026-03-08
**Status**: Ready to implement — all 21 issues from `9_5-review.md` resolved
**API version**: Bot API 9.5 (March 1, 2026)

This document contains **complete, copy-paste-ready code** for every file touched by the 9.5 implementation. All code follows the actual codebase patterns verified against the source files.

For the full list of issues that were corrected, see `9_5-review.md`.

---

## Table of Contents

- [Phase 0: JSON Fixtures](#phase-0-json-fixtures-the-contract)
- [Phase 1: Stub Types](#phase-1-stub-types)
- [Phase 2: Tests](#phase-2-tests)
- [Phase 3: Implementation](#phase-3-implementation)
- [Phase 4: Testbot E2E](#phase-4-testbot-e2e)
- [Phase 5: Verification](#phase-5-verification)
- [Change Manifest](#change-manifest)

---

## Phase 0: JSON Fixtures (The Contract)

Fixtures are unchanged from the original plan — they represent the Telegram API contract and were correct.

<details>
<summary>Click to expand all fixtures (0A–0O)</summary>

### Fixture 0A: MessageEntity with `date_time` type

```json
{
    "type": "date_time",
    "offset": 0,
    "length": 16,
    "unix_time": 1647531900,
    "date_time_format": "wDT"
}
```

### Fixture 0B: MessageEntity `date_time` — relative format

```json
{
    "type": "date_time",
    "offset": 5,
    "length": 12,
    "unix_time": 1647531900,
    "date_time_format": "r"
}
```

### Fixture 0C: MessageEntity `date_time` — empty format (fallback)

```json
{
    "type": "date_time",
    "offset": 0,
    "length": 10,
    "unix_time": 1647531900
}
```

### Fixture 0D: Message with `sender_tag`

```json
{
    "message_id": 42,
    "date": 1647531900,
    "chat": {"id": -1001234567890, "type": "supergroup", "title": "Test"},
    "from": {"id": 123456, "is_bot": false, "first_name": "Alice"},
    "text": "Hello",
    "sender_tag": "Team Lead"
}
```

### Fixture 0E: ChatMemberMember with `tag`

```json
{
    "status": "member",
    "user": {"id": 123456, "is_bot": false, "first_name": "Alice"},
    "tag": "Developer"
}
```

### Fixture 0F: ChatMemberRestricted with `tag` + `can_edit_tag`

```json
{
    "status": "restricted",
    "user": {"id": 123456, "is_bot": false, "first_name": "Alice"},
    "is_member": true,
    "can_send_messages": true,
    "can_send_audios": true,
    "can_send_documents": true,
    "can_send_photos": true,
    "can_send_videos": true,
    "can_send_video_notes": true,
    "can_send_voice_notes": true,
    "can_send_polls": true,
    "can_send_other_messages": true,
    "can_add_web_page_previews": true,
    "can_change_info": false,
    "can_invite_users": false,
    "can_pin_messages": false,
    "can_manage_topics": false,
    "until_date": 0,
    "tag": "Intern",
    "can_edit_tag": false
}
```

### Fixture 0G: ChatPermissions with `can_edit_tag`

```json
{
    "can_send_messages": true,
    "can_edit_tag": true
}
```

### Fixture 0H: ChatMemberAdministrator with `can_manage_tags`

```json
{
    "status": "administrator",
    "user": {"id": 789, "is_bot": true, "first_name": "Bot"},
    "can_be_edited": true,
    "can_manage_chat": true,
    "can_delete_messages": true,
    "can_manage_video_chats": true,
    "can_restrict_members": true,
    "can_promote_members": false,
    "can_change_info": true,
    "can_invite_users": true,
    "can_pin_messages": true,
    "can_manage_topics": true,
    "can_manage_tags": true,
    "is_anonymous": false
}
```

### Fixture 0I: ChatAdministratorRights with `can_manage_tags`

```json
{
    "can_manage_chat": true,
    "can_delete_messages": false,
    "can_manage_video_chats": false,
    "can_restrict_members": false,
    "can_promote_members": false,
    "can_change_info": false,
    "can_invite_users": false,
    "can_pin_messages": true,
    "can_manage_topics": false,
    "can_manage_tags": true
}
```

### Fixture 0J: setChatMemberTag request — SET tag

```json
{
    "chat_id": -1001234567890,
    "user_id": 123456,
    "tag": "Team Lead"
}
```

### Fixture 0K: setChatMemberTag request — REMOVE tag

```json
{
    "chat_id": -1001234567890,
    "user_id": 123456,
    "tag": ""
}
```

### Fixture 0L: sendMessageDraft request

```json
{
    "chat_id": 123456789,
    "draft_id": 42,
    "text": "Generating response...",
    "parse_mode": "HTML"
}
```

### Fixture 0M: promoteChatMember request with `can_manage_tags`

```json
{
    "chat_id": -1001234567890,
    "user_id": 789,
    "can_manage_tags": true
}
```

### Fixture 0N: sendMessageDraft with draft_id=0 (INVALID)

```json
{
    "chat_id": 123456789,
    "draft_id": 0,
    "text": "This should be rejected client-side"
}
```

### Fixture 0O: setChatMemberTag with tag too long (INVALID)

```json
{
    "chat_id": -1001234567890,
    "user_id": 123456,
    "tag": "This Tag Is Way Too Long For Telegram"
}
```

</details>

---

## Phase 1: Stub Types

### 1A. `tg/types.go` — Add to MessageEntity struct (line ~142, before closing brace)

```go
// 9.5: date_time entity fields
UnixTime       int64  `json:"unix_time,omitempty"`
DateTimeFormat string `json:"date_time_format,omitempty"`
```

### 1B. `tg/types.go` — Add constant (near top of file, after imports)

```go
// EntityDateTime is the entity type for formatted date/time display (9.5).
const EntityDateTime = "date_time"
```

### 1C. `tg/types.go` — Add to Message struct (line ~66, before ReplyMarkup)

```go
SenderTag string `json:"sender_tag,omitempty"` // 9.5
```

### 1D. `tg/chat_member.go` — Add to ChatMemberMember (line ~73, before closing brace)

```go
Tag string `json:"tag,omitempty"` // 9.5
```

### 1E. `tg/chat_member.go` — Add to ChatMemberRestricted (line ~97, before closing brace)

```go
Tag        string `json:"tag,omitempty"`      // 9.5
CanEditTag bool   `json:"can_edit_tag"`       // 9.5 — NOT omitempty (always present in restriction set)
```

> **Why `bool` not `*bool`?** All existing fields in `ChatMemberRestricted` are `bool` (always present when Telegram returns a restricted member). `CanEditTag` follows this same pattern.

### 1F. `tg/chat_permissions.go` — Add to ChatPermissions (line ~19, before closing brace)

```go
CanEditTag *bool `json:"can_edit_tag,omitempty"` // 9.5
```

> **Why `*bool`?** All existing fields in `ChatPermissions` are `*bool` to distinguish "not set" (nil) from "explicitly false". `CanEditTag` follows this pattern.

### 1G. `tg/chat_member.go` — Add to ChatMemberAdministrator (line ~64, before CustomTitle)

```go
CanManageTags *bool `json:"can_manage_tags,omitempty"` // 9.5
```

### 1H. `tg/chat_admin_rights.go` — Add to ChatAdministratorRights (line ~20, before closing brace)

```go
CanManageTags *bool `json:"can_manage_tags,omitempty"` // 9.5
```

### 1I. `sender/chat_moderation.go` — Add request struct (after existing request structs)

```go
// SetChatMemberTagRequest represents a setChatMemberTag request.
// Added in Bot API 9.5.
type SetChatMemberTagRequest struct {
	ChatID tg.ChatID `json:"chat_id"`
	UserID int64     `json:"user_id"`
	Tag    string    `json:"tag"` // NO omitempty — empty string removes tag
}
```

### 1J. `sender/streaming.go` — New file, stub request struct

```go
package sender

import "github.com/prilive-com/galigo/tg"

// SendMessageDraftRequest represents a sendMessageDraft request.
// Added in Bot API 9.5.
type SendMessageDraftRequest struct {
	ChatID          tg.ChatID          `json:"chat_id"`
	DraftID         int                `json:"draft_id"`          // Must be non-zero
	Text            string             `json:"text"`
	MessageThreadID int                `json:"message_thread_id,omitempty"`
	ParseMode       tg.ParseMode       `json:"parse_mode,omitempty"`
	Entities        []tg.MessageEntity `json:"entities,omitempty"`
}
```

### 1K. `sender/chat_admin.go` — Add to PromoteChatMemberRequest (line ~29, before closing brace)

```go
CanManageTags *bool `json:"can_manage_tags,omitempty"` // 9.5
```

---

## Phase 2: Tests

### 2A. `tg/entity_test.go` — New file

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
```

### 2B. `tg/chat_member_test.go` — Append to existing file

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

### 2C. `tg/chat_permissions_test.go` — Append to existing file

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

### 2D. `tg/types_test.go` — Append to existing file

> **Note**: This file uses `package tg_test` — use `tg.` prefix.

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

### 2E. `sender/chat_moderation_test.go` — Append to existing file

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

### 2F. `sender/streaming_test.go` — New file

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
		// chat_id must be numeric (not quoted string)
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

### 2G. `sender/chat_admin_test.go` — Append to existing file

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

Update the existing `TestPromoteOptions` test to include the new option:

```go
// Replace existing TestPromoteOptions (line ~157):
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

## Phase 3: Implementation

### 3A. `tg/` package — All stub fields from Phase 1 become the real implementation

Additionally, update the helper functions:

#### `tg/chat_permissions.go` — Update `AllPermissions()`

Add after `CanManageTopics`:
```go
CanEditTag: boolPtr(true),
```

#### `tg/chat_permissions.go` — Update `NoPermissions()`

Add after `CanManageTopics`:
```go
CanEditTag: boolPtr(false),
```

> `ReadOnlyPermissions()` and `TextOnlyPermissions()` do NOT need `CanEditTag` — they're for message-level restrictions, not admin-level features. Leaving it as `nil` (unset) is correct.

#### `tg/chat_admin_rights.go` — Update `FullAdminRights()`

Add after `CanManageDirectMessages`:
```go
CanManageTags: boolPtr(true),
```

#### `tg/chat_admin_rights.go` — Update `ModeratorRights()`

Add after `CanPinMessages`:
```go
CanManageTags: boolPtr(true),
```

> Moderators typically manage tags. `ContentManagerRights()` does NOT need it — content managers manage content, not members.

**Verify**: `go test ./tg/...` — all new and existing tests pass.

### 3B. `sender/chat_moderation.go` — New method

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

### 3C. `sender/streaming.go` — New method (add below the request struct from Phase 1J)

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

### 3D. `sender/chat_admin.go` — New promote option (after `WithCanManageTopics`)

```go
// WithCanManageTags grants ability to manage member tags (9.5).
func WithCanManageTags(can bool) PromoteOption {
	return func(r *PromoteChatMemberRequest) {
		r.CanManageTags = &can
	}
}
```

### 3E. `sender/chat_admin.go` — Update `PromoteChatMemberWithRights`

Add after the `CanManageTopics` line (line ~89):
```go
CanManageTags:   rights.CanManageTags,
```

### 3F. `sender/chat_admin.go` — Update `DemoteChatMember`

Add after the `CanPinMessages` line (line ~117):
```go
CanManageTags: &f,
```

**Verify**: `go test ./sender/...` — all new and existing tests pass.

### 3G. (Optional) `tg/datetime.go` — New file with helpers

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

### 3H. (Optional) `tg/entity_test.go` — Add helper tests (append to Phase 2A file)

```go
// ==================== Helpers ====================

func TestUTF16Len(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"", 0},
		{"\U0001F389", 2},       // emoji = surrogate pair
		{"Hello \U0001F30D!", 9}, // 7 ASCII + 1 surrogate pair + 1
		{"\u65E5\u672C\u8A9E", 3},               // CJK = 1 UTF-16 unit each
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

---

## Phase 4: Testbot E2E

### 4A. `cmd/galigo-testbot/engine/steps_api95.go` — New file

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

### 4B. `cmd/galigo-testbot/suites/api95.go` — New file

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

### 4C. `cmd/galigo-testbot/engine/scenario.go` — Add to SenderClient interface

```go
// 9.5: Tags & Streaming
SetChatMemberTag(ctx context.Context, chatID int64, userID int64, tag string) error
SendMessageDraft(ctx context.Context, req sender.SendMessageDraftRequest) error
```

> `GetChatMember` already exists in the interface — no change needed for S45 readback.

### 4D. `cmd/galigo-testbot/engine/scenario.go` — Add `CanManageTags` to ChatContext

```go
// Add to ChatContext struct, after CanInviteUsers:
CanManageTags bool // 9.5
```

### 4E. `cmd/galigo-testbot/engine/scenario.go` — Update ProbeChat

Add after the `CanManageTopics` block (line ~189):
```go
if admin.CanManageTags != nil {
	rt.ChatCtx.CanManageTags = *admin.CanManageTags
}
```

### 4F. `cmd/galigo-testbot/engine/adapter.go` — Add adapter methods

```go
func (a *SenderAdapter) SetChatMemberTag(ctx context.Context, chatID int64, userID int64, tag string) error {
	return a.client.SetChatMemberTag(ctx, chatID, userID, tag)
}

func (a *SenderAdapter) SendMessageDraft(ctx context.Context, req sender.SendMessageDraftRequest) error {
	return a.client.SendMessageDraft(ctx, req)
}
```

### 4G. `cmd/galigo-testbot/registry/registry.go` — Add methods

Add after `getUserProfilePhotos`:
```go
{Name: "getUserProfileAudios", Category: CategoryMessaging},
```

Add in the Chat Administration section:
```go
{Name: "setChatMemberTag", Category: CategoryChatAdmin},
```

Add in the Messaging section:
```go
{Name: "sendMessageDraft", Category: CategoryMessaging},
```

### 4H. `cmd/galigo-testbot/main.go` — Wire suite

Add after the api94 cases:
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

Add to the `"all"` case:
```go
scenarios = append(scenarios, suites.AllAPI95Scenarios()...)
```

Add to the `--status` scenario collection:
```go
scenarios = append(scenarios, suites.AllAPI95Scenarios()...)
```

Update the available suites help string to include:
```
api95, datetime-entity, member-tags, message-streaming
```

---

## Phase 5: Verification

### Run all tests:
```bash
# 9.5 type tests
go test ./tg/... -v -run "DateTime|SenderTag|Tag|ManageTags|Permissions|UTF16|FullAdmin|Moderator"

# 9.5 sender tests
go test ./sender/... -v -run "SetChatMemberTag|SendMessageDraft|PromoteChatMember.*ManageTags|DemoteChatMember.*ManageTags|PromoteOptions"

# Full suite with race detector
go test ./... -race -cover
```

### Critical assertions verified:

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
| New fields on existing types | 8 | `tg/types.go`, `tg/chat_member.go`, `tg/chat_permissions.go`, `tg/chat_admin_rights.go` |
| New constant | 1 | `tg/types.go` |
| Updated helper functions | 6 | `tg/chat_permissions.go` (2), `tg/chat_admin_rights.go` (2), `sender/chat_admin.go` (2) |
| New helper functions | 2 | `tg/datetime.go` (optional) |
| New request structs | 2 | `sender/chat_moderation.go`, `sender/streaming.go` |
| New sender methods | 2 | `SetChatMemberTag`, `SendMessageDraft` |
| New promote option | 1 | `sender/chat_admin.go` |
| Updated existing methods | 2 | `PromoteChatMemberWithRights`, `DemoteChatMember` |
| New unit tests | ~28 | `tg/*_test.go`, `sender/*_test.go` |
| New testbot scenarios | 3 | `suites/api95.go` |
| New testbot steps | 4 | `engine/steps_api95.go` |
| Updated testbot infra | 4 | `scenario.go`, `adapter.go`, `registry.go`, `main.go` |

**Total new/modified files**: ~16
**Estimated implementation time**: ~5 hours (following TDD order)

---

## Deferred: Out of Scope

| Item | Reason |
|---|---|
| `BottomButton.IconCustomEmojiID` (WebApps) | WebApps support is not yet in galigo. Track for future. |
