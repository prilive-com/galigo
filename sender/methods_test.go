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

func TestSendPhoto_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendPhoto", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 123)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendPhoto(context.Background(), sender.SendPhotoRequest{
		ChatID: testutil.TestChatID,
		Photo:  "https://example.com/photo.jpg",
	})

	require.NoError(t, err)
	assert.Equal(t, 123, msg.MessageID)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "chat_id", float64(testutil.TestChatID))
	cap.AssertJSONField(t, "photo", "https://example.com/photo.jpg")
}

func TestSendPhoto_WithCaption(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendPhoto", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 124)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendPhoto(context.Background(), sender.SendPhotoRequest{
		ChatID:    testutil.TestChatID,
		Photo:     "photo_file_id",
		Caption:   "Nice photo!",
		ParseMode: "HTML",
	})

	require.NoError(t, err)
	assert.Equal(t, 124, msg.MessageID)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "caption", "Nice photo!")
	cap.AssertJSONField(t, "parse_mode", "HTML")
}

func TestEditMessageCaption_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/editMessageCaption", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 456)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.EditMessageCaption(context.Background(), sender.EditMessageCaptionRequest{
		ChatID:    testutil.TestChatID,
		MessageID: 456,
		Caption:   "Updated caption",
	})

	require.NoError(t, err)
	assert.Equal(t, 456, msg.MessageID)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "caption", "Updated caption")
}

func TestEditMessageReplyMarkup_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/editMessageReplyMarkup", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 789)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.EditMessageReplyMarkup(context.Background(), sender.EditMessageReplyMarkupRequest{
		ChatID:    testutil.TestChatID,
		MessageID: 789,
	})

	require.NoError(t, err)
	assert.Equal(t, 789, msg.MessageID)
}

// Convenience method tests using Editable interface

type mockEditable struct {
	msgID  string
	chatID int64
}

func (m mockEditable) MessageSig() (string, int64) {
	return m.msgID, m.chatID
}

func TestEdit_WithEditable(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/editMessageText", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 100)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	editable := mockEditable{msgID: "100", chatID: testutil.TestChatID}
	msg, err := client.Edit(context.Background(), editable, "New text")

	require.NoError(t, err)
	assert.Equal(t, 100, msg.MessageID)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "text", "New text")
	cap.AssertJSONField(t, "message_id", float64(100))
}

func TestEdit_InlineMessage(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/editMessageText", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 0) // Inline messages return empty
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	// Inline message has no chatID
	editable := mockEditable{msgID: "inline_msg_123", chatID: 0}
	_, err := client.Edit(context.Background(), editable, "New text")

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "inline_message_id", "inline_msg_123")
	cap.AssertJSONFieldAbsent(t, "chat_id")
}

func TestDelete_WithEditable(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/deleteMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	editable := mockEditable{msgID: "200", chatID: testutil.TestChatID}
	err := client.Delete(context.Background(), editable)

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "message_id", float64(200))
}

func TestDelete_InlineMessage_Error(t *testing.T) {
	client := testutil.NewTestClient(t, "http://localhost:9999") // Won't be called

	// Inline message has no chatID - cannot delete
	editable := mockEditable{msgID: "inline_msg", chatID: 0}
	err := client.Delete(context.Background(), editable)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot delete inline messages")
}

func TestForward_WithEditable(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/forwardMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 300)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	editable := mockEditable{msgID: "100", chatID: int64(111)}
	msg, err := client.Forward(context.Background(), editable, int64(222))

	require.NoError(t, err)
	assert.Equal(t, 300, msg.MessageID)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "from_chat_id", float64(111))
	cap.AssertJSONField(t, "chat_id", float64(222))
	cap.AssertJSONField(t, "message_id", float64(100))
}

func TestForward_InlineMessage_Error(t *testing.T) {
	client := testutil.NewTestClient(t, "http://localhost:9999")

	editable := mockEditable{msgID: "inline_msg", chatID: 0}
	_, err := client.Forward(context.Background(), editable, int64(123))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot forward inline messages")
}

func TestCopy_WithEditable(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/copyMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessageID(w, 400)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	editable := mockEditable{msgID: "100", chatID: int64(111)}
	msgID, err := client.Copy(context.Background(), editable, int64(222))

	require.NoError(t, err)
	assert.Equal(t, 400, msgID.MessageID)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "from_chat_id", float64(111))
	cap.AssertJSONField(t, "chat_id", float64(222))
}

func TestCopy_InlineMessage_Error(t *testing.T) {
	client := testutil.NewTestClient(t, "http://localhost:9999")

	editable := mockEditable{msgID: "inline_msg", chatID: 0}
	_, err := client.Copy(context.Background(), editable, int64(123))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot copy inline messages")
}

func TestAnswer_CallbackQuery(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/answerCallbackQuery", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	cb := &tg.CallbackQuery{
		ID:   "callback_123",
		Data: "button_data",
	}

	err := client.Answer(context.Background(), cb, sender.AnswerText("Done!"))

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "callback_query_id", "callback_123")
	cap.AssertJSONField(t, "text", "Done!")
}

func TestAnswer_WithAlert(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/answerCallbackQuery", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	cb := &tg.CallbackQuery{ID: "callback_456"}

	err := client.Answer(context.Background(), cb,
		sender.AnswerText("Alert!"),
		sender.Alert(),
	)

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "show_alert", true)
}

func TestAcknowledge_CallbackQuery(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/answerCallbackQuery", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	cb := &tg.CallbackQuery{ID: "callback_789"}

	err := client.Acknowledge(context.Background(), cb)

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "callback_query_id", "callback_789")
	// No text - silent acknowledgement
	cap.AssertJSONFieldAbsent(t, "text")
}

// Test answer options

func TestAnswerOption_WithURL(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/answerCallbackQuery", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	cb := &tg.CallbackQuery{ID: "cb"}
	err := client.Answer(context.Background(), cb, sender.WithAnswerURL("https://example.com"))

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "url", "https://example.com")
}

func TestAnswerOption_WithCacheTime(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/answerCallbackQuery", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	cb := &tg.CallbackQuery{ID: "cb"}
	err := client.Answer(context.Background(), cb, sender.WithAnswerCacheTime(60))

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "cache_time", float64(60))
}

// Test edit options

func TestEditOption_WithParseMode(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/editMessageText", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	editable := mockEditable{msgID: "1", chatID: testutil.TestChatID}
	_, err := client.Edit(context.Background(), editable, "<b>Bold</b>",
		sender.WithEditParseMode("HTML"),
	)

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "parse_mode", "HTML")
}

func TestEditOption_DisableWebPreview(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/editMessageText", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	editable := mockEditable{msgID: "1", chatID: testutil.TestChatID}
	_, err := client.Edit(context.Background(), editable, "Check https://example.com",
		sender.WithDisableWebPreview(true),
	)

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "disable_web_page_preview", true)
}

// Test copy options

func TestCopyOption_WithCaption(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/copyMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessageID(w, 1)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	editable := mockEditable{msgID: "1", chatID: int64(111)}
	_, err := client.Copy(context.Background(), editable, int64(222),
		sender.WithCopyCaption("New caption"),
		sender.WithCopyParseMode("Markdown"),
	)

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "caption", "New caption")
	cap.AssertJSONField(t, "parse_mode", "Markdown")
}

func TestCopyOption_Silent(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/copyMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessageID(w, 1)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	editable := mockEditable{msgID: "1", chatID: int64(111)}
	_, err := client.Copy(context.Background(), editable, int64(222),
		sender.CopySilent(),
	)

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "disable_notification", true)
}

func TestCopyOption_Protected(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/copyMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessageID(w, 1)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	editable := mockEditable{msgID: "1", chatID: int64(111)}
	_, err := client.Copy(context.Background(), editable, int64(222),
		sender.CopyProtected(),
	)

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "protect_content", true)
}

// Test SendMessage options

func TestSendMessage_Silent(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID:              testutil.TestChatID,
		Text:                "Silent message",
		DisableNotification: true,
	})

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "disable_notification", true)
}

func TestSendMessage_Protected(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID:         testutil.TestChatID,
		Text:           "Protected message",
		ProtectContent: true,
	})

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "protect_content", true)
}

// Test uncovered options

func TestForwardOption_Silent(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/forwardMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	editable := mockEditable{msgID: "1", chatID: int64(111)}
	_, err := client.Forward(context.Background(), editable, int64(222),
		sender.Silent(),
	)

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "disable_notification", true)
}

func TestForwardOption_Protected(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/forwardMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	editable := mockEditable{msgID: "1", chatID: int64(111)}
	_, err := client.Forward(context.Background(), editable, int64(222),
		sender.Protected(),
	)

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "protect_content", true)
}

func TestEditOption_WithKeyboard(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/editMessageText", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	editable := mockEditable{msgID: "1", chatID: int64(111)}
	keyboard := &tg.InlineKeyboardMarkup{
		InlineKeyboard: [][]tg.InlineKeyboardButton{
			{{Text: "Button", CallbackData: "data"}},
		},
	}

	_, err := client.Edit(context.Background(), editable, "New text",
		sender.WithEditKeyboard(keyboard),
	)

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONFieldExists(t, "reply_markup")
}

func TestCopyOption_WithReply(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/copyMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessageID(w, 1)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	editable := mockEditable{msgID: "1", chatID: int64(111)}
	_, err := client.Copy(context.Background(), editable, int64(222),
		sender.WithCopyReply(999),
	)

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "reply_to_message_id", float64(999))
}

func TestCopyOption_WithKeyboard(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/copyMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessageID(w, 1)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	editable := mockEditable{msgID: "1", chatID: int64(111)}
	keyboard := &tg.InlineKeyboardMarkup{
		InlineKeyboard: [][]tg.InlineKeyboardButton{
			{{Text: "Button", CallbackData: "data"}},
		},
	}

	_, err := client.Copy(context.Background(), editable, int64(222),
		sender.WithCopyKeyboard(keyboard),
	)

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONFieldExists(t, "reply_markup")
}

// Test ValidationError

func TestValidationError(t *testing.T) {
	err := sender.NewValidationError("chat_id", "must be non-zero")

	assert.Equal(t, "galigo/sender: validation: chat_id - must be non-zero", err.Error())
	assert.Equal(t, "chat_id", err.Field)
	assert.Equal(t, "must be non-zero", err.Message)
}
