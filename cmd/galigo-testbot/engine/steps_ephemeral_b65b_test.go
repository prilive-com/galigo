package engine

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-65 65b: EditEphemeralStep, the four edit adapter methods, and S73 with the edits (E65b/TEST-PLAN.md §3).

func (f *b65aFake) edit(method string, req any) error {
	f.edits = append(f.edits, req)
	return f.editErr[method]
}

func (f *b65aFake) EditEphemeralMessageText(_ context.Context, req sender.EditEphemeralMessageTextRequest) error {
	return f.edit("editEphemeralMessageText", req)
}

func (f *b65aFake) EditEphemeralMessageCaption(_ context.Context, req sender.EditEphemeralMessageCaptionRequest) error {
	return f.edit("editEphemeralMessageCaption", req)
}

func (f *b65aFake) EditEphemeralMessageMedia(_ context.Context, req sender.EditEphemeralMessageMediaRequest) error {
	return f.edit("editEphemeralMessageMedia", req)
}

func (f *b65aFake) EditEphemeralMessageReplyMarkup(_ context.Context, req sender.EditEphemeralMessageReplyMarkupRequest) error {
	return f.edit("editEphemeralMessageReplyMarkup", req)
}

// b65bChatOld and b65bChatNew are the chats of the two records: the step must use the newest (the last recorded).
const (
	b65bChatOld = int64(-1001000000001)
	b65bChatNew = int64(-1001000000002)
	b65bRecvOld = int64(1<<33 + 7)
	b65bRecvNew = int64(1<<34 + 9)
)

// b65bPhoto checks that media is exactly the planned photo upload of the fixture: a FromBytes Source (fresh bytes per
// call) with the planned file name, and no other field set (A-T2-02: no caption, parse mode, spoiler, ...).
func b65bPhoto(t *testing.T, media sender.InputMedia) {
	t.Helper()
	assert.Equal(t, "photo", media.Type)
	f := media.Media
	assert.Empty(t, f.FileID)
	assert.Empty(t, f.URL)
	assert.Nil(t, f.Reader, "a Source, not a single-use Reader")
	assert.Equal(t, "galigo-ephemeral-edit.jpg", f.FileName)
	require.NotNil(t, f.Source)
	for range 2 { // a retry gets the same bytes again
		b, err := io.ReadAll(f.Source())
		require.NoError(t, err)
		assert.Equal(t, fixtures.PhotoBytes(), b)
	}
	media.Media.Source = nil // a func is not comparable; the rest must be the plain photo
	assert.Equal(t, sender.NewInputMediaPhoto(sender.InputFile{FileName: "galigo-ephemeral-edit.jpg"}), media)
}

// b65bRuntime returns a runtime with two records: (old chat, old receiver, 7) then (new chat, new receiver, 9).
func b65bRuntime(f *b65aFake) *Runtime {
	rt := b65aRuntime(f, b65aAdmin)
	rt.recordEphemeral(b65bChatOld, b65bRecvOld, 7)
	rt.recordEphemeral(b65bChatNew, b65bRecvNew, 9)
	return rt
}

// T-step-text, T-step-markup, T-step-caption, T-step-media: one edit per step, of the newest record, with the planned
// content; the step reports no message, and the records stay.
func TestB65B_EditEphemeralStep(t *testing.T) {
	target := func(t *testing.T, chat tg.ChatID, recv int64, id int) {
		t.Helper()
		assert.Equal(t, tg.ChatID(b65bChatNew), chat)
		assert.Equal(t, b65bRecvNew, recv)
		assert.Equal(t, 9, id)
	}
	rows := []struct {
		method string
		check  func(t *testing.T, req any)
	}{
		{"editEphemeralMessageText", func(t *testing.T, req any) {
			r, ok := req.(sender.EditEphemeralMessageTextRequest)
			require.True(t, ok, "%T", req)
			target(t, r.ChatID, r.ReceiverUserID, r.EphemeralMessageID)
			assert.Empty(t, r.Text)
			assert.Empty(t, r.ParseMode)
			assert.Nil(t, r.Entities)
			assert.Nil(t, r.LinkPreviewOptions)
			assert.Nil(t, r.ReplyMarkup)
			require.NotNil(t, r.RichMessage)
			rm := *r.RichMessage
			got := rm
			got.Media = nil // checked below; every other field must be the plan's (A-T3-01: no IsRTL, no SkipEntityDetection)
			assert.Equal(t, tg.InputRichMessage{Markdown: "galigo ephemeral **rich** edit\n\n![](tg://photo?id=e)"}, got)
			require.Len(t, rm.Media, 1)
			assert.Equal(t, "e", rm.Media[0].ID)
			m, ok := rm.Media[0].Media.(sender.InputMedia)
			require.True(t, ok, "%T", rm.Media[0].Media)
			b65bPhoto(t, m)
		}},
		{"editEphemeralMessageReplyMarkup", func(t *testing.T, req any) {
			r, ok := req.(sender.EditEphemeralMessageReplyMarkupRequest)
			require.True(t, ok, "%T", req)
			target(t, r.ChatID, r.ReceiverUserID, r.EphemeralMessageID)
			assert.Equal(t, &tg.InlineKeyboardMarkup{InlineKeyboard: [][]tg.InlineKeyboardButton{{{Text: "galigo", CallbackData: "b65b"}}}},
				r.ReplyMarkup)
		}},
		{"editEphemeralMessageCaption", func(t *testing.T, req any) {
			r, ok := req.(sender.EditEphemeralMessageCaptionRequest)
			require.True(t, ok, "%T", req)
			target(t, r.ChatID, r.ReceiverUserID, r.EphemeralMessageID)
			assert.Equal(t, sender.EditEphemeralMessageCaptionRequest{ChatID: r.ChatID, ReceiverUserID: r.ReceiverUserID,
				EphemeralMessageID: r.EphemeralMessageID, Caption: "galigo ephemeral caption", ShowCaptionAboveMedia: true}, r)
		}},
		{"editEphemeralMessageMedia", func(t *testing.T, req any) {
			r, ok := req.(sender.EditEphemeralMessageMediaRequest)
			require.True(t, ok, "%T", req)
			target(t, r.ChatID, r.ReceiverUserID, r.EphemeralMessageID)
			assert.Nil(t, r.ReplyMarkup)
			b65bPhoto(t, r.Media)
		}},
	}
	for _, r := range rows {
		t.Run(r.method, func(t *testing.T) {
			f := newB65aFake()
			rt := b65bRuntime(f)
			before := ephTargets(rt)
			step := &EditEphemeralStep{Method: r.method}
			assert.Equal(t, r.method+" (10.3)", step.Name())
			res, err := step.Execute(context.Background(), rt)
			require.NoError(t, err)
			require.Len(t, f.edits, 1)
			r.check(t, f.edits[0])
			assert.Empty(t, f.sent, "no send")
			assert.Empty(t, f.log, "no delete")
			require.NotNil(t, res)
			assert.Equal(t, r.method, res.Method)
			assert.Equal(t, r.method+" (10.3)", res.StepName, "the report's step_name (A-P3-N1)")
			assert.Empty(t, res.MessageIDs)
			assert.Empty(t, res.EphemeralMessageIDs, "an edit is not a new message")
			assert.Equal(t, map[string]any{"ephemeral_message_id": 9}, res.Evidence)
			assert.Equal(t, before, ephTargets(rt), "the records stay for the cleanup")
		})
	}
}

// T-step-none, T-step-unknown: no record, or an unknown method: an error and no call.
func TestB65B_EditEphemeralStepRefuses(t *testing.T) {
	t.Run("no record", func(t *testing.T) {
		f := newB65aFake()
		_, err := (&EditEphemeralStep{Method: "editEphemeralMessageCaption"}).Execute(context.Background(), b65aRuntime(f, b65aAdmin))
		require.EqualError(t, err, "editEphemeralMessageCaption: no ephemeral message to edit")
		assert.Empty(t, f.edits)
	})
	t.Run("unknown method", func(t *testing.T) {
		f := newB65aFake()
		_, err := (&EditEphemeralStep{Method: "x"}).Execute(context.Background(), b65bRuntime(f))
		require.EqualError(t, err, `unknown method "x"`)
		assert.Empty(t, f.edits)
	})
}

// T-step-err: a server error is the step's error (wrapped), and the records stay.
func TestB65B_EditEphemeralStepError(t *testing.T) {
	for _, m := range []string{"editEphemeralMessageText", "editEphemeralMessageReplyMarkup", "editEphemeralMessageCaption",
		"editEphemeralMessageMedia"} {
		t.Run(m, func(t *testing.T) {
			f := newB65aFake()
			f.editErr = map[string]error{m: tg.NewAPIError(m, 400, "Bad Request: message to edit not found")}
			rt := b65bRuntime(f)
			before := ephTargets(rt)
			res, err := (&EditEphemeralStep{Method: m}).Execute(context.Background(), rt)
			var apiErr *tg.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, 400, apiErr.Code)
			assert.Contains(t, err.Error(), m+": ")
			require.NotNil(t, res)
			assert.Equal(t, m+" (10.3)", res.StepName, "the failed step keeps its name in the report")
			assert.Equal(t, m, res.Method)
			assert.Nil(t, res.Evidence)
			assert.Len(t, f.edits, 1)
			assert.Equal(t, before, ephTargets(rt))
		})
	}
}

// T-budget, T-run (edits): S73 through the real runner; the edits are not counted; an edit failure stops the
// scenario and the stop cleanup deletes both messages.
func TestB65B_RunnerS73Edits(t *testing.T) {
	newFake := func() *b65aFake {
		f := newB65aFake()
		f.reply = b65aResp{err: b65aCallbackRefusal()}
		return f
	}
	t.Run("pass, budget 3", func(t *testing.T) {
		f := newFake()
		r, rt := b65aRunner(f, RunnerConfig{MaxMessages: 3})
		res := r.Run(context.Background(), b65aS73())
		require.True(t, res.Success, res.Error)
		assert.Equal(t, 2, r.messageCount)
		require.Len(t, f.edits, 4)
		ids := func(chat tg.ChatID, recv int64, id int) [3]any { return [3]any{chat, recv, id} }
		var got [][3]any
		for _, e := range f.edits {
			switch r := e.(type) {
			case sender.EditEphemeralMessageTextRequest:
				got = append(got, ids(r.ChatID, r.ReceiverUserID, r.EphemeralMessageID))
			case sender.EditEphemeralMessageReplyMarkupRequest:
				got = append(got, ids(r.ChatID, r.ReceiverUserID, r.EphemeralMessageID))
			case sender.EditEphemeralMessageCaptionRequest:
				got = append(got, ids(r.ChatID, r.ReceiverUserID, r.EphemeralMessageID))
			case sender.EditEphemeralMessageMediaRequest:
				got = append(got, ids(r.ChatID, r.ReceiverUserID, r.EphemeralMessageID))
			}
		}
		// the text message (9) is edited twice, then the photo (10) twice
		assert.Equal(t, [][3]any{ids(b30Chat, b65aAdmin, 9), ids(b30Chat, b65aAdmin, 9), ids(b30Chat, b65aAdmin, 10),
			ids(b30Chat, b65aAdmin, 10)}, got)
		assert.IsType(t, sender.EditEphemeralMessageTextRequest{}, f.edits[0])
		assert.IsType(t, sender.EditEphemeralMessageReplyMarkupRequest{}, f.edits[1])
		assert.IsType(t, sender.EditEphemeralMessageCaptionRequest{}, f.edits[2])
		assert.IsType(t, sender.EditEphemeralMessageMediaRequest{}, f.edits[3])
		assert.Equal(t, []string{ephKey(b30Chat, b65aAdmin, 9), ephKey(b30Chat, b65aAdmin, 10)}, f.keys())
		assert.Empty(t, rt.ephemeralMessages)
	})
	t.Run("caption edit fails", func(t *testing.T) {
		f := newFake()
		f.editErr = map[string]error{"editEphemeralMessageCaption": tg.NewAPIError("editEphemeralMessageCaption", 400, "Bad Request: x")}
		r, rt := b65aRunner(f, RunnerConfig{})
		res := r.Run(context.Background(), b65aS73())
		assert.False(t, res.Success)
		assert.Len(t, f.edits, 3, "the media edit does not run")
		assert.True(t, hasCleanupAfterStop(res))
		assert.Equal(t, []string{ephKey(b30Chat, b65aAdmin, 9), ephKey(b30Chat, b65aAdmin, 10)}, f.keys())
		assert.Equal(t, map[string]any{"deleted_count": 0, "ephemeral_deleted": 2}, evMap(t, lastStep(res)))
		assert.Empty(t, rt.ephemeralMessages)
	})
}

// T-adapter: each adapter method sends exactly the request a direct sender.Client call sends (every field set, the
// text request on the multipart path; file parts compared with their bytes), and returns the server's error.
func TestB65B_AdapterEdits(t *testing.T) {
	photo := sender.NewInputMediaPhoto(sender.FromBytes([]byte("img"), "p.jpg"))
	kb := &tg.InlineKeyboardMarkup{InlineKeyboard: [][]tg.InlineKeyboardButton{{{Text: "B", CallbackData: "d"}}}}
	ents := []tg.MessageEntity{{Type: "bold", Offset: 0, Length: 1}}
	text := sender.EditEphemeralMessageTextRequest{ChatID: b30Chat, ReceiverUserID: b65bRecvNew, EphemeralMessageID: 9,
		Text: "t", ParseMode: tg.ParseModeHTML, Entities: ents, RichMessage: &tg.InputRichMessage{Markdown: "![](tg://photo?id=e)",
			Media: []tg.InputRichMessageMedia{{ID: "e", Media: photo}}},
		LinkPreviewOptions: &tg.LinkPreviewOptions{IsDisabled: true}, ReplyMarkup: kb}
	caption := sender.EditEphemeralMessageCaptionRequest{ChatID: b30Chat, ReceiverUserID: b65bRecvNew, EphemeralMessageID: 9,
		Caption: "c", ParseMode: tg.ParseModeHTML, CaptionEntities: ents, ShowCaptionAboveMedia: true, ReplyMarkup: kb}
	media := sender.EditEphemeralMessageMediaRequest{ChatID: b30Chat, ReceiverUserID: b65bRecvNew, EphemeralMessageID: 9, Media: photo,
		ReplyMarkup: kb}
	markup := sender.EditEphemeralMessageReplyMarkupRequest{ChatID: b30Chat, ReceiverUserID: b65bRecvNew, EphemeralMessageID: 9,
		ReplyMarkup: kb}
	for name, req := range map[string]any{"text": text, "caption": caption, "media": media, "markup": markup} {
		assert.Empty(t, zeroFields(req), "%s: every field set", name)
	}
	textJSON := text // the JSON path: a file_id item, no upload (A-T2-04)
	textJSON.RichMessage = &tg.InputRichMessage{Markdown: "![](tg://photo?id=e)",
		Media: []tg.InputRichMessageMedia{{ID: "e", Media: sender.NewInputMediaPhoto(sender.FromFileID("AgAD-fake"))}}}
	assert.Empty(t, zeroFields(textJSON), "text-json: every field set")
	rows := []struct {
		method string
		viaA   func(context.Context, *SenderAdapter) error
		direct func(context.Context, *sender.Client) error
	}{
		{"editEphemeralMessageText", func(ctx context.Context, a *SenderAdapter) error { return a.EditEphemeralMessageText(ctx, text) },
			func(ctx context.Context, c *sender.Client) error { return c.EditEphemeralMessageText(ctx, text) }},
		{"editEphemeralMessageText", func(ctx context.Context, a *SenderAdapter) error { return a.EditEphemeralMessageText(ctx, textJSON) },
			func(ctx context.Context, c *sender.Client) error { return c.EditEphemeralMessageText(ctx, textJSON) }},
		{"editEphemeralMessageCaption", func(ctx context.Context, a *SenderAdapter) error { return a.EditEphemeralMessageCaption(ctx, caption) },
			func(ctx context.Context, c *sender.Client) error { return c.EditEphemeralMessageCaption(ctx, caption) }},
		{"editEphemeralMessageMedia", func(ctx context.Context, a *SenderAdapter) error { return a.EditEphemeralMessageMedia(ctx, media) },
			func(ctx context.Context, c *sender.Client) error { return c.EditEphemeralMessageMedia(ctx, media) }},
		{"editEphemeralMessageReplyMarkup", func(ctx context.Context, a *SenderAdapter) error {
			return a.EditEphemeralMessageReplyMarkup(ctx, markup)
		},
			func(ctx context.Context, c *sender.Client) error {
				return c.EditEphemeralMessageReplyMarkup(ctx, markup)
			}},
	}
	ok := func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyBool(w, true) }
	for _, r := range rows {
		t.Run(r.method, func(t *testing.T) {
			a, s := b65aAdapter(t, r.method, ok)
			require.NoError(t, r.viaA(context.Background(), a))
			require.Equal(t, 1, s.CaptureCount())
			got := s.LastCapture()

			ds := testutil.NewMockServer(t)
			ds.On("/bot"+testutil.TestToken+"/"+r.method, ok)
			require.NoError(t, r.direct(context.Background(), testutil.NewTestClient(t, ds.BaseURL())))
			require.Equal(t, 1, ds.CaptureCount())
			want := ds.LastCapture()

			assert.Equal(t, want.Path, got.Path)
			assert.Equal(t, b65aArgs(t, want), b65aArgs(t, got))
		})
		t.Run(r.method+"/429", func(t *testing.T) {
			a, s := b65aAdapter(t, r.method, func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyRateLimit(w, 1) })
			err := r.viaA(context.Background(), a)
			var apiErr *tg.APIError
			require.True(t, errors.As(err, &apiErr), "error %v", err)
			assert.Equal(t, 429, apiErr.Code)
			assert.Equal(t, 1, s.CaptureCount())
		})
	}
}

// zeroFields names the fields of the struct v that hold their zero value.
func zeroFields(v any) []string {
	rv := reflect.ValueOf(v)
	var out []string
	for i := range rv.NumField() {
		if rv.Field(i).IsZero() {
			out = append(out, rv.Type().Field(i).Name)
		}
	}
	return out
}
