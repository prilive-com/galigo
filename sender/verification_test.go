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

// ==================== SetPassportDataErrors ====================

func TestSetPassportDataErrors(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setPassportDataErrors", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetPassportDataErrors(context.Background(), sender.SetPassportDataErrorsRequest{
		UserID: 456,
		Errors: []tg.PassportElementError{
			tg.PassportElementErrorDataField{
				Type:      "personal_details",
				FieldName: "first_name",
				DataHash:  "abc123",
				Message:   "Name is incorrect",
			},
		},
	})
	require.NoError(t, err)
}

func TestSetPassportDataErrors_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.SetPassportDataErrorsRequest
		want string
	}{
		{"missing user_id", sender.SetPassportDataErrorsRequest{
			Errors: []tg.PassportElementError{tg.PassportElementErrorDataField{}},
		}, "user_id"},
		{"missing errors", sender.SetPassportDataErrorsRequest{UserID: 1}, "errors"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.SetPassportDataErrors(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== VerifyUser ====================

func TestVerifyUser(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/verifyUser", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.VerifyUser(context.Background(), sender.VerifyUserRequest{
		UserID:            456,
		CustomDescription: "Verified org",
	})
	require.NoError(t, err)
}

func TestVerifyUser_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.VerifyUser(context.Background(), sender.VerifyUserRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user_id")
}

// ==================== VerifyChat ====================

func TestVerifyChat(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/verifyChat", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.VerifyChat(context.Background(), sender.VerifyChatRequest{
		ChatID: int64(123),
	})
	require.NoError(t, err)
}

func TestVerifyChat_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.VerifyChat(context.Background(), sender.VerifyChatRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat_id")
}

// ==================== RemoveUserVerification ====================

func TestRemoveUserVerification(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/removeUserVerification", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.RemoveUserVerification(context.Background(), 456)
	require.NoError(t, err)
}

func TestRemoveUserVerification_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.RemoveUserVerification(context.Background(), 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user_id")
}

// ==================== RemoveChatVerification ====================

func TestRemoveChatVerification(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/removeChatVerification", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.RemoveChatVerification(context.Background(), int64(123))
	require.NoError(t, err)
}

func TestRemoveChatVerification_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.RemoveChatVerification(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat_id")
}
