package engine

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-48b2 TEST-PLAN §2.2, T-st-*: SendPaidMediaStep.

// fakePaid is a SenderClient for SendPaidMediaStep: it embeds the interface (any other method panics), records each
// SendPaidMedia request and returns the scripted reply.
type fakePaid struct {
	SenderClient
	reqs []sender.SendPaidMediaRequest
	msg  *tg.Message
	err  error
}

func (f *fakePaid) SendPaidMedia(_ context.Context, req sender.SendPaidMediaRequest) (*tg.Message, error) {
	f.reqs = append(f.reqs, req)
	return f.msg, f.err
}

func paidMsg(stars int, caption string, kinds ...string) *tg.Message {
	items := make([]tg.PaidMedia, 0, len(kinds))
	for _, k := range kinds {
		items = append(items, tg.PaidMedia{Type: k})
	}
	return &tg.Message{MessageID: 4242, Caption: caption, PaidMedia: &tg.PaidMediaInfo{StarCount: stars, PaidMedia: items}}
}

func uploadBytes(t *testing.T, f sender.InputFile) []byte {
	t.Helper()
	require.NotNil(t, f.Source, "an upload with a Source (FromBytes)")
	assert.Empty(t, f.FileID)
	assert.Empty(t, f.URL)
	b, err := io.ReadAll(f.Source())
	require.NoError(t, err)
	return b
}

func TestB48B2_Step(t *testing.T) {
	const caption = "galigo paid media"
	run := func(t *testing.T, f *fakePaid) (*Runtime, *StepResult, error) {
		t.Helper()
		rt := NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
		res, err := (&SendPaidMediaStep{}).Execute(context.Background(), rt)
		require.Len(t, f.reqs, 1, "one call")
		return rt, res, err
	}
	tracked := []CreatedMessage{{ChatID: testutil.TestChatID, MessageID: 4242}}

	t.Run("ok", func(t *testing.T) {
		f := &fakePaid{msg: paidMsg(1, caption, "photo", "video", "live_photo")}
		rt, res, err := run(t, f)
		require.NoError(t, err)
		assert.Equal(t, "sendPaidMedia (7.6)", (&SendPaidMediaStep{}).Name())
		assert.Equal(t, "sendPaidMedia", res.Method)

		req := f.reqs[0]
		assert.Equal(t, any(testutil.TestChatID), req.ChatID)
		assert.Equal(t, 1, req.StarCount)
		assert.Equal(t, "galigo-paid-media", req.Payload)
		assert.Equal(t, caption, req.Caption)
		require.Len(t, req.Media, 3)
		assert.Equal(t, []string{"photo", "video", "live_photo"},
			[]string{req.Media[0].Type, req.Media[1].Type, req.Media[2].Type})
		names := []string{"galigo-paid.jpg", "galigo-paid.mp4", "galigo-paid-live.mp4"}
		data := [][]byte{fixtures.PhotoBytes(), fixtures.VideoBytes(), fixtures.VideoBytes()}
		for i, m := range req.Media {
			assert.Equal(t, names[i], m.Media.FileName, "item %d", i)
			assert.Equal(t, data[i], uploadBytes(t, m.Media), "item %d", i)
			assert.Nil(t, m.Thumbnail, "item %d", i)
			assert.Nil(t, m.Cover, "item %d", i)
		}
		assert.Nil(t, req.Media[0].Media.Photo)
		assert.Nil(t, req.Media[1].Media.Photo)
		require.NotNil(t, req.Media[2].Media.Photo)
		assert.Equal(t, "galigo-paid-live.jpg", req.Media[2].Media.Photo.FileName)
		assert.Equal(t, fixtures.PhotoBytes(), uploadBytes(t, *req.Media[2].Media.Photo))

		assert.Equal(t, tracked, rt.CreatedMessages)
		assert.Equal(t, f.msg, rt.LastMessage)
		assert.Equal(t, []int{4242}, res.MessageIDs)
		assert.Equal(t, map[string]any{"message_id": 4242, "star_count": 1,
			"kinds": []string{"photo", "video", "live_photo"}}, res.Evidence)
	})
	t.Run("send_error", func(t *testing.T) {
		boom := errors.New("boom")
		rt, _, err := run(t, &fakePaid{err: boom})
		assert.ErrorIs(t, err, boom)
		assert.Empty(t, rt.CreatedMessages)
	})
	t.Run("no_message", func(t *testing.T) {
		rt, _, err := run(t, &fakePaid{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no message")
		assert.Empty(t, rt.CreatedMessages)
	})
	all := []string{"photo", "video", "live_photo"}
	ev := func(stars int, kinds ...string) map[string]any {
		return map[string]any{"message_id": 4242, "star_count": stars, "kinds": kinds}
	}
	bad := []struct {
		name string
		msg  *tg.Message
		text string
		ev   map[string]any // nil: the step fails before it writes evidence
	}{
		{"no_paid_media", &tg.Message{MessageID: 4242, Caption: caption}, "no paid_media", nil},
		{"star_count_0", paidMsg(0, caption, all...), "star_count 0, want 1", ev(0, all...)},
		{"star_count_2", paidMsg(2, caption, all...), "star_count 2, want 1", ev(2, all...)},
		{"kinds_order", paidMsg(1, caption, "video", "photo", "live_photo"), "kinds [video photo live_photo]", ev(1, "video", "photo", "live_photo")},
		{"kinds_missing", paidMsg(1, caption, "photo", "video"), "kinds [photo video]", ev(1, "photo", "video")},
		{"caption", paidMsg(1, "other", all...), `caption "other"`, ev(1, all...)},
	}
	for _, b := range bad {
		t.Run(b.name, func(t *testing.T) {
			rt, res, err := run(t, &fakePaid{msg: b.msg})
			require.Error(t, err)
			assert.Contains(t, err.Error(), b.text)
			assert.Equal(t, tracked, rt.CreatedMessages, "tracked, so the cleanup deletes it")
			// the evidence records what the reply carried, so a failed run shows it (A-T1-07)
			if b.ev == nil {
				assert.Nil(t, res.Evidence)
			} else {
				assert.Equal(t, b.ev, res.Evidence)
			}
		})
	}
}

// T-ad (E-0123): the adapter is a pass-through: for a request that sets every field (16 form keys) and a video
// item with every option, the form it sends equals the one sender.Client sends, it returns the same message, and it
// returns the client's error (O-T1-04).
func TestSenderAdapterB48B2(t *testing.T) {
	still := sender.FromBytes([]byte("still"), "s.jpg")
	thumb := sender.FromBytes([]byte("thumb"), "t.jpg")
	cover := sender.FromFileID("CV")
	video := sender.NewInputPaidMediaVideo(sender.FromBytes([]byte("video"), "v.mp4"))
	video.Thumbnail, video.Cover, video.StartTimestamp, video.Width, video.Height, video.Duration, video.SupportsStreaming =
		&thumb, &cover, 1, 320, 240, 5, true
	req := sender.SendPaidMediaRequest{
		BusinessConnectionID: "bc", ChatID: int64(-1001234567890), MessageThreadID: 3, DirectMessagesTopicID: 4,
		StarCount: 5, Payload: "pl", Caption: "cap", ParseMode: tg.ParseModeHTML,
		CaptionEntities:       []tg.MessageEntity{{Type: "bold", Offset: 0, Length: 1}},
		ShowCaptionAboveMedia: true, DisableNotification: true, ProtectContent: true, AllowPaidBroadcast: true,
		ReplyParameters: &tg.ReplyParameters{MessageID: 9},
		ReplyMarkup:     tg.InlineKeyboardMarkup{InlineKeyboard: [][]tg.InlineKeyboardButton{{{Text: "b", CallbackData: "d"}}}},
		Media: []sender.InputPaidMedia{
			sender.NewInputPaidMediaPhoto(sender.FromBytes([]byte("photo"), "p.jpg")),
			sender.NewInputPaidMediaLivePhoto(sender.FromFileID("LV"), still),
			video,
		},
	}
	type sent struct {
		msg   *tg.Message
		err   error
		n     int
		args  map[string]string
		files map[string][2]string
	}
	// paidReply is a sendPaidMedia reply with a caption and paid media (fake ids), so a dropped field shows (O-T2-01).
	paidReply := func(w http.ResponseWriter) {
		testutil.ReplyOK(w, map[string]any{
			"message_id": 5, "date": 1234567890, "chat": map[string]any{"id": -1001234567890, "type": "supergroup"},
			"caption": "cap",
			"paid_media": map[string]any{"star_count": 5, "paid_media": []any{
				map[string]any{"type": "photo", "photo": []any{map[string]any{"file_id": "P1", "file_unique_id": "P1-u", "width": 90, "height": 90}}},
				map[string]any{"type": "video", "video": map[string]any{"file_id": "V1", "file_unique_id": "V1-u", "width": 320, "height": 240, "duration": 5}},
			}},
		})
	}
	send := func(t *testing.T, ctx context.Context, reply func(w http.ResponseWriter),
		call func(ctx context.Context, c *sender.Client) (*tg.Message, error)) sent {
		server := testutil.NewMockServer(t)
		server.On("/bot"+testutil.TestToken+"/sendPaidMedia", func(w http.ResponseWriter, _ *http.Request) { reply(w) })
		msg, err := call(ctx, testutil.NewTestClient(t, server.BaseURL()))
		out := sent{msg: msg, err: err, n: server.CaptureCount()}
		if out.n > 0 {
			server.LastCapture().AssertPath(t, "/bot"+testutil.TestToken+"/sendPaidMedia")
			out.args, out.files = adapterForm(t, server.LastCapture())
		}
		return out
	}
	viaClient := func(ctx context.Context, c *sender.Client) (*tg.Message, error) { return c.SendPaidMedia(ctx, req) }
	viaAdapter := func(ctx context.Context, c *sender.Client) (*tg.Message, error) {
		return NewSenderAdapter(c).SendPaidMedia(ctx, req)
	}
	bg := context.Background()

	t.Run("ok", func(t *testing.T) {
		want, got := send(t, bg, paidReply, viaClient), send(t, bg, paidReply, viaAdapter)
		require.NoError(t, want.err)
		require.NoError(t, got.err)
		require.Equal(t, 1, got.n)
		require.NotNil(t, got.msg)
		assert.Equal(t, 5, got.msg.MessageID)
		assert.Equal(t, "cap", got.msg.Caption, "the whole reply, not only its id")
		require.NotNil(t, got.msg.PaidMedia)
		assert.Equal(t, 5, got.msg.PaidMedia.StarCount)
		assert.Len(t, got.msg.PaidMedia.PaidMedia, 2)
		assert.Equal(t, want.msg, got.msg)
		assert.Equal(t, want.args, got.args)
		assert.Equal(t, want.files, got.files)
		assert.Len(t, got.args, 16, "every request field reaches the form")
		assert.JSONEq(t, `[{"type":"photo","media":"attach://file0"},{"type":"live_photo","media":"LV","photo":"attach://file1"},`+
			`{"type":"video","media":"attach://file2","thumbnail":"attach://file3","cover":"CV","start_timestamp":1,`+
			`"width":320,"height":240,"duration":5,"supports_streaming":true}]`, got.args["media"])
		assert.Equal(t, map[string][2]string{"file0": {"p.jpg", "photo"}, "file1": {"s.jpg", "still"},
			"file2": {"v.mp4", "video"}, "file3": {"t.jpg", "thumb"}}, got.files)
	})
	t.Run("rate_limited", func(t *testing.T) {
		// the client's *tg.APIError reaches the caller as it is: RetryAfter and the sentinel kept (O-T2-01)
		limited := func(w http.ResponseWriter) { testutil.ReplyRateLimit(w, 7) }
		want, got := send(t, bg, limited, viaClient), send(t, bg, limited, viaAdapter)
		require.Error(t, want.err)
		require.Error(t, got.err)
		require.Equal(t, 1, got.n, "one request: no resend after an error (O-T3-01)")
		assert.Nil(t, got.msg)
		assert.Equal(t, want.err, got.err, "the error as it is, not wrapped (A-T3-03)")
		assert.Equal(t, want.err.Error(), got.err.Error())
		var apiErr *tg.APIError
		require.True(t, errors.As(got.err, &apiErr), "got %T", got.err)
		assert.Equal(t, http.StatusTooManyRequests, apiErr.Code)
		assert.Equal(t, 7*time.Second, apiErr.RetryAfter)
		assert.ErrorIs(t, got.err, tg.ErrTooManyRequests)
	})
	t.Run("cancelled", func(t *testing.T) {
		// the adapter passes the caller's context on (O-T2-02)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		got := send(t, ctx, paidReply, viaAdapter)
		require.Error(t, got.err)
		assert.ErrorIs(t, got.err, context.Canceled)
		assert.Nil(t, got.msg)
		assert.Equal(t, 0, got.n, "no request after the cancel")
	})
	t.Run("cancelled_in_flight", func(t *testing.T) {
		// a cancel after the request has started still reaches the client (A-T3-02/O-T3-02)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		server := testutil.NewMockServer(t)
		server.On("/bot"+testutil.TestToken+"/sendPaidMedia", func(w http.ResponseWriter, r *http.Request) {
			cancel()
			select {
			case <-r.Context().Done():
			case <-time.After(3 * time.Second): // the caller's cancel was lost: reply, so the row fails
				paidReply(w)
			}
		})
		msg, err := NewSenderAdapter(testutil.NewTestClient(t, server.BaseURL())).SendPaidMedia(ctx, req)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, msg)
		assert.Equal(t, 1, server.CaptureCount())
	})
	t.Run("error", func(t *testing.T) {
		bad := func(w http.ResponseWriter) { testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway") }
		want, got := send(t, bg, bad, viaClient), send(t, bg, bad, viaAdapter)
		require.Error(t, want.err)
		require.Error(t, got.err)
		require.Equal(t, 1, got.n, "one request: no resend after an error (O-T3-01)")
		assert.Nil(t, got.msg)
		assert.Equal(t, want.err, got.err, "the error as it is, not wrapped (A-T3-03)")
		assert.Equal(t, want.err.Error(), got.err.Error())
		var apiErr *tg.APIError
		require.True(t, errors.As(got.err, &apiErr), "got %T", got.err)
		assert.Equal(t, http.StatusBadGateway, apiErr.Code)
		assert.Equal(t, want.args, got.args)
	})
}
