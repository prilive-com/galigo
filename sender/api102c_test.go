package sender_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.2 part c: the five ephemeral message methods (B-25 10.2c, TEST-PLAN §2, rows T-txt-1 … T-shape).
//
// Every id has a distinct value: a group chat id, a receiver id above 2^32 and an ephemeral message id. Request
// bodies are compared whole, with literal JSON or a literal argument map.
const (
	api102cChat = int64(-1001234567890)
	api102cRecv = int64(5000000001)
	api102cEph  = 4242
	// api102cBody is the JSON of the three common fields.
	api102cBody = `"chat_id":-1001234567890,"receiver_user_id":5000000001,"ephemeral_message_id":4242`
)

func api102cPath(method string) string { return "/bot" + testutil.TestToken + "/" + method }

func api102cOK(server *testutil.MockTelegramServer, methods ...string) {
	for _, m := range methods {
		server.On(api102cPath(m), func(w http.ResponseWriter, _ *http.Request) {
			api96ReplyRaw(w, `{"ok":true,"result":true}`)
		})
	}
}

func api102cKeyboard() *tg.InlineKeyboardMarkup {
	return &tg.InlineKeyboardMarkup{InlineKeyboard: [][]tg.InlineKeyboardButton{{{Text: "B", CallbackData: "d"}}}}
}

const api102cKeyboardJSON = `{"inline_keyboard":[[{"text":"B","callback_data":"d"}]]}`

// api102cTarget is one ephemeral message target.
type api102cTarget struct {
	chat tg.ChatID
	recv int64
	eph  int
}

var api102cValid = api102cTarget{api102cChat, api102cRecv, api102cEph}

// api102cMethods calls each new method with the target and the smallest valid request.
var api102cMethods = []struct {
	method string
	call   func(context.Context, *sender.Client, api102cTarget) error
}{
	{"editEphemeralMessageText", func(ctx context.Context, c *sender.Client, x api102cTarget) error {
		return c.EditEphemeralMessageText(ctx, sender.EditEphemeralMessageTextRequest{
			ChatID: x.chat, ReceiverUserID: x.recv, EphemeralMessageID: x.eph, Text: "t"})
	}},
	{"editEphemeralMessageMedia", func(ctx context.Context, c *sender.Client, x api102cTarget) error {
		return c.EditEphemeralMessageMedia(ctx, sender.EditEphemeralMessageMediaRequest{
			ChatID: x.chat, ReceiverUserID: x.recv, EphemeralMessageID: x.eph,
			Media: sender.InputMedia{Type: "photo", Media: sender.FromFileID("AgAD-fake")}})
	}},
	{"editEphemeralMessageCaption", func(ctx context.Context, c *sender.Client, x api102cTarget) error {
		return c.EditEphemeralMessageCaption(ctx, sender.EditEphemeralMessageCaptionRequest{
			ChatID: x.chat, ReceiverUserID: x.recv, EphemeralMessageID: x.eph})
	}},
	{"editEphemeralMessageReplyMarkup", func(ctx context.Context, c *sender.Client, x api102cTarget) error {
		return c.EditEphemeralMessageReplyMarkup(ctx, sender.EditEphemeralMessageReplyMarkupRequest{
			ChatID: x.chat, ReceiverUserID: x.recv, EphemeralMessageID: x.eph})
	}},
	{"deleteEphemeralMessage", func(ctx context.Context, c *sender.Client, x api102cTarget) error {
		return c.DeleteEphemeralMessage(ctx, sender.DeleteEphemeralMessageRequest{
			ChatID: x.chat, ReceiverUserID: x.recv, EphemeralMessageID: x.eph})
	}},
}

// api102cJSONCall sends one JSON request and checks its path, content type and whole body.
func api102cJSONCall(t *testing.T, method, wantBody string, call func(context.Context, *sender.Client) error) {
	t.Helper()
	server := testutil.NewMockServer(t)
	api102cOK(server, method)
	client := testutil.NewTestClient(t, server.BaseURL())
	require.NoError(t, call(context.Background(), client))
	require.Equal(t, 1, server.CaptureCount())
	c := server.LastCapture()
	c.AssertPath(t, api102cPath(method))
	c.AssertContentType(t, "application/json")
	assert.JSONEq(t, wantBody, c.BodyString())
}

func api102cReplyTrue(w http.ResponseWriter, _ *http.Request) {
	api96ReplyRaw(w, `{"ok":true,"result":true}`)
}

// T-txt-1
func TestAPI102C_EditText(t *testing.T) {
	api102cJSONCall(t, "editEphemeralMessageText",
		`{`+api102cBody+`,"text":"hi <b>x</b>","parse_mode":"HTML","entities":[{"type":"bold","offset":0,"length":2}],`+
			`"link_preview_options":{"is_disabled":true},"reply_markup":`+api102cKeyboardJSON+`}`,
		func(ctx context.Context, c *sender.Client) error {
			return c.EditEphemeralMessageText(ctx, sender.EditEphemeralMessageTextRequest{
				ChatID: api102cChat, ReceiverUserID: api102cRecv, EphemeralMessageID: api102cEph,
				Text: "hi <b>x</b>", ParseMode: tg.ParseModeHTML,
				Entities:           []tg.MessageEntity{{Type: "bold", Offset: 0, Length: 2}},
				LinkPreviewOptions: &tg.LinkPreviewOptions{IsDisabled: true},
				ReplyMarkup:        api102cKeyboard()})
		})
}

// T-txt-2: text is required, so an empty text is sent as ""; no optional key is sent.
func TestAPI102C_EditTextMinimal(t *testing.T) {
	api102cJSONCall(t, "editEphemeralMessageText", `{`+api102cBody+`,"text":""}`,
		func(ctx context.Context, c *sender.Client) error {
			return c.EditEphemeralMessageText(ctx, sender.EditEphemeralMessageTextRequest{
				ChatID: api102cChat, ReceiverUserID: api102cRecv, EphemeralMessageID: api102cEph})
		})
}

// T-med-1: the media request goes as multipart/form-data with media as a JSON form field and no file part.
func TestAPI102C_EditMediaFileID(t *testing.T) {
	mediaType, args, files, err := wireCall(t, "editEphemeralMessageMedia", api102cReplyTrue,
		func(ctx context.Context, c *sender.Client) error {
			return c.EditEphemeralMessageMedia(ctx, sender.EditEphemeralMessageMediaRequest{
				ChatID: api102cChat, ReceiverUserID: api102cRecv, EphemeralMessageID: api102cEph,
				Media:       sender.InputMedia{Type: "photo", Media: sender.FromFileID("AgAD-fake")},
				ReplyMarkup: api102cKeyboard()})
		})
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	assertArgs(t, map[string]string{
		"chat_id": "-1001234567890", "receiver_user_id": "5000000001", "ephemeral_message_id": "4242",
		"media":        `{"type":"photo","media":"AgAD-fake"}`,
		"reply_markup": api102cKeyboardJSON,
	}, args)
	assertFiles(t, map[string]wireFile{}, files)
}

// T-med-3 (O-T3-02): a media by URL is sent as the URL; an upload by FromReader or FromBytes is a file part
// named in media with attach://.
func TestAPI102C_EditMediaSources(t *testing.T) {
	rows := []struct {
		name  string
		file  sender.InputFile
		media string
		files map[string]wireFile
	}{
		{"url", sender.FromURL("https://e.x/p.jpg"), `{"type":"photo","media":"https://e.x/p.jpg"}`, map[string]wireFile{}},
		{"reader", sender.FromReader(strings.NewReader("d102c-r"), "r.jpg"), `{"type":"photo","media":"attach://file0"}`,
			map[string]wireFile{"file0": {FileName: "r.jpg", Data: []byte("d102c-r")}}},
		{"bytes", sender.FromBytes([]byte("d102c-b"), "b.jpg"), `{"type":"photo","media":"attach://file0"}`,
			map[string]wireFile{"file0": {FileName: "b.jpg", Data: []byte("d102c-b")}}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			mediaType, args, files, err := wireCall(t, "editEphemeralMessageMedia", api102cReplyTrue,
				func(ctx context.Context, c *sender.Client) error {
					return c.EditEphemeralMessageMedia(ctx, sender.EditEphemeralMessageMediaRequest{
						ChatID: api102cChat, ReceiverUserID: api102cRecv, EphemeralMessageID: api102cEph,
						Media: sender.InputMedia{Type: "photo", Media: r.file}})
				})
			require.NoError(t, err)
			assert.Equal(t, "multipart/form-data", mediaType)
			assertArgs(t, map[string]string{
				"chat_id": "-1001234567890", "receiver_user_id": "5000000001", "ephemeral_message_id": "4242",
				"media": r.media,
			}, args)
			assertFiles(t, r.files, files)
		})
	}
}

// T-med-2
func TestAPI102C_EditMediaMinimal(t *testing.T) {
	mediaType, args, files, err := wireCall(t, "editEphemeralMessageMedia", api102cReplyTrue,
		func(ctx context.Context, c *sender.Client) error {
			return c.EditEphemeralMessageMedia(ctx, sender.EditEphemeralMessageMediaRequest{
				ChatID: api102cChat, ReceiverUserID: api102cRecv, EphemeralMessageID: api102cEph,
				Media: sender.InputMedia{Type: "photo", Media: sender.FromFileID("AgAD-fake")}})
		})
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	assertArgs(t, map[string]string{
		"chat_id": "-1001234567890", "receiver_user_id": "5000000001", "ephemeral_message_id": "4242",
		"media": `{"type":"photo","media":"AgAD-fake"}`,
	}, args)
	assertFiles(t, map[string]wireFile{}, files)
}

// T-cap-1
func TestAPI102C_EditCaption(t *testing.T) {
	api102cJSONCall(t, "editEphemeralMessageCaption",
		`{`+api102cBody+`,"caption":"c","parse_mode":"MarkdownV2","caption_entities":[{"type":"italic","offset":0,"length":1}],`+
			`"reply_markup":`+api102cKeyboardJSON+`}`,
		func(ctx context.Context, c *sender.Client) error {
			return c.EditEphemeralMessageCaption(ctx, sender.EditEphemeralMessageCaptionRequest{
				ChatID: api102cChat, ReceiverUserID: api102cRecv, EphemeralMessageID: api102cEph,
				Caption: "c", ParseMode: tg.ParseModeMarkdownV2,
				CaptionEntities: []tg.MessageEntity{{Type: "italic", Offset: 0, Length: 1}},
				ReplyMarkup:     api102cKeyboard()})
		})
}

// T-cap-2
func TestAPI102C_EditCaptionMinimal(t *testing.T) {
	api102cJSONCall(t, "editEphemeralMessageCaption", `{`+api102cBody+`}`,
		func(ctx context.Context, c *sender.Client) error {
			return c.EditEphemeralMessageCaption(ctx, sender.EditEphemeralMessageCaptionRequest{
				ChatID: api102cChat, ReceiverUserID: api102cRecv, EphemeralMessageID: api102cEph})
		})
}

// T-rm-1
func TestAPI102C_EditReplyMarkup(t *testing.T) {
	api102cJSONCall(t, "editEphemeralMessageReplyMarkup",
		`{`+api102cBody+`,"reply_markup":{"inline_keyboard":[[{"text":"A","callback_data":"a"}],[{"text":"U","url":"https://e.x/"}]]}}`,
		func(ctx context.Context, c *sender.Client) error {
			return c.EditEphemeralMessageReplyMarkup(ctx, sender.EditEphemeralMessageReplyMarkupRequest{
				ChatID: api102cChat, ReceiverUserID: api102cRecv, EphemeralMessageID: api102cEph,
				ReplyMarkup: &tg.InlineKeyboardMarkup{InlineKeyboard: [][]tg.InlineKeyboardButton{
					{{Text: "A", CallbackData: "a"}}, {{Text: "U", URL: "https://e.x/"}}}}})
		})
}

// T-rm-2: no reply markup removes the keyboard; the key is not sent.
func TestAPI102C_EditReplyMarkupMinimal(t *testing.T) {
	api102cJSONCall(t, "editEphemeralMessageReplyMarkup", `{`+api102cBody+`}`,
		func(ctx context.Context, c *sender.Client) error {
			return c.EditEphemeralMessageReplyMarkup(ctx, sender.EditEphemeralMessageReplyMarkupRequest{
				ChatID: api102cChat, ReceiverUserID: api102cRecv, EphemeralMessageID: api102cEph})
		})
}

// T-del-1
func TestAPI102C_Delete(t *testing.T) {
	api102cJSONCall(t, "deleteEphemeralMessage", `{`+api102cBody+`}`,
		func(ctx context.Context, c *sender.Client) error {
			return c.DeleteEphemeralMessage(ctx, sender.DeleteEphemeralMessageRequest{
				ChatID: api102cChat, ReceiverUserID: api102cRecv, EphemeralMessageID: api102cEph})
		})
}

// T-res-1: a true result is nil; an API error is a *tg.APIError named for the method.
func TestAPI102C_Results(t *testing.T) {
	for _, m := range api102cMethods {
		t.Run(m.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			var n atomic.Int32
			server.On(api102cPath(m.method), func(w http.ResponseWriter, _ *http.Request) {
				if n.Add(1) == 1 {
					api96ReplyRaw(w, `{"ok":true,"result":true}`)
					return
				}
				testutil.ReplyBadRequest(w, "message to edit not found") // the helper adds "Bad Request: "
			})
			client := testutil.NewTestClient(t, server.BaseURL(), sender.WithPerChatRateLimit(1000, 100))
			require.NoError(t, m.call(context.Background(), client, api102cValid))
			err := m.call(context.Background(), client, api102cValid)
			var apiErr *tg.APIError
			require.True(t, errors.As(err, &apiErr), "error %v", err)
			assert.Equal(t, 400, apiErr.Code)
			assert.Equal(t, "Bad Request: message to edit not found", apiErr.Description)
			assert.Equal(t, m.method, apiErr.Method)
			assert.Equal(t, 2, server.CaptureCount())
		})
	}
}

// T-val-1: a bad target is refused before any request and before any rate limiter.
func TestAPI102C_Validation(t *testing.T) {
	rows := []struct {
		name   string
		target api102cTarget
		text   string // the exact error text
		field  string // a *tg.ValidationError field, or "" for the chat id errors
	}{
		{"chat nil", api102cTarget{nil, api102cRecv, api102cEph}, "galigo: chat_id is required", ""},
		{"chat int64 0", api102cTarget{int64(0), api102cRecv, api102cEph}, "galigo: chat_id cannot be zero", ""},
		{"chat int 0", api102cTarget{int(0), api102cRecv, api102cEph}, "galigo: chat_id cannot be zero", ""},
		{"chat empty", api102cTarget{"", api102cRecv, api102cEph}, "galigo: chat_id cannot be empty string", ""},
		{"chat int32", api102cTarget{int32(5), api102cRecv, api102cEph}, "galigo: chat_id must be int64, int, or string, got int32", ""},
		{"receiver 0", api102cTarget{api102cChat, 0, api102cEph}, "galigo: validation: receiver_user_id - must be positive", "receiver_user_id"},
		{"receiver -1", api102cTarget{api102cChat, -1, api102cEph}, "galigo: validation: receiver_user_id - must be positive", "receiver_user_id"},
		{"ephemeral 0", api102cTarget{api102cChat, api102cRecv, 0}, "galigo: validation: ephemeral_message_id - required", "ephemeral_message_id"},
	}
	for _, m := range api102cMethods {
		for _, r := range rows {
			t.Run(m.method+" "+r.name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				api102cOK(server, m.method)
				client := testutil.NewTestClient(t, server.BaseURL())
				err := m.call(context.Background(), client, r.target)
				require.Error(t, err)
				assert.Equal(t, r.text, err.Error())
				var ve *tg.ValidationError
				if r.field == "" {
					assert.False(t, errors.As(err, &ve), "a chat id error is not a ValidationError")
				} else {
					require.True(t, errors.As(err, &ve), "error %v", err)
					assert.Equal(t, r.field, ve.Field)
				}
				assert.Equal(t, 0, server.CaptureCount(), "no request")
				assert.Equal(t, 0, client.ChatLimiterCount(), "no limiter")
			})
		}
	}
}

// T-val-2: the values just past each refused one are sent: a negative ephemeral id (no sign rule), receiver id 1,
// a channel username, and a Go int chat id (validateChatID accepts int, sender/validate.go:21).
func TestAPI102C_ValidationAccepts(t *testing.T) {
	rows := []struct {
		name   string
		target api102cTarget
		key    string
		want   string // the value as the server reads it
	}{
		{"ephemeral -1", api102cTarget{api102cChat, api102cRecv, -1}, "ephemeral_message_id", "-1"},
		{"receiver 1", api102cTarget{api102cChat, 1, api102cEph}, "receiver_user_id", "1"},
		{"chat username", api102cTarget{"@chan", api102cRecv, api102cEph}, "chat_id", "@chan"},
		{"chat int", api102cTarget{int(123456789), api102cRecv, api102cEph}, "chat_id", "123456789"},
	}
	for _, m := range api102cMethods {
		for _, r := range rows {
			t.Run(m.method+" "+r.name, func(t *testing.T) {
				_, args, _, err := wireCall(t, m.method, api102cReplyTrue,
					func(ctx context.Context, c *sender.Client) error { return m.call(ctx, c, r.target) })
				require.NoError(t, err)
				assert.Equal(t, r.want, args[r.key])
			})
		}
	}
}

// T-lpo-1: conflicting link preview options are refused before any request.
func TestAPI102C_EditTextLinkPreview(t *testing.T) {
	server := testutil.NewMockServer(t)
	api102cOK(server, "editEphemeralMessageText")
	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.EditEphemeralMessageText(context.Background(), sender.EditEphemeralMessageTextRequest{
		ChatID: api102cChat, ReceiverUserID: api102cRecv, EphemeralMessageID: api102cEph, Text: "t",
		LinkPreviewOptions: &tg.LinkPreviewOptions{PreferSmallMedia: true, PreferLargeMedia: true}})
	require.Error(t, err)
	assert.Equal(t, "link_preview_options: PreferSmallMedia and PreferLargeMedia are mutually exclusive", err.Error())
	assert.Equal(t, 0, server.CaptureCount())
	assert.Equal(t, 0, client.ChatLimiterCount())
}

// api102cLimited returns a client with the given limiter options, its server answering every new method and
// sendMessage.
func api102cLimited(t *testing.T, opts ...sender.Option) (*sender.Client, *testutil.MockTelegramServer) {
	t.Helper()
	server := testutil.NewMockServer(t)
	for _, m := range api102cMethods {
		api102cOK(server, m.method)
	}
	server.On(api102cPath("sendMessage"), func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyMessage(w, 1) })
	client, err := sender.New(testutil.TestToken, append([]sender.Option{sender.WithBaseURL(server.BaseURL()),
		sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()), sender.WithRetries(0)}, opts...)...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return client, server
}

// T-rl-1: each method uses the per-chat limiter of the request's chat, the same one as SendMessage.
func TestAPI102C_PerChatLimiter(t *testing.T) {
	const chatA, chatB = int64(123456789), int64(987654321)
	for _, m := range api102cMethods {
		t.Run("count "+m.method, func(t *testing.T) {
			client, server := api102cLimited(t, sender.WithPerChatRateLimit(1000, 100))
			require.Equal(t, 0, client.ChatLimiterCount())
			require.NoError(t, m.call(context.Background(), client, api102cTarget{chatA, api102cRecv, api102cEph}))
			assert.Equal(t, 1, client.ChatLimiterCount(), "after the call on chat A")
			require.NoError(t, m.call(context.Background(), client, api102cTarget{chatA, api102cRecv, api102cEph}))
			assert.Equal(t, 1, client.ChatLimiterCount(), "after a second call on chat A")
			require.NoError(t, m.call(context.Background(), client, api102cTarget{chatB, api102cRecv, api102cEph}))
			assert.Equal(t, 2, client.ChatLimiterCount(), "after the call on chat B")
			assert.Equal(t, 3, server.CaptureCount())
		})
		// With 1 call per 100 s and burst 1, a call after a SendMessage to the same chat fails in the limiter at once
		// (the 5 s deadline is far below the refill) and sends nothing; the same call on another chat is sent.
		t.Run("shares "+m.method, func(t *testing.T) {
			client, server := api102cLimited(t, sender.WithPerChatRateLimit(0.01, 1))
			_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: chatA, Text: "x"})
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err = m.call(ctx, client, api102cTarget{chatA, api102cRecv, api102cEph})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "would exceed context deadline")
			assert.Equal(t, 1, server.CaptureCount())
			require.NoError(t, m.call(context.Background(), client, api102cTarget{chatB, api102cRecv, api102cEph}))
			assert.Equal(t, 2, server.CaptureCount())
		})
	}
}

// T-rl-2: a group chat id uses the group limiter of that chat (sender/client.go:737-741).
func TestAPI102C_GroupLimiter(t *testing.T) {
	const otherGroup = int64(-1009876543210)
	for _, m := range api102cMethods {
		t.Run(m.method, func(t *testing.T) {
			client, server := api102cLimited(t, sender.WithPerChatRateLimit(1000, 100), sender.WithGroupRateLimit(0.01, 1))
			_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api102cChat, Text: "x"})
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err = m.call(ctx, client, api102cValid)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "would exceed context deadline")
			assert.Equal(t, 1, server.CaptureCount())
			require.NoError(t, m.call(context.Background(), client, api102cTarget{otherGroup, api102cRecv, api102cEph}))
			assert.Equal(t, 2, server.CaptureCount())
		})
	}
}

// T-rl-3: each method also waits on the global limiter (sender/client.go:705-711): with global burst 0 the call fails
// there at once, after the per-chat limiter ran, and sends nothing. A zero burst, not a timing guess.
func TestAPI102C_GlobalLimiter(t *testing.T) {
	for _, m := range api102cMethods {
		t.Run(m.method, func(t *testing.T) {
			client, server := api102cLimited(t, sender.WithPerChatRateLimit(1000, 100), sender.WithGroupRateLimit(1000, 100),
				sender.WithRateLimit(0, 0))
			err := m.call(context.Background(), client, api102cValid)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "exceeds limiter's burst 0")
			assert.Equal(t, 0, server.CaptureCount(), "no request")
			assert.Equal(t, 1, client.ChatLimiterCount(), "the per-chat limiter ran first")
		})
	}
}

// T-nr-1: no method retries a retryable API reply (502, 429); SendMessage, which does, is the control.
func TestAPI102C_NoRetry(t *testing.T) {
	sets := []struct {
		name  string
		code  int
		first func(http.ResponseWriter)
	}{
		{"502", 502, func(w http.ResponseWriter) { testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway") }},
		{"429", 429, func(w http.ResponseWriter) { testutil.ReplyRateLimit(w, 1) }},
	}
	for _, set := range sets {
		t.Run(set.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			paths := []string{"sendMessage"}
			for _, m := range api102cMethods {
				paths = append(paths, m.method)
			}
			for _, p := range paths {
				var n atomic.Int32
				server.On(api102cPath(p), func(w http.ResponseWriter, _ *http.Request) {
					if n.Add(1) == 1 {
						set.first(w)
						return
					}
					if strings.HasSuffix(p, "sendMessage") {
						testutil.ReplyMessage(w, 1)
						return
					}
					api96ReplyRaw(w, `{"ok":true,"result":true}`)
				})
			}
			sleeper := &testutil.FakeSleeper{}
			client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRateLimit(1000, 100),
				sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
			for _, m := range api102cMethods {
				server.ResetCaptures()
				err := m.call(context.Background(), client, api102cValid)
				var apiErr *tg.APIError
				require.True(t, errors.As(err, &apiErr), "%s: error %v", m.method, err)
				assert.Equal(t, set.code, apiErr.Code, m.method)
				assert.Equal(t, 1, server.CaptureCount(), "%s: exactly one request", m.method)
			}
			assert.Equal(t, 0, sleeper.CallCount(), "no backoff sleep before the control")

			server.ResetCaptures()
			_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api102cChat, Text: "hi"})
			require.NoError(t, err)
			assert.Equal(t, 2, server.CaptureCount(), "the control call is retried once")
		})
	}
}

// T-shape: the 27 fields of the five request types, with their Go types and tags, and nothing else.
func TestAPI102C_Shapes(t *testing.T) {
	type field struct{ name, typ, tag string }
	common := []field{
		{"ChatID", "interface {}", `json:"chat_id"`},
		{"ReceiverUserID", "int64", `json:"receiver_user_id"`},
		{"EphemeralMessageID", "int", `json:"ephemeral_message_id"`},
	}
	rm := field{"ReplyMarkup", "*tg.InlineKeyboardMarkup", `json:"reply_markup,omitempty"`}
	pm := field{"ParseMode", "tg.ParseMode", `json:"parse_mode,omitempty"`}
	cat := func(extra ...field) []field { return append(append([]field{}, common...), extra...) }
	rows := []struct {
		typ    reflect.Type
		fields []field
	}{
		{reflect.TypeFor[sender.EditEphemeralMessageTextRequest](), cat(
			field{"Text", "string", `json:"text"`}, pm,
			field{"Entities", "[]tg.MessageEntity", `json:"entities,omitempty"`},
			field{"LinkPreviewOptions", "*tg.LinkPreviewOptions", `json:"link_preview_options,omitempty"`}, rm)},
		{reflect.TypeFor[sender.EditEphemeralMessageMediaRequest](), cat(
			field{"Media", "sender.InputMedia", `json:"media"`}, rm)},
		{reflect.TypeFor[sender.EditEphemeralMessageCaptionRequest](), cat(
			field{"Caption", "string", `json:"caption,omitempty"`}, pm,
			field{"CaptionEntities", "[]tg.MessageEntity", `json:"caption_entities,omitempty"`}, rm)},
		{reflect.TypeFor[sender.EditEphemeralMessageReplyMarkupRequest](), cat(rm)},
		{reflect.TypeFor[sender.DeleteEphemeralMessageRequest](), cat()},
	}
	total := 0
	for _, r := range rows {
		t.Run(r.typ.Name(), func(t *testing.T) {
			require.Equal(t, len(r.fields), r.typ.NumField(), "field count")
			for i, want := range r.fields {
				f := r.typ.Field(i)
				assert.Equal(t, want.name, f.Name, "field %d", i)
				assert.Equal(t, want.typ, f.Type.String(), "%s type", f.Name)
				assert.Equal(t, want.tag, string(f.Tag), "%s tag", f.Name)
				assert.False(t, f.Anonymous, "%s is not embedded", f.Name)
				key := strings.Split(f.Tag.Get("json"), ",")[0]
				assert.NotContains(t, []string{"rich_message", "show_caption_above_media"}, key, "10.3 key")
			}
			// O-T3-08: no exported method (pattern: joinreq101b_test.go:433-434)
			assert.Equal(t, 0, r.typ.NumMethod(), "no method on the value type")
			assert.Equal(t, 0, reflect.PointerTo(r.typ).NumMethod(), "no method on the pointer type")
		})
		total += len(r.fields)
	}
	assert.Equal(t, 27, total)
	assert.Equal(t, []int{8, 5, 7, 4, 3}, []int{rows[0].typ.NumField(), rows[1].typ.NumField(), rows[2].typ.NumField(),
		rows[3].typ.NumField(), rows[4].typ.NumField()})
}

// T-nr-2 (O-T1-06): no method retries a network timeout; SendMessage, which does, is the control
// (api101bFailTransport: joinreq101b_test.go).
func TestAPI102C_NoRetryNetworkError(t *testing.T) {
	rt := &api101bFailTransport{}
	sleeper := &testutil.FakeSleeper{}
	client := testutil.NewRetryTestClient(t, "http://galigo.invalid", sleeper, sender.WithHTTPClient(&http.Client{Transport: rt}),
		sender.WithRateLimit(1000, 100), sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
	for _, m := range api102cMethods {
		rt.n.Store(0)
		err := m.call(context.Background(), client, api102cValid)
		require.Error(t, err, m.method)
		var ne net.Error
		require.True(t, errors.As(err, &ne) && ne.Timeout(), "%s: error %v", m.method, err)
		assert.Equal(t, int32(1), rt.n.Load(), "%s: exactly one round trip", m.method)
	}
	assert.Equal(t, 0, sleeper.CallCount(), "no backoff sleep before the control")

	// control, last: SendMessage retries a network timeout
	rt.n.Store(0)
	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api102cChat, Text: "hi"})
	require.Error(t, err)
	assert.GreaterOrEqual(t, rt.n.Load(), int32(2), "the control call is retried")
}

// api102cBreakerClient is the breaker test client with limiters that never wait, so that only the breaker decides
// (with the default group limiter the third call waits ~3 s, longer than the test breaker's open time).
func api102cBreakerClient(t *testing.T, server *testutil.MockTelegramServer) *sender.Client {
	t.Helper()
	return testutil.NewBreakerTestClient(t, server.BaseURL(), sender.WithRateLimit(1000, 100),
		sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
}

// T-cb-1 (O-T1-05): every method goes through the client's circuit breaker, and shares it with the other methods.
func TestAPI102C_Breaker(t *testing.T) {
	fail := func(server *testutil.MockTelegramServer) {
		for _, p := range []string{"deleteMessage", "editEphemeralMessageText", "editEphemeralMessageMedia",
			"editEphemeralMessageCaption", "editEphemeralMessageReplyMarkup", "deleteEphemeralMessage"} {
			server.On(api102cPath(p), func(w http.ResponseWriter, _ *http.Request) {
				testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway")
			})
		}
	}
	for _, m := range api102cMethods {
		t.Run(m.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			fail(server)
			client := api102cBreakerClient(t, server)
			for i := 1; i <= 2; i++ {
				err := m.call(context.Background(), client, api102cValid)
				require.Error(t, err)
				assert.False(t, errors.Is(err, gobreaker.ErrOpenState), "call %d: %v", i, err)
				assert.Equal(t, i, server.CaptureCount(), "call %d sends a request", i)
			}
			err := m.call(context.Background(), client, api102cValid)
			require.Error(t, err)
			assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "call 3: %v", err)
			assert.Equal(t, 2, server.CaptureCount(), "call 3 sends no request")
		})
	}

	// shared: an existing method (DeleteMessage) opens the breaker; then no new method sends a request
	t.Run("shared", func(t *testing.T) {
		server := testutil.NewMockServer(t)
		fail(server)
		client := api102cBreakerClient(t, server)
		for range 2 {
			require.Error(t, client.DeleteMessage(context.Background(), sender.DeleteMessageRequest{ChatID: api102cChat, MessageID: 1}))
		}
		require.Equal(t, 2, server.CaptureCount())
		for _, m := range api102cMethods {
			err := m.call(context.Background(), client, api102cValid)
			assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "%s: error %v", m.method, err)
		}
		assert.Equal(t, 2, server.CaptureCount(), "no request while the breaker is open")
	})
}

// T-sig (O-T1-07): the five methods have exactly the planned signatures (an extra variadic parameter would still
// compile at every call site).
func TestAPI102C_Signatures(t *testing.T) {
	rows := []struct{ got, want reflect.Type }{
		{reflect.TypeOf((*sender.Client).EditEphemeralMessageText),
			reflect.TypeFor[func(*sender.Client, context.Context, sender.EditEphemeralMessageTextRequest) error]()},
		{reflect.TypeOf((*sender.Client).EditEphemeralMessageMedia),
			reflect.TypeFor[func(*sender.Client, context.Context, sender.EditEphemeralMessageMediaRequest) error]()},
		{reflect.TypeOf((*sender.Client).EditEphemeralMessageCaption),
			reflect.TypeFor[func(*sender.Client, context.Context, sender.EditEphemeralMessageCaptionRequest) error]()},
		{reflect.TypeOf((*sender.Client).EditEphemeralMessageReplyMarkup),
			reflect.TypeFor[func(*sender.Client, context.Context, sender.EditEphemeralMessageReplyMarkupRequest) error]()},
		{reflect.TypeOf((*sender.Client).DeleteEphemeralMessage),
			reflect.TypeFor[func(*sender.Client, context.Context, sender.DeleteEphemeralMessageRequest) error]()},
	}
	for _, r := range rows {
		assert.Equal(t, r.want.String(), r.got.String())
	}
}
