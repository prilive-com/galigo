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

func TestCallJSON_ViaGetMe(t *testing.T) {
	// callJSON is internal; test it via a method that uses executeRequest+parseMessage
	// pattern. GetMe exercises the same code path.
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getMe", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyUser(w)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	user, err := client.GetMe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, testutil.TestBotID, user.ID)
	assert.True(t, user.IsBot)
	assert.Equal(t, "Test Bot", user.FirstName)
}

func TestCallJSON_APIError(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/deleteMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyError(w, 400, "Bad Request: message to delete not found", nil)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.DeleteMessage(context.Background(), sender.DeleteMessageRequest{
		ChatID:    int64(123),
		MessageID: 999,
	})
	require.Error(t, err)
	var apiErr *tg.APIError
	assert.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 400, apiErr.Code)
}

func TestCallJSON_VoidMethod(t *testing.T) {
	// deleteMessage returns bool (true), which we ignore — test it doesn't panic
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/deleteMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.DeleteMessage(context.Background(), sender.DeleteMessageRequest{
		ChatID:    int64(123),
		MessageID: 1,
	})
	assert.NoError(t, err)
}
