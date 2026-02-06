package sender_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// ==================== GetBusinessConnection ====================

func TestGetBusinessConnection(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getBusinessConnection", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"id":         "bc_123",
			"user":       map[string]any{"id": int64(456), "is_bot": false, "first_name": "Alice"},
			"date":       1700000000,
			"can_reply":  true,
			"is_enabled": true,
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	conn, err := client.GetBusinessConnection(context.Background(), "bc_123")
	require.NoError(t, err)
	assert.Equal(t, "bc_123", conn.ID)
	assert.Equal(t, int64(456), conn.User.ID)
	assert.True(t, conn.IsEnabled)
}

func TestGetBusinessConnection_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.GetBusinessConnection(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "business_connection_id")
}

// ==================== SetBusinessAccountName ====================

func TestSetBusinessAccountName(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setBusinessAccountName", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetBusinessAccountName(context.Background(), sender.SetBusinessAccountNameRequest{
		BusinessConnectionID: "bc_123",
		FirstName:            "Alice",
		LastName:             "Smith",
	})
	require.NoError(t, err)
}

func TestSetBusinessAccountName_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.SetBusinessAccountNameRequest
		want string
	}{
		{"missing connection_id", sender.SetBusinessAccountNameRequest{FirstName: "A"}, "business_connection_id"},
		{"missing first_name", sender.SetBusinessAccountNameRequest{BusinessConnectionID: "bc"}, "first_name"},
		{"first_name too long", sender.SetBusinessAccountNameRequest{
			BusinessConnectionID: "bc",
			FirstName:            string(make([]byte, 65)),
		}, "first_name"},
		{"last_name too long", sender.SetBusinessAccountNameRequest{
			BusinessConnectionID: "bc",
			FirstName:            "A",
			LastName:             string(make([]byte, 65)),
		}, "last_name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.SetBusinessAccountName(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== SetBusinessAccountBio ====================

func TestSetBusinessAccountBio(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setBusinessAccountBio", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetBusinessAccountBio(context.Background(), sender.SetBusinessAccountBioRequest{
		BusinessConnectionID: "bc_123",
		Bio:                  "Hello world",
	})
	require.NoError(t, err)
}

func TestSetBusinessAccountBio_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.SetBusinessAccountBioRequest
		want string
	}{
		{"missing connection_id", sender.SetBusinessAccountBioRequest{Bio: "hi"}, "business_connection_id"},
		{"bio too long", sender.SetBusinessAccountBioRequest{
			BusinessConnectionID: "bc",
			Bio:                  string(make([]byte, 141)),
		}, "bio"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.SetBusinessAccountBio(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== SetBusinessAccountUsername ====================

func TestSetBusinessAccountUsername(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setBusinessAccountUsername", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetBusinessAccountUsername(context.Background(), sender.SetBusinessAccountUsernameRequest{
		BusinessConnectionID: "bc_123",
		Username:             "alice_biz",
	})
	require.NoError(t, err)
}

func TestSetBusinessAccountUsername_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetBusinessAccountUsername(context.Background(), sender.SetBusinessAccountUsernameRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "business_connection_id")
}

// ==================== SetBusinessAccountGiftSettings ====================

func TestSetBusinessAccountGiftSettings(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setBusinessAccountGiftSettings", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetBusinessAccountGiftSettings(context.Background(), sender.SetBusinessAccountGiftSettingsRequest{
		BusinessConnectionID: "bc_123",
		ShowGiftButton:       true,
		AcceptedGiftTypes:    tg.AcceptedGiftTypes{UnlimitedGifts: true},
	})
	require.NoError(t, err)
}

func TestSetBusinessAccountGiftSettings_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetBusinessAccountGiftSettings(context.Background(), sender.SetBusinessAccountGiftSettingsRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "business_connection_id")
}

// ==================== PostStory ====================

func TestPostStory(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/postStory", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"id":   1,
			"chat": map[string]any{"id": int64(123), "type": "private"},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	story, err := client.PostStory(context.Background(), sender.PostStoryRequest{
		BusinessConnectionID: "bc_123",
		Content:              &sender.InputStoryContentPhoto{Photo: sender.FromFileID("photo_123")},
		Caption:              "My story",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, story.ID)
}

func TestPostStory_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.PostStoryRequest
		want string
	}{
		{"missing connection_id", sender.PostStoryRequest{Content: &sender.InputStoryContentPhoto{}}, "business_connection_id"},
		{"missing content", sender.PostStoryRequest{BusinessConnectionID: "bc"}, "content"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.PostStory(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== EditStory ====================

func TestEditStory(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/editStory", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"id":   1,
			"chat": map[string]any{"id": int64(123), "type": "private"},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	story, err := client.EditStory(context.Background(), sender.EditStoryRequest{
		BusinessConnectionID: "bc_123",
		StoryID:              1,
		Caption:              "Updated",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, story.ID)
}

func TestEditStory_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.EditStoryRequest
		want string
	}{
		{"missing connection_id", sender.EditStoryRequest{StoryID: 1}, "business_connection_id"},
		{"invalid story_id", sender.EditStoryRequest{BusinessConnectionID: "bc", StoryID: 0}, "story_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.EditStory(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== DeleteStory ====================

func TestDeleteStory(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/deleteStory", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.DeleteStory(context.Background(), sender.DeleteStoryRequest{
		BusinessConnectionID: "bc_123",
		StoryID:              1,
	})
	require.NoError(t, err)
}

func TestDeleteStory_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.DeleteStoryRequest
		want string
	}{
		{"missing connection_id", sender.DeleteStoryRequest{StoryID: 1}, "business_connection_id"},
		{"invalid story_id", sender.DeleteStoryRequest{BusinessConnectionID: "bc"}, "story_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.DeleteStory(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== TransferBusinessAccountStars ====================

func TestTransferBusinessAccountStars(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/transferBusinessAccountStars", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.TransferBusinessAccountStars(context.Background(), sender.TransferBusinessAccountStarsRequest{
		BusinessConnectionID: "bc_123",
		StarCount:            100,
	})
	require.NoError(t, err)
}

func TestTransferBusinessAccountStars_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.TransferBusinessAccountStarsRequest
		want string
	}{
		{"missing connection_id", sender.TransferBusinessAccountStarsRequest{StarCount: 1}, "business_connection_id"},
		{"invalid star_count", sender.TransferBusinessAccountStarsRequest{BusinessConnectionID: "bc", StarCount: 0}, "star_count"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.TransferBusinessAccountStars(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== GetBusinessAccountStarBalance ====================

func TestGetBusinessAccountStarBalance(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getBusinessAccountStarBalance", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{"amount": 500, "nanostar_amount": 0})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	balance, err := client.GetBusinessAccountStarBalance(context.Background(), "bc_123")
	require.NoError(t, err)
	assert.Equal(t, 500, balance.Amount)
}

func TestGetBusinessAccountStarBalance_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.GetBusinessAccountStarBalance(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "business_connection_id")
}

// ==================== SetBusinessAccountProfilePhoto ====================

func TestSetBusinessAccountProfilePhoto(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setBusinessAccountProfilePhoto", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetBusinessAccountProfilePhoto(context.Background(), sender.SetBusinessAccountProfilePhotoRequest{
		BusinessConnectionID: "bc_123",
		Photo:                &sender.InputProfilePhotoStatic{Photo: sender.FromFileID("photo_123")},
	})
	require.NoError(t, err)
}

func TestSetBusinessAccountProfilePhoto_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.SetBusinessAccountProfilePhotoRequest
		want string
	}{
		{"missing connection_id", sender.SetBusinessAccountProfilePhotoRequest{Photo: &sender.InputProfilePhotoStatic{}}, "business_connection_id"},
		{"missing photo", sender.SetBusinessAccountProfilePhotoRequest{BusinessConnectionID: "bc"}, "photo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.SetBusinessAccountProfilePhoto(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== RemoveBusinessAccountProfilePhoto ====================

func TestRemoveBusinessAccountProfilePhoto(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/removeBusinessAccountProfilePhoto", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.RemoveBusinessAccountProfilePhoto(context.Background(), sender.RemoveBusinessAccountProfilePhotoRequest{
		BusinessConnectionID: "bc_123",
	})
	require.NoError(t, err)
}

func TestRemoveBusinessAccountProfilePhoto_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.RemoveBusinessAccountProfilePhoto(context.Background(), sender.RemoveBusinessAccountProfilePhotoRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "business_connection_id")
}
