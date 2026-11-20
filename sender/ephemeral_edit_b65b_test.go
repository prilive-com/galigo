package sender_test

// B-65 65b: the Bot API 10.3 changes of the ephemeral edit methods (TEST-PLAN §2): rich_message on
// editEphemeralMessageText (text optional), show_caption_above_media on editEphemeralMessageCaption. Bodies are
// compared whole; the common target is api102cBody (api102c_test.go).

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// b65bText returns an editEphemeralMessageText request for the api102c target with the given content.
func b65bText(text string, rm *tg.InputRichMessage) sender.EditEphemeralMessageTextRequest {
	return sender.EditEphemeralMessageTextRequest{ChatID: api102cChat, ReceiverUserID: api102cRecv,
		EphemeralMessageID: api102cEph, Text: text, RichMessage: rm}
}

// T-rich-json, T-rich-full, T-text-only, T-empty: the JSON body of editEphemeralMessageText.
func TestB65B_EditTextBody(t *testing.T) {
	rows := []struct {
		name string
		req  sender.EditEphemeralMessageTextRequest
		want string
	}{
		{"rich only", b65bText("", &tg.InputRichMessage{Markdown: "m"}),
			`{` + api102cBody + `,"rich_message":{"markdown":"m"}}`},
		{"text only", b65bText("t", nil), `{` + api102cBody + `,"text":"t"}`},
		{"empty, no rich", b65bText("", nil), `{` + api102cBody + `}`},
		{"all fields", sender.EditEphemeralMessageTextRequest{ChatID: api102cChat, ReceiverUserID: api102cRecv,
			EphemeralMessageID: api102cEph, Text: "t", ParseMode: tg.ParseModeHTML,
			Entities:           []tg.MessageEntity{{Type: "bold", Offset: 0, Length: 1}},
			RichMessage:        &tg.InputRichMessage{HTML: "<p>h</p>", IsRTL: true},
			LinkPreviewOptions: &tg.LinkPreviewOptions{IsDisabled: true}, ReplyMarkup: api102cKeyboard()},
			`{` + api102cBody + `,"text":"t","parse_mode":"HTML","entities":[{"type":"bold","offset":0,"length":1}],` +
				`"rich_message":{"html":"<p>h</p>","is_rtl":true},"link_preview_options":{"is_disabled":true},` +
				`"reply_markup":` + api102cKeyboardJSON + `}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			api102cJSONCall(t, "editEphemeralMessageText", r.want, func(ctx context.Context, c *sender.Client) error {
				return c.EditEphemeralMessageText(ctx, r.req)
			})
		})
	}
}

// T-rich-upload: an upload in RichMessage.Media (value and pointer item) is a file part with an attach:// reference;
// the request goes as multipart/form-data with exactly the target arguments and rich_message.
func TestB65B_EditTextRichUpload(t *testing.T) {
	data := []byte("b65b-photo-bytes")
	media := sender.NewInputMediaPhoto(sender.FromBytes(data, "p.jpg"))
	rows := []struct {
		name string
		item tg.InputRichMedia
	}{
		{"value", media},
		{"pointer", &media},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			rm := &tg.InputRichMessage{Markdown: "![](tg://photo?id=e)", Media: []tg.InputRichMessageMedia{{ID: "e", Media: r.item}}}
			mediaType, args, files, err := wireCall(t, "editEphemeralMessageText", api102cReplyTrue,
				func(ctx context.Context, c *sender.Client) error {
					return c.EditEphemeralMessageText(ctx, b65bText("", rm))
				})
			require.NoError(t, err)
			assert.Equal(t, "multipart/form-data", mediaType)
			assertArgs(t, map[string]string{
				"chat_id": "-1001234567890", "receiver_user_id": "5000000001", "ephemeral_message_id": "4242",
				"rich_message": `{"markdown":"![](tg://photo?id=e)","media":[{"id":"e","media":{"media":"attach://file0","type":"photo"}}]}`,
			}, args)
			assertFiles(t, map[string]wireFile{"file0": {FileName: "p.jpg", Data: data}}, files)
		})
	}
}

// T-rich-fileid: a file_id item needs no form: the request stays JSON.
func TestB65B_EditTextRichFileID(t *testing.T) {
	rm := &tg.InputRichMessage{Markdown: "![](tg://photo?id=e)", Media: []tg.InputRichMessageMedia{
		{ID: "e", Media: sender.NewInputMediaPhoto(sender.FromFileID("AgAD-fake"))}}}
	api102cJSONCall(t, "editEphemeralMessageText",
		`{`+api102cBody+`,"rich_message":{"markdown":"![](tg://photo?id=e)","media":[{"id":"e","media":{"type":"photo","media":"AgAD-fake"}}]}}`,
		func(ctx context.Context, c *sender.Client) error {
			return c.EditEphemeralMessageText(ctx, b65bText("", rm))
		})
}

// T-rich-invalid, T-rich-blocks: RichMessage must hold exactly one of html, markdown, blocks (validateInputRichMessage,
// the EditMessageText rule); a refused one sends nothing.
func TestB65B_EditTextRichValidation(t *testing.T) {
	block := []tg.InputRichBlock{tg.InputRichBlockDivider{}}
	rows := []struct {
		name string
		rm   tg.InputRichMessage
		want string // "" = accepted
	}{
		{"none", tg.InputRichMessage{IsRTL: true}, "galigo: validation: rich_message - html, markdown or blocks required"},
		{"markdown and html", tg.InputRichMessage{Markdown: "m", HTML: "h"}, "galigo: validation: rich_message - use only one of html, markdown, blocks"},
		{"all three", tg.InputRichMessage{Markdown: "m", HTML: "h", Blocks: block}, "galigo: validation: rich_message - use only one of html, markdown, blocks"},
		{"blocks only", tg.InputRichMessage{Blocks: block}, ""},
		{"html only", tg.InputRichMessage{HTML: "<p>h</p>"}, ""},
	}
	for _, r := range rows {
		for _, text := range []string{"", "t"} { // "" is S73's case: the rich message alone (A-T1-04)
			t.Run(r.name+"/text="+text, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				api102cOK(server, "editEphemeralMessageText")
				client := testutil.NewTestClient(t, server.BaseURL())
				rm := r.rm
				err := client.EditEphemeralMessageText(context.Background(), b65bText(text, &rm))
				if r.want == "" {
					require.NoError(t, err)
					assert.Equal(t, 1, server.CaptureCount())
					return
				}
				require.Error(t, err)
				assert.Equal(t, r.want, err.Error())
				var ve *tg.ValidationError
				require.True(t, errors.As(err, &ve), "error %v", err)
				assert.Equal(t, "rich_message", ve.Field)
				assert.Equal(t, 0, server.CaptureCount(), "no request")
				assert.Equal(t, 0, client.ChatLimiterCount(), "no limiter")
			})
		}
	}
}

// T-rich-invalid with an upload (A-T2-03, E-0159): an invalid rich message that carries an upload in its media list is
// refused before any request, and the upload is not opened.
func TestB65B_EditTextRichInvalidUpload(t *testing.T) {
	var cnt api102eCounter
	server := testutil.NewMockServer(t)
	api102cOK(server, "editEphemeralMessageText")
	client := testutil.NewTestClient(t, server.BaseURL())
	rm := &tg.InputRichMessage{Markdown: "m", HTML: "h", Media: []tg.InputRichMessageMedia{
		{ID: "e", Media: sender.NewInputMediaPhoto(cnt.source("x"))}}}
	err := client.EditEphemeralMessageText(context.Background(), b65bText("", rm))
	var ve *tg.ValidationError
	require.True(t, errors.As(err, &ve), "error %v", err)
	assert.Equal(t, "rich_message", ve.Field)
	assert.Equal(t, "galigo: validation: rich_message - use only one of html, markdown, blocks", err.Error())
	assert.Equal(t, 0, server.CaptureCount(), "no request")
	cnt.assertUnused(t)
}

// T-order: the target is checked first, then the link preview, then the rich message.
func TestB65B_EditTextValidationOrder(t *testing.T) {
	bad := &tg.InputRichMessage{}
	rows := []struct {
		name string
		req  sender.EditEphemeralMessageTextRequest
		want string
	}{
		{"target before rich", sender.EditEphemeralMessageTextRequest{ChatID: api102cChat, ReceiverUserID: 0,
			EphemeralMessageID: api102cEph, RichMessage: bad}, "galigo: validation: receiver_user_id - must be positive"},
		{"link preview before rich", sender.EditEphemeralMessageTextRequest{ChatID: api102cChat, ReceiverUserID: api102cRecv,
			EphemeralMessageID: api102cEph, RichMessage: bad,
			LinkPreviewOptions: &tg.LinkPreviewOptions{PreferSmallMedia: true, PreferLargeMedia: true}},
			"link_preview_options: PreferSmallMedia and PreferLargeMedia are mutually exclusive"},
		{"rich after a valid target and link preview", sender.EditEphemeralMessageTextRequest{ChatID: api102cChat,
			ReceiverUserID: api102cRecv, EphemeralMessageID: api102cEph, RichMessage: bad,
			LinkPreviewOptions: &tg.LinkPreviewOptions{PreferSmallMedia: true}},
			"galigo: validation: rich_message - html, markdown or blocks required"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			api102cOK(server, "editEphemeralMessageText")
			client := testutil.NewTestClient(t, server.BaseURL())
			err := client.EditEphemeralMessageText(context.Background(), r.req)
			require.Error(t, err)
			assert.Equal(t, r.want, err.Error())
			assert.Equal(t, 0, server.CaptureCount())
		})
	}
}

// T-cap-above: show_caption_above_media true is sent; false is left out (TestAPI102C_EditCaptionMinimal).
func TestB65B_EditCaptionAbove(t *testing.T) {
	api102cJSONCall(t, "editEphemeralMessageCaption", `{`+api102cBody+`,"caption":"c","show_caption_above_media":true}`,
		func(ctx context.Context, c *sender.Client) error {
			return c.EditEphemeralMessageCaption(ctx, sender.EditEphemeralMessageCaptionRequest{
				ChatID: api102cChat, ReceiverUserID: api102cRecv, EphemeralMessageID: api102cEph,
				Caption: "c", ShowCaptionAboveMedia: true})
		})
	api102cJSONCall(t, "editEphemeralMessageCaption", `{`+api102cBody+`,"caption":"c"}`,
		func(ctx context.Context, c *sender.Client) error {
			return c.EditEphemeralMessageCaption(ctx, sender.EditEphemeralMessageCaptionRequest{
				ChatID: api102cChat, ReceiverUserID: api102cRecv, EphemeralMessageID: api102cEph, Caption: "c"})
		})
}

// T-noretry (E-0118): with a client that retries (NewRetryTestClient, as TestAPI102C_NoRetry), a rich upload answered
// with 502 is sent once, as multipart, with no backoff sleep; the control call on the same client is retried.
func TestB65B_EditTextRichUploadNoRetry(t *testing.T) {
	server := testutil.NewMockServer(t)
	var edits, sends atomic.Int32
	reply502 := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":502,"description":"Bad Gateway"}`))
	}
	server.On(api102cPath("editEphemeralMessageText"), func(w http.ResponseWriter, _ *http.Request) {
		if edits.Add(1) == 1 {
			reply502(w)
			return
		}
		testutil.ReplyBool(w, true)
	})
	server.On(api102cPath("sendMessage"), func(w http.ResponseWriter, _ *http.Request) {
		if sends.Add(1) == 1 {
			reply502(w)
			return
		}
		testutil.ReplyMessage(w, 1)
	})
	sleeper := &testutil.FakeSleeper{}
	client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRateLimit(1000, 100),
		sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
	rm := &tg.InputRichMessage{Markdown: "![](tg://photo?id=e)", Media: []tg.InputRichMessageMedia{
		{ID: "e", Media: sender.NewInputMediaPhoto(sender.FromBytes([]byte("x"), "p.jpg"))}}}
	err := client.EditEphemeralMessageText(context.Background(), b65bText("", rm))
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr), "error %v", err)
	assert.Equal(t, 502, apiErr.Code)
	assert.Equal(t, int32(1), edits.Load(), "sent once")
	require.Equal(t, 1, server.CaptureCount())
	mt, _, err := mime.ParseMediaType(server.LastCapture().ContentType)
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mt, "the upload request itself (G-T1-01)")
	assert.Equal(t, 0, sleeper.CallCount(), "no backoff sleep")

	_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api102cChat, Text: "hi"})
	require.NoError(t, err)
	assert.Equal(t, int32(2), sends.Load(), "the control call is retried once")
}

// T-rich-block-upload (B-61 for this method): an upload in a rich media block, also a nested one, is refused before
// any request, and its reader is not opened (TestAPI102E_UploadElsewhereRefused for the 10.2 callers).
func TestB65B_EditTextRichBlockUpload(t *testing.T) {
	for _, k := range []string{"block", "in-collage", "document"} {
		t.Run(k, func(t *testing.T) {
			var cnt api102eCounter
			var b tg.InputRichBlock = tg.InputRichBlockPhoto{Photo: sender.NewInputMediaPhoto(cnt.source("x"))}
			switch k {
			case "in-collage":
				b = tg.InputRichBlockCollage{Blocks: []tg.InputRichBlock{
					tg.InputRichBlockAnimation{Animation: sender.NewInputMediaAnimation(cnt.reader("x"))}}}
			case "document": // Bot API 10.3 (B-65 65c)
				b = tg.InputRichBlockDocument{Document: sender.NewInputMediaDocument(cnt.reader("x"))}
			}
			server := testutil.NewMockServer(t)
			api102cOK(server, "editEphemeralMessageText")
			client := testutil.NewTestClient(t, server.BaseURL())
			err := client.EditEphemeralMessageText(context.Background(), b65bText("", &tg.InputRichMessage{Blocks: []tg.InputRichBlock{b}}))
			require.Error(t, err)
			assert.Contains(t, err.Error(), api102eUploadErr)
			assert.Equal(t, 0, server.CaptureCount(), "no request")
			cnt.assertUnused(t)
		})
	}
}
