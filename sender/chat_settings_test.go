package sender_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== SetChatPermissions ====================

func TestSetChatPermissions(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setChatPermissions", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		perms, ok := req["permissions"].(map[string]any)
		assert.True(t, ok)
		assert.Equal(t, false, perms["can_send_messages"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetChatPermissions(context.Background(), int64(-100123), tg.ReadOnlyPermissions())
	assert.NoError(t, err)
}

func TestSetChatPermissions_WithIndependent(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setChatPermissions", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, true, req["use_independent_chat_permissions"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetChatPermissions(context.Background(), int64(-100123), tg.NoPermissions(),
		sender.WithIndependentPermissionsForChat(),
	)
	assert.NoError(t, err)
}

func TestSetChatPermissions_Validation_InvalidChatID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetChatPermissions(context.Background(), nil, tg.NoPermissions())
	require.Error(t, err)
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestSetChatPermissions_Error_Forbidden(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setChatPermissions", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyForbidden(w, "not enough rights")
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetChatPermissions(context.Background(), int64(-100123), tg.NoPermissions())

	require.Error(t, err)
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 403, apiErr.Code)
}

// ==================== SetChatTitle ====================

func TestSetChatTitle(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setChatTitle", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "New Title", req["title"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetChatTitle(context.Background(), int64(-100123), "New Title")
	assert.NoError(t, err)
}

func TestSetChatTitle_Validation_Empty(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetChatTitle(context.Background(), int64(-100123), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestSetChatTitle_Validation_TooLong(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	longTitle := make([]byte, 129)
	for i := range longTitle {
		longTitle[i] = 'a'
	}
	err := client.SetChatTitle(context.Background(), int64(-100123), string(longTitle))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "128 characters")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

// ==================== SetChatDescription ====================

func TestSetChatDescription(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setChatDescription", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetChatDescription(context.Background(), int64(-100123), "A new description")
	assert.NoError(t, err)
}

func TestSetChatDescription_EmptyAllowed(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setChatDescription", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetChatDescription(context.Background(), int64(-100123), "")
	assert.NoError(t, err)
}

func TestSetChatDescription_Validation_TooLong(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	longDesc := make([]byte, 256)
	for i := range longDesc {
		longDesc[i] = 'a'
	}
	err := client.SetChatDescription(context.Background(), int64(-100123), string(longDesc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "255 characters")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

// ==================== DeleteChatPhoto ====================

func TestDeleteChatPhoto(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/deleteChatPhoto", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.DeleteChatPhoto(context.Background(), int64(-100123))
	assert.NoError(t, err)
}

// ==================== SetChatPhoto ====================

func TestSetChatPhoto(t *testing.T) {
	photoData := make([]byte, 512)
	for i := range photoData {
		photoData[i] = byte(i % 256)
	}

	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setChatPhoto", func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		assert.True(t, strings.HasPrefix(contentType, "multipart/form-data"), "expected multipart request")

		err := r.ParseMultipartForm(32 << 20)
		require.NoError(t, err)

		file, header, err := r.FormFile("photo")
		require.NoError(t, err)
		defer file.Close()

		assert.Equal(t, "photo.png", header.Filename)

		content, err := io.ReadAll(file)
		require.NoError(t, err)
		assert.Equal(t, 512, len(content), "photo part must contain full file")

		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetChatPhoto(context.Background(), int64(-100123), sender.FromBytes(photoData, "photo.png"))
	assert.NoError(t, err)
}

func TestSetChatPhoto_Validation_InvalidChatID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetChatPhoto(context.Background(), nil, sender.FromBytes([]byte{1, 2, 3}, "photo.png"))
	require.Error(t, err)
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestSetChatPhoto_Error_Forbidden(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setChatPhoto", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyForbidden(w, "not enough rights")
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetChatPhoto(context.Background(), int64(-100123), sender.FromBytes([]byte{1, 2, 3}, "photo.png"))

	require.Error(t, err)
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 403, apiErr.Code)
}
