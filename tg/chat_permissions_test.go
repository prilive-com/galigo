package tg

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllPermissions_AllTrue(t *testing.T) {
	p := AllPermissions()
	assert.NotNil(t, p.CanSendMessages)
	assert.True(t, *p.CanSendMessages)
	assert.NotNil(t, p.CanSendPhotos)
	assert.True(t, *p.CanSendPhotos)
	assert.NotNil(t, p.CanManageTopics)
	assert.True(t, *p.CanManageTopics)
}

func TestNoPermissions_AllFalse(t *testing.T) {
	p := NoPermissions()
	assert.NotNil(t, p.CanSendMessages)
	assert.False(t, *p.CanSendMessages)
	assert.NotNil(t, p.CanSendPhotos)
	assert.False(t, *p.CanSendPhotos)
	assert.NotNil(t, p.CanManageTopics)
	assert.False(t, *p.CanManageTopics)
}

func TestReadOnlyPermissions_NoSending(t *testing.T) {
	p := ReadOnlyPermissions()
	assert.NotNil(t, p.CanSendMessages)
	assert.False(t, *p.CanSendMessages)
	// Admin-level fields should be nil (not set)
	assert.Nil(t, p.CanChangeInfo)
	assert.Nil(t, p.CanInviteUsers)
	assert.Nil(t, p.CanPinMessages)
}

func TestTextOnlyPermissions(t *testing.T) {
	p := TextOnlyPermissions()
	assert.True(t, *p.CanSendMessages)
	assert.False(t, *p.CanSendPhotos)
	assert.False(t, *p.CanSendAudios)
	assert.False(t, *p.CanSendDocuments)
}

func TestChatPermissions_JSON_OmitsNil(t *testing.T) {
	// An empty ChatPermissions should produce "{}" (all nil = all omitted)
	p := ChatPermissions{}
	data, err := json.Marshal(p)
	require.NoError(t, err)
	assert.Equal(t, "{}", string(data))
}

func TestChatPermissions_JSON_IncludesFalse(t *testing.T) {
	// Explicitly false should be included (pointer to false)
	p := ChatPermissions{
		CanSendMessages: boolPtr(false),
	}
	data, err := json.Marshal(p)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"can_send_messages":false`)
}

func TestChatPermissions_JSON_Roundtrip(t *testing.T) {
	original := AllPermissions()
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded ChatPermissions
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.NotNil(t, decoded.CanSendMessages)
	assert.True(t, *decoded.CanSendMessages)
	assert.NotNil(t, decoded.CanManageTopics)
	assert.True(t, *decoded.CanManageTopics)
}

func TestFullAdminRights(t *testing.T) {
	r := FullAdminRights()
	assert.True(t, r.CanManageChat)
	assert.True(t, r.CanDeleteMessages)
	assert.True(t, r.CanPromoteMembers)
	assert.NotNil(t, r.CanPostMessages)
	assert.True(t, *r.CanPostMessages)
	assert.NotNil(t, r.CanManageDirectMessages)
	assert.True(t, *r.CanManageDirectMessages)
}

func TestModeratorRights(t *testing.T) {
	r := ModeratorRights()
	assert.True(t, r.CanManageChat)
	assert.True(t, r.CanDeleteMessages)
	assert.True(t, r.CanRestrictMembers)
	assert.False(t, r.CanPromoteMembers)
	assert.False(t, r.CanChangeInfo)
	assert.NotNil(t, r.CanPinMessages)
	assert.True(t, *r.CanPinMessages)
}

func TestContentManagerRights(t *testing.T) {
	r := ContentManagerRights()
	assert.True(t, r.CanManageChat)
	assert.True(t, r.CanDeleteMessages)
	assert.True(t, r.CanChangeInfo)
	assert.NotNil(t, r.CanPostMessages)
	assert.True(t, *r.CanPostMessages)
	assert.False(t, r.CanRestrictMembers)
	assert.False(t, r.CanPromoteMembers)
}
