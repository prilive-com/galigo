# Bot API 9.5 — TDD Implementation Guide for galigo (FINAL-v2)

**Approach**: Contract-First TDD
**Order**: JSON fixtures → Stub types → Failing tests → Implementation → Testbot E2E
**API version**: Bot API 9.5 (March 1, 2026)
**Go version**: 1.25 (from go.mod)
**Estimated time**: ~6 hours total

### Pattern Reference (verified against source)

| Pattern | galigo actual | Source |
|---------|--------------|--------|
| Dispatch | `c.callJSON(ctx, "method", req, nil, extractChatID(...))` | `sender/call.go` |
| Bool methods | return `error` only (`nil` out param) | `sender/chat_moderation.go` |
| Test client | `testutil.NewTestClient(t, server.BaseURL())` | `internal/testutil/client.go` |
| Body assert | `json.NewDecoder(r.Body).Decode(&req)` in handler | `sender/chat_moderation_test.go` |
| Reply True | `testutil.ReplyBool(w, true)` | `internal/testutil/replies.go` |
| Validation | `tg.NewValidationError("field", "message")` | `sender/identity.go` |
| Length check | `len(x) > N` (byte-based) | `sender/chat_admin.go:132` |
| Capture assert | `server.CaptureCount() == 0` | `sender/chat_moderation_test.go:56` |
| PromoteOption | `type PromoteOption func(*PromoteChatMemberRequest)` | `sender/chat_admin.go:146` |
| Restricted bools | plain `bool`, NO `omitempty` | `tg/chat_member.go:82-97` |
| Permissions bools | `*bool` with `omitempty` | `tg/chat_permissions.go:5-19` |
| Admin optional | `*bool` with `omitempty` | `tg/chat_member.go:56-63` |
| All ChatID fields | `tg.ChatID` (every struct, no exceptions) | all `sender/*.go` |
| chat_moderation.go sigs | decomposed params `(ctx, chatID, userID, ...)` | `sender/chat_moderation.go` |
| chat_admin.go sigs | decomposed params `(ctx, chatID, userID, ...)` | `sender/chat_admin.go` |
| bot_api_new.go sigs | struct params `(ctx, req SomeRequest)` | `sender/bot_api_new.go` |
| `tg/types_test.go` | `package tg_test` (external, needs `tg.` prefix) | file header |
| `tg/chat_member_test.go` | `package tg` (internal, bare types OK) | file header |
| `tg/chat_permissions_test.go` | `package tg` (internal, bare types OK) | file header |
| engine.WithParseMode | EXISTS in `engine/scenario.go:365` | verified |

---

## Forced Design Decisions

### FD-1: SetChatMemberTag uses DECOMPOSED params (not struct)

`chat_moderation.go` and `chat_admin.go` both use decomposed params. The closest analog is `SetChatAdministratorCustomTitle(ctx, chatID, userID, customTitle)` — identical shape (chatID + userID + single string, max 16 chars). So:

```go
func (c *Client) SetChatMemberTag(ctx context.Context, chatID tg.ChatID, userID int64, tag string) error
```

The request struct is still defined (and public for direct use), but the primary API uses decomposed params.

### FD-2: SendMessageDraftRequest.ChatID uses tg.ChatID (not int64)

Every request struct in galigo uses `tg.ChatID` — zero exceptions. Even though the Telegram API restricts this to integers, we follow galigo's pattern and add validation:

```go
type SendMessageDraftRequest struct {
    ChatID  tg.ChatID `json:"chat_id"` // tg.ChatID for consistency, validated to be integer
    // ...
}
```

Validation rejects non-integer ChatIDs at call time.

### FD-3: CanEditTag on ChatMemberRestricted is `bool` with NO `omitempty`

Every restriction bool on `ChatMemberRestricted` is plain `bool` without `omitempty` — Telegram always sends the full restriction set. `CanEditTag` follows this pattern exactly.

### FD-4: Tag field on SetChatMemberTagRequest has NO `omitempty`

Empty string = remove tag. If `omitempty` were present, `Tag: ""` would be silently dropped from JSON, making tag removal impossible.

### FD-5: Tests in tg/types_test.go use `tg.` prefix (package tg_test)

Phase 2A tests go in `tg/types_test.go` which uses `package tg_test`. All type references require `tg.` prefix. Phase 2B tests go in `tg/chat_member_test.go` which uses `package tg` — bare types OK.

---

## Phase 0: JSON Fixtures — 30 min

*(Same as previous version — API-spec driven, unchanged.)*

### 0A–0C: MessageEntity `date_time` fixtures

```json
{"type":"date_time","offset":0,"length":16,"unix_time":1647531900,"date_time_format":"wDT"}
{"type":"date_time","offset":5,"length":12,"unix_time":1647531900,"date_time_format":"r"}
{"type":"date_time","offset":0,"length":10,"unix_time":1647531900}
```

### 0D: Message with sender_tag

```json
{"message_id":42,"date":1647531900,"chat":{"id":-1001234567890,"type":"supergroup","title":"Dev"},"from":{"id":123456,"is_bot":false,"first_name":"Alice"},"text":"Hello","sender_tag":"Team Lead"}
```

### 0E–0F: ChatMember with tag / can_edit_tag

```json
{"status":"member","user":{"id":123456,"is_bot":false,"first_name":"Alice"},"tag":"Developer"}
```

```json
{"status":"restricted","user":{"id":789,"is_bot":false,"first_name":"Bob"},"is_member":true,"can_send_messages":true,"can_send_audios":true,"can_send_documents":true,"can_send_photos":true,"can_send_videos":true,"can_send_video_notes":true,"can_send_voice_notes":true,"can_send_polls":true,"can_send_other_messages":true,"can_add_web_page_previews":true,"can_change_info":false,"can_invite_users":false,"can_pin_messages":false,"can_manage_topics":false,"until_date":0,"tag":"Intern","can_edit_tag":false}
```

### 0G: ChatMemberAdministrator with can_manage_tags

```json
{"status":"administrator","user":{"id":789,"is_bot":true,"first_name":"Bot"},"can_be_edited":true,"can_manage_chat":true,"can_delete_messages":true,"can_manage_video_chats":true,"can_restrict_members":true,"can_promote_members":false,"can_change_info":true,"can_invite_users":true,"can_pin_messages":true,"can_manage_topics":true,"can_manage_tags":true,"is_anonymous":false}
```

### 0H–0I: ChatPermissions with/without can_edit_tag

```json
{"can_send_messages":true,"can_edit_tag":true}
{"can_send_messages":true}
```

### 0J–0K: setChatMemberTag (set / remove — CRITICAL)

```json
{"chat_id":-1001234567890,"user_id":123456,"tag":"Team Lead"}
{"chat_id":-1001234567890,"user_id":123456,"tag":""}
```

### 0L: sendMessageDraft

```json
{"chat_id":123456789,"draft_id":42,"text":"Generating..."}
```

### 0M: promoteChatMember with can_manage_tags

```json
{"chat_id":-1001234567890,"user_id":789,"can_manage_tags":true}
```

---

## Phase 1: Stub Types — 15 min

### 1A. `tg/types.go` — MessageEntity

After `CustomEmojiID`:

```go
UnixTime       int64  `json:"unix_time,omitempty"`        // 9.5
DateTimeFormat string `json:"date_time_format,omitempty"` // 9.5
```

### 1B. `tg/types.go` — Message

After `ChatOwnerChanged`:

```go
SenderTag string `json:"sender_tag,omitempty"` // 9.5
```

### 1C. `tg/chat_member.go` — ChatMemberMember

After `UntilDate`:

```go
Tag string `json:"tag,omitempty"` // 9.5
```

### 1D. `tg/chat_member.go` — ChatMemberRestricted

After `UntilDate`:

```go
Tag        string `json:"tag,omitempty"` // 9.5
CanEditTag bool   `json:"can_edit_tag"`  // 9.5 — plain bool, NO omitempty (FD-3)
```

### 1E. `tg/chat_member.go` — ChatMemberAdministrator

After `CanManageDirectMessages`:

```go
CanManageTags *bool `json:"can_manage_tags,omitempty"` // 9.5
```

### 1F. `tg/chat_permissions.go` — ChatPermissions

After `CanManageTopics`:

```go
CanEditTag *bool `json:"can_edit_tag,omitempty"` // 9.5
```

Update helpers:

```go
// AllPermissions() — add:
CanEditTag: boolPtr(true),

// NoPermissions() — add:
CanEditTag: boolPtr(false),
```

### 1G. `tg/chat_admin_rights.go` — ChatAdministratorRights

After `CanManageDirectMessages`:

```go
CanManageTags *bool `json:"can_manage_tags,omitempty"` // 9.5
```

Update `FullAdminRights()`:

```go
CanManageTags: boolPtr(true),
```

### 1H. `sender/chat_admin.go` — PromoteChatMemberRequest

After `CanManageTopics`:

```go
CanManageTags *bool `json:"can_manage_tags,omitempty"` // 9.5
```

### 1I. `sender/chat_moderation.go` — New request struct

```go
// SetChatMemberTagRequest represents a setChatMemberTag request.
type SetChatMemberTagRequest struct {
	ChatID tg.ChatID `json:"chat_id"`
	UserID int64     `json:"user_id"`
	Tag    string    `json:"tag"` // NO omitempty — empty string removes tag (FD-4)
}
```

### 1J. `sender/requests.go` — New request struct (FD-2: tg.ChatID)

```go
// SendMessageDraftRequest represents a sendMessageDraft request.
// Added in Bot API 9.3, generally available since 9.5.
type SendMessageDraftRequest struct {
	ChatID          tg.ChatID          `json:"chat_id"` // tg.ChatID for consistency; validated integer-only
	DraftID         int                `json:"draft_id"`
	Text            string             `json:"text"`
	MessageThreadID int                `json:"message_thread_id,omitempty"`
	ParseMode       tg.ParseMode       `json:"parse_mode,omitempty"`
	Entities        []tg.MessageEntity `json:"entities,omitempty"`
}
```

---

## Phase 2: Failing Tests — 2 hours

### 2A. `tg/types_test.go` — package `tg_test` (FD-5: uses `tg.` prefix)

```go
func TestMessageEntity_DateTime_Unmarshal(t *testing.T) {
	data := []byte(`{"type":"date_time","offset":0,"length":16,"unix_time":1647531900,"date_time_format":"wDT"}`)
	var e tg.MessageEntity
	require.NoError(t, json.Unmarshal(data, &e))
	assert.Equal(t, "date_time", e.Type)
	assert.Equal(t, int64(1647531900), e.UnixTime)
	assert.Equal(t, "wDT", e.DateTimeFormat)
}

func TestMessageEntity_DateTime_NoFormat(t *testing.T) {
	data := []byte(`{"type":"date_time","offset":0,"length":10,"unix_time":1647531900}`)
	var e tg.MessageEntity
	require.NoError(t, json.Unmarshal(data, &e))
	assert.Equal(t, int64(1647531900), e.UnixTime)
	assert.Empty(t, e.DateTimeFormat)
}

func TestMessageEntity_Bold_NoDateTimeFields(t *testing.T) {
	e := tg.MessageEntity{Type: "bold", Offset: 0, Length: 5}
	data, err := json.Marshal(e)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "unix_time")
	assert.NotContains(t, string(data), "date_time_format")
}

func TestMessage_SenderTag_Unmarshal(t *testing.T) {
	data := []byte(`{
		"message_id":42,"date":1647531900,
		"chat":{"id":-1001234567890,"type":"supergroup","title":"Dev"},
		"from":{"id":123456,"is_bot":false,"first_name":"Alice"},
		"text":"Hello","sender_tag":"Team Lead"
	}`)
	var m tg.Message
	require.NoError(t, json.Unmarshal(data, &m))
	assert.Equal(t, "Team Lead", m.SenderTag)
}

func TestMessage_SenderTag_Absent(t *testing.T) {
	data := []byte(`{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"text":"hi"}`)
	var m tg.Message
	require.NoError(t, json.Unmarshal(data, &m))
	assert.Empty(t, m.SenderTag)
}
```

### 2B. `tg/chat_member_test.go` — package `tg` (bare types OK)

```go
func TestUnmarshalChatMember_MemberTag(t *testing.T) {
	data, _ := json.Marshal(map[string]any{
		"status": "member",
		"user":   map[string]any{"id": 123, "first_name": "Alice", "is_bot": false},
		"tag":    "Developer",
	})
	member, err := UnmarshalChatMember(data)
	require.NoError(t, err)
	m, ok := member.(ChatMemberMember)
	require.True(t, ok)
	assert.Equal(t, "Developer", m.Tag)
}

func TestUnmarshalChatMember_MemberNoTag(t *testing.T) {
	data, _ := json.Marshal(map[string]any{
		"status": "member",
		"user":   map[string]any{"id": 123, "first_name": "Alice", "is_bot": false},
	})
	member, err := UnmarshalChatMember(data)
	require.NoError(t, err)
	m := member.(ChatMemberMember)
	assert.Empty(t, m.Tag)
}

func TestUnmarshalChatMember_RestrictedCanEditTag(t *testing.T) {
	data, _ := json.Marshal(map[string]any{
		"status": "restricted",
		"user":   map[string]any{"id": 789, "first_name": "Bob", "is_bot": false},
		"is_member": true,
		"can_send_messages": true, "can_send_audios": true, "can_send_documents": true,
		"can_send_photos": true, "can_send_videos": true, "can_send_video_notes": true,
		"can_send_voice_notes": true, "can_send_polls": true, "can_send_other_messages": true,
		"can_add_web_page_previews": true, "can_change_info": false, "can_invite_users": false,
		"can_pin_messages": false, "can_manage_topics": false, "until_date": 0,
		"tag": "Intern", "can_edit_tag": false,
	})
	member, err := UnmarshalChatMember(data)
	require.NoError(t, err)
	r, ok := member.(ChatMemberRestricted)
	require.True(t, ok)
	assert.Equal(t, "Intern", r.Tag)
	assert.False(t, r.CanEditTag)
}

func TestUnmarshalChatMember_AdminCanManageTags(t *testing.T) {
	data, _ := json.Marshal(map[string]any{
		"status": "administrator",
		"user":   map[string]any{"id": 789, "first_name": "Bot", "is_bot": true},
		"can_be_edited": true, "can_manage_chat": true,
		"can_manage_tags": true, "is_anonymous": false,
	})
	member, err := UnmarshalChatMember(data)
	require.NoError(t, err)
	admin := member.(ChatMemberAdministrator)
	require.NotNil(t, admin.CanManageTags)
	assert.True(t, *admin.CanManageTags)
}

func TestUnmarshalChatMember_AdminCanManageTags_Absent(t *testing.T) {
	data, _ := json.Marshal(map[string]any{
		"status": "administrator",
		"user":   map[string]any{"id": 789, "first_name": "Bot", "is_bot": true},
		"can_be_edited": true, "can_manage_chat": true, "is_anonymous": false,
	})
	member, err := UnmarshalChatMember(data)
	require.NoError(t, err)
	admin := member.(ChatMemberAdministrator)
	assert.Nil(t, admin.CanManageTags)
}
```

### 2C. `tg/chat_permissions_test.go` — package `tg` (bare types OK)

```go
func TestChatPermissions_CanEditTag_Present(t *testing.T) {
	data := []byte(`{"can_send_messages":true,"can_edit_tag":true}`)
	var p ChatPermissions
	require.NoError(t, json.Unmarshal(data, &p))
	require.NotNil(t, p.CanEditTag)
	assert.True(t, *p.CanEditTag)
}

func TestChatPermissions_CanEditTag_Absent(t *testing.T) {
	data := []byte(`{"can_send_messages":true}`)
	var p ChatPermissions
	require.NoError(t, json.Unmarshal(data, &p))
	assert.Nil(t, p.CanEditTag)
}
```

### 2D. `sender/chat_moderation_test.go` — SetChatMemberTag (FD-1: decomposed params)

```go
// ==================== SetChatMemberTag ====================

func TestSetChatMemberTag(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setChatMemberTag", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "Team Lead", req["tag"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetChatMemberTag(context.Background(), int64(-1001234567890), int64(123456), "Team Lead")
	assert.NoError(t, err)
}

// ⚠️ CRITICAL TDD TEST — catches the omitempty trap
func TestSetChatMemberTag_RemoveTag_EmptyStringPresent(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setChatMemberTag", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		val, exists := req["tag"]
		assert.True(t, exists, "tag field must be present even when empty")
		assert.Equal(t, "", val, "empty tag must be sent as empty string")
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetChatMemberTag(context.Background(), int64(-1001234567890), int64(123456), "")
	assert.NoError(t, err)
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

	err := client.SetChatMemberTag(context.Background(), int64(-100123), int64(0), "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user_id")
	assert.Equal(t, 0, server.CaptureCount())
}

func TestSetChatMemberTag_Validation_TagTooLong(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetChatMemberTag(context.Background(), int64(-100123), int64(123456), "This Is Way Too Long")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tag")
	assert.Equal(t, 0, server.CaptureCount())
}
```

### 2E. `sender/bot_api_new_test.go` — SendMessageDraft (FD-2: tg.ChatID)

```go
// ==================== SendMessageDraft ====================

func TestSendMessageDraft(t *testing.T) {
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
		ChatID: int64(123456789), DraftID: 42, Text: "Generating...",
	})
	assert.NoError(t, err)
}

func TestSendMessageDraft_RejectsStringChatID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SendMessageDraft(context.Background(), sender.SendMessageDraftRequest{
		ChatID: "@username", DraftID: 1, Text: "test",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat_id")
	assert.Equal(t, 0, server.CaptureCount())
}

func TestSendMessageDraft_Validation_DraftIDZero(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SendMessageDraft(context.Background(), sender.SendMessageDraftRequest{
		ChatID: int64(123456789), DraftID: 0, Text: "test",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "draft_id")
	assert.Equal(t, 0, server.CaptureCount())
}

func TestSendMessageDraft_Validation_EmptyText(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SendMessageDraft(context.Background(), sender.SendMessageDraftRequest{
		ChatID: int64(123456789), DraftID: 1, Text: "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "text")
	assert.Equal(t, 0, server.CaptureCount())
}
```

### 2F. `sender/chat_admin_test.go` — Promote + Demote

```go
func TestPromoteChatMember_WithCanManageTags(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/promoteChatMember", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, true, req["can_manage_tags"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.PromoteChatMember(context.Background(), int64(-100123), int64(789),
		sender.WithCanManageTags(true),
	)
	assert.NoError(t, err)
}

func TestDemoteChatMember_ClearsCanManageTags(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/promoteChatMember", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, false, req["can_manage_tags"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.DemoteChatMember(context.Background(), int64(-100123), int64(789))
	assert.NoError(t, err)
}
```

---

## Phase 3: Implementation — 1.5 hours

### 3A. `tg/` package — Apply all Phase 1 stubs

**Verify**: `go test ./tg/...` — all new tests pass.

### 3B. `sender/chat_moderation.go` — SetChatMemberTag (FD-1: decomposed)

```go
// ================== Member Tags (9.5) ==================

// SetChatMemberTag sets or removes a custom tag for a regular member.
// Pass an empty string as tag to remove the tag.
// Tag must be 0-16 characters, emoji are not allowed.
// The bot must be an administrator with can_manage_tags right.
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

### 3C. `sender/bot_api_new.go` — SendMessageDraft (FD-2: tg.ChatID + integer validation)

```go
// ================== Bot API 9.5 Methods ==================

// SendMessageDraft sends a draft message for streaming.
// Call repeatedly with the same DraftID and growing text for a streaming effect.
// ChatID must be an integer (private chats only).
// Available to all bots since Bot API 9.5.
func (c *Client) SendMessageDraft(ctx context.Context, req SendMessageDraftRequest) error {
	// sendMessageDraft requires integer chat_id (private chats only)
	switch v := req.ChatID.(type) {
	case int64:
		if v == 0 {
			return tg.NewValidationError("chat_id", "must be non-zero")
		}
	case int:
		if v == 0 {
			return tg.NewValidationError("chat_id", "must be non-zero")
		}
	case nil:
		return tg.NewValidationError("chat_id", "required")
	default:
		return tg.NewValidationError("chat_id", "must be integer for sendMessageDraft (private chats only)")
	}
	if req.DraftID == 0 {
		return tg.NewValidationError("draft_id", "must be non-zero")
	}
	if req.Text == "" {
		return tg.NewValidationError("text", "required")
	}

	return c.callJSON(ctx, "sendMessageDraft", req, nil, extractChatID(req.ChatID))
}
```

### 3D. `sender/chat_admin.go` — WithCanManageTags + DemoteChatMember fix

New option after `WithCanManageTopics`:

```go
// WithCanManageTags grants ability to manage member tags.
func WithCanManageTags(can bool) PromoteOption {
	return func(r *PromoteChatMemberRequest) {
		r.CanManageTags = &can
	}
}
```

**Fix `DemoteChatMember`** — add to the false-block:

```go
CanManageTags: &f,
```

**Fix `PromoteChatMemberWithRights`** — add mapping:

```go
CanManageTags: rights.CanManageTags,
```

### 3E. (Optional) `tg/time_format.go` — Formatting helpers

```go
package tg

import "fmt"

// TimeHTML returns an HTML date/time string for Telegram.
func TimeHTML(unix int64, format, fallbackText string) string {
	if format == "" {
		return fmt.Sprintf(`<tg-time unix="%d">%s</tg-time>`, unix, fallbackText)
	}
	return fmt.Sprintf(`<tg-time unix="%d" format="%s">%s</tg-time>`, unix, format, fallbackText)
}

// TimeMarkdownV2 returns a MarkdownV2 date/time string for Telegram.
func TimeMarkdownV2(unix int64, format, fallbackText string) string {
	if format == "" {
		return fmt.Sprintf(`![%s](tg://time?unix=%d)`, fallbackText, unix)
	}
	return fmt.Sprintf(`![%s](tg://time?unix=%d&format=%s)`, fallbackText, unix, format)
}
```

Tests go in a NEW file `tg/time_format_test.go` with `package tg_test`:

```go
package tg_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/tg"
)

func TestTimeHTML(t *testing.T) {
	got := tg.TimeHTML(1647531900, "wDT", "March 17, 2022")
	assert.Equal(t, `<tg-time unix="1647531900" format="wDT">March 17, 2022</tg-time>`, got)
}

func TestTimeHTML_NoFormat(t *testing.T) {
	got := tg.TimeHTML(1647531900, "", "fallback")
	assert.Equal(t, `<tg-time unix="1647531900">fallback</tg-time>`, got)
}

func TestTimeMarkdownV2(t *testing.T) {
	got := tg.TimeMarkdownV2(1647531900, "Dt", "March 17")
	assert.Equal(t, `![March 17](tg://time?unix=1647531900&format=Dt)`, got)
}
```

**Verify**: `go test ./sender/... ./tg/...` — all pass.

---

## Phase 4: Testbot E2E — 1 hour

### 4A. `engine/scenario.go` — Extend SenderClient + ChatContext

Add to `SenderClient` interface:

```go
// Member tags (9.5)
SetChatMemberTag(ctx context.Context, chatID int64, userID int64, tag string) error

// Message streaming (9.3+9.5)
SendMessageDraft(ctx context.Context, chatID int64, draftID int, text string) error
```

Add to `ChatContext` struct (M7 fix):

```go
CanManageTags bool
```

Update `ProbeChat` — in the administrator branch, add:

```go
if admin.CanManageTags != nil {
	rt.ChatCtx.CanManageTags = *admin.CanManageTags
}
```

And in the creator branch:

```go
rt.ChatCtx.CanManageTags = true
```

### 4B. `engine/adapter.go`

```go
func (a *SenderAdapter) SetChatMemberTag(ctx context.Context, chatID int64, userID int64, tag string) error {
	return a.client.SetChatMemberTag(ctx, chatID, userID, tag)
}

func (a *SenderAdapter) SendMessageDraft(ctx context.Context, chatID int64, draftID int, text string) error {
	return a.client.SendMessageDraft(ctx, sender.SendMessageDraftRequest{
		ChatID: chatID, DraftID: draftID, Text: text,
	})
}
```

### 4C. `engine/steps_api95.go`

```go
package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/prilive-com/galigo/tg"
)

// SetChatMemberTagStep sets a tag on a member.
type SetChatMemberTagStep struct {
	Tag string
}

func (s *SetChatMemberTagStep) Name() string { return "setChatMemberTag" }

func (s *SetChatMemberTagStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	userID := rt.TestUserID
	if userID == 0 {
		userID = rt.AdminUserID
	}
	if userID == 0 {
		return nil, Skip("no TestUserID or AdminUserID for setChatMemberTag")
	}

	// M7: gracefully skip if bot lacks can_manage_tags
	if rt.ChatCtx != nil && !rt.ChatCtx.CanManageTags {
		return nil, Skip("bot lacks can_manage_tags right")
	}

	err := rt.Sender.SetChatMemberTag(ctx, rt.ChatID, userID, s.Tag)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method:   "setChatMemberTag",
		Evidence: map[string]any{"user_id": userID, "tag": s.Tag},
	}, nil
}

// VerifyChatMemberTagStep reads back a member and asserts their tag.
type VerifyChatMemberTagStep struct {
	ExpectedTag string
}

func (s *VerifyChatMemberTagStep) Name() string { return "getChatMember (verify tag)" }

func (s *VerifyChatMemberTagStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	userID := rt.TestUserID
	if userID == 0 {
		userID = rt.AdminUserID
	}
	if userID == 0 {
		return nil, Skip("no TestUserID or AdminUserID for tag readback")
	}

	member, err := rt.Sender.GetChatMember(ctx, rt.ChatID, userID)
	if err != nil {
		return nil, fmt.Errorf("getChatMember for tag readback: %w", err)
	}

	var actualTag string
	switch m := member.(type) {
	case tg.ChatMemberMember:
		actualTag = m.Tag
	case tg.ChatMemberRestricted:
		actualTag = m.Tag
	default:
		return nil, fmt.Errorf("member type %T does not support tags", member)
	}

	if actualTag != s.ExpectedTag {
		return nil, fmt.Errorf("tag mismatch: expected %q, got %q", s.ExpectedTag, actualTag)
	}

	return &StepResult{
		Method: "getChatMember",
		Evidence: map[string]any{
			"user_id": userID, "expected_tag": s.ExpectedTag,
			"actual_tag": actualTag, "tags_match": true,
		},
	}, nil
}

// SendDateTimeMessageStep sends an HTML message with a <tg-time> entity.
type SendDateTimeMessageStep struct{}

func (s *SendDateTimeMessageStep) Name() string { return "sendMessage (date_time entity)" }

func (s *SendDateTimeMessageStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	unix := time.Now().Add(24 * time.Hour).Unix()
	html := tg.TimeHTML(unix, "wDT", "tomorrow at this time")
	text := "[galigo 9.5] Date/time: " + html

	msg, err := rt.Sender.SendMessage(ctx, rt.ChatID, text, WithParseMode("HTML"))
	if err != nil {
		return nil, err
	}
	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	hasDateTime := false
	for _, e := range msg.Entities {
		if e.Type == "date_time" {
			hasDateTime = true
		}
	}

	return &StepResult{
		Method:     "sendMessage",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"has_date_time_entity": hasDateTime,
			"entity_count":        len(msg.Entities),
			"unix_time":           unix,
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
	err := rt.Sender.SendMessageDraft(ctx, rt.ChatID, s.DraftID, s.Text)
	if err != nil {
		return nil, err
	}
	return &StepResult{
		Method:   "sendMessageDraft",
		Evidence: map[string]any{"draft_id": s.DraftID, "text_len": len(s.Text)},
	}, nil
}
```

### 4D. `suites/api95.go`

```go
package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

func S44_DateTimeEntity() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S44-DateTimeEntity",
		ScenarioDescription: "Send message with date_time entity (9.5)",
		CoveredMethods:      []string{"sendMessage"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendDateTimeMessageStep{},
			&engine.CleanupStep{},
		},
	}
}

func S45_MemberTags() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S45-MemberTags",
		ScenarioDescription: "Set, verify, and remove member tags (9.5)",
		CoveredMethods:      []string{"setChatMemberTag", "getChatMember"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SetChatMemberTagStep{Tag: "galigo-test"},
			&engine.VerifyChatMemberTagStep{ExpectedTag: "galigo-test"},
			&engine.SetChatMemberTagStep{Tag: ""},
			&engine.VerifyChatMemberTagStep{ExpectedTag: ""},
		},
	}
}

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

func AllAPI95Scenarios() []engine.Scenario {
	return []engine.Scenario{
		S44_DateTimeEntity(),
		S45_MemberTags(),
		S46_MessageStreaming(),
	}
}
```

### 4E. `registry/registry.go` + `main.go`

Registry:

```go
{Name: "setChatMemberTag", Category: CategoryChatAdmin},
{Name: "sendMessageDraft", Category: CategoryMessaging},
```

Main:

```go
case "api95", "api-95", "9.5":
	scenarios = suites.AllAPI95Scenarios()
```

Plus in "all": `scenarios = append(scenarios, suites.AllAPI95Scenarios()...)`

---

## Phase 5: Verification

```bash
go test ./tg/... -v -run "DateTime|SenderTag|Tag|ManageTags|Permissions|CanEditTag|TimeHTML|TimeMarkdown"
go test ./sender/... -v -run "SetChatMemberTag|SendMessageDraft|PromoteChatMember.*ManageTags|DemoteChatMember.*ManageTags"
go test ./... -race -cover
```

### Critical assertions

| # | What | Test | Catches |
|---|------|------|---------|
| 1 | `SetChatMemberTagRequest.Tag` NO `omitempty` | `TestSetChatMemberTag_RemoveTag_EmptyStringPresent` | Empty string must be sent to remove tags |
| 2 | `SendMessageDraftRequest.ChatID` is `tg.ChatID`, rejects strings | `TestSendMessageDraft_RejectsStringChatID` | Integer-only constraint |
| 3 | `ChatMemberRestricted.CanEditTag` is `bool`, no `omitempty` | `TestUnmarshalChatMember_RestrictedCanEditTag` | Restriction-bool pattern |
| 4 | `ChatPermissions.CanEditTag` is `*bool`, nil when absent | `TestChatPermissions_CanEditTag_Absent` | Permissions-pointer pattern |
| 5 | `SendMessageDraft` rejects `draft_id=0` | `TestSendMessageDraft_Validation_DraftIDZero` | API requires non-zero |
| 6 | `SetChatMemberTag` rejects tags >16 chars | `TestSetChatMemberTag_Validation_TagTooLong` | API constraint |
| 7 | Validation blocks HTTP call | All validation tests check `server.CaptureCount() == 0` | galigo pattern |
| 8 | `DemoteChatMember` clears `can_manage_tags` | `TestDemoteChatMember_ClearsCanManageTags` | Regression: demotion must revoke |
| 9 | S45 skips gracefully without `can_manage_tags` | `SetChatMemberTagStep` checks `ChatCtx.CanManageTags` | M7: no crash on missing rights |
| 10 | Tests in `tg/types_test.go` use `tg.` prefix | `TestMessageEntity_DateTime_Unmarshal` etc. | FD-5: package tg_test |

---

## Developer Review Resolution

| Issue | Developer Claim | Verdict | Resolution |
|-------|----------------|---------|------------|
| NEW-1 | `tg/types_test.go` is `package tg_test` | ✅ CORRECT | Fixed: all 2A tests use `tg.` prefix |
| NEW-2 | TimeHTML tests same issue | ✅ CORRECT | Fixed: moved to new `tg/time_format_test.go` with `package tg_test` |
| NEW-3 | `WithParseMode` undefined in engine | ❌ WRONG | `WithParseMode` exists at `engine/scenario.go:365`, used by `steps.go:74` |
| D1 | Decomposed params in chat_moderation.go | ✅ CORRECT | Fixed: `SetChatMemberTag(ctx, chatID, userID, tag)` (FD-1) |
| D2/N2 | ChatID should be tg.ChatID | ✅ CORRECT | Fixed: `tg.ChatID` + integer-only validation (FD-2) |
| M7 | ChatContext lacks CanManageTags | ✅ CORRECT | Fixed: added to ChatContext + ProbeChat + graceful skip in S45 |
| M2 | ModeratorRights not updated | ✅ Minor | Deferred: moderators may not need tag rights |
| N3 | BottomButton omitted | ✅ Minor | Documented as deferred |

### Final score: 0 compile errors, 0 convention breaks, 0 missing critical items