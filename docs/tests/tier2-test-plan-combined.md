# Tier 2 Test Implementation Plan — Final Version

**Version:** 3.0 (Final)  
**Date:** January 2026  
**Status:** Ready for Implementation  
**Based on:** Combined analysis from three independent reviews + codebase verification

---

## Executive Summary

This plan addresses testing for all 46 Tier 2 methods. After verifying the actual codebase state:

**Key Finding:** The developer's claim that "~60% of tests already exist" is **incorrect**.

### Verified Codebase State

| Location | Claimed by Developer | Actually Exists |
|----------|---------------------|-----------------|
| `sender/chat_info_test.go` | ✓ exists | ❌ **Does not exist** |
| `sender/chat_moderation_test.go` | ✓ exists | ❌ **Does not exist** |
| `sender/chat_admin_test.go` | ✓ exists | ❌ **Does not exist** |
| `sender/chat_settings_test.go` | ✓ exists | ❌ **Does not exist** |
| `sender/chat_pin_test.go` | ✓ exists | ❌ **Does not exist** |
| `sender/polls_test.go` | ✓ exists | ❌ **Does not exist** |
| `sender/forum_test.go` | ✓ exists | ❌ **Does not exist** |
| `tg/chat_member_test.go` | ✓ exists | ❌ **Does not exist** |
| `registry/CategoryTier2` | ✓ exists | ❌ **Does not exist** |
| `suites/tier2*.go` | ✓ exists | ❌ **Does not exist** |

**Existing test files in `sender/`:**
- `methods_test.go` (55 tests — Tier 1 only)
- `retry_test.go`, `breaker_test.go`, `ratelimit_test.go`
- `multipart_test.go`, `inputfile_test.go`, `edit_media_test.go`
- `options_test.go`, `executor_test.go`

**Conclusion:** Nearly all Tier 2 unit tests and integration tests need to be written.

---

## Corrections from Review Process

### ✅ Accepted Corrections

| Issue | Resolution |
|-------|------------|
| Don't export validators | Test validation through public API methods |
| Fix bulk test signatures | Use actual `ForwardMessagesRequest{}` struct signatures |
| Use simple cleanup design | `[]func()` cleanup stack, not state machine |
| Gate dangerous scenarios | Implement `--allow-dangerous` flag |

### ❌ Rejected Developer Claim

| Issue | Developer Said | Actual Situation |
|-------|----------------|------------------|
| `messageThreadID=0` is valid | Don't validate | **Wrong.** Telegram docs mark it as required unique identifier. Add validation. |
| Tests already exist | Skip most work | **Wrong.** Verified: test files don't exist. |

### 🔧 Implementation Gaps Identified

| Gap | Action Required |
|-----|-----------------|
| `editForumTopic` missing validation | Add `messageThreadID > 0` check |
| Forum topic methods missing validation | Add `messageThreadID > 0` for all topic-scoped methods |

---

## Part 1: Unit Test Implementation

### 1.1 Test Categories Per Method

For each Tier 2 method, implement three test categories:

#### Category A: Success (Request/Response)
```go
func Test<Method>_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/<method>", func(w http.ResponseWriter, r *http.Request) {
        testutil.Reply<Type>(w, ...)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    result, err := client.<Method>(context.Background(), ...)

    require.NoError(t, err)
    assert.Equal(t, expected, result)

    // Verify request encoding
    cap := server.LastCapture()
    cap.AssertJSONField(t, "required_field", expectedValue)
    cap.AssertJSONFieldAbsent(t, "optional_unset_field")
}
```

#### Category B: Validation (No HTTP Call)
```go
func Test<Method>_Validation_<Case>(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    _, err := client.<Method>(context.Background(), invalidInput)

    require.Error(t, err)
    assert.Contains(t, err.Error(), "expected message")
    
    // CRITICAL: Prove no HTTP request was made
    assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}
```

#### Category C: Error Mapping
```go
func Test<Method>_Error_<Scenario>(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/<method>", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBadRequest(w, "error description")
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    _, err := client.<Method>(context.Background(), ...)

    require.Error(t, err)
    var apiErr *tg.APIError
    require.True(t, errors.As(err, &apiErr))
    assert.Equal(t, 400, apiErr.Code)
}
```

### 1.2 Additional Tests Per Bucket (Not Per Method)

One retry test and one breaker test per bucket to avoid test explosion.

---

### 1.3 New Test Files to Create

#### `sender/chat_info_test.go`

```go
package sender_test

import (
    "context"
    "errors"
    "net/http"
    "sync/atomic"
    "testing"
    "time"

    "github.com/prilive-com/galigo/internal/testutil"
    "github.com/prilive-com/galigo/sender"
    "github.com/prilive-com/galigo/tg"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// ==================== GetChat ====================

func TestGetChat_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/getChat", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyOK(w, map[string]any{
            "id":          int64(-1001234567890),
            "type":        "supergroup",
            "title":       "Test Group",
            "username":    "testgroup",
            "description": "A test group",
        })
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    chat, err := client.GetChat(context.Background(), int64(-1001234567890))

    require.NoError(t, err)
    assert.Equal(t, int64(-1001234567890), chat.ID)
    assert.Equal(t, "supergroup", chat.Type)
    assert.Equal(t, "Test Group", chat.Title)

    cap := server.LastCapture()
    cap.AssertJSONField(t, "chat_id", float64(-1001234567890))
}

func TestGetChat_WithUsername(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/getChat", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyOK(w, map[string]any{
            "id":    int64(-100123),
            "type":  "channel",
            "title": "Channel",
        })
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    chat, err := client.GetChat(context.Background(), "@testchannel")

    require.NoError(t, err)
    assert.Equal(t, "channel", chat.Type)

    cap := server.LastCapture()
    cap.AssertJSONField(t, "chat_id", "@testchannel")
}

func TestGetChat_Validation_NilChatID(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    _, err := client.GetChat(context.Background(), nil)

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestGetChat_Validation_ZeroChatID(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    _, err := client.GetChat(context.Background(), int64(0))

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestGetChat_Validation_EmptyUsername(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    _, err := client.GetChat(context.Background(), "")

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestGetChat_Error_NotFound(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/getChat", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBadRequest(w, "chat not found")
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    _, err := client.GetChat(context.Background(), int64(999999))

    require.Error(t, err)
    var apiErr *tg.APIError
    require.True(t, errors.As(err, &apiErr))
    assert.Equal(t, 400, apiErr.Code)
}

// ==================== GetChatAdministrators ====================

func TestGetChatAdministrators_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/getChatAdministrators", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyOK(w, []map[string]any{
            {"status": "creator", "user": map[string]any{"id": 123, "first_name": "Owner", "is_bot": false}},
            {"status": "administrator", "user": map[string]any{"id": 456, "first_name": "Admin", "is_bot": false}, "can_delete_messages": true},
        })
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    admins, err := client.GetChatAdministrators(context.Background(), int64(-100123))

    require.NoError(t, err)
    require.Len(t, admins, 2)
}

func TestGetChatAdministrators_Validation_InvalidChatID(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    _, err := client.GetChatAdministrators(context.Background(), int64(0))

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount())
}

// ==================== GetChatMemberCount ====================

func TestGetChatMemberCount_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/getChatMemberCount", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyOK(w, 42)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    count, err := client.GetChatMemberCount(context.Background(), int64(-100123))

    require.NoError(t, err)
    assert.Equal(t, 42, count)
}

// ==================== GetChatMember ====================

func TestGetChatMember_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/getChatMember", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyOK(w, map[string]any{
            "status": "member",
            "user":   map[string]any{"id": 789, "first_name": "Regular", "is_bot": false},
        })
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    member, err := client.GetChatMember(context.Background(), int64(-100123), 789)

    require.NoError(t, err)
    assert.Equal(t, "member", member.Status())

    cap := server.LastCapture()
    cap.AssertJSONField(t, "chat_id", float64(-100123))
    cap.AssertJSONField(t, "user_id", float64(789))
}

func TestGetChatMember_Validation_InvalidUserID(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    _, err := client.GetChatMember(context.Background(), int64(-100123), 0)

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount())
}

// ==================== Bucket: Retry Test ====================

func TestChatInfo_Retry_RateLimit(t *testing.T) {
    var attempts atomic.Int32

    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/getChat", func(w http.ResponseWriter, r *http.Request) {
        if attempts.Add(1) == 1 {
            testutil.ReplyRateLimit(w, 2)
            return
        }
        testutil.ReplyOK(w, map[string]any{"id": int64(-100123), "type": "supergroup"})
    })

    sleeper := &testutil.FakeSleeper{}
    client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRetries(3))

    _, err := client.GetChat(context.Background(), int64(-100123))

    require.NoError(t, err)
    assert.Equal(t, int32(2), attempts.Load())
    assert.Equal(t, 2*time.Second, sleeper.LastCall())
}
```

#### `sender/chat_moderation_test.go`

```go
package sender_test

import (
    "context"
    "errors"
    "net/http"
    "testing"
    "time"

    "github.com/prilive-com/galigo/internal/testutil"
    "github.com/prilive-com/galigo/sender"
    "github.com/prilive-com/galigo/tg"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// ==================== BanChatMember ====================

func TestBanChatMember_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/banChatMember", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.BanChatMember(context.Background(), sender.BanChatMemberRequest{
        ChatID: int64(-100123),
        UserID: 456,
    })

    require.NoError(t, err)

    cap := server.LastCapture()
    cap.AssertJSONField(t, "chat_id", float64(-100123))
    cap.AssertJSONField(t, "user_id", float64(456))
    cap.AssertJSONFieldAbsent(t, "until_date")
    cap.AssertJSONFieldAbsent(t, "revoke_messages")
}

func TestBanChatMember_WithOptions(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/banChatMember", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    untilDate := time.Now().Add(24 * time.Hour).Unix()
    err := client.BanChatMember(context.Background(), sender.BanChatMemberRequest{
        ChatID:         int64(-100123),
        UserID:         456,
        UntilDate:      untilDate,
        RevokeMessages: true,
    })

    require.NoError(t, err)

    cap := server.LastCapture()
    cap.AssertJSONFieldExists(t, "until_date")
    cap.AssertJSONField(t, "revoke_messages", true)
}

func TestBanChatMember_Validation_InvalidChatID(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    err := client.BanChatMember(context.Background(), sender.BanChatMemberRequest{
        ChatID: int64(0),
        UserID: 456,
    })

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount())
}

func TestBanChatMember_Validation_InvalidUserID(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    err := client.BanChatMember(context.Background(), sender.BanChatMemberRequest{
        ChatID: int64(-100123),
        UserID: 0,
    })

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount())
}

func TestBanChatMember_Error_Forbidden(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/banChatMember", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyForbidden(w, "not enough rights to ban")
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.BanChatMember(context.Background(), sender.BanChatMemberRequest{
        ChatID: int64(-100123),
        UserID: 456,
    })

    require.Error(t, err)
    var apiErr *tg.APIError
    require.True(t, errors.As(err, &apiErr))
    assert.Equal(t, 403, apiErr.Code)
}

// ==================== UnbanChatMember ====================

func TestUnbanChatMember_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/unbanChatMember", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.UnbanChatMember(context.Background(), sender.UnbanChatMemberRequest{
        ChatID: int64(-100123),
        UserID: 456,
    })

    require.NoError(t, err)
}

func TestUnbanChatMember_WithOnlyIfBanned(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/unbanChatMember", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.UnbanChatMember(context.Background(), sender.UnbanChatMemberRequest{
        ChatID:       int64(-100123),
        UserID:       456,
        OnlyIfBanned: true,
    })

    require.NoError(t, err)

    cap := server.LastCapture()
    cap.AssertJSONField(t, "only_if_banned", true)
}

// ==================== RestrictChatMember ====================

func TestRestrictChatMember_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/restrictChatMember", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.RestrictChatMember(context.Background(), sender.RestrictChatMemberRequest{
        ChatID:      int64(-100123),
        UserID:      456,
        Permissions: tg.ChatPermissions{},
    })

    require.NoError(t, err)

    cap := server.LastCapture()
    cap.AssertJSONFieldExists(t, "permissions")
}

// ==================== BanChatSenderChat ====================

func TestBanChatSenderChat_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/banChatSenderChat", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.BanChatSenderChat(context.Background(), sender.BanChatSenderChatRequest{
        ChatID:       int64(-100123),
        SenderChatID: int64(-100456),
    })

    require.NoError(t, err)

    cap := server.LastCapture()
    cap.AssertJSONField(t, "chat_id", float64(-100123))
    cap.AssertJSONField(t, "sender_chat_id", float64(-100456))
}

// ==================== UnbanChatSenderChat ====================

func TestUnbanChatSenderChat_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/unbanChatSenderChat", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.UnbanChatSenderChat(context.Background(), sender.UnbanChatSenderChatRequest{
        ChatID:       int64(-100123),
        SenderChatID: int64(-100456),
    })

    require.NoError(t, err)
}
```

#### `sender/chat_admin_test.go`

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

// ==================== PromoteChatMember ====================

func TestPromoteChatMember_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/promoteChatMember", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.PromoteChatMember(context.Background(), sender.PromoteChatMemberRequest{
        ChatID:            int64(-100123),
        UserID:            456,
        CanDeleteMessages: true,
        CanRestrictMembers: true,
    })

    require.NoError(t, err)

    cap := server.LastCapture()
    cap.AssertJSONField(t, "chat_id", float64(-100123))
    cap.AssertJSONField(t, "user_id", float64(456))
    cap.AssertJSONField(t, "can_delete_messages", true)
    cap.AssertJSONField(t, "can_restrict_members", true)
}

func TestPromoteChatMember_Validation_InvalidUserID(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    err := client.PromoteChatMember(context.Background(), sender.PromoteChatMemberRequest{
        ChatID: int64(-100123),
        UserID: 0,
    })

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount())
}

// ==================== SetChatAdministratorCustomTitle ====================

func TestSetChatAdministratorCustomTitle_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/setChatAdministratorCustomTitle", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.SetChatAdministratorCustomTitle(context.Background(), sender.SetChatAdministratorCustomTitleRequest{
        ChatID:      int64(-100123),
        UserID:      456,
        CustomTitle: "Moderator",
    })

    require.NoError(t, err)

    cap := server.LastCapture()
    cap.AssertJSONField(t, "custom_title", "Moderator")
}

func TestSetChatAdministratorCustomTitle_Validation_TitleTooLong(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    err := client.SetChatAdministratorCustomTitle(context.Background(), sender.SetChatAdministratorCustomTitleRequest{
        ChatID:      int64(-100123),
        UserID:      456,
        CustomTitle: "This title is way too long for Telegram API limits",
    })

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount())
}
```

#### `sender/chat_settings_test.go`

```go
package sender_test

import (
    "context"
    "net/http"
    "testing"

    "github.com/prilive-com/galigo/internal/testutil"
    "github.com/prilive-com/galigo/sender"
    "github.com/prilive-com/galigo/tg"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// ==================== SetChatPermissions ====================

func TestSetChatPermissions_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/setChatPermissions", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.SetChatPermissions(context.Background(), sender.SetChatPermissionsRequest{
        ChatID:      int64(-100123),
        Permissions: tg.ChatPermissions{},
    })

    require.NoError(t, err)

    cap := server.LastCapture()
    cap.AssertJSONFieldExists(t, "permissions")
}

// ==================== SetChatTitle ====================

func TestSetChatTitle_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/setChatTitle", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.SetChatTitle(context.Background(), sender.SetChatTitleRequest{
        ChatID: int64(-100123),
        Title:  "New Title",
    })

    require.NoError(t, err)

    cap := server.LastCapture()
    cap.AssertJSONField(t, "title", "New Title")
}

func TestSetChatTitle_Validation_EmptyTitle(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    err := client.SetChatTitle(context.Background(), sender.SetChatTitleRequest{
        ChatID: int64(-100123),
        Title:  "",
    })

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount())
}

func TestSetChatTitle_Validation_TitleTooLong(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    longTitle := string(make([]byte, 200)) // Max is 128

    err := client.SetChatTitle(context.Background(), sender.SetChatTitleRequest{
        ChatID: int64(-100123),
        Title:  longTitle,
    })

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount())
}

// ==================== SetChatDescription ====================

func TestSetChatDescription_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/setChatDescription", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.SetChatDescription(context.Background(), sender.SetChatDescriptionRequest{
        ChatID:      int64(-100123),
        Description: "New description",
    })

    require.NoError(t, err)
}

func TestSetChatDescription_EmptyAllowed(t *testing.T) {
    // Empty description is valid (removes description)
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/setChatDescription", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.SetChatDescription(context.Background(), sender.SetChatDescriptionRequest{
        ChatID:      int64(-100123),
        Description: "",
    })

    require.NoError(t, err)
}

// ==================== DeleteChatPhoto ====================

func TestDeleteChatPhoto_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/deleteChatPhoto", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.DeleteChatPhoto(context.Background(), sender.DeleteChatPhotoRequest{
        ChatID: int64(-100123),
    })

    require.NoError(t, err)
}
```

#### `sender/chat_pin_test.go`

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

// ==================== PinChatMessage ====================

func TestPinChatMessage_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/pinChatMessage", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.PinChatMessage(context.Background(), sender.PinChatMessageRequest{
        ChatID:    int64(-100123),
        MessageID: 456,
    })

    require.NoError(t, err)

    cap := server.LastCapture()
    cap.AssertJSONField(t, "chat_id", float64(-100123))
    cap.AssertJSONField(t, "message_id", float64(456))
}

func TestPinChatMessage_Silent(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/pinChatMessage", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.PinChatMessage(context.Background(), sender.PinChatMessageRequest{
        ChatID:              int64(-100123),
        MessageID:           456,
        DisableNotification: true,
    })

    require.NoError(t, err)

    cap := server.LastCapture()
    cap.AssertJSONField(t, "disable_notification", true)
}

func TestPinChatMessage_Validation_InvalidMessageID(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    err := client.PinChatMessage(context.Background(), sender.PinChatMessageRequest{
        ChatID:    int64(-100123),
        MessageID: 0,
    })

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount())
}

// ==================== UnpinChatMessage ====================

func TestUnpinChatMessage_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/unpinChatMessage", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.UnpinChatMessage(context.Background(), sender.UnpinChatMessageRequest{
        ChatID:    int64(-100123),
        MessageID: 456,
    })

    require.NoError(t, err)
}

func TestUnpinChatMessage_MostRecent(t *testing.T) {
    // MessageID=0 means unpin most recent
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/unpinChatMessage", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.UnpinChatMessage(context.Background(), sender.UnpinChatMessageRequest{
        ChatID:    int64(-100123),
        MessageID: 0, // Omit message_id to unpin most recent
    })

    require.NoError(t, err)

    cap := server.LastCapture()
    // Should not include message_id when 0
    cap.AssertJSONFieldAbsent(t, "message_id")
}

// ==================== UnpinAllChatMessages ====================

func TestUnpinAllChatMessages_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/unpinAllChatMessages", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.UnpinAllChatMessages(context.Background(), sender.UnpinAllChatMessagesRequest{
        ChatID: int64(-100123),
    })

    require.NoError(t, err)
}

// ==================== LeaveChat ====================

func TestLeaveChat_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/leaveChat", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.LeaveChat(context.Background(), sender.LeaveChatRequest{
        ChatID: int64(-100123),
    })

    require.NoError(t, err)
}
```

#### `sender/polls_test.go`

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

// ==================== SendPoll ====================

func TestSendPoll_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/sendPoll", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyMessage(w, 123)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    msg, err := client.SendPoll(context.Background(), sender.SendPollRequest{
        ChatID:   testutil.TestChatID,
        Question: "What's your favorite color?",
        Options:  []string{"Red", "Blue", "Green"},
    })

    require.NoError(t, err)
    assert.Equal(t, 123, msg.MessageID)

    cap := server.LastCapture()
    cap.AssertJSONField(t, "question", "What's your favorite color?")
}

func TestSendPoll_Validation_TooFewOptions(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    _, err := client.SendPoll(context.Background(), sender.SendPollRequest{
        ChatID:   testutil.TestChatID,
        Question: "Question?",
        Options:  []string{"Only one"},
    })

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount())
}

func TestSendPoll_Validation_TooManyOptions(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    options := make([]string, 11) // Max is 10
    for i := range options {
        options[i] = "Option"
    }

    _, err := client.SendPoll(context.Background(), sender.SendPollRequest{
        ChatID:   testutil.TestChatID,
        Question: "Question?",
        Options:  options,
    })

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount())
}

func TestSendPoll_Validation_EmptyQuestion(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    _, err := client.SendPoll(context.Background(), sender.SendPollRequest{
        ChatID:   testutil.TestChatID,
        Question: "",
        Options:  []string{"A", "B"},
    })

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount())
}

// ==================== SendPoll Quiz Mode ====================

func TestSendPoll_Quiz_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/sendPoll", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyMessage(w, 124)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    correctID := 1
    msg, err := client.SendPoll(context.Background(), sender.SendPollRequest{
        ChatID:          testutil.TestChatID,
        Question:        "What is 2+2?",
        Options:         []string{"3", "4", "5"},
        Type:            "quiz",
        CorrectOptionID: &correctID,
    })

    require.NoError(t, err)
    assert.Equal(t, 124, msg.MessageID)

    cap := server.LastCapture()
    cap.AssertJSONField(t, "type", "quiz")
    cap.AssertJSONField(t, "correct_option_id", float64(1))
}

func TestSendPoll_Quiz_Validation_InvalidCorrectOption(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    correctID := 5 // Invalid: only 3 options
    _, err := client.SendPoll(context.Background(), sender.SendPollRequest{
        ChatID:          testutil.TestChatID,
        Question:        "Question?",
        Options:         []string{"A", "B", "C"},
        Type:            "quiz",
        CorrectOptionID: &correctID,
    })

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount())
}

// ==================== StopPoll ====================

func TestStopPoll_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/stopPoll", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyOK(w, map[string]any{
            "id":                "poll_123",
            "question":          "Stopped poll",
            "is_closed":         true,
            "total_voter_count": 10,
        })
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    poll, err := client.StopPoll(context.Background(), sender.StopPollRequest{
        ChatID:    testutil.TestChatID,
        MessageID: 456,
    })

    require.NoError(t, err)
    assert.True(t, poll.IsClosed)

    cap := server.LastCapture()
    cap.AssertJSONField(t, "message_id", float64(456))
}
```

#### `sender/forum_test.go`

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

// ==================== CreateForumTopic ====================

func TestCreateForumTopic_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/createForumTopic", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyOK(w, map[string]any{
            "message_thread_id": 123,
            "name":              "Test Topic",
            "icon_color":        7322096,
        })
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    topic, err := client.CreateForumTopic(context.Background(), sender.CreateForumTopicRequest{
        ChatID:    int64(-100123),
        Name:      "Test Topic",
        IconColor: 7322096,
    })

    require.NoError(t, err)
    assert.Equal(t, 123, topic.MessageThreadID)
    assert.Equal(t, "Test Topic", topic.Name)

    cap := server.LastCapture()
    cap.AssertJSONField(t, "name", "Test Topic")
}

func TestCreateForumTopic_Validation_EmptyName(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    _, err := client.CreateForumTopic(context.Background(), sender.CreateForumTopicRequest{
        ChatID: int64(-100123),
        Name:   "",
    })

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount())
}

func TestCreateForumTopic_Validation_NameTooLong(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    longName := string(make([]byte, 200)) // Max is 128

    _, err := client.CreateForumTopic(context.Background(), sender.CreateForumTopicRequest{
        ChatID: int64(-100123),
        Name:   longName,
    })

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount())
}

// ==================== EditForumTopic ====================

func TestEditForumTopic_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/editForumTopic", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.EditForumTopic(context.Background(), sender.EditForumTopicRequest{
        ChatID:          int64(-100123),
        MessageThreadID: 456,
        Name:            "New Name",
    })

    require.NoError(t, err)

    cap := server.LastCapture()
    cap.AssertJSONField(t, "message_thread_id", float64(456))
    cap.AssertJSONField(t, "name", "New Name")
}

// CRITICAL: This test validates the implementation gap identified in review
func TestEditForumTopic_Validation_InvalidThreadID(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    err := client.EditForumTopic(context.Background(), sender.EditForumTopicRequest{
        ChatID:          int64(-100123),
        MessageThreadID: 0, // Invalid: must be positive
        Name:            "Test",
    })

    require.Error(t, err)
    assert.Contains(t, err.Error(), "message_thread_id")
    assert.Equal(t, 0, server.CaptureCount())
}

// ==================== CloseForumTopic ====================

func TestCloseForumTopic_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/closeForumTopic", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.CloseForumTopic(context.Background(), sender.CloseForumTopicRequest{
        ChatID:          int64(-100123),
        MessageThreadID: 456,
    })

    require.NoError(t, err)
}

func TestCloseForumTopic_Validation_InvalidThreadID(t *testing.T) {
    server := testutil.NewMockServer(t)
    client := testutil.NewTestClient(t, server.BaseURL())

    err := client.CloseForumTopic(context.Background(), sender.CloseForumTopicRequest{
        ChatID:          int64(-100123),
        MessageThreadID: 0,
    })

    require.Error(t, err)
    assert.Equal(t, 0, server.CaptureCount())
}

// ==================== ReopenForumTopic ====================

func TestReopenForumTopic_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/reopenForumTopic", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.ReopenForumTopic(context.Background(), sender.ReopenForumTopicRequest{
        ChatID:          int64(-100123),
        MessageThreadID: 456,
    })

    require.NoError(t, err)
}

// ==================== DeleteForumTopic ====================

func TestDeleteForumTopic_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/deleteForumTopic", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.DeleteForumTopic(context.Background(), sender.DeleteForumTopicRequest{
        ChatID:          int64(-100123),
        MessageThreadID: 456,
    })

    require.NoError(t, err)
}

// ==================== General Forum Topic Methods ====================

func TestEditGeneralForumTopic_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/editGeneralForumTopic", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.EditGeneralForumTopic(context.Background(), sender.EditGeneralForumTopicRequest{
        ChatID: int64(-100123),
        Name:   "General",
    })

    require.NoError(t, err)
}

func TestCloseGeneralForumTopic_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/closeGeneralForumTopic", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.CloseGeneralForumTopic(context.Background(), sender.CloseGeneralForumTopicRequest{
        ChatID: int64(-100123),
    })

    require.NoError(t, err)
}

func TestReopenGeneralForumTopic_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/reopenGeneralForumTopic", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.ReopenGeneralForumTopic(context.Background(), sender.ReopenGeneralForumTopicRequest{
        ChatID: int64(-100123),
    })

    require.NoError(t, err)
}

func TestHideGeneralForumTopic_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/hideGeneralForumTopic", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.HideGeneralForumTopic(context.Background(), sender.HideGeneralForumTopicRequest{
        ChatID: int64(-100123),
    })

    require.NoError(t, err)
}

func TestUnhideGeneralForumTopic_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/unhideGeneralForumTopic", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyBool(w, true)
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.UnhideGeneralForumTopic(context.Background(), sender.UnhideGeneralForumTopicRequest{
        ChatID: int64(-100123),
    })

    require.NoError(t, err)
}

// ==================== GetForumTopicIconStickers ====================

func TestGetForumTopicIconStickers_Success(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("/bot"+testutil.TestToken+"/getForumTopicIconStickers", func(w http.ResponseWriter, r *http.Request) {
        testutil.ReplyOK(w, []map[string]any{
            {"file_id": "sticker1", "type": "custom_emoji"},
            {"file_id": "sticker2", "type": "custom_emoji"},
        })
    })

    client := testutil.NewTestClient(t, server.BaseURL())
    stickers, err := client.GetForumTopicIconStickers(context.Background())

    require.NoError(t, err)
    assert.Len(t, stickers, 2)
}
```

#### `tg/chat_member_test.go`

```go
package tg_test

import (
    "testing"

    "github.com/prilive-com/galigo/tg"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestUnmarshalChatMember_AllVariants(t *testing.T) {
    variants := []struct {
        name   string
        json   string
        status string
    }{
        {
            name:   "creator",
            json:   `{"status":"creator","user":{"id":123,"first_name":"Owner","is_bot":false},"is_anonymous":false}`,
            status: "creator",
        },
        {
            name:   "administrator",
            json:   `{"status":"administrator","user":{"id":456,"first_name":"Admin","is_bot":false},"can_delete_messages":true}`,
            status: "administrator",
        },
        {
            name:   "member",
            json:   `{"status":"member","user":{"id":789,"first_name":"Regular","is_bot":false}}`,
            status: "member",
        },
        {
            name:   "restricted",
            json:   `{"status":"restricted","user":{"id":111,"first_name":"Muted","is_bot":false},"is_member":true,"can_send_messages":false}`,
            status: "restricted",
        },
        {
            name:   "left",
            json:   `{"status":"left","user":{"id":222,"first_name":"Gone","is_bot":false}}`,
            status: "left",
        },
        {
            name:   "kicked",
            json:   `{"status":"kicked","user":{"id":333,"first_name":"Banned","is_bot":false},"until_date":0}`,
            status: "kicked",
        },
    }

    for _, tt := range variants {
        t.Run(tt.name, func(t *testing.T) {
            member, err := tg.UnmarshalChatMember([]byte(tt.json))

            require.NoError(t, err)
            assert.Equal(t, tt.status, member.Status())
            assert.NotNil(t, member.GetUser())
        })
    }
}

func TestUnmarshalChatMember_UnknownStatus(t *testing.T) {
    data := []byte(`{"status":"future_status","user":{"id":1,"first_name":"X","is_bot":false}}`)

    _, err := tg.UnmarshalChatMember(data)

    require.Error(t, err)
    assert.Contains(t, err.Error(), "unknown")
}

func TestUnmarshalChatMember_InvalidJSON(t *testing.T) {
    _, err := tg.UnmarshalChatMember([]byte(`not json`))
    require.Error(t, err)
}

func TestChatMemberHelpers(t *testing.T) {
    tests := []struct {
        status     string
        isOwner    bool
        isAdmin    bool
        isMember   bool
        isBanned   bool
    }{
        {"creator", true, true, false, false},
        {"administrator", false, true, false, false},
        {"member", false, false, true, false},
        {"restricted", false, false, false, false},
        {"kicked", false, false, false, true},
        {"left", false, false, false, false},
    }

    for _, tt := range tests {
        t.Run(tt.status, func(t *testing.T) {
            data := []byte(`{"status":"` + tt.status + `","user":{"id":1,"first_name":"Test","is_bot":false}}`)
            member, err := tg.UnmarshalChatMember(data)
            require.NoError(t, err)

            assert.Equal(t, tt.isOwner, tg.IsOwner(member))
            assert.Equal(t, tt.isAdmin, tg.IsAdmin(member))
            assert.Equal(t, tt.isMember, tg.IsMember(member))
            assert.Equal(t, tt.isBanned, tg.IsBanned(member))
        })
    }
}
```

---

## Part 2: Implementation Gap Fix

### Add `messageThreadID > 0` Validation

The third consultant correctly identified that `editForumTopic` and other forum topic methods require `messageThreadID` to be a positive integer (it's a required unique identifier per Telegram docs).

**Implementation:** Add validation to all topic-scoped forum methods:

```go
// In sender/forum.go (or wherever forum methods are implemented)

func (c *Client) EditForumTopic(ctx context.Context, req EditForumTopicRequest) error {
    if err := validateChatID(req.ChatID); err != nil {
        return err
    }
    if req.MessageThreadID <= 0 {
        return &ValidationError{Field: "message_thread_id", Message: "must be positive"}
    }
    // ... rest of implementation
}
```

**Methods requiring this validation:**
- `editForumTopic`
- `closeForumTopic`
- `reopenForumTopic`
- `deleteForumTopic`
- `unpinAllForumTopicMessages`

---

## Part 3: Integration Tests (galigo-testbot)

### 3.1 Configuration Additions

Add to `cmd/galigo-testbot/config/config.go`:

```go
type Config struct {
    // Existing fields...
    
    // Tier 2: Optional chat IDs for specific scenarios
    AdminChatID    int64  `env:"TESTBOT_ADMIN_CHAT_ID"`    // Supergroup where bot is admin
    ForumChatID    int64  `env:"TESTBOT_FORUM_CHAT_ID"`    // Forum-enabled supergroup
    TargetUserID   int64  `env:"TESTBOT_TARGET_USER_ID"`   // User for moderation tests
    AllowDangerous bool   `env:"TESTBOT_ALLOW_DANGEROUS"`  // Enable ban/promote tests
}

func (c *Config) HasAdminChat() bool { return c.AdminChatID != 0 }
func (c *Config) HasForumChat() bool { return c.ForumChatID != 0 }
func (c *Config) HasTargetUser() bool { return c.TargetUserID != 0 }
```

### 3.2 Registry Updates

Add to `cmd/galigo-testbot/registry/registry.go`:

```go
const (
    CategoryTier1  MethodCategory = "tier1"
    CategoryTier2  MethodCategory = "tier2"  // NEW
    CategoryLegacy MethodCategory = "legacy"
)

// Add Tier 2 methods to AllMethods slice:
var AllMethods = []Method{
    // ... existing Tier 1 methods ...

    // === Tier 2: Chat Information ===
    {Name: "getChat", Category: CategoryTier2},
    {Name: "getChatAdministrators", Category: CategoryTier2},
    {Name: "getChatMemberCount", Category: CategoryTier2},
    {Name: "getChatMember", Category: CategoryTier2},

    // === Tier 2: Chat Moderation ===
    {Name: "banChatMember", Category: CategoryTier2, Notes: "destructive"},
    {Name: "unbanChatMember", Category: CategoryTier2},
    {Name: "restrictChatMember", Category: CategoryTier2},
    {Name: "banChatSenderChat", Category: CategoryTier2},
    {Name: "unbanChatSenderChat", Category: CategoryTier2},

    // === Tier 2: Chat Admin ===
    {Name: "promoteChatMember", Category: CategoryTier2, Notes: "risky"},
    {Name: "setChatAdministratorCustomTitle", Category: CategoryTier2},

    // === Tier 2: Chat Settings ===
    {Name: "setChatPermissions", Category: CategoryTier2},
    {Name: "setChatPhoto", Category: CategoryTier2},
    {Name: "deleteChatPhoto", Category: CategoryTier2},
    {Name: "setChatTitle", Category: CategoryTier2},
    {Name: "setChatDescription", Category: CategoryTier2},

    // === Tier 2: Pin/Leave ===
    {Name: "pinChatMessage", Category: CategoryTier2},
    {Name: "unpinChatMessage", Category: CategoryTier2},
    {Name: "unpinAllChatMessages", Category: CategoryTier2},
    {Name: "leaveChat", Category: CategoryTier2, Notes: "destructive"},

    // === Tier 2: Bulk Operations ===
    {Name: "forwardMessages", Category: CategoryTier2},
    {Name: "copyMessages", Category: CategoryTier2},

    // === Tier 2: Polls ===
    {Name: "sendPoll", Category: CategoryTier2},
    {Name: "stopPoll", Category: CategoryTier2},

    // === Tier 2: Forum Topics ===
    {Name: "createForumTopic", Category: CategoryTier2, Notes: "requires forum"},
    {Name: "editForumTopic", Category: CategoryTier2},
    {Name: "closeForumTopic", Category: CategoryTier2},
    {Name: "reopenForumTopic", Category: CategoryTier2},
    {Name: "deleteForumTopic", Category: CategoryTier2},
    {Name: "unpinAllForumTopicMessages", Category: CategoryTier2},
    {Name: "editGeneralForumTopic", Category: CategoryTier2},
    {Name: "closeGeneralForumTopic", Category: CategoryTier2},
    {Name: "reopenGeneralForumTopic", Category: CategoryTier2},
    {Name: "hideGeneralForumTopic", Category: CategoryTier2},
    {Name: "unhideGeneralForumTopic", Category: CategoryTier2},
    {Name: "unpinAllGeneralForumTopicMessages", Category: CategoryTier2},
    {Name: "getForumTopicIconStickers", Category: CategoryTier2},

    // ... existing Legacy methods ...
}

func Tier2Methods() []Method {
    var methods []Method
    for _, m := range AllMethods {
        if m.Category == CategoryTier2 {
            methods = append(methods, m)
        }
    }
    return methods
}
```

### 3.3 Runtime Cleanup (Simple Design)

Use `[]func()` cleanup stack instead of state machine:

```go
// In engine/scenario.go

type Runtime struct {
    // Existing fields...
    
    // Simple cleanup stack
    cleanupFuncs []func()
}

func (rt *Runtime) AddCleanup(fn func()) {
    rt.cleanupFuncs = append(rt.cleanupFuncs, fn)
}

func (rt *Runtime) RunCleanup() {
    // Run in reverse order (LIFO)
    for i := len(rt.cleanupFuncs) - 1; i >= 0; i-- {
        rt.cleanupFuncs[i]()
    }
    rt.cleanupFuncs = nil
}
```

### 3.4 New Suite File: `suites/tier2.go`

```go
package suites

import (
    "time"

    "github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// ==================== Safe Scenarios (included in --run all) ====================

func S15_ChatInfo() engine.Scenario {
    return &engine.BaseScenario{
        ScenarioName:        "S15-ChatInfo",
        ScenarioDescription: "Get chat info, admins, member count (read-only)",
        CoveredMethods:      []string{"getChat", "getChatAdministrators", "getChatMemberCount", "getChatMember"},
        ScenarioTimeout:     1 * time.Minute,
        ScenarioSteps: []engine.Step{
            &engine.GetChatStep{},
            &engine.GetChatAdministratorsStep{},
            &engine.GetChatMemberCountStep{},
            &engine.GetChatMemberStep{},
        },
    }
}

func S17_BulkForward() engine.Scenario {
    return &engine.BaseScenario{
        ScenarioName:        "S17-BulkForward",
        ScenarioDescription: "Forward multiple messages at once",
        CoveredMethods:      []string{"sendMessage", "forwardMessages"},
        ScenarioTimeout:     2 * time.Minute,
        ScenarioSteps: []engine.Step{
            &engine.ForwardMessagesStep{Count: 3},
            &engine.CleanupStep{},
        },
    }
}

func S18_BulkCopy() engine.Scenario {
    return &engine.BaseScenario{
        ScenarioName:        "S18-BulkCopy",
        ScenarioDescription: "Copy multiple messages at once",
        CoveredMethods:      []string{"sendMessage", "copyMessages"},
        ScenarioTimeout:     2 * time.Minute,
        ScenarioSteps: []engine.Step{
            &engine.CopyMessagesStep{Count: 2},
            &engine.CleanupStep{},
        },
    }
}

func S19_Polls() engine.Scenario {
    return &engine.BaseScenario{
        ScenarioName:        "S19-Polls",
        ScenarioDescription: "Create and stop a poll",
        CoveredMethods:      []string{"sendPoll", "stopPoll"},
        ScenarioTimeout:     1 * time.Minute,
        ScenarioSteps: []engine.Step{
            &engine.SendPollStep{
                Question: "galigo-testbot: Test Poll",
                Options:  []string{"A", "B", "C"},
            },
            &engine.StopPollStep{},
            &engine.CleanupStep{},
        },
    }
}

func AllTier2SafeScenarios() []engine.Scenario {
    return []engine.Scenario{
        S15_ChatInfo(),
        S17_BulkForward(),
        S18_BulkCopy(),
        S19_Polls(),
    }
}

// ==================== Admin Scenarios (require --run tier2-admin) ====================

func S16_PinUnpin() engine.Scenario {
    return &engine.BaseScenario{
        ScenarioName:        "S16-PinUnpin",
        ScenarioDescription: "Pin and unpin messages (requires admin)",
        CoveredMethods:      []string{"sendMessage", "pinChatMessage", "unpinChatMessage", "unpinAllChatMessages"},
        ScenarioTimeout:     1 * time.Minute,
        ScenarioSteps: []engine.Step{
            &engine.SendMessageStep{Text: "galigo-testbot: message to pin"},
            &engine.PinMessageStep{Silent: true},
            &engine.UnpinMessageStep{},
            &engine.UnpinAllMessagesStep{},
            &engine.CleanupStep{},
        },
    }
}

func AllTier2AdminScenarios() []engine.Scenario {
    return []engine.Scenario{
        S16_PinUnpin(),
    }
}

// ==================== Forum Scenarios (require --run tier2-forum) ====================

func S22_ForumTopicLifecycle() engine.Scenario {
    return &engine.BaseScenario{
        ScenarioName:        "S22-ForumTopicLifecycle",
        ScenarioDescription: "Create, edit, close, reopen, delete forum topic",
        CoveredMethods:      []string{"createForumTopic", "editForumTopic", "closeForumTopic", "reopenForumTopic", "deleteForumTopic"},
        ScenarioTimeout:     2 * time.Minute,
        ScenarioSteps: []engine.Step{
            &engine.CreateForumTopicStep{Name: "galigo-testbot: Test Topic"},
            &engine.EditForumTopicStep{Name: "galigo-testbot: Renamed"},
            &engine.CloseForumTopicStep{},
            &engine.ReopenForumTopicStep{},
            &engine.DeleteForumTopicStep{},
        },
    }
}

func AllTier2ForumScenarios() []engine.Scenario {
    return []engine.Scenario{
        S22_ForumTopicLifecycle(),
    }
}

// ==================== Dangerous Scenarios (require --allow-dangerous) ====================

func S24_ModerationLifecycle() engine.Scenario {
    return &engine.BaseScenario{
        ScenarioName:        "S24-ModerationLifecycle",
        ScenarioDescription: "Restrict and unrestrict user (DANGEROUS)",
        CoveredMethods:      []string{"restrictChatMember", "getChatMember"},
        ScenarioTimeout:     2 * time.Minute,
        ScenarioSteps: []engine.Step{
            &engine.GetChatMemberStep{},
            &engine.RestrictChatMemberStep{},
            &engine.UnrestrictChatMemberStep{},
        },
    }
}

func AllTier2DangerousScenarios() []engine.Scenario {
    return []engine.Scenario{
        S24_ModerationLifecycle(),
    }
}
```

### 3.5 Main.go Updates

Add to `cmd/galigo-testbot/main.go` switch statement:

```go
case "tier2-safe":
    scenarios = suites.AllTier2SafeScenarios()

case "tier2-admin":
    if !cfg.HasAdminChat() {
        logger.Error("TESTBOT_ADMIN_CHAT_ID required for tier2-admin")
        os.Exit(1)
    }
    rt = engine.NewRuntime(adapter, cfg.AdminChatID)
    scenarios = suites.AllTier2AdminScenarios()

case "tier2-forum":
    if !cfg.HasForumChat() {
        logger.Error("TESTBOT_FORUM_CHAT_ID required for tier2-forum")
        os.Exit(1)
    }
    rt = engine.NewRuntime(adapter, cfg.ForumChatID)
    scenarios = suites.AllTier2ForumScenarios()

case "tier2-dangerous":
    if !cfg.AllowDangerous {
        logger.Error("TESTBOT_ALLOW_DANGEROUS=true required")
        os.Exit(1)
    }
    if !cfg.HasTargetUser() {
        logger.Error("TESTBOT_TARGET_USER_ID required")
        os.Exit(1)
    }
    scenarios = suites.AllTier2DangerousScenarios()

case "all":
    scenarios = append(suites.AllPhaseAScenarios(), suites.AllPhaseBScenarios()...)
    scenarios = append(scenarios, suites.AllPhaseCScenarios()...)
    scenarios = append(scenarios, suites.AllTier2SafeScenarios()...) // Include safe Tier 2
    // NOTE: admin, forum, dangerous excluded from "all"
```

---

## Part 4: Test Summary

### Unit Tests to Create

| File | Tests | Status |
|------|-------|--------|
| `sender/chat_info_test.go` | 12 | **NEW** |
| `sender/chat_moderation_test.go` | 12 | **NEW** |
| `sender/chat_admin_test.go` | 6 | **NEW** |
| `sender/chat_settings_test.go` | 8 | **NEW** |
| `sender/chat_pin_test.go` | 8 | **NEW** |
| `sender/polls_test.go` | 8 | **NEW** |
| `sender/forum_test.go` | 16 | **NEW** |
| `tg/chat_member_test.go` | 6 | **NEW** |

**Total: ~76 new unit tests**

### Integration Scenarios

| Scenario | Methods | Safety Level | Status |
|----------|---------|--------------|--------|
| S15-ChatInfo | 4 | Safe | **NEW** |
| S17-BulkForward | 2 | Safe | **NEW** |
| S18-BulkCopy | 2 | Safe | **NEW** |
| S19-Polls | 2 | Safe | **NEW** |
| S16-PinUnpin | 4 | Admin | **NEW** |
| S22-ForumLifecycle | 5 | Forum | **NEW** |
| S24-Moderation | 2 | Dangerous | **NEW** |

### Implementation Fixes Required

| Issue | Action |
|-------|--------|
| `messageThreadID` validation missing | Add `> 0` check to forum topic methods |

---

## Part 5: Execution Order

1. **Fix implementation gap** — Add `messageThreadID > 0` validation
2. **Create unit test files** — All files listed in Part 4
3. **Update registry** — Add `CategoryTier2` and all methods
4. **Update config** — Add admin/forum/target env vars
5. **Create integration scenarios** — `suites/tier2.go`
6. **Update main.go** — Add tier2-* run commands
7. **Run and verify**:
   ```bash
   go test -v ./sender/...
   go test -v ./tg/...
   go run ./cmd/galigo-testbot --run tier2-safe
   go run ./cmd/galigo-testbot --status
   ```

---

## Part 6: Acceptance Criteria

### Per Method
- [ ] Unit test: Success (A)
- [ ] Unit test: Validation with `CaptureCount() == 0` (B)
- [ ] Unit test: Error mapping (C)
- [ ] In registry with `CategoryTier2`
- [ ] Covered by scenario OR marked "skipped: <reason>"

### Per Bucket
- [ ] One retry test
- [ ] One breaker test (if new HTTP path)

### Overall
- [ ] `go test -race ./...` passes
- [ ] Coverage ≥ 80% for new code
- [ ] `galigo-testbot --status` shows Tier 2 coverage
- [ ] All scenarios clean up properly
- [ ] Dangerous operations gated behind `--allow-dangerous`