package sender_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// ==================== PinChatMessage ====================

func TestPinChatMessage(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/pinChatMessage", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, float64(42), req["message_id"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.PinChatMessage(context.Background(), int64(-100123), 42)
	assert.NoError(t, err)
}

func TestPinChatMessage_Silent(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/pinChatMessage", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, true, req["disable_notification"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.PinChatMessage(context.Background(), int64(-100123), 42, sender.WithSilentPin())
	assert.NoError(t, err)
}

func TestPinChatMessage_Validation_InvalidMessageID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.PinChatMessage(context.Background(), int64(-100123), 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message_id")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestPinChatMessage_Validation_InvalidChatID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.PinChatMessage(context.Background(), nil, 42)
	require.Error(t, err)
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestPinChatMessage_Error_BadRequest(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/pinChatMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBadRequest(w, "message to pin not found")
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.PinChatMessage(context.Background(), int64(-100123), 999)

	require.Error(t, err)
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 400, apiErr.Code)
}

// ==================== UnpinChatMessage ====================

func TestUnpinChatMessage(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/unpinChatMessage", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, float64(42), req["message_id"])
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.UnpinChatMessage(context.Background(), int64(-100123), 42)
	assert.NoError(t, err)
}

func TestUnpinChatMessage_MostRecent(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/unpinChatMessage", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		_, hasMessageID := req["message_id"]
		assert.False(t, hasMessageID, "message_id should not be present when 0")
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.UnpinChatMessage(context.Background(), int64(-100123), 0)
	assert.NoError(t, err)
}

// ==================== UnpinAllChatMessages ====================

func TestUnpinAllChatMessages(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/unpinAllChatMessages", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.UnpinAllChatMessages(context.Background(), int64(-100123))
	assert.NoError(t, err)
}

// ==================== LeaveChat ====================

func TestLeaveChat(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/leaveChat", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.LeaveChat(context.Background(), int64(-100123))
	assert.NoError(t, err)
}
