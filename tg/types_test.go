package tg_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

func TestLinkPreviewOptions_Validate(t *testing.T) {
	tests := []struct {
		name    string
		opts    *tg.LinkPreviewOptions
		wantErr bool
	}{
		{
			name:    "nil options valid",
			opts:    nil,
			wantErr: false,
		},
		{
			name:    "empty options valid",
			opts:    &tg.LinkPreviewOptions{},
			wantErr: false,
		},
		{
			name: "disabled preview valid",
			opts: &tg.LinkPreviewOptions{
				IsDisabled: true,
			},
			wantErr: false,
		},
		{
			name: "prefer small media valid",
			opts: &tg.LinkPreviewOptions{
				PreferSmallMedia: true,
			},
			wantErr: false,
		},
		{
			name: "prefer large media valid",
			opts: &tg.LinkPreviewOptions{
				PreferLargeMedia: true,
			},
			wantErr: false,
		},
		{
			name: "show above text valid",
			opts: &tg.LinkPreviewOptions{
				ShowAboveText: true,
			},
			wantErr: false,
		},
		{
			name: "custom URL valid",
			opts: &tg.LinkPreviewOptions{
				URL: "https://example.com",
			},
			wantErr: false,
		},
		{
			name: "full valid config",
			opts: &tg.LinkPreviewOptions{
				URL:              "https://example.com",
				PreferLargeMedia: true,
				ShowAboveText:    true,
			},
			wantErr: false,
		},
		{
			name: "mutually exclusive error",
			opts: &tg.LinkPreviewOptions{
				PreferSmallMedia: true,
				PreferLargeMedia: true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "mutually exclusive")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ==================== Bot API 9.4 Types ====================

func TestVideoQuality_Unmarshal(t *testing.T) {
	raw := `{"file_id":"abc","file_unique_id":"xyz","width":1920,"height":1080,"codec":"h265","file_size":12345678}`
	var vq tg.VideoQuality
	require.NoError(t, json.Unmarshal([]byte(raw), &vq))
	assert.Equal(t, "abc", vq.FileID)
	assert.Equal(t, 1920, vq.Width)
	assert.Equal(t, "h265", vq.Codec)
	assert.Equal(t, int64(12345678), vq.FileSize)
}

func TestVideo_WithQualities(t *testing.T) {
	raw := `{"file_id":"vid1","file_unique_id":"u1","width":1920,"height":1080,"duration":60,"qualities":[{"file_id":"q1","file_unique_id":"qu1","width":640,"height":360,"codec":"h264"}]}`
	var v tg.Video
	require.NoError(t, json.Unmarshal([]byte(raw), &v))
	require.Len(t, v.Qualities, 1)
	assert.Equal(t, 640, v.Qualities[0].Width)
	assert.Equal(t, "h264", v.Qualities[0].Codec)
}

func TestChatOwnerLeft_Unmarshal(t *testing.T) {
	raw := `{"new_owner":{"id":123,"is_bot":false,"first_name":"Alice"}}`
	var col tg.ChatOwnerLeft
	require.NoError(t, json.Unmarshal([]byte(raw), &col))
	require.NotNil(t, col.NewOwner)
	assert.Equal(t, int64(123), col.NewOwner.ID)
}

func TestChatOwnerLeft_WithoutNewOwner(t *testing.T) {
	raw := `{}`
	var col tg.ChatOwnerLeft
	require.NoError(t, json.Unmarshal([]byte(raw), &col))
	assert.Nil(t, col.NewOwner)
}

func TestChatOwnerChanged_Unmarshal(t *testing.T) {
	raw := `{"new_owner":{"id":456,"is_bot":false,"first_name":"Bob"}}`
	var coc tg.ChatOwnerChanged
	require.NoError(t, json.Unmarshal([]byte(raw), &coc))
	require.NotNil(t, coc.NewOwner)
	assert.Equal(t, int64(456), coc.NewOwner.ID)
}

func TestUserProfileAudios_Unmarshal(t *testing.T) {
	raw := `{"total_count":2,"audios":[{"file_id":"aud1","file_unique_id":"u1","duration":180}]}`
	var upa tg.UserProfileAudios
	require.NoError(t, json.Unmarshal([]byte(raw), &upa))
	assert.Equal(t, 2, upa.TotalCount)
	require.Len(t, upa.Audios, 1)
	assert.Equal(t, "aud1", upa.Audios[0].FileID)
}

func TestUser_AllowsUsersToCreateTopics(t *testing.T) {
	raw := `{"id":1,"is_bot":true,"first_name":"Bot","allows_users_to_create_topics":true}`
	var u tg.User
	require.NoError(t, json.Unmarshal([]byte(raw), &u))
	assert.True(t, u.AllowsUsersToCreateTopics)
}

func TestMessage_ChatOwnerServiceMessages(t *testing.T) {
	raw := `{"message_id":1,"date":1234,"chat":{"id":1,"type":"group"},"chat_owner_left":{"new_owner":{"id":99,"is_bot":false,"first_name":"X"}}}`
	var m tg.Message
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	require.NotNil(t, m.ChatOwnerLeft)
	assert.Equal(t, int64(99), m.ChatOwnerLeft.NewOwner.ID)
	assert.Nil(t, m.ChatOwnerChanged)
}
