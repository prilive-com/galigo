package sender_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.1 rich messages, send side (B-25 10.1d, TEST-PLAN §3, rows T-srm-1 … T-nr-2).

const (
	// api101dAsGiven is a value each standard normaliser would change (E-0117).
	api101dAsGiven = " **B** \t\n<b>&amp;</b> HTTPS://E.x/A%2fb?a=+ ü "
	// api101dID is an identifier-like as-given value.
	api101dID   = " bC-Ab+%2fx/Y== "
	api101dChat = int64(123456789)

	// api101dReply is the result of the live probe 02 (E11/lookups/probe101d/02-sendRichMessage-skip-absent.json),
	// with the ids, names and date replaced by fake values.
	api101dReply = `{"message_id":4242,"from":{"id":7001,"is_bot":true,"first_name":"galigo test bot","username":"galigo_test_bot"},` +
		`"chat":{"id":123456789,"title":"galigo test","is_forum":true,"type":"supergroup"},"date":1700000000,` +
		`"rich_message":{"blocks":[{"type":"paragraph","text":["visit ",{"type":"url","text":"https://t.me","url":"https://t.me"},` +
		`" and ",{"type":"mention","text":"@username","username":"username"}," and ",{"type":"hashtag","text":"#tag","hashtag":"tag"}]}]}}`

	api101dTextRequired = "galigo: chat_id is required"
	api101dTextZero     = "galigo: chat_id cannot be zero"
	api101dTextEmpty    = "galigo: chat_id cannot be empty string"
	api101dDraftInteger = "must be integer for sendRichMessageDraft (private chats only)"
	// api101dHTMLMixed is valid HTML with mixed case, an entity, quotes and non-ASCII text: a sender that changes the
	// HTML after validation changes it (C1-07).
	api101dHTMLMixed = `<p>Hello <B>World</B> &amp; "Ünï" ✓</p>`
)

func api101dPath(method string) string { return "/bot" + testutil.TestToken + "/" + method }

func api101dOnResult(server *testutil.MockTelegramServer, method, result string) {
	server.On(api101dPath(method), func(w http.ResponseWriter, _ *http.Request) {
		api96ReplyRaw(w, `{"ok":true,"result":`+result+`}`)
	})
}

// api101dRefused asserts a refusal: a *tg.ValidationError with field and message, or (field "") a plain error with
// exactly the text msg; and no request.
func api101dRefused(t *testing.T, server *testutil.MockTelegramServer, err error, field, msg string) {
	t.Helper()
	require.Error(t, err)
	if field == "" {
		var ve *tg.ValidationError
		assert.False(t, errors.As(err, &ve), "a plain error, not a ValidationError: %v", err)
		assert.Equal(t, msg, err.Error())
	} else {
		var ve *tg.ValidationError
		require.True(t, errors.As(err, &ve), "error %v", err)
		assert.Equal(t, field, ve.Field)
		assert.Equal(t, msg, ve.Message)
	}
	assert.Equal(t, 0, server.CaptureCount(), "a refusal sends no request")
}

// T-srm-1
func TestAPI101D_SendRichMessageBody(t *testing.T) {
	server := testutil.NewMockServer(t)
	api101dOnResult(server, "sendRichMessage", api101dReply)
	client := testutil.NewTestClient(t, server.BaseURL())
	_, err := client.SendRichMessage(context.Background(), sender.SendRichMessageRequest{
		BusinessConnectionID:  api101dID,
		ChatID:                api101dChat,
		MessageThreadID:       7,
		DirectMessagesTopicID: 8,
		RichMessage:           tg.InputRichMessage{Markdown: api101dAsGiven, IsRTL: true, SkipEntityDetection: true},
		DisableNotification:   true,
		ProtectContent:        true,
		AllowPaidBroadcast:    true,
		MessageEffectID:       api101dID,
		ReplyParameters:       &tg.ReplyParameters{MessageID: 5},
		ReplyMarkup:           testutil.TestInlineKeyboard([]tg.InlineKeyboardButton{testutil.TestInlineButton("b", "d")}),
	})
	require.NoError(t, err)
	require.Equal(t, 1, server.CaptureCount())
	c := server.LastCapture()
	c.AssertPath(t, api101dPath("sendRichMessage"))
	assert.JSONEq(t, `{"business_connection_id":" bC-Ab+%2fx/Y== ","chat_id":123456789,"message_thread_id":7,`+
		`"direct_messages_topic_id":8,"rich_message":{"markdown":" **B** \t\n<b>&amp;</b> HTTPS://E.x/A%2fb?a=+ ü ",`+
		`"is_rtl":true,"skip_entity_detection":true},"disable_notification":true,"protect_content":true,`+
		`"allow_paid_broadcast":true,"message_effect_id":" bC-Ab+%2fx/Y== ","reply_parameters":{"message_id":5},`+
		`"reply_markup":{"inline_keyboard":[[{"text":"b","callback_data":"d"}]]}}`, c.BodyString())
}

// T-srm-2
func TestAPI101D_SendRichMessageMinimal(t *testing.T) {
	server := testutil.NewMockServer(t)
	api101dOnResult(server, "sendRichMessage", api101dReply)
	client := testutil.NewTestClient(t, server.BaseURL())
	_, err := client.SendRichMessage(context.Background(), sender.SendRichMessageRequest{
		ChatID: api101dChat, RichMessage: tg.InputRichMessage{HTML: api101dHTMLMixed}})
	require.NoError(t, err)
	require.Equal(t, 1, server.CaptureCount())
	assert.JSONEq(t, `{"chat_id":123456789,"rich_message":{"html":"<p>Hello <B>World</B> &amp; \"Ünï\" ✓</p>"}}`,
		server.LastCapture().BodyString())
}

// T-srm-3
func TestAPI101D_SendRichMessageChatUsername(t *testing.T) {
	server := testutil.NewMockServer(t)
	api101dOnResult(server, "sendRichMessage", api101dReply)
	client := testutil.NewTestClient(t, server.BaseURL())
	_, err := client.SendRichMessage(context.Background(), sender.SendRichMessageRequest{
		ChatID: "@galigo_chan", RichMessage: tg.InputRichMessage{Markdown: "m"}})
	require.NoError(t, err)
	require.Equal(t, 1, server.CaptureCount())
	assert.JSONEq(t, `{"chat_id":"@galigo_chan","rich_message":{"markdown":"m"}}`, server.LastCapture().BodyString())
}

// T-srm-4
func TestAPI101D_SendRichMessageResult(t *testing.T) {
	server := testutil.NewMockServer(t)
	api101dOnResult(server, "sendRichMessage", api101dReply)
	client := testutil.NewTestClient(t, server.BaseURL())
	msg, err := client.SendRichMessage(context.Background(), sender.SendRichMessageRequest{
		ChatID: api101dChat, RichMessage: tg.InputRichMessage{Markdown: "m"}})
	require.NoError(t, err)
	require.NotNil(t, msg)
	assert.Equal(t, 4242, msg.MessageID)
	assert.Equal(t, "", msg.Text)
	require.NotNil(t, msg.RichMessage)
	require.Len(t, msg.RichMessage.Blocks, 1)
	p, ok := msg.RichMessage.Blocks[0].(tg.RichBlockParagraph)
	require.True(t, ok, "block %T", msg.RichMessage.Blocks[0])
	require.Len(t, p.Text.Parts, 6)
	u, ok := p.Text.Parts[1].Node.(tg.RichTextURL)
	require.True(t, ok, "part 1 %T", p.Text.Parts[1].Node)
	assert.Equal(t, "https://t.me", u.URL)
}

// T-srm-5
func TestAPI101D_SendRichMessageValidation(t *testing.T) {
	rows := []struct {
		name        string
		chat        tg.ChatID
		rich        tg.InputRichMessage
		field, msg  string // a refusal; both "" with body set = a valid request
		wantRichKey string
	}{
		{"chat nil", nil, tg.InputRichMessage{Markdown: "m"}, "", api101dTextRequired, ""},
		{"chat zero", int64(0), tg.InputRichMessage{Markdown: "m"}, "", api101dTextZero, ""},
		{"chat empty string", "", tg.InputRichMessage{Markdown: "m"}, "", api101dTextEmpty, ""},
		{"neither", api101dChat, tg.InputRichMessage{IsRTL: true}, "rich_message", "html or markdown required", ""},
		{"both", api101dChat, tg.InputRichMessage{HTML: "h", Markdown: "m"}, "rich_message", "use only one of html, markdown", ""},
		{"bad chat and bad rich", nil, tg.InputRichMessage{HTML: "h", Markdown: "m"}, "", api101dTextRequired, ""},
		{"markdown, html empty", api101dChat, tg.InputRichMessage{Markdown: "m", HTML: ""}, "", "", `{"markdown":"m"}`},
		{"whitespace kept", api101dChat, tg.InputRichMessage{Markdown: " "}, "", "", `{"markdown":" "}`},
		{"whitespace html", api101dChat, tg.InputRichMessage{HTML: " \t "}, "", "", `{"html":" \t "}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			api101dOnResult(server, "sendRichMessage", api101dReply)
			client := testutil.NewTestClient(t, server.BaseURL())
			_, err := client.SendRichMessage(context.Background(), sender.SendRichMessageRequest{ChatID: r.chat, RichMessage: r.rich})
			if r.wantRichKey == "" {
				api101dRefused(t, server, err, r.field, r.msg)
				return
			}
			require.NoError(t, err)
			require.Equal(t, 1, server.CaptureCount())
			var body struct {
				RichMessage json.RawMessage `json:"rich_message"`
			}
			server.LastCapture().BodyJSON(t, &body)
			assert.JSONEq(t, r.wantRichKey, string(body.RichMessage))
		})
	}
}

// T-srm-6
func TestAPI101D_SendRichMessageAPIError(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On(api101dPath("sendRichMessage"), func(w http.ResponseWriter, _ *http.Request) {
		testutil.ReplyBadRequest(w, "RICH_MESSAGE_MARKDOWN_INVALID")
	})
	client := testutil.NewTestClient(t, server.BaseURL())
	msg, err := client.SendRichMessage(context.Background(), sender.SendRichMessageRequest{
		ChatID: api101dChat, RichMessage: tg.InputRichMessage{Markdown: "m"}})
	assert.Nil(t, msg)
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr), "error %v", err)
	assert.Equal(t, 400, apiErr.Code)
	assert.Equal(t, "Bad Request: RICH_MESSAGE_MARKDOWN_INVALID", apiErr.Description)
	assert.Equal(t, "sendRichMessage", apiErr.Method)
}

// T-srd-1
func TestAPI101D_SendRichMessageDraftBody(t *testing.T) {
	server := testutil.NewMockServer(t)
	api101dOnResult(server, "sendRichMessageDraft", "true")
	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SendRichMessageDraft(context.Background(), sender.SendRichMessageDraftRequest{
		ChatID: api101dChat, MessageThreadID: 7, DraftID: 3, RichMessage: tg.InputRichMessage{Markdown: api101dAsGiven}})
	require.NoError(t, err)
	require.Equal(t, 1, server.CaptureCount())
	c := server.LastCapture()
	c.AssertPath(t, api101dPath("sendRichMessageDraft"))
	assert.JSONEq(t, `{"chat_id":123456789,"message_thread_id":7,"draft_id":3,`+
		`"rich_message":{"markdown":" **B** \t\n<b>&amp;</b> HTTPS://E.x/A%2fb?a=+ ü "}}`, c.BodyString())

	server.ResetCaptures()
	require.NoError(t, client.SendRichMessageDraft(context.Background(), sender.SendRichMessageDraftRequest{
		ChatID: api101dChat, DraftID: 3, RichMessage: tg.InputRichMessage{HTML: api101dHTMLMixed}}))
	require.Equal(t, 1, server.CaptureCount())
	assert.JSONEq(t, `{"chat_id":123456789,"draft_id":3,"rich_message":{"html":"<p>Hello <B>World</B> &amp; \"Ünï\" ✓</p>"}}`,
		server.LastCapture().BodyString())
}

// api101dDraftRow calls SendRichMessageDraft on a fresh server; field/msg "" = a valid request whose body chat_id and
// draft_id are checked.
func api101dDraftRow(t *testing.T, req sender.SendRichMessageDraftRequest, field, msg, wantBody string) {
	t.Helper()
	server := testutil.NewMockServer(t)
	api101dOnResult(server, "sendRichMessageDraft", "true")
	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SendRichMessageDraft(context.Background(), req)
	if wantBody == "" {
		api101dRefused(t, server, err, field, msg)
		return
	}
	require.NoError(t, err)
	require.Equal(t, 1, server.CaptureCount())
	assert.JSONEq(t, wantBody, server.LastCapture().BodyString())
}

// T-srd-2
func TestAPI101D_SendRichMessageDraftChatID(t *testing.T) {
	rich := tg.InputRichMessage{Markdown: "m"}
	rows := []struct {
		name       string
		chat       tg.ChatID
		field, msg string
		wantBody   string
	}{
		{"int64", int64(5), "", "", `{"chat_id":5,"draft_id":1,"rich_message":{"markdown":"m"}}`},
		{"int", int(5), "", "", `{"chat_id":5,"draft_id":1,"rich_message":{"markdown":"m"}}`},
		{"int64 zero", int64(0), "chat_id", "must be non-zero", ""},
		{"int zero", int(0), "chat_id", "must be non-zero", ""},
		{"nil", nil, "chat_id", "required", ""},
		{"string", "@x", "chat_id", api101dDraftInteger, ""},
		{"int32", int32(5), "chat_id", api101dDraftInteger, ""},
		{"uint", uint(5), "chat_id", api101dDraftInteger, ""}, // C1-04
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			api101dDraftRow(t, sender.SendRichMessageDraftRequest{ChatID: r.chat, DraftID: 1, RichMessage: rich}, r.field, r.msg, r.wantBody)
		})
	}
}

// T-srd-3
func TestAPI101D_SendRichMessageDraftID(t *testing.T) {
	rich := tg.InputRichMessage{Markdown: "m"}
	for _, r := range []struct {
		id       int
		wantBody string
	}{
		{0, ""},
		{-7, `{"chat_id":123456789,"draft_id":-7,"rich_message":{"markdown":"m"}}`},
		{1, `{"chat_id":123456789,"draft_id":1,"rich_message":{"markdown":"m"}}`},
	} {
		t.Run(strconv.Itoa(r.id), func(t *testing.T) {
			api101dDraftRow(t, sender.SendRichMessageDraftRequest{ChatID: api101dChat, DraftID: r.id, RichMessage: rich},
				"draft_id", "must be non-zero", r.wantBody)
		})
	}
}

// T-srd-4
func TestAPI101D_SendRichMessageDraftRich(t *testing.T) {
	both := tg.InputRichMessage{HTML: "h", Markdown: "m"}
	rows := []struct {
		name       string
		req        sender.SendRichMessageDraftRequest
		field, msg string
	}{
		{"neither", sender.SendRichMessageDraftRequest{ChatID: api101dChat, DraftID: 1}, "rich_message", "html or markdown required"},
		{"both", sender.SendRichMessageDraftRequest{ChatID: api101dChat, DraftID: 1, RichMessage: both}, "rich_message", "use only one of html, markdown"},
		{"all bad: chat first", sender.SendRichMessageDraftRequest{ChatID: "@x", RichMessage: both}, "chat_id", api101dDraftInteger},
		{"draft and rich bad: draft first", sender.SendRichMessageDraftRequest{ChatID: api101dChat, RichMessage: both}, "draft_id", "must be non-zero"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) { api101dDraftRow(t, r.req, r.field, r.msg, "") })
	}
	t.Run("whitespace html kept", func(t *testing.T) { // T1-01
		api101dDraftRow(t, sender.SendRichMessageDraftRequest{ChatID: api101dChat, DraftID: 1,
			RichMessage: tg.InputRichMessage{HTML: " \t "}}, "", "",
			`{"chat_id":123456789,"draft_id":1,"rich_message":{"html":" \t "}}`)
	})
}

// api101dEdit runs EditMessageText on a fresh server that answers with reply.
func api101dEdit(t *testing.T, req sender.EditMessageTextRequest, reply func(http.ResponseWriter)) (*testutil.MockTelegramServer, *tg.Message, error) {
	t.Helper()
	server := testutil.NewMockServer(t)
	server.On(api101dPath("editMessageText"), func(w http.ResponseWriter, _ *http.Request) { reply(w) })
	client := testutil.NewTestClient(t, server.BaseURL())
	msg, err := client.EditMessageText(context.Background(), req)
	return server, msg, err
}

func api101dReplyRich(w http.ResponseWriter) {
	api96ReplyRaw(w, `{"ok":true,"result":`+api101dReply+`}`)
}

// T-emt-1
func TestAPI101D_EditMessageTextRich(t *testing.T) {
	server, msg, err := api101dEdit(t, sender.EditMessageTextRequest{ChatID: api101dChat, MessageID: 9,
		RichMessage: &tg.InputRichMessage{Markdown: "m"}}, api101dReplyRich)
	require.NoError(t, err)
	require.Equal(t, 1, server.CaptureCount())
	assert.JSONEq(t, `{"chat_id":123456789,"message_id":9,"rich_message":{"markdown":"m"}}`, server.LastCapture().BodyString())
	require.NotNil(t, msg)
	assert.NotNil(t, msg.RichMessage)

	// C1-07: HTML is sent as given.
	server, _, err = api101dEdit(t, sender.EditMessageTextRequest{ChatID: api101dChat, MessageID: 9,
		RichMessage: &tg.InputRichMessage{HTML: api101dHTMLMixed}}, api101dReplyRich)
	require.NoError(t, err)
	require.Equal(t, 1, server.CaptureCount())
	assert.JSONEq(t, `{"chat_id":123456789,"message_id":9,"rich_message":{"html":"<p>Hello <B>World</B> &amp; \"Ünï\" ✓</p>"}}`,
		server.LastCapture().BodyString())
}

// T-emt-2: the bytes of a text edit are the base encoding (struct field order), not only JSON-equal.
func TestAPI101D_EditMessageTextUnchanged(t *testing.T) {
	rows := []struct {
		name string
		req  sender.EditMessageTextRequest
		want string
	}{
		{"text", sender.EditMessageTextRequest{ChatID: api101dChat, MessageID: 9, Text: "t"},
			`{"chat_id":123456789,"message_id":9,"text":"t"}`},
		{"text and options", sender.EditMessageTextRequest{ChatID: api101dChat, MessageID: 9, Text: "t", ParseMode: tg.ParseModeHTML,
			LinkPreviewOptions: &tg.LinkPreviewOptions{IsDisabled: true},
			ReplyMarkup:        testutil.TestInlineKeyboard([]tg.InlineKeyboardButton{testutil.TestInlineButton("b", "d")})},
			`{"chat_id":123456789,"message_id":9,"text":"t","parse_mode":"HTML","link_preview_options":{"is_disabled":true},` +
				`"reply_markup":{"inline_keyboard":[[{"text":"b","callback_data":"d"}]]}}`},
		{"inline", sender.EditMessageTextRequest{InlineMessageID: "im", Text: "t"}, `{"inline_message_id":"im","text":"t"}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server, _, err := api101dEdit(t, r.req, api101dReplyRich)
			require.NoError(t, err)
			require.Equal(t, 1, server.CaptureCount())
			assert.Equal(t, r.want, server.LastCapture().BodyString())
		})
	}
}

// T-emt-3
func TestAPI101D_EditMessageTextBoth(t *testing.T) {
	server, _, err := api101dEdit(t, sender.EditMessageTextRequest{ChatID: api101dChat, MessageID: 9, Text: "t",
		RichMessage: &tg.InputRichMessage{HTML: "<p>h</p>"}}, api101dReplyRich)
	require.NoError(t, err)
	require.Equal(t, 1, server.CaptureCount())
	assert.JSONEq(t, `{"chat_id":123456789,"message_id":9,"text":"t","rich_message":{"html":"<p>h</p>"}}`, server.LastCapture().BodyString())
}

// T-emt-4
func TestAPI101D_EditMessageTextNeither(t *testing.T) {
	server, msg, err := api101dEdit(t, sender.EditMessageTextRequest{ChatID: api101dChat, MessageID: 9},
		func(w http.ResponseWriter) { testutil.ReplyBadRequest(w, "message text is empty") })
	assert.Nil(t, msg)
	require.Equal(t, 1, server.CaptureCount(), "no local check: the request is sent")
	assert.JSONEq(t, `{"chat_id":123456789,"message_id":9}`, server.LastCapture().BodyString())
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr), "error %v", err)
	assert.Equal(t, "Bad Request: message text is empty", apiErr.Description)
}

// T-emt-5
func TestAPI101D_EditMessageTextRichInvalid(t *testing.T) {
	both := &tg.InputRichMessage{HTML: "h", Markdown: "m"}
	rows := []struct {
		name       string
		req        sender.EditMessageTextRequest
		field, msg string
	}{
		{"both, no text", sender.EditMessageTextRequest{ChatID: api101dChat, MessageID: 9, RichMessage: both},
			"rich_message", "use only one of html, markdown"},
		{"both, with text", sender.EditMessageTextRequest{ChatID: api101dChat, MessageID: 9, Text: "t", RichMessage: both},
			"rich_message", "use only one of html, markdown"},
		{"neither", sender.EditMessageTextRequest{ChatID: api101dChat, MessageID: 9, RichMessage: &tg.InputRichMessage{}},
			"rich_message", "html or markdown required"},
		{"link preview first", sender.EditMessageTextRequest{ChatID: api101dChat, MessageID: 9, RichMessage: both,
			LinkPreviewOptions: &tg.LinkPreviewOptions{IsDisabled: true, PreferSmallMedia: true, PreferLargeMedia: true}},
			"", "link_preview_options: PreferSmallMedia and PreferLargeMedia are mutually exclusive"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server, _, err := api101dEdit(t, r.req, api101dReplyRich)
			api101dRefused(t, server, err, r.field, r.msg)
		})
	}
	t.Run("whitespace html kept", func(t *testing.T) { // T1-01
		server, _, err := api101dEdit(t, sender.EditMessageTextRequest{ChatID: api101dChat, MessageID: 9,
			RichMessage: &tg.InputRichMessage{HTML: " \t "}}, api101dReplyRich)
		require.NoError(t, err)
		require.Equal(t, 1, server.CaptureCount())
		assert.JSONEq(t, `{"chat_id":123456789,"message_id":9,"rich_message":{"html":" \t "}}`, server.LastCapture().BodyString())
	})
}

// T-emt-6
func TestAPI101D_EditHelper(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On(api101dPath("editMessageText"), func(w http.ResponseWriter, _ *http.Request) { api101dReplyRich(w) })
	client := testutil.NewTestClient(t, server.BaseURL())
	m := testutil.TestMessageInChat(9, api101dChat, "old")
	_, err := client.Edit(context.Background(), m, "t")
	require.NoError(t, err)
	assert.JSONEq(t, `{"chat_id":123456789,"message_id":9,"text":"t"}`, server.LastCapture().BodyString())
	server.ResetCaptures()
	_, err = client.Edit(context.Background(), m, "")
	require.NoError(t, err)
	assert.JSONEq(t, `{"chat_id":123456789,"message_id":9}`, server.LastCapture().BodyString())
}

type api101dField struct{ name, typ, tag string }

func api101dFields(v any) []api101dField {
	rt := reflect.TypeOf(v)
	var out []api101dField
	for i := range rt.NumField() {
		f := rt.Field(i)
		out = append(out, api101dField{f.Name, f.Type.String(), f.Tag.Get("json")})
	}
	return out
}

// T-emt-7: every field of EditMessageTextRequest; only Text's tag and the new RichMessage differ from the base.
func TestAPI101D_EditMessageTextShape(t *testing.T) {
	assert.Equal(t, []api101dField{
		{"ChatID", "interface {}", "chat_id,omitempty"},
		{"MessageID", "int", "message_id,omitempty"},
		{"InlineMessageID", "string", "inline_message_id,omitempty"},
		{"Text", "string", "text,omitempty"},
		{"ParseMode", "tg.ParseMode", "parse_mode,omitempty"},
		{"LinkPreviewOptions", "*tg.LinkPreviewOptions", "link_preview_options,omitempty"},
		{"RichMessage", "*tg.InputRichMessage", "rich_message,omitempty"},
		{"ReplyMarkup", "interface {}", "reply_markup,omitempty"},
		{"DisableWebPagePreview", "bool", "disable_web_page_preview,omitempty"},
	}, api101dFields(sender.EditMessageTextRequest{}))
}

// T-shape
func TestAPI101D_RequestShapes(t *testing.T) {
	assert.Equal(t, []api101dField{
		{"BusinessConnectionID", "string", "business_connection_id,omitempty"},
		{"ChatID", "interface {}", "chat_id"},
		{"MessageThreadID", "int", "message_thread_id,omitempty"},
		{"DirectMessagesTopicID", "int", "direct_messages_topic_id,omitempty"},
		{"RichMessage", "tg.InputRichMessage", "rich_message"},
		{"DisableNotification", "bool", "disable_notification,omitempty"},
		{"ProtectContent", "bool", "protect_content,omitempty"},
		{"AllowPaidBroadcast", "bool", "allow_paid_broadcast,omitempty"},
		{"MessageEffectID", "string", "message_effect_id,omitempty"},
		{"ReplyParameters", "*tg.ReplyParameters", "reply_parameters,omitempty"},
		{"ReplyMarkup", "interface {}", "reply_markup,omitempty"},
	}, api101dFields(sender.SendRichMessageRequest{}))
	assert.Equal(t, []api101dField{
		{"ChatID", "interface {}", "chat_id"},
		{"MessageThreadID", "int", "message_thread_id,omitempty"},
		{"DraftID", "int", "draft_id"},
		{"RichMessage", "tg.InputRichMessage", "rich_message"},
	}, api101dFields(sender.SendRichMessageDraftRequest{}))
}

// T-irc-3
func TestAPI101D_AnswerInlineQueryRich(t *testing.T) {
	server := testutil.NewMockServer(t)
	api101dOnResult(server, "answerInlineQuery", "true")
	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.AnswerInlineQuery(context.Background(), sender.AnswerInlineQueryRequest{InlineQueryID: "q",
		Results: []tg.InlineQueryResult{&tg.InlineQueryResultArticle{Type: "article", ID: "1", Title: "t",
			InputMessageContent: tg.InputRichMessageContent{RichMessage: tg.InputRichMessage{Markdown: "m"}}}}})
	require.NoError(t, err)
	var body struct {
		Results []struct {
			InputMessageContent json.RawMessage `json:"input_message_content"`
		} `json:"results"`
	}
	server.LastCapture().BodyJSON(t, &body)
	require.Len(t, body.Results, 1)
	assert.JSONEq(t, `{"rich_message":{"markdown":"m"}}`, string(body.Results[0].InputMessageContent))
}

// api101dMethods calls each new method with a valid request.
var api101dMethods = []struct {
	method string
	call   func(context.Context, *sender.Client) error
}{
	{"sendRichMessage", func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendRichMessage(ctx, sender.SendRichMessageRequest{ChatID: api100Chat, RichMessage: tg.InputRichMessage{Markdown: "m"}})
		return err
	}},
	{"sendRichMessageDraft", func(ctx context.Context, c *sender.Client) error {
		return c.SendRichMessageDraft(ctx, sender.SendRichMessageDraftRequest{ChatID: api100Chat, DraftID: 1,
			RichMessage: tg.InputRichMessage{Markdown: "m"}})
	}},
}

// T-nr-1: neither method retries a retryable API reply (502, 429); SendMessage, which does, is the control.
func TestAPI101D_NoRetry(t *testing.T) {
	sets := []struct {
		name  string
		first func(http.ResponseWriter)
		check func(*testing.T, error)
	}{
		{"502", func(w http.ResponseWriter) { testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway") },
			func(t *testing.T, err error) {
				var apiErr *tg.APIError
				require.True(t, errors.As(err, &apiErr), "error %v", err)
				assert.Equal(t, 502, apiErr.Code)
			}},
		{"429", func(w http.ResponseWriter) { testutil.ReplyRateLimit(w, 1) },
			func(t *testing.T, err error) {
				var apiErr *tg.APIError
				require.True(t, errors.As(err, &apiErr), "error %v", err)
				assert.Equal(t, 429, apiErr.Code)
			}},
	}
	for _, set := range sets {
		t.Run(set.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			for _, p := range []string{"sendMessage", "sendRichMessage", "sendRichMessageDraft"} {
				var n atomic.Int32
				server.On(api101dPath(p), func(w http.ResponseWriter, _ *http.Request) {
					if n.Add(1) == 1 {
						set.first(w)
						return
					}
					testutil.ReplyMessage(w, 4242)
				})
			}
			sleeper := &testutil.FakeSleeper{}
			client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRateLimit(1000, 100),
				sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
			for _, m := range api101dMethods {
				server.ResetCaptures()
				set.check(t, m.call(context.Background(), client))
				assert.Equal(t, 1, server.CaptureCount(), "%s: exactly one request", m.method)
			}
			assert.Equal(t, 0, sleeper.CallCount(), "no backoff sleep before the control")

			server.ResetCaptures()
			_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api100Chat, Text: "hi"})
			require.NoError(t, err)
			assert.Equal(t, 2, server.CaptureCount(), "the control call is retried once")
			assert.GreaterOrEqual(t, sleeper.CallCount(), 1, "the sleeper is wired")
		})
	}
}

// api101dTimeout is a network timeout; isRetryable (sender/client.go) retries it.
type api101dTimeout struct{}

func (api101dTimeout) Error() string   { return "synthetic network timeout" }
func (api101dTimeout) Timeout() bool   { return true }
func (api101dTimeout) Temporary() bool { return true }

type api101dFailTransport struct{ n atomic.Int32 }

func (rt *api101dFailTransport) RoundTrip(*http.Request) (*http.Response, error) {
	rt.n.Add(1)
	return nil, api101dTimeout{}
}

// T-nr-2: neither method retries a network timeout; SendMessage, which does, is the control.
func TestAPI101D_NoRetryNetworkError(t *testing.T) {
	rt := &api101dFailTransport{}
	sleeper := &testutil.FakeSleeper{}
	client := testutil.NewRetryTestClient(t, "http://galigo.invalid", sleeper, sender.WithHTTPClient(&http.Client{Transport: rt}),
		sender.WithRateLimit(1000, 100), sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
	for _, m := range api101dMethods {
		rt.n.Store(0)
		err := m.call(context.Background(), client)
		require.Error(t, err, m.method)
		var ne net.Error
		require.True(t, errors.As(err, &ne) && ne.Timeout(), "%s: error %v", m.method, err)
		assert.Equal(t, int32(1), rt.n.Load(), "%s: exactly one round trip", m.method)
	}
	assert.Equal(t, 0, sleeper.CallCount(), "no backoff sleep before the control")

	rt.n.Store(0)
	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api100Chat, Text: "hi"})
	require.Error(t, err)
	assert.GreaterOrEqual(t, rt.n.Load(), int32(2), "the control call is retried")
}

// T-rl-1 (T1-02, T2-01): each new call uses the per-chat rate limiter of the request's chat (sender/client.go:600-603):
// one limiter after a call, the same one for a second call on that chat, a second limiter for another chat. A refused
// call adds none.
func TestAPI101D_PerChatLimiter(t *testing.T) {
	rich := tg.InputRichMessage{Markdown: "m"}
	const other = int64(987654321)
	rows := []struct {
		method, result string
		call           func(c *sender.Client, chat int64) error
	}{
		{"sendRichMessage", api101dReply, func(c *sender.Client, chat int64) error {
			_, err := c.SendRichMessage(context.Background(), sender.SendRichMessageRequest{ChatID: chat, RichMessage: rich})
			return err
		}},
		{"sendRichMessageDraft", "true", func(c *sender.Client, chat int64) error {
			return c.SendRichMessageDraft(context.Background(), sender.SendRichMessageDraftRequest{ChatID: chat, DraftID: 1, RichMessage: rich})
		}},
		{"editMessageText", api101dReply, func(c *sender.Client, chat int64) error {
			_, err := c.EditMessageText(context.Background(), sender.EditMessageTextRequest{ChatID: chat, MessageID: 9, RichMessage: &rich})
			return err
		}},
	}
	newClient := func(t *testing.T, method, result string) (*sender.Client, *testutil.MockTelegramServer) {
		server := testutil.NewMockServer(t)
		api101dOnResult(server, method, result)
		client, err := sender.New(testutil.TestToken, sender.WithBaseURL(server.BaseURL()),
			sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()), sender.WithRetries(0))
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Close() })
		return client, server
	}
	for _, r := range rows {
		t.Run(r.method, func(t *testing.T) {
			client, server := newClient(t, r.method, r.result)
			require.Equal(t, 0, client.ChatLimiterCount())
			require.NoError(t, r.call(client, api101dChat))
			assert.Equal(t, 1, client.ChatLimiterCount(), "after the call on chat A")
			require.NoError(t, r.call(client, api101dChat))
			assert.Equal(t, 1, client.ChatLimiterCount(), "after a second call on chat A")
			require.NoError(t, r.call(client, other))
			assert.Equal(t, 2, client.ChatLimiterCount(), "after the call on chat B")
			assert.Equal(t, 3, server.CaptureCount())
		})
	}
	// Each method WAITS on the chat's limiter (C2-01): with 1 call per 100 s and burst 1, a second call on the same
	// chat with a 5 s deadline fails in the limiter at once and sends no request. The deadline is far below the 100 s
	// refill, so the limiter refuses the reservation; a short deadline could expire first on a slow runner and give
	// ctx.Err() instead (C3-03).
	waits := []struct {
		method, result string
		call           func(ctx context.Context, c *sender.Client) error
	}{
		{"sendRichMessage", api101dReply, func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendRichMessage(ctx, sender.SendRichMessageRequest{ChatID: api101dChat, RichMessage: rich})
			return err
		}},
		{"sendRichMessageDraft", "true", func(ctx context.Context, c *sender.Client) error {
			return c.SendRichMessageDraft(ctx, sender.SendRichMessageDraftRequest{ChatID: api101dChat, DraftID: 1, RichMessage: rich})
		}},
		{"editMessageText", api101dReply, func(ctx context.Context, c *sender.Client) error {
			_, err := c.EditMessageText(ctx, sender.EditMessageTextRequest{ChatID: api101dChat, MessageID: 9, RichMessage: &rich})
			return err
		}},
	}
	for _, r := range waits {
		t.Run("waits "+r.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			api101dOnResult(server, r.method, r.result)
			client, err := sender.New(testutil.TestToken, sender.WithBaseURL(server.BaseURL()),
				sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()), sender.WithRetries(0),
				sender.WithPerChatRateLimit(0.01, 1))
			require.NoError(t, err)
			t.Cleanup(func() { _ = client.Close() })
			require.NoError(t, r.call(context.Background(), client))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err = r.call(ctx, client)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "would exceed context deadline")
			assert.Equal(t, 1, server.CaptureCount())
		})
	}
	// Each method uses the SAME chat limiter as the other methods (C3-01): after a SendMessage to the chat, a call of
	// the method on that chat fails in the limiter and sends no request. A method keyed on another string for the
	// same chat would get its own limiter and pass the rows above.
	for _, r := range waits {
		t.Run("shares "+r.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			api101dOnResult(server, "sendMessage", api101dReply)
			api101dOnResult(server, r.method, r.result)
			client, err := sender.New(testutil.TestToken, sender.WithBaseURL(server.BaseURL()),
				sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()), sender.WithRetries(0),
				sender.WithPerChatRateLimit(0.01, 1))
			require.NoError(t, err)
			t.Cleanup(func() { _ = client.Close() })
			_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api101dChat, Text: "x"})
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err = r.call(ctx, client)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "would exceed context deadline")
			assert.Equal(t, 1, server.CaptureCount())
		})
	}

	// A refused call (bad rich message) of each method, on a fresh client: no limiter, no request (C1-06).
	refused := []struct {
		method, result string
		call           func(c *sender.Client) error
	}{
		{"sendRichMessage", api101dReply, func(c *sender.Client) error {
			_, err := c.SendRichMessage(context.Background(), sender.SendRichMessageRequest{ChatID: api101dChat})
			return err
		}},
		{"sendRichMessageDraft", "true", func(c *sender.Client) error {
			return c.SendRichMessageDraft(context.Background(), sender.SendRichMessageDraftRequest{ChatID: api101dChat, DraftID: 1})
		}},
		{"editMessageText", api101dReply, func(c *sender.Client) error {
			_, err := c.EditMessageText(context.Background(), sender.EditMessageTextRequest{ChatID: api101dChat, MessageID: 9,
				RichMessage: &tg.InputRichMessage{}})
			return err
		}},
	}
	for _, r := range refused {
		t.Run("refused "+r.method, func(t *testing.T) {
			client, server := newClient(t, r.method, r.result)
			require.Error(t, r.call(client))
			assert.Equal(t, 0, client.ChatLimiterCount())
			assert.Equal(t, 0, server.CaptureCount())
		})
	}
}
