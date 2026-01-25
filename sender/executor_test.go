package sender_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutor_SendMessage_Success(t *testing.T) {
	server := testutil.NewMockServer(t)

	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 123)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello, World!",
	})

	require.NoError(t, err)
	assert.Equal(t, 123, msg.MessageID)

	// Verify request
	cap := server.LastCapture()
	cap.AssertMethod(t, "POST")
	cap.AssertPath(t, "/bot"+testutil.TestToken+"/sendMessage")
	cap.AssertContentType(t, "application/json")
	cap.AssertJSONField(t, "chat_id", float64(testutil.TestChatID))
	cap.AssertJSONField(t, "text", "Hello, World!")
}

func TestExecutor_SendMessage_WithParseMode(t *testing.T) {
	server := testutil.NewMockServer(t)

	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 124)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID:    testutil.TestChatID,
		Text:      "*Bold* and _italic_",
		ParseMode: "Markdown",
	})

	require.NoError(t, err)
	assert.Equal(t, 124, msg.MessageID)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "parse_mode", "Markdown")
}

func TestExecutor_TelegramError_BadRequest(t *testing.T) {
	server := testutil.NewMockServer(t)

	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBadRequest(w, "chat not found")
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, sender.ErrChatNotFound)

	var apiErr *sender.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 400, apiErr.Code)
	assert.Contains(t, apiErr.Description, "chat not found")
}

func TestExecutor_TelegramError_Forbidden(t *testing.T) {
	server := testutil.NewMockServer(t)

	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyForbidden(w, "bot was blocked by the user")
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, sender.ErrBotBlocked)
}

func TestExecutor_TelegramError_NotFound(t *testing.T) {
	server := testutil.NewMockServer(t)

	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyNotFound(w, "message to edit not found")
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, sender.ErrMessageNotFound)
}

func TestExecutor_ContextCancellation(t *testing.T) {
	server := testutil.NewMockServer(t)

	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(5 * time.Second)
		testutil.ReplyMessage(w, 123)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.SendMessage(ctx, sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled),
		"expected context error, got: %v", err)
}

func TestExecutor_NonJSONResponse(t *testing.T) {
	server := testutil.NewMockServer(t)

	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html>Bad Gateway</html>"))
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestExecutor_EditMessageText_Success(t *testing.T) {
	server := testutil.NewMockServer(t)

	server.On("/bot"+testutil.TestToken+"/editMessageText", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 456)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.EditMessageText(context.Background(), sender.EditMessageTextRequest{
		ChatID:    testutil.TestChatID,
		MessageID: 456,
		Text:      "Updated text",
	})

	require.NoError(t, err)
	assert.Equal(t, 456, msg.MessageID)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "message_id", float64(456))
	cap.AssertJSONField(t, "text", "Updated text")
}

func TestExecutor_DeleteMessage_Success(t *testing.T) {
	server := testutil.NewMockServer(t)

	server.On("/bot"+testutil.TestToken+"/deleteMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.DeleteMessage(context.Background(), sender.DeleteMessageRequest{
		ChatID:    testutil.TestChatID,
		MessageID: 789,
	})

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "chat_id", float64(testutil.TestChatID))
	cap.AssertJSONField(t, "message_id", float64(789))
}

func TestExecutor_ForwardMessage_Success(t *testing.T) {
	server := testutil.NewMockServer(t)

	server.On("/bot"+testutil.TestToken+"/forwardMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 999)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.ForwardMessage(context.Background(), sender.ForwardMessageRequest{
		ChatID:     testutil.TestChatID,
		FromChatID: int64(111111),
		MessageID:  222,
	})

	require.NoError(t, err)
	assert.Equal(t, 999, msg.MessageID)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "from_chat_id", float64(111111))
	cap.AssertJSONField(t, "message_id", float64(222))
}

func TestExecutor_CopyMessage_Success(t *testing.T) {
	server := testutil.NewMockServer(t)

	server.On("/bot"+testutil.TestToken+"/copyMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessageID(w, 1001)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msgID, err := client.CopyMessage(context.Background(), sender.CopyMessageRequest{
		ChatID:     testutil.TestChatID,
		FromChatID: int64(111111),
		MessageID:  222,
	})

	require.NoError(t, err)
	assert.Equal(t, 1001, msgID.MessageID)
}

func TestExecutor_AnswerCallbackQuery_Success(t *testing.T) {
	server := testutil.NewMockServer(t)

	server.On("/bot"+testutil.TestToken+"/answerCallbackQuery", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.AnswerCallbackQuery(context.Background(), sender.AnswerCallbackQueryRequest{
		CallbackQueryID: "query_123",
		Text:            "Done!",
	})

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "callback_query_id", "query_123")
	cap.AssertJSONField(t, "text", "Done!")
}

func TestExecutor_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		response func(http.ResponseWriter, *http.Request)
		wantErr  bool
		errCode  int
		sentinel error
	}{
		{
			name: "success",
			response: func(w http.ResponseWriter, r *http.Request) {
				testutil.ReplyMessage(w, 1)
			},
			wantErr: false,
		},
		{
			name: "unauthorized",
			response: func(w http.ResponseWriter, r *http.Request) {
				testutil.ReplyError(w, 401, "Unauthorized", nil)
			},
			wantErr:  true,
			errCode:  401,
			sentinel: sender.ErrUnauthorized,
		},
		{
			name: "bot kicked",
			response: func(w http.ResponseWriter, r *http.Request) {
				testutil.ReplyForbidden(w, "bot was kicked from the group chat")
			},
			wantErr:  true,
			errCode:  403,
			sentinel: sender.ErrBotKicked,
		},
		{
			name: "message not modified",
			response: func(w http.ResponseWriter, r *http.Request) {
				testutil.ReplyBadRequest(w, "message is not modified")
			},
			wantErr:  true,
			errCode:  400,
			sentinel: sender.ErrMessageNotModified,
		},
		{
			name: "not enough rights",
			response: func(w http.ResponseWriter, r *http.Request) {
				testutil.ReplyForbidden(w, "not enough rights to send text messages")
			},
			wantErr:  true,
			errCode:  403,
			sentinel: sender.ErrNoRights,
		},
		{
			name: "user deactivated",
			response: func(w http.ResponseWriter, r *http.Request) {
				testutil.ReplyForbidden(w, "user is deactivated")
			},
			wantErr:  true,
			errCode:  403,
			sentinel: sender.ErrUserDeactivated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/sendMessage", tt.response)

			client := testutil.NewTestClient(t, server.BaseURL())
			_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
				ChatID: testutil.TestChatID,
				Text:   "Test",
			})

			if tt.wantErr {
				require.Error(t, err)
				if tt.sentinel != nil {
					assert.ErrorIs(t, err, tt.sentinel)
				}
				if tt.errCode != 0 {
					var apiErr *sender.APIError
					if errors.As(err, &apiErr) {
						assert.Equal(t, tt.errCode, apiErr.Code)
					}
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestExecutor_RequestCapture(t *testing.T) {
	server := testutil.NewMockServer(t)

	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	// Make multiple requests
	for i := 0; i < 3; i++ {
		client.SendMessage(context.Background(), sender.SendMessageRequest{
			ChatID: testutil.TestChatID,
			Text:   "Message " + string(rune('A'+i)),
		})
	}

	assert.Equal(t, 3, server.CaptureCount())

	// Check each capture
	cap0 := server.CaptureAt(0)
	body0 := cap0.BodyMap(t)
	assert.Equal(t, "Message A", body0["text"])

	cap2 := server.CaptureAt(2)
	body2 := cap2.BodyMap(t)
	assert.Equal(t, "Message C", body2["text"])
}

func TestExecutor_JSONSerialization(t *testing.T) {
	server := testutil.NewMockServer(t)

	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		// Verify the request is valid JSON
		var req map[string]any
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		testutil.ReplyMessage(w, 1)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID:              testutil.TestChatID,
		Text:                "Test with special chars: <>&\"'",
		DisableNotification: true,
		ProtectContent:      true,
	})

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "disable_notification", true)
	cap.AssertJSONField(t, "protect_content", true)
}
