package sender_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-47: an inline edit returns True; the four EditMessage* methods then
// return a nil Message and a nil error.

type b47Method struct {
	name, method string
	inlineArgs   map[string]string // the whole argument map of the inline request
	chatArgs     map[string]string // the whole argument map of the chat request
	inline       func(ctx context.Context, c *sender.Client) (*tg.Message, error)
	chat         func(ctx context.Context, c *sender.Client) (*tg.Message, error)
}

func b47Methods() []b47Method {
	return []b47Method{
		{"text", "editMessageText",
			map[string]string{"inline_message_id": "im-1", "text": "x"},
			map[string]string{"chat_id": "100", "message_id": "7", "text": "x"},
			func(ctx context.Context, c *sender.Client) (*tg.Message, error) {
				return c.EditMessageText(ctx, sender.EditMessageTextRequest{InlineMessageID: "im-1", Text: "x"})
			},
			func(ctx context.Context, c *sender.Client) (*tg.Message, error) {
				return c.EditMessageText(ctx, sender.EditMessageTextRequest{ChatID: int64(100), MessageID: 7, Text: "x"})
			}},
		{"caption", "editMessageCaption",
			map[string]string{"inline_message_id": "im-1", "caption": "x"},
			map[string]string{"chat_id": "100", "message_id": "7", "caption": "x"},
			func(ctx context.Context, c *sender.Client) (*tg.Message, error) {
				return c.EditMessageCaption(ctx, sender.EditMessageCaptionRequest{InlineMessageID: "im-1", Caption: "x"})
			},
			func(ctx context.Context, c *sender.Client) (*tg.Message, error) {
				return c.EditMessageCaption(ctx, sender.EditMessageCaptionRequest{ChatID: int64(100), MessageID: 7, Caption: "x"})
			}},
		{"markup", "editMessageReplyMarkup",
			map[string]string{"inline_message_id": "im-1"},
			map[string]string{"chat_id": "100", "message_id": "7"},
			func(ctx context.Context, c *sender.Client) (*tg.Message, error) {
				return c.EditMessageReplyMarkup(ctx, sender.EditMessageReplyMarkupRequest{InlineMessageID: "im-1"})
			},
			func(ctx context.Context, c *sender.Client) (*tg.Message, error) {
				return c.EditMessageReplyMarkup(ctx, sender.EditMessageReplyMarkupRequest{ChatID: int64(100), MessageID: 7})
			}},
		{"media", "editMessageMedia",
			map[string]string{"inline_message_id": "im-1", "media": `{"type":"photo","media":"A"}`},
			map[string]string{"chat_id": "100", "message_id": "7", "media": `{"type":"photo","media":"A"}`},
			func(ctx context.Context, c *sender.Client) (*tg.Message, error) {
				return c.EditMessageMedia(ctx, sender.EditMessageMediaRequest{InlineMessageID: "im-1", Media: sender.NewInputMediaPhoto(sender.FromFileID("A"))})
			},
			func(ctx context.Context, c *sender.Client) (*tg.Message, error) {
				return c.EditMessageMedia(ctx, sender.EditMessageMediaRequest{ChatID: int64(100), MessageID: 7, Media: sender.NewInputMediaPhoto(sender.FromFileID("A"))})
			}},
	}
}

func b47Raw(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

// b47Run sends one call to a mock server answering body. wireCall requires
// exactly one request to the method's path and an empty URL query; the
// returned map holds every argument (JSON or multipart).
func b47Run(t *testing.T, method, body string, call func(ctx context.Context, c *sender.Client) (*tg.Message, error)) (map[string]string, *tg.Message, error) {
	t.Helper()
	var msg *tg.Message
	_, args, _, err := wireCall(t, method, b47Raw(body), func(ctx context.Context, c *sender.Client) error {
		var callErr error
		msg, callErr = call(ctx, c)
		return callErr
	})
	return args, msg, err
}

// T-1: the inline success, also with JSON whitespace around the value. The
// result alone decides (owner decision, PLAN D2): a chat request answered
// with true is also (nil, nil) [T1-01].
func TestB47_True(t *testing.T) {
	bodies := map[string]string{
		"plain":  `{"ok":true,"result":true}`,
		"spaces": `{"ok":true,"result": true }`,
		"crlf":   "{\"ok\":true,\r\n \"result\":\r\n\ttrue\r\n}",
	}
	for _, m := range b47Methods() {
		for name, body := range bodies {
			t.Run(m.name+"/"+name, func(t *testing.T) {
				args, msg, err := b47Run(t, m.method, body, m.inline)
				require.NoError(t, err)
				assert.Nil(t, msg)
				assertArgs(t, m.inlineArgs, args)
			})
		}
		t.Run(m.name+"/chat-request", func(t *testing.T) {
			args, msg, err := b47Run(t, m.method, `{"ok":true,"result":true}`, m.chat)
			require.NoError(t, err)
			assert.Nil(t, msg)
			assertArgs(t, m.chatArgs, args)
		})
	}
}

// T-2: a Message result is returned as today, whole; text "true" is not the
// boolean; an unknown key is ignored. An inline request answered with a Message
// also returns it [T1-01, T1-02, T2-02].
func TestB47_Message(t *testing.T) {
	// future_server_field: a key tg.Message does not have is ignored, as today [T2-02].
	body := `{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":100,"type":"private"},"text":"true","future_server_field":true}}`
	want := &tg.Message{MessageID: 7, Date: 1, Chat: &tg.Chat{ID: 100, Type: "private"}, Text: "true"}
	for _, m := range b47Methods() {
		t.Run(m.name, func(t *testing.T) {
			args, msg, err := b47Run(t, m.method, body, m.chat)
			require.NoError(t, err)
			assert.Equal(t, want, msg)
			assertArgs(t, m.chatArgs, args)
		})
		t.Run(m.name+"/inline-request", func(t *testing.T) {
			args, msg, err := b47Run(t, m.method, body, m.inline)
			require.NoError(t, err)
			assert.Equal(t, want, msg)
			assertArgs(t, m.inlineArgs, args)
		})
		// Outer whitespace in Text is kept, and an unknown key with a number too
		// large for float64 is ignored, as json.Unmarshal into tg.Message does
		// today (round t3 T3-01, T3-02: fixed after the third review).
		t.Run(m.name+"/odd-values", func(t *testing.T) {
			_, msg, err := b47Run(t, m.method, `{"ok":true,"result":{"message_id":7,"text":"  true\n","future_server_field":1e400}}`, m.chat)
			require.NoError(t, err)
			assert.Equal(t, &tg.Message{MessageID: 7, Text: "  true\n"}, msg)
		})
	}
}

// T-3: any other result keeps today's parse error (the exact text and the
// wrapped encoding/json error) and a nil Message [T1-03], also for an object
// with a wrong field type ("bad-field", T2-01). "one" and
// "array-true" are Grok t1's extra-literal programs.
func TestB47_NotTrue(t *testing.T) {
	const pre = "failed to parse message: json: "
	rows := []struct {
		name, body, want string
		syntax           bool // *json.SyntaxError, else *json.UnmarshalTypeError
	}{
		{"false", `{"ok":true,"result":false}`, pre + "cannot unmarshal bool into Go value of type tg.Message", false},
		{"number", `{"ok":true,"result":0}`, pre + "cannot unmarshal number into Go value of type tg.Message", false},
		{"one", `{"ok":true,"result":1}`, pre + "cannot unmarshal number into Go value of type tg.Message", false},
		{"string", `{"ok":true,"result":"true"}`, pre + "cannot unmarshal string into Go value of type tg.Message", false},
		{"array", `{"ok":true,"result":[]}`, pre + "cannot unmarshal array into Go value of type tg.Message", false},
		{"array-true", `{"ok":true,"result":[true]}`, pre + "cannot unmarshal array into Go value of type tg.Message", false},
		{"no-result", `{"ok":true}`, "failed to parse message: unexpected end of JSON input", true},
		{"bad-field", `{"ok":true,"result":{"message_id":"bad"}}`, pre + "cannot unmarshal string into Go struct field Message.message_id of type int", false}, // [T2-01]
	}
	for _, m := range b47Methods() {
		for _, row := range rows {
			t.Run(m.name+"/"+row.name, func(t *testing.T) {
				_, msg, err := b47Run(t, m.method, row.body, m.inline)
				require.EqualError(t, err, row.want)
				if row.syntax {
					var se *json.SyntaxError
					assert.ErrorAs(t, err, &se)
				} else {
					var te *json.UnmarshalTypeError
					assert.ErrorAs(t, err, &te)
				}
				assert.Nil(t, msg)
			})
		}
	}
}

// T-4: null keeps today's outcome: a zero Message and no error.
func TestB47_Null(t *testing.T) {
	for _, m := range b47Methods() {
		for name, body := range map[string]string{"plain": `{"ok":true,"result":null}`, "spaces": `{"ok":true,"result": null }`, "empty-object": `{"ok":true,"result":{}}`} {
			t.Run(m.name+"/"+name, func(t *testing.T) {
				_, msg, err := b47Run(t, m.method, body, m.inline)
				require.NoError(t, err)
				assert.Equal(t, &tg.Message{}, msg) // [T1-02]
			})
		}
	}
}

// T-5: a rich inline edit (backlog B-47, Codex C1-09 of 10.1d).
func TestB47_RichInline(t *testing.T) {
	args, msg, err := b47Run(t, "editMessageText", `{"ok":true,"result":true}`, func(ctx context.Context, c *sender.Client) (*tg.Message, error) {
		return c.EditMessageText(ctx, sender.EditMessageTextRequest{InlineMessageID: "im-1", RichMessage: &tg.InputRichMessage{Markdown: "m"}})
	})
	require.NoError(t, err)
	assert.Nil(t, msg)
	assertArgs(t, map[string]string{"inline_message_id": "im-1", "rich_message": `{"markdown":"m"}`}, args)
}

// T-6 (sender): Client.Edit on an inline message passes (nil, nil) through,
// and passes the parse error of any other result through (Grok t1's
// wrapper-swallow program).
func TestB47_EditWrappers(t *testing.T) {
	edit := func(ctx context.Context, c *sender.Client) (*tg.Message, error) {
		return c.Edit(ctx, tg.InlineMessage{InlineMessageID: "im-1"}, "x")
	}
	args, msg, err := b47Run(t, "editMessageText", `{"ok":true,"result":true}`, edit)
	require.NoError(t, err)
	assert.Nil(t, msg)
	assertArgs(t, map[string]string{"inline_message_id": "im-1", "text": "x"}, args)

	_, msg, err = b47Run(t, "editMessageText", `{"ok":true,"result":false}`, edit)
	require.EqualError(t, err, "failed to parse message: json: cannot unmarshal bool into Go value of type tg.Message")
	assert.Nil(t, msg)
}

// T-7: no retry wrapper: an error reply sends exactly one request.
func TestB47_NoRetry(t *testing.T) {
	replies := map[string]http.HandlerFunc{
		"429": func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyRateLimit(w, 1) },
		"500": func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyServerError(w, 500, "Internal") },
	}
	for _, m := range b47Methods() {
		for name, reply := range replies {
			t.Run(m.name+"/"+name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				server.On("/bot"+testutil.TestToken+"/"+m.method, reply)
				client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRetries(1), sender.WithSleeper(&testutil.FakeSleeper{}))
				_, err := m.inline(context.Background(), client)
				require.Error(t, err)
				assert.Equal(t, 1, server.CaptureCount())
			})
		}
	}
}
