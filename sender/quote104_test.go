package sender_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-65 65g: Bot API 10.4 ReplyParameters.quote_rich_texts on the send paths, and the verifyUser / verifyChat
// custom description fields. TEST-PLAN section 3. Ids are fake.

const (
	// b65gRPLegacy also sets the old quote fields: galigo sends them as given; Telegram ignores them when
	// quote_rich_texts is non-empty (PLAN AC2, O-T1-03). The rich quote holds a code node, a kind the server replaces
	// with its text, which galigo must not filter.
	b65gRPLegacy = `{"message_id":1,"quote":"legacy","quote_parse_mode":"HTML","quote_entities":[{"type":"italic","offset":0,"length":1}],` +
		`"quote_rich_texts":[["quote ",{"type":"bold","text":"bold"}],{"type":"code","text":"c"}],"quote_position":7}`
)

func b65gReplyParameters() *tg.ReplyParameters {
	return &tg.ReplyParameters{MessageID: 1, QuotePosition: 7, QuoteRichTexts: []tg.RichText{{Parts: []tg.RichText{
		{Plain: "quote "}, {Node: tg.RichTextBold{Type: "bold", Text: tg.RichText{Plain: "bold"}}}}}}}
}

// S-multipart: an upload makes sendLivePhoto multipart; the reply_parameters form field holds the same bytes as the
// JSON row, old quote fields included (O-T2-03).
func TestB65G_SendLivePhotoMultipart(t *testing.T) {
	mediaType, args, files, err := wireCall(t, "sendLivePhoto", replyMessage, sendLive(sender.SendLivePhotoRequest{
		ChatID:          testutil.TestChatID,
		LivePhoto:       sender.FromBytes(lpVideoBytes, "v.mp4"),
		Photo:           sender.FromBytes(lpPhotoBytes, "p.jpg"),
		ReplyParameters: b65gReplyParametersLegacy(),
	}))
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	assert.Equal(t, map[string]string{"chat_id": wireChatID, "reply_parameters": b65gRPLegacy}, args)
	assertFiles(t, map[string]wireFile{
		"live_photo": {FileName: "v.mp4", Data: lpVideoBytes},
		"photo":      {FileName: "p.jpg", Data: lpPhotoBytes},
	}, files)
}

func b65gReplyParametersLegacy() *tg.ReplyParameters {
	rp := b65gReplyParameters()
	rp.Quote, rp.QuoteParseMode = "legacy", "HTML"
	rp.QuoteEntities = []tg.MessageEntity{{Type: "italic", Offset: 0, Length: 1}}
	rp.QuoteRichTexts = append(rp.QuoteRichTexts, tg.RichText{Node: tg.RichTextCode{Type: "code", Text: tg.RichText{Plain: "c"}}})
	return rp
}

// S-json and S-retry (one row, A-T1-09): a 429 then OK. SendMessage retries (withRetry; the default test client has
// no retries, so WithRetries(1) and a FakeSleeper) and both JSON bodies carry the exact reply_parameters bytes, with
// the old quote fields set too.
func TestB65G_SendMessageRetry(t *testing.T) {
	server := testutil.NewMockServer(t)
	var n atomic.Int32
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, _ *http.Request) {
		if n.Add(1) == 1 {
			testutil.ReplyRateLimit(w, 0)
			return
		}
		testutil.ReplyMessage(w, 42)
	})
	client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRetries(1), sender.WithSleeper(&testutil.FakeSleeper{}))
	msg, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: int64(-1009876543210), Text: "r", ReplyParameters: b65gReplyParametersLegacy()})
	require.NoError(t, err)
	require.NotNil(t, msg)
	assert.Equal(t, 42, msg.MessageID)
	require.Equal(t, 2, server.CaptureCount())
	for i := range 2 {
		mediaType, args, files := wireArgs(t, capturedRequest(server.CaptureAt(i)))
		assert.Equal(t, "application/json", mediaType, "attempt %d", i)
		assert.Equal(t, map[string]string{"chat_id": "-1009876543210", "text": "r", "reply_parameters": b65gRPLegacy}, args, "attempt %d", i)
		assert.Empty(t, files)
	}
}

// E-sender-malformed (O-P3-03): ok:true with a non-array quote.rich_texts gives a nil message and a non-API error.
func TestB65G_SendMessageMalformedQuote(t *testing.T) {
	reply := func(w http.ResponseWriter, _ *http.Request) {
		api96ReplyRaw(w, `{"ok":true,"result":{"message_id":42,"date":1,"chat":{"id":-1009876543210,"type":"supergroup","title":"G"},`+
			`"text":"r","quote":{"text":"","position":7,"rich_texts":{}}}}`)
	}
	var msg *tg.Message
	_, _, _, err := wireCall(t, "sendMessage", reply, func(ctx context.Context, c *sender.Client) error {
		var err error
		msg, err = c.SendMessage(ctx, sender.SendMessageRequest{ChatID: int64(-1009876543210), Text: "r", ReplyParameters: b65gReplyParameters()})
		return err
	})
	require.Error(t, err)
	assert.Nil(t, msg)
	var apiErr *tg.APIError
	assert.False(t, errors.As(err, &apiErr), "a decode error, not an API error: %v", err)
	var typeErr *json.UnmarshalTypeError
	require.True(t, errors.As(err, &typeErr), "the decoder of quote.rich_texts failed: %v", err)
	assert.Contains(t, typeErr.Field, "rich_texts")
}

// S-verify: the 10.4 custom description fields of verifyUser and verifyChat.
func TestB65G_Verify(t *testing.T) {
	link := []tg.MessageEntity{{Type: "text_link", Offset: 0, Length: 1, URL: "https://e.x/a"}}
	bold := []tg.MessageEntity{{Type: "bold", Offset: 0, Length: 1}}
	const linkJSON = `[{"type":"text_link","offset":0,"length":1,"url":"https://e.x/a"}]`
	user := func(r sender.VerifyUserRequest) func(ctx context.Context, c *sender.Client) error {
		return func(ctx context.Context, c *sender.Client) error { return c.VerifyUser(ctx, r) }
	}
	chat := func(r sender.VerifyChatRequest) func(ctx context.Context, c *sender.Client) error {
		return func(ctx context.Context, c *sender.Client) error { return c.VerifyChat(ctx, r) }
	}
	ok := func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyBool(w, true) }
	for _, tc := range []struct {
		name, method string
		call         func(ctx context.Context, c *sender.Client) error
		want         map[string]string
	}{
		{"user full", "verifyUser", user(sender.VerifyUserRequest{UserID: 456, CustomDescription: "d", CustomDescriptionParseMode: tg.ParseModeHTML, CustomDescriptionEntities: link}),
			map[string]string{"user_id": "456", "custom_description": "d", "custom_description_parse_mode": "HTML", "custom_description_entities": linkJSON}},
		{"user minimal", "verifyUser", user(sender.VerifyUserRequest{UserID: 456}), map[string]string{"user_id": "456"}},
		{"user parse mode only", "verifyUser", user(sender.VerifyUserRequest{UserID: 456, CustomDescription: "d", CustomDescriptionParseMode: tg.ParseModeHTML}),
			map[string]string{"user_id": "456", "custom_description": "d", "custom_description_parse_mode": "HTML"}},
		{"user entities only", "verifyUser", user(sender.VerifyUserRequest{UserID: 456, CustomDescription: "d", CustomDescriptionEntities: link}),
			map[string]string{"user_id": "456", "custom_description": "d", "custom_description_entities": linkJSON}},
		{"user empty entities omitted", "verifyUser", user(sender.VerifyUserRequest{UserID: 456, CustomDescriptionEntities: []tg.MessageEntity{}}), map[string]string{"user_id": "456"}},
		{"user bold sent as given", "verifyUser", user(sender.VerifyUserRequest{UserID: 456, CustomDescription: "d", CustomDescriptionEntities: bold}),
			map[string]string{"user_id": "456", "custom_description": "d", "custom_description_entities": `[{"type":"bold","offset":0,"length":1}]`}},
		{"chat full", "verifyChat", chat(sender.VerifyChatRequest{ChatID: int64(-1009876543210), CustomDescription: "d", CustomDescriptionParseMode: tg.ParseModeHTML, CustomDescriptionEntities: link}),
			map[string]string{"chat_id": "-1009876543210", "custom_description": "d", "custom_description_parse_mode": "HTML", "custom_description_entities": linkJSON}},
		{"chat minimal", "verifyChat", chat(sender.VerifyChatRequest{ChatID: int64(-1009876543210)}), map[string]string{"chat_id": "-1009876543210"}},
		{"chat parse mode only", "verifyChat", chat(sender.VerifyChatRequest{ChatID: int64(-1009876543210), CustomDescription: "d", CustomDescriptionParseMode: tg.ParseModeHTML}),
			map[string]string{"chat_id": "-1009876543210", "custom_description": "d", "custom_description_parse_mode": "HTML"}},
		{"chat entities only", "verifyChat", chat(sender.VerifyChatRequest{ChatID: int64(-1009876543210), CustomDescription: "d", CustomDescriptionEntities: link}),
			map[string]string{"chat_id": "-1009876543210", "custom_description": "d", "custom_description_entities": linkJSON}},
		{"chat empty entities omitted", "verifyChat", chat(sender.VerifyChatRequest{ChatID: int64(-1009876543210), CustomDescriptionEntities: []tg.MessageEntity{}}), map[string]string{"chat_id": "-1009876543210"}},
		{"chat bold sent as given", "verifyChat", chat(sender.VerifyChatRequest{ChatID: int64(-1009876543210), CustomDescription: "d", CustomDescriptionEntities: bold}),
			map[string]string{"chat_id": "-1009876543210", "custom_description": "d", "custom_description_entities": `[{"type":"bold","offset":0,"length":1}]`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mediaType, args, files, err := wireCall(t, tc.method, ok, tc.call)
			require.NoError(t, err)
			assert.Equal(t, "application/json", mediaType)
			assert.Equal(t, tc.want, args)
			assert.Empty(t, files)
		})
	}
}

// S-shapes: the four new request fields, with their neighbours. The comparability break (PLAN section 9) is proved by
// the G-api gate, not here (no mutant could fail such a row).
func TestB65G_Shapes(t *testing.T) {
	type field struct{ Name, Type, Tag string }
	fields := func(v any) []field {
		rt := reflect.TypeOf(v)
		var out []field
		for i := range rt.NumField() {
			f := rt.Field(i)
			out = append(out, field{f.Name, f.Type.String(), f.Tag.Get("json")})
		}
		return out
	}
	tail := []field{
		{"CustomDescription", "string", "custom_description,omitempty"},
		{"CustomDescriptionParseMode", "tg.ParseMode", "custom_description_parse_mode,omitempty"},
		{"CustomDescriptionEntities", "[]tg.MessageEntity", "custom_description_entities,omitempty"},
	}
	assert.Equal(t, append([]field{{"UserID", "int64", "user_id"}}, tail...), fields(sender.VerifyUserRequest{}))
	assert.Equal(t, append([]field{{"ChatID", "interface {}" /* tg.ChatID is an alias of any */, "chat_id"}}, tail...), fields(sender.VerifyChatRequest{}))
}
