package sender_test

// Bot API 10.0 part b: sendLivePhoto (B-25 10.0b, TEST-PLAN §3, rows T-s-1 … T-s-8, T-s-20, T-s-21). Every request
// row decodes the request as the server reads it (wireCall) and compares the whole argument map with literals.

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// api100bProbe01 is the probe 01 reply of 2026-11-05 (E9/lookups/probe100b/01-sendLivePhoto.json) with the same
// edits as tg/api100_test.go: testutil bot and chat, synthetic file ids; the rest byte-exact.
const api100bProbe01 = `{"message_id":1047,"from":{"id":123456789,"is_bot":true,"first_name":"Test Bot","username":"testbot"},` +
	`"chat":{"id":123456789,"title":"Test Chat","is_forum":true,"type":"supergroup"},"date":1793904606,` +
	`"live_photo":{"photo":[{"file_id":"LPS90-file","file_unique_id":"LPS90-uniq","file_size":341,"width":90,"height":90},` +
	`{"file_id":"LPS100-file","file_unique_id":"LPS100-uniq","file_size":361,"width":100,"height":100}],` +
	`"duration":1,"width":320,"height":240,"mime_type":"video/mp4","file_id":"LPV-file","file_unique_id":"LPV-uniq","file_size":663},` +
	`"photo":[{"file_id":"LPS90-file","file_unique_id":"LPS90-uniq","file_size":341,"width":90,"height":90},` +
	`{"file_id":"LPS100-file","file_unique_id":"LPS100-uniq","file_size":361,"width":100,"height":100}],` +
	`"caption":"probe100b upload"}`

func api100bWantProbe01() *tg.Message {
	sizes := []tg.PhotoSize{
		{FileID: "LPS90-file", FileUniqueID: "LPS90-uniq", Width: 90, Height: 90, FileSize: 341},
		{FileID: "LPS100-file", FileUniqueID: "LPS100-uniq", Width: 100, Height: 100, FileSize: 361},
	}
	return &tg.Message{
		MessageID: 1047, Date: 1793904606, Caption: "probe100b upload",
		From:  &tg.User{ID: testutil.TestBotID, IsBot: true, FirstName: "Test Bot", Username: testutil.TestBotUsername},
		Chat:  &tg.Chat{ID: testutil.TestChatID, Title: "Test Chat", IsForum: true, Type: "supergroup"},
		Photo: sizes,
		LivePhoto: &tg.LivePhoto{
			Photo: []tg.PhotoSize{sizes[0], sizes[1]}, FileID: "LPV-file", FileUniqueID: "LPV-uniq",
			Width: 320, Height: 240, Duration: 1, MimeType: "video/mp4", FileSize: 663,
		},
	}
}

func replyProbe01(w http.ResponseWriter, _ *http.Request) {
	api96ReplyRaw(w, `{"ok":true,"result":`+api100bProbe01+`}`)
}

var (
	lpVideoBytes = []byte("live-video-bytes")
	lpPhotoBytes = []byte("static-photo-bytes")
)

func sendLive(req sender.SendLivePhotoRequest) func(ctx context.Context, c *sender.Client) error {
	return func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendLivePhoto(ctx, req)
		return err
	}
}

func TestSendLivePhoto_FullByFileID(t *testing.T) { // API100B T-s-1
	mediaType, args, files, err := wireCall(t, "sendLivePhoto", replyMessage, sendLive(sender.SendLivePhotoRequest{
		BusinessConnectionID:  "bc-1",
		ChatID:                int64(-1001234567890),
		MessageThreadID:       7,
		DirectMessagesTopicID: 9,
		LivePhoto:             sender.FromFileID("V1"),
		Photo:                 sender.FromFileID("P1"),
		Caption:               "c",
		ParseMode:             "HTML",
		CaptionEntities:       []tg.MessageEntity{{Type: "bold", Offset: 0, Length: 1}},
		ShowCaptionAboveMedia: true,
		HasSpoiler:            true,
		DisableNotification:   true,
		ProtectContent:        true,
		AllowPaidBroadcast:    true,
		MessageEffectID:       "e-1",
		ReplyParameters:       &tg.ReplyParameters{MessageID: 5},
		ReplyMarkup: &tg.InlineKeyboardMarkup{InlineKeyboard: [][]tg.InlineKeyboardButton{
			{{Text: "b", CallbackData: "cb"}},
		}},
	}))
	require.NoError(t, err)
	assert.Equal(t, "application/json", mediaType)
	assertArgs(t, map[string]string{
		"business_connection_id":   "bc-1",
		"chat_id":                  "-1001234567890",
		"message_thread_id":        "7",
		"direct_messages_topic_id": "9",
		"live_photo":               "V1",
		"photo":                    "P1",
		"caption":                  "c",
		"parse_mode":               "HTML",
		"caption_entities":         `[{"type":"bold","offset":0,"length":1}]`,
		"show_caption_above_media": "true",
		"has_spoiler":              "true",
		"disable_notification":     "true",
		"protect_content":          "true",
		"allow_paid_broadcast":     "true",
		"message_effect_id":        "e-1",
		"reply_parameters":         `{"message_id":5}`,
		"reply_markup":             `{"inline_keyboard":[[{"text":"b","callback_data":"cb"}]]}`,
	}, args)
	assertFiles(t, map[string]wireFile{}, files)
}

func TestSendLivePhoto_Minimal(t *testing.T) { // API100B T-s-2
	mediaType, args, files, err := wireCall(t, "sendLivePhoto", replyMessage, sendLive(sender.SendLivePhotoRequest{
		ChatID: testutil.TestChatID, LivePhoto: sender.FromFileID("V1"), Photo: sender.FromFileID("P1"),
	}))
	require.NoError(t, err)
	assert.Equal(t, "application/json", mediaType)
	assertArgs(t, map[string]string{"chat_id": wireChatID, "live_photo": "V1", "photo": "P1"}, args)
	assertFiles(t, map[string]wireFile{}, files)
}

func TestSendLivePhoto_TwoUploads(t *testing.T) { // API100B T-s-3
	mediaType, args, files, err := wireCall(t, "sendLivePhoto", replyMessage, sendLive(sender.SendLivePhotoRequest{
		ChatID:          testutil.TestChatID,
		LivePhoto:       sender.FromBytes(lpVideoBytes, "v.mp4"),
		Photo:           sender.FromBytes(lpPhotoBytes, "p.jpg"),
		Caption:         "c",
		ReplyParameters: &tg.ReplyParameters{MessageID: 5},
		CaptionEntities: []tg.MessageEntity{{Type: "italic", Offset: 0, Length: 1}},
	}))
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	assertArgs(t, map[string]string{
		"chat_id":          wireChatID,
		"caption":          "c",
		"reply_parameters": `{"message_id":5}`,
		"caption_entities": `[{"type":"italic","offset":0,"length":1}]`,
	}, args)
	assertFiles(t, map[string]wireFile{
		"live_photo": {FileName: "v.mp4", Data: lpVideoBytes},
		"photo":      {FileName: "p.jpg", Data: lpPhotoBytes},
	}, files)
}

func TestSendLivePhoto_MixedInputs(t *testing.T) { // API100B T-s-4
	t.Run("video upload, photo by id", func(t *testing.T) {
		mediaType, args, files, err := wireCall(t, "sendLivePhoto", replyMessage, sendLive(sender.SendLivePhotoRequest{
			ChatID: testutil.TestChatID, LivePhoto: sender.FromBytes(lpVideoBytes, "v.mp4"), Photo: sender.FromFileID("P1"),
		}))
		require.NoError(t, err)
		assert.Equal(t, "multipart/form-data", mediaType)
		assertArgs(t, map[string]string{"chat_id": wireChatID, "photo": "P1"}, args)
		assertFiles(t, map[string]wireFile{"live_photo": {FileName: "v.mp4", Data: lpVideoBytes}}, files)
	})
	t.Run("video by id, photo upload", func(t *testing.T) {
		mediaType, args, files, err := wireCall(t, "sendLivePhoto", replyMessage, sendLive(sender.SendLivePhotoRequest{
			ChatID: testutil.TestChatID, LivePhoto: sender.FromFileID("V1"), Photo: sender.FromBytes(lpPhotoBytes, "p.jpg"),
		}))
		require.NoError(t, err)
		assert.Equal(t, "multipart/form-data", mediaType)
		assertArgs(t, map[string]string{"chat_id": wireChatID, "live_photo": "V1"}, args)
		assertFiles(t, map[string]wireFile{"photo": {FileName: "p.jpg", Data: lpPhotoBytes}}, files)
	})
}

func TestSendLivePhoto_StringChatID(t *testing.T) { // API100B T-s-5
	mediaType, args, _, err := wireCall(t, "sendLivePhoto", replyMessage, sendLive(sender.SendLivePhotoRequest{
		ChatID: "@chan", LivePhoto: sender.FromFileID("V1"), Photo: sender.FromFileID("P1"),
	}))
	require.NoError(t, err)
	assert.Equal(t, "application/json", mediaType)
	assertArgs(t, map[string]string{"chat_id": "@chan", "live_photo": "V1", "photo": "P1"}, args)
}

func TestSendLivePhoto_DecodesReply(t *testing.T) { // API100B T-s-6
	var got *tg.Message
	_, _, _, err := wireCall(t, "sendLivePhoto", replyProbe01, func(ctx context.Context, c *sender.Client) error {
		var err error
		got, err = c.SendLivePhoto(ctx, sender.SendLivePhotoRequest{
			ChatID: testutil.TestChatID, LivePhoto: sender.FromFileID("V1"), Photo: sender.FromFileID("P1"),
		})
		return err
	})
	require.NoError(t, err)
	assert.Equal(t, api100bWantProbe01(), got)
}

func TestSendLivePhoto_Validation(t *testing.T) { // API100B T-s-7
	p := sender.FromFileID("P1")
	rows := []struct {
		name  string
		req   sender.SendLivePhotoRequest
		field string // a *tg.ValidationError field, or "" for a chat id error
		text  string
	}{
		{"live_photo empty", sender.SendLivePhotoRequest{ChatID: testutil.TestChatID, Photo: p},
			"live_photo", "galigo: validation: live_photo - required"},
		{"photo empty", sender.SendLivePhotoRequest{ChatID: testutil.TestChatID, LivePhoto: sender.FromFileID("V1")},
			"photo", "galigo: validation: photo - required"},
		{"live_photo outer empty, only its Photo set", sender.SendLivePhotoRequest{
			ChatID: testutil.TestChatID, LivePhoto: sender.InputFile{Photo: &p}, Photo: p},
			"live_photo", "galigo: validation: live_photo - required"},
		{"chat id nil", sender.SendLivePhotoRequest{LivePhoto: sender.FromFileID("V1"), Photo: p},
			"", "galigo: chat_id is required"},
		{"chat id empty string", sender.SendLivePhotoRequest{ChatID: "", LivePhoto: sender.FromFileID("V1"), Photo: p},
			"", "galigo: chat_id cannot be empty string"},
		{"chat id zero", sender.SendLivePhotoRequest{ChatID: int64(0), LivePhoto: sender.FromFileID("V1"), Photo: p},
			"", "galigo: chat_id cannot be zero"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			client := testutil.NewTestClient(t, server.BaseURL())
			msg, err := client.SendLivePhoto(context.Background(), r.req)
			require.Error(t, err)
			assert.Nil(t, msg)
			assert.Equal(t, r.text, err.Error())
			if r.field != "" {
				var ve *tg.ValidationError
				require.True(t, errors.As(err, &ve), "want *tg.ValidationError, got %T", err)
				assert.Equal(t, r.field, ve.Field)
			}
			assert.Equal(t, 0, server.CaptureCount(), "no request sent")
		})
	}
}

func TestSendLivePhoto_NoRetry(t *testing.T) { // API100B T-s-8
	rows := []struct {
		name  string
		first func(w http.ResponseWriter)
	}{
		{"500", func(w http.ResponseWriter) {
			testutil.ReplyServerError(w, http.StatusInternalServerError, "Internal Server Error")
		}},
		{"429 retry_after 1", func(w http.ResponseWriter) { testutil.ReplyRateLimit(w, 1) }},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			var calls atomic.Int32
			server.On("/bot"+testutil.TestToken+"/sendLivePhoto", func(w http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 {
					r.first(w)
					return
				}
				testutil.ReplyMessage(w, 1)
			})
			sleeper := &testutil.FakeSleeper{}
			client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper)
			_, err := client.SendLivePhoto(context.Background(), sender.SendLivePhotoRequest{
				ChatID: testutil.TestChatID, LivePhoto: sender.FromFileID("V1"), Photo: sender.FromFileID("P1"),
			})
			require.Error(t, err)
			var apiErr *tg.APIError
			assert.True(t, errors.As(err, &apiErr), "the API error is returned, got %v", err)
			assert.Equal(t, 1, server.CaptureCount(), "exactly one request: no retry")
			assert.Empty(t, sleeper.Calls(), "no backoff sleep")
		})
	}
}

func TestSendLivePhoto_URLNotRejected(t *testing.T) { // API100B T-s-20
	t.Run("both by URL", func(t *testing.T) {
		var got *tg.Message
		mediaType, args, files, err := wireCall(t, "sendLivePhoto", replyMessage, func(ctx context.Context, c *sender.Client) error {
			var err error
			got, err = c.SendLivePhoto(ctx, sender.SendLivePhotoRequest{
				ChatID: testutil.TestChatID, LivePhoto: sender.FromURL("https://x/v.mp4"), Photo: sender.FromURL("https://x/p.jpg"),
			})
			return err
		})
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, 42, got.MessageID)
		assert.Equal(t, "application/json", mediaType)
		assertArgs(t, map[string]string{"chat_id": wireChatID, "live_photo": "https://x/v.mp4", "photo": "https://x/p.jpg"}, args)
		assertFiles(t, map[string]wireFile{}, files)
	})
	t.Run("video upload, photo by URL", func(t *testing.T) {
		mediaType, args, files, err := wireCall(t, "sendLivePhoto", replyMessage, sendLive(sender.SendLivePhotoRequest{
			ChatID: testutil.TestChatID, LivePhoto: sender.FromBytes(lpVideoBytes, "v.mp4"), Photo: sender.FromURL("https://x/p2.jpg"),
		}))
		require.NoError(t, err)
		assert.Equal(t, "multipart/form-data", mediaType)
		assertArgs(t, map[string]string{"chat_id": wireChatID, "photo": "https://x/p2.jpg"}, args)
		assertFiles(t, map[string]wireFile{"live_photo": {FileName: "v.mp4", Data: lpVideoBytes}}, files)
	})
}

func TestSendLivePhoto_LimiterKey(t *testing.T) { // API100B T-s-21
	api100LimiterRow(t, "sendLivePhoto", `{"message_id":1,"date":1,"chat":{"id":-1001234567890,"type":"supergroup"}}`,
		func(c *sender.Client, chat tg.ChatID) error {
			_, err := c.SendLivePhoto(context.Background(), sender.SendLivePhotoRequest{
				ChatID: chat, LivePhoto: sender.FromFileID("V1"), Photo: sender.FromFileID("P1"),
			})
			return err
		})
}
