package sender_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// ==================== CreateForumTopic ====================

func TestCreateForumTopic(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/createForumTopic", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"message_thread_id": 123,
			"name":              "Discussion",
			"icon_color":        tg.ForumColorBlue,
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	topic, err := client.CreateForumTopic(context.Background(), int64(-100123), "Discussion")
	require.NoError(t, err)
	assert.Equal(t, 123, topic.MessageThreadID)
	assert.Equal(t, "Discussion", topic.Name)
}

func TestCreateForumTopic_WithOptions(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/createForumTopic", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"message_thread_id": 456,
			"name":              "Test",
			"icon_color":        tg.ForumColorGreen,
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.CreateForumTopic(context.Background(), int64(-100123), "Test",
		sender.WithTopicColor(tg.ForumColorGreen),
	)
	assert.NoError(t, err)
}

func TestCreateForumTopic_Validation_EmptyName(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.CreateForumTopic(context.Background(), int64(-100123), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestCreateForumTopic_Validation_NameTooLong(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	longName := make([]byte, 129)
	for i := range longName {
		longName[i] = 'a'
	}
	_, err := client.CreateForumTopic(context.Background(), int64(-100123), string(longName))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "128 characters")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestCreateForumTopic_Error_Forbidden(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/createForumTopic", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyForbidden(w, "not enough rights to manage topics")
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	_, err := client.CreateForumTopic(context.Background(), int64(-100123), "Test")

	require.Error(t, err)
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 403, apiErr.Code)
}

// ==================== EditForumTopic ====================

func TestEditForumTopic(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/editForumTopic", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.EditForumTopic(context.Background(), int64(-100123), 42,
		sender.WithEditTopicName("New Name"),
	)
	assert.NoError(t, err)
}

func TestEditForumTopic_Validation_InvalidThreadID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.EditForumTopic(context.Background(), int64(-100123), 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message_thread_id")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

// ==================== CloseForumTopic ====================

func TestCloseForumTopic(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/closeForumTopic", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.CloseForumTopic(context.Background(), int64(-100123), 42)
	assert.NoError(t, err)
}

func TestCloseForumTopic_Validation_InvalidThreadID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.CloseForumTopic(context.Background(), int64(-100123), 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message_thread_id")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

// ==================== ReopenForumTopic ====================

func TestReopenForumTopic(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/reopenForumTopic", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.ReopenForumTopic(context.Background(), int64(-100123), 42)
	assert.NoError(t, err)
}

func TestReopenForumTopic_Validation_InvalidThreadID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.ReopenForumTopic(context.Background(), int64(-100123), 0)
	require.Error(t, err)
	assert.Equal(t, 0, server.CaptureCount())
}

// ==================== DeleteForumTopic ====================

func TestDeleteForumTopic(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/deleteForumTopic", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.DeleteForumTopic(context.Background(), int64(-100123), 42)
	assert.NoError(t, err)
}

func TestDeleteForumTopic_Validation_InvalidThreadID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.DeleteForumTopic(context.Background(), int64(-100123), 0)
	require.Error(t, err)
	assert.Equal(t, 0, server.CaptureCount())
}

// ==================== UnpinAllForumTopicMessages ====================

func TestUnpinAllForumTopicMessages_Validation_InvalidThreadID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.UnpinAllForumTopicMessages(context.Background(), int64(-100123), 0)
	require.Error(t, err)
	assert.Equal(t, 0, server.CaptureCount())
}

// ==================== General Forum Topic Methods ====================

func TestCloseGeneralForumTopic(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/closeGeneralForumTopic", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.CloseGeneralForumTopic(context.Background(), int64(-100123))
	assert.NoError(t, err)
}

func TestReopenGeneralForumTopic(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/reopenGeneralForumTopic", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.ReopenGeneralForumTopic(context.Background(), int64(-100123))
	assert.NoError(t, err)
}

func TestHideGeneralForumTopic(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/hideGeneralForumTopic", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.HideGeneralForumTopic(context.Background(), int64(-100123))
	assert.NoError(t, err)
}

func TestUnhideGeneralForumTopic(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/unhideGeneralForumTopic", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.UnhideGeneralForumTopic(context.Background(), int64(-100123))
	assert.NoError(t, err)
}

// ==================== EditGeneralForumTopic ====================

func TestEditGeneralForumTopic(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/editGeneralForumTopic", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.EditGeneralForumTopic(context.Background(), int64(-100123), "New General Name")
	assert.NoError(t, err)
}

func TestEditGeneralForumTopic_Validation_InvalidChatID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.EditGeneralForumTopic(context.Background(), nil, "Name")
	require.Error(t, err)
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

// ==================== UnpinAllGeneralForumTopicMessages ====================

func TestUnpinAllGeneralForumTopicMessages(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/unpinAllGeneralForumTopicMessages", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.UnpinAllGeneralForumTopicMessages(context.Background(), int64(-100123))
	assert.NoError(t, err)
}

func TestUnpinAllGeneralForumTopicMessages_Validation_InvalidChatID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.UnpinAllGeneralForumTopicMessages(context.Background(), nil)
	require.Error(t, err)
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

// ==================== GetForumTopicIconStickers ====================

func TestGetForumTopicIconStickers(t *testing.T) {
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
