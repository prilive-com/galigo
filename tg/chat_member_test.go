package tg

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalChatMember_AllStatuses(t *testing.T) {
	tests := []struct {
		status     string
		wantType   string
		wantHelper func(ChatMember) bool
	}{
		{"creator", "creator", IsOwner},
		{"administrator", "administrator", func(m ChatMember) bool { _, ok := m.(ChatMemberAdministrator); return ok }},
		{"member", "member", IsMember},
		{"restricted", "restricted", IsRestricted},
		{"left", "left", HasLeft},
		{"kicked", "kicked", IsBanned},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			data, _ := json.Marshal(map[string]any{
				"status": tt.status,
				"user":   map[string]any{"id": 123, "first_name": "Test", "is_bot": false},
			})

			member, err := UnmarshalChatMember(data)
			require.NoError(t, err)
			assert.Equal(t, tt.wantType, member.Status())
			assert.Equal(t, int64(123), member.GetUser().ID)
			assert.True(t, tt.wantHelper(member))
		})
	}
}

func TestUnmarshalChatMember_UnknownStatus(t *testing.T) {
	data := []byte(`{"status":"unknown","user":{"id":1,"first_name":"X","is_bot":false}}`)
	_, err := UnmarshalChatMember(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown chat member status")
}

func TestUnmarshalChatMember_InvalidJSON(t *testing.T) {
	_, err := UnmarshalChatMember([]byte(`{invalid`))
	assert.Error(t, err)
}

func TestUnmarshalChatMember_AdminFields(t *testing.T) {
	canPost := true
	data, _ := json.Marshal(map[string]any{
		"status":              "administrator",
		"user":                map[string]any{"id": 456, "first_name": "Admin", "is_bot": false},
		"can_be_edited":       true,
		"can_delete_messages": true,
		"can_promote_members": false,
		"can_post_messages":   canPost,
		"custom_title":        "Mod",
	})

	member, err := UnmarshalChatMember(data)
	require.NoError(t, err)

	admin, ok := member.(ChatMemberAdministrator)
	require.True(t, ok)
	assert.True(t, admin.CanBeEdited)
	assert.True(t, admin.CanDeleteMessages)
	assert.False(t, admin.CanPromoteMembers)
	assert.NotNil(t, admin.CanPostMessages)
	assert.True(t, *admin.CanPostMessages)
	assert.Equal(t, "Mod", admin.CustomTitle)
}

func TestUnmarshalChatMember_RestrictedFields(t *testing.T) {
	data, _ := json.Marshal(map[string]any{
		"status":            "restricted",
		"user":              map[string]any{"id": 789, "first_name": "Restricted", "is_bot": false},
		"is_member":         true,
		"can_send_messages": false,
		"can_send_photos":   true,
		"until_date":        int64(1700000000),
	})

	member, err := UnmarshalChatMember(data)
	require.NoError(t, err)

	restricted, ok := member.(ChatMemberRestricted)
	require.True(t, ok)
	assert.True(t, restricted.IsMember)
	assert.False(t, restricted.CanSendMessages)
	assert.True(t, restricted.CanSendPhotos)
	assert.Equal(t, int64(1700000000), restricted.UntilDate)
}

func TestUnmarshalChatMember_OwnerFields(t *testing.T) {
	data, _ := json.Marshal(map[string]any{
		"status":       "creator",
		"user":         map[string]any{"id": 1, "first_name": "Owner", "is_bot": false},
		"is_anonymous": true,
		"custom_title": "Boss",
	})

	member, err := UnmarshalChatMember(data)
	require.NoError(t, err)

	owner, ok := member.(ChatMemberOwner)
	require.True(t, ok)
	assert.True(t, owner.IsAnonymous)
	assert.Equal(t, "Boss", owner.CustomTitle)
}

func TestIsAdmin_IncludesOwner(t *testing.T) {
	owner := ChatMemberOwner{}
	admin := ChatMemberAdministrator{}
	member := ChatMemberMember{}

	assert.True(t, IsAdmin(owner))
	assert.True(t, IsAdmin(admin))
	assert.False(t, IsAdmin(member))
}

func TestChatMemberUpdated_UnmarshalJSON(t *testing.T) {
	data := []byte(`{
		"chat": {"id": -100123, "type": "supergroup", "title": "Test"},
		"from": {"id": 1, "first_name": "Admin", "is_bot": false},
		"date": 1700000000,
		"old_chat_member": {"status": "member", "user": {"id": 999, "first_name": "Bob", "is_bot": false}},
		"new_chat_member": {"status": "kicked", "user": {"id": 999, "first_name": "Bob", "is_bot": false}, "until_date": 0}
	}`)

	var updated ChatMemberUpdated
	err := json.Unmarshal(data, &updated)
	require.NoError(t, err)

	assert.Equal(t, int64(-100123), updated.Chat.ID)
	assert.Equal(t, "member", updated.OldChatMember.Status())
	assert.Equal(t, "kicked", updated.NewChatMember.Status())
	assert.True(t, IsMember(updated.OldChatMember))
	assert.True(t, IsBanned(updated.NewChatMember))
}
