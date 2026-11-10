package sender_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

func TestSendPhoto_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendPhoto", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 123)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendPhoto(context.Background(), sender.SendPhotoRequest{
		ChatID: testutil.TestChatID,
		Photo:  sender.FromURL("https://example.com/photo.jpg"),
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
		Photo:     sender.FromFileID("photo_file_id"),
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
		testutil.ReplyOK(w, true) // the Bot API returns True for an inline message (B-47)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	// Inline message has no chatID
	editable := mockEditable{msgID: "inline_msg_123", chatID: 0}
	msg, err := client.Edit(context.Background(), editable, "New text")

	require.NoError(t, err)
	require.Nil(t, msg)

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

	assert.Equal(t, "galigo: validation: chat_id - must be non-zero", err.Error())
	assert.Equal(t, "chat_id", err.Field)
	assert.Equal(t, "must be non-zero", err.Message)
}

// ================== Bot Identity Methods ==================

func TestGetMe_Success(t *testing.T) {
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

func TestLogOut_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/logOut", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.LogOut(context.Background())

	require.NoError(t, err)
}

func TestCloseBot_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/close", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.CloseBot(context.Background())

	require.NoError(t, err)
}

// ================== Media Methods ==================

func TestSendDocument_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendDocument", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 100)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendDocument(context.Background(), sender.SendDocumentRequest{
		ChatID:   testutil.TestChatID,
		Document: sender.FromFileID("file_id_123"),
	})

	require.NoError(t, err)
	assert.Equal(t, 100, msg.MessageID)
}

func TestSendVideo_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendVideo", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 101)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendVideo(context.Background(), sender.SendVideoRequest{
		ChatID: testutil.TestChatID,
		Video:  sender.FromURL("https://example.com/video.mp4"),
	})

	require.NoError(t, err)
	assert.Equal(t, 101, msg.MessageID)
}

func TestSendAudio_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendAudio", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 102)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendAudio(context.Background(), sender.SendAudioRequest{
		ChatID: testutil.TestChatID,
		Audio:  sender.FromFileID("audio_file_id"),
	})

	require.NoError(t, err)
	assert.Equal(t, 102, msg.MessageID)
}

func TestSendVoice_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendVoice", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 103)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendVoice(context.Background(), sender.SendVoiceRequest{
		ChatID: testutil.TestChatID,
		Voice:  sender.FromFileID("voice_file_id"),
	})

	require.NoError(t, err)
	assert.Equal(t, 103, msg.MessageID)
}

func TestSendAnimation_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendAnimation", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 104)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendAnimation(context.Background(), sender.SendAnimationRequest{
		ChatID:    testutil.TestChatID,
		Animation: sender.FromURL("https://example.com/animation.gif"),
	})

	require.NoError(t, err)
	assert.Equal(t, 104, msg.MessageID)
}

func TestSendVideoNote_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendVideoNote", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 105)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendVideoNote(context.Background(), sender.SendVideoNoteRequest{
		ChatID:    testutil.TestChatID,
		VideoNote: sender.FromFileID("video_note_id"),
	})

	require.NoError(t, err)
	assert.Equal(t, 105, msg.MessageID)
}

func TestSendSticker_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendSticker", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 106)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendSticker(context.Background(), sender.SendStickerRequest{
		ChatID:  testutil.TestChatID,
		Sticker: sender.FromFileID("sticker_file_id"),
	})

	require.NoError(t, err)
	assert.Equal(t, 106, msg.MessageID)
}

func TestSendMediaGroup_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMediaGroup", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, []map[string]any{
			{"message_id": 107, "date": 1234567890},
			{"message_id": 108, "date": 1234567890},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msgs, err := client.SendMediaGroup(context.Background(), sender.SendMediaGroupRequest{
		ChatID: testutil.TestChatID,
		Media: []sender.InputFile{
			sender.FromURL("https://example.com/photo1.jpg").WithMediaType("photo"),
			sender.FromURL("https://example.com/photo2.jpg").WithMediaType("photo"),
		},
	})

	require.NoError(t, err)
	assert.Len(t, msgs, 2)
	assert.Equal(t, 107, msgs[0].MessageID)
	assert.Equal(t, 108, msgs[1].MessageID)
}

// ================== Utility Methods ==================

func TestGetFile_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getFile", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"file_id":        "file_id_123",
			"file_unique_id": "unique_123",
			"file_size":      12345,
			"file_path":      "documents/file.pdf",
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	file, err := client.GetFile(context.Background(), "file_id_123")

	require.NoError(t, err)
	assert.Equal(t, "file_id_123", file.FileID)
	assert.Equal(t, "unique_123", file.FileUniqueID)
	assert.Equal(t, int64(12345), file.FileSize)
	assert.Equal(t, "documents/file.pdf", file.FilePath)
}

func TestSendChatAction_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendChatAction", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SendChatAction(context.Background(), testutil.TestChatID, "typing")

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "action", "typing")
}

func TestGetUserProfilePhotos_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getUserProfilePhotos", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"total_count": 2,
			"photos": [][]map[string]any{
				{{"file_id": "photo1", "width": 100, "height": 100}},
				{{"file_id": "photo2", "width": 100, "height": 100}},
			},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	photos, err := client.GetUserProfilePhotos(context.Background(), 123456)

	require.NoError(t, err)
	assert.Equal(t, 2, photos.TotalCount)
	assert.Len(t, photos.Photos, 2)
}

func TestGetUserProfilePhotos_WithOptions(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getUserProfilePhotos", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"total_count": 1,
			"photos":      [][]map[string]any{},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.GetUserProfilePhotos(context.Background(), 123456,
		sender.WithPhotosOffset(5),
		sender.WithPhotosLimit(3),
	)

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "offset", float64(5))
	cap.AssertJSONField(t, "limit", float64(3))
}

// ================== Profile Audios (Bot API 9.4) ==================

func TestGetUserProfileAudios_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getUserProfileAudios", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"total_count": 1,
			"audios": []map[string]any{
				{"file_id": "audio1", "file_unique_id": "au1", "duration": 120},
			},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	audios, err := client.GetUserProfileAudios(context.Background(), 12345)

	require.NoError(t, err)
	assert.Equal(t, 1, audios.TotalCount)
	require.Len(t, audios.Audios, 1)
	assert.Equal(t, "audio1", audios.Audios[0].FileID)
}

func TestGetUserProfileAudios_WithOptions(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getUserProfileAudios", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"total_count": 5,
			"audios":      []map[string]any{},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.GetUserProfileAudios(context.Background(), 12345,
		sender.WithAudiosOffset(10),
		sender.WithAudiosLimit(50),
	)

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "offset", float64(10))
	cap.AssertJSONField(t, "limit", float64(50))
}

// Bot API 10.0 getUserPersonalChatMessages (B-25 slice 2a, TEST-PLAN §3).
func TestAPI100_GetUserPersonalChatMessages(t *testing.T) {
	// API100 T-s-13
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getUserPersonalChatMessages", func(w http.ResponseWriter, _ *http.Request) {
		api96ReplyRaw(w, `{"ok":true,"result":[`+
			`{"message_id":301,"date":1793799001,"chat":{"id":-1007770001111,"type":"channel","title":"Personal"},"text":"first"},`+
			`{"message_id":302,"date":1793799002,"chat":{"id":-1007770001111,"type":"channel","title":"Personal"},"text":"second"}]}`)
	})
	client := testutil.NewTestClient(t, server.BaseURL())

	got, err := client.GetUserPersonalChatMessages(context.Background(), sender.GetUserPersonalChatMessagesRequest{UserID: 42, Limit: 5})
	require.NoError(t, err)
	ch := &tg.Chat{ID: -1007770001111, Type: "channel", Title: "Personal"}
	assert.Equal(t, []*tg.Message{
		{MessageID: 301, Date: 1793799001, Chat: ch, Text: "first"},
		{MessageID: 302, Date: 1793799002, Chat: ch, Text: "second"},
	}, got)
	require.Equal(t, 1, server.CaptureCount())
	server.LastCapture().AssertPath(t, "/bot"+testutil.TestToken+"/getUserPersonalChatMessages")
	assert.JSONEq(t, `{"user_id":42,"limit":5}`, server.LastCapture().BodyString())

	// the spec's limit range 1-20 is not checked by the client: 0 and 21 are sent as given
	for _, limit := range []int{0, 21} {
		server.ResetCaptures()
		_, err := client.GetUserPersonalChatMessages(context.Background(), sender.GetUserPersonalChatMessagesRequest{UserID: 42, Limit: limit})
		require.NoError(t, err)
		require.Equal(t, 1, server.CaptureCount())
		assert.JSONEq(t, fmt.Sprintf(`{"user_id":42,"limit":%d}`, limit), server.LastCapture().BodyString())
	}

	for _, id := range []int64{0, -1} {
		server.ResetCaptures()
		_, err := client.GetUserPersonalChatMessages(context.Background(), sender.GetUserPersonalChatMessagesRequest{UserID: id, Limit: 5})
		var ve *tg.ValidationError
		require.True(t, errors.As(err, &ve), "user %d: error %v", id, err)
		assert.Equal(t, "user_id", ve.Field)
		assert.Equal(t, "galigo: validation: user_id - must be positive", err.Error()) // the whole text (A-C1-02)
		assert.Equal(t, 0, server.CaptureCount())
	}

	t.Run("malformed result", func(t *testing.T) {
		bad := testutil.NewMockServer(t)
		bad.On("/bot"+testutil.TestToken+"/getUserPersonalChatMessages", func(w http.ResponseWriter, _ *http.Request) {
			api96ReplyRaw(w, `{"ok":true,"result":"x"}`)
		})
		_, err := testutil.NewTestClient(t, bad.BaseURL()).GetUserPersonalChatMessages(context.Background(),
			sender.GetUserPersonalChatMessagesRequest{UserID: 42, Limit: 5})
		// a malformed result: callJSON's wrap names the method and keeps the decode error (A-C1-03)
		require.Error(t, err)
		assert.True(t, strings.HasPrefix(err.Error(), "galigo: getUserPersonalChatMessages: failed to parse response: "), err.Error())
		var ute *json.UnmarshalTypeError
		assert.True(t, errors.As(err, &ute), "error %v", err)
	})
}

// ================== Location/Contact Methods ==================

func TestSendLocation_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendLocation", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 110)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendLocation(context.Background(), sender.SendLocationRequest{
		ChatID:    testutil.TestChatID,
		Latitude:  40.7128,
		Longitude: -74.0060,
	})

	require.NoError(t, err)
	assert.Equal(t, 110, msg.MessageID)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "latitude", 40.7128)
	cap.AssertJSONField(t, "longitude", -74.0060)
}

func TestSendVenue_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendVenue", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 111)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendVenue(context.Background(), sender.SendVenueRequest{
		ChatID:    testutil.TestChatID,
		Latitude:  40.7128,
		Longitude: -74.0060,
		Title:     "Statue of Liberty",
		Address:   "New York, NY",
	})

	require.NoError(t, err)
	assert.Equal(t, 111, msg.MessageID)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "title", "Statue of Liberty")
	cap.AssertJSONField(t, "address", "New York, NY")
}

func TestSendContact_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendContact", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 112)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendContact(context.Background(), sender.SendContactRequest{
		ChatID:      testutil.TestChatID,
		PhoneNumber: "+1234567890",
		FirstName:   "John",
		LastName:    "Doe",
	})

	require.NoError(t, err)
	assert.Equal(t, 112, msg.MessageID)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "phone_number", "+1234567890")
	cap.AssertJSONField(t, "first_name", "John")
}

func TestSendPoll_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendPoll", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 113)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendPoll(context.Background(), sender.SendPollRequest{
		ChatID:   testutil.TestChatID,
		Question: "What's your favorite color?",
		Options: []sender.InputPollOption{
			{Text: "Red"},
			{Text: "Blue"},
			{Text: "Green"},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, 113, msg.MessageID)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "question", "What's your favorite color?")
}

func TestSendDice_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendDice", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 114)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendDice(context.Background(), testutil.TestChatID)

	require.NoError(t, err)
	assert.Equal(t, 114, msg.MessageID)
}

func TestSendDice_WithEmoji(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendDice", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 115)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.SendDice(context.Background(), testutil.TestChatID,
		sender.WithDiceEmoji("\U0001F3B2"), // 🎲
	)

	require.NoError(t, err)
	assert.Equal(t, 115, msg.MessageID)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "emoji", "\U0001F3B2")
}

// ================== Bulk Operations ==================

func TestForwardMessages_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/forwardMessages", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, []map[string]any{
			{"message_id": 200},
			{"message_id": 201},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	ids, err := client.ForwardMessages(context.Background(), sender.ForwardMessagesRequest{
		ChatID:     testutil.TestChatID,
		FromChatID: int64(111),
		MessageIDs: []int{1, 2, 3},
	})

	require.NoError(t, err)
	assert.Len(t, ids, 2)
	assert.Equal(t, 200, ids[0].MessageID)
	assert.Equal(t, 201, ids[1].MessageID)
}

func TestCopyMessages_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/copyMessages", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, []map[string]any{
			{"message_id": 300},
			{"message_id": 301},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	ids, err := client.CopyMessages(context.Background(), sender.CopyMessagesRequest{
		ChatID:     testutil.TestChatID,
		FromChatID: int64(222),
		MessageIDs: []int{10, 20},
	})

	require.NoError(t, err)
	assert.Len(t, ids, 2)
	assert.Equal(t, 300, ids[0].MessageID)
}

func TestDeleteMessages_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/deleteMessages", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.DeleteMessages(context.Background(), testutil.TestChatID, []int{1, 2, 3})

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONFieldExists(t, "message_ids")
}

func TestSetMessageReaction_Success(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setMessageReaction", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBool(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetMessageReaction(context.Background(), sender.SetMessageReactionRequest{
		ChatID:    testutil.TestChatID,
		MessageID: 123,
		Reaction: []sender.ReactionType{
			{Type: "emoji", Emoji: "\U0001F44D"}, // 👍
		},
	})

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONField(t, "message_id", float64(123))
}

// ================== LinkPreviewOptions Tests ==================

func TestSendMessage_WithLinkPreviewOptions(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Check this link https://example.com",
		LinkPreviewOptions: &tg.LinkPreviewOptions{
			URL:              "https://example.com",
			PreferLargeMedia: true,
			ShowAboveText:    true,
		},
	})

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONFieldExists(t, "link_preview_options")
}

func TestSendMessage_LinkPreviewOptions_MutuallyExclusiveError(t *testing.T) {
	// No server needed - validation fails before request
	client := testutil.NewTestClient(t, "http://localhost:9999")

	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Test",
		LinkPreviewOptions: &tg.LinkPreviewOptions{
			PreferSmallMedia: true,
			PreferLargeMedia: true, // Invalid: mutually exclusive
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestEditMessageText_WithLinkPreviewOptions(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/editMessageText", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.EditMessageText(context.Background(), sender.EditMessageTextRequest{
		ChatID:    testutil.TestChatID,
		MessageID: 1,
		Text:      "Updated with link https://example.com",
		LinkPreviewOptions: &tg.LinkPreviewOptions{
			IsDisabled: true,
		},
	})

	require.NoError(t, err)

	cap := server.LastCapture()
	cap.AssertJSONFieldExists(t, "link_preview_options")
}

func TestEditMessageText_LinkPreviewOptions_MutuallyExclusiveError(t *testing.T) {
	// No server needed - validation fails before request
	client := testutil.NewTestClient(t, "http://localhost:9999")

	_, err := client.EditMessageText(context.Background(), sender.EditMessageTextRequest{
		ChatID:    testutil.TestChatID,
		MessageID: 1,
		Text:      "Test",
		LinkPreviewOptions: &tg.LinkPreviewOptions{
			PreferSmallMedia: true,
			PreferLargeMedia: true, // Invalid: mutually exclusive
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

// T-s-9 (B-25 slice 1, Bot API 9.6): SendMessageRequest.ReplyParameters is sent as reply_parameters, with
// poll_option_id; absent when not set.
func TestSendMessage96_ReplyParameters(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 4242)
	})
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: int64(-100123), Text: "hi", ReplyParameters: &tg.ReplyParameters{MessageID: 5, PollOptionID: "1"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, server.CaptureCount())
	assert.JSONEq(t, `{"chat_id":-100123,"text":"hi","reply_parameters":{"message_id":5,"poll_option_id":"1"}}`,
		server.LastCapture().BodyString())

	_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: int64(-100123), Text: "hi"})
	require.NoError(t, err)
	require.Equal(t, 2, server.CaptureCount())
	assert.JSONEq(t, `{"chat_id":-100123,"text":"hi"}`, server.LastCapture().BodyString())
}
