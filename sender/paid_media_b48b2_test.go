package sender_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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

// B-48b2: sendPaidMedia (Bot API 7.6; the live photo item since 10.0). TEST-PLAN §2.1.

// b48b2RequestKeys is the spec's sendPaidMedia key list in order, without suggested_post_parameters (B-48c).
var b48b2RequestKeys = []string{
	"business_connection_id", "chat_id", "message_thread_id", "direct_messages_topic_id", "star_count", "media",
	"payload", "caption", "parse_mode", "caption_entities", "show_caption_above_media", "disable_notification",
	"protect_content", "allow_paid_broadcast", "reply_parameters", "reply_markup",
}

func sendPaid(req sender.SendPaidMediaRequest) func(ctx context.Context, c *sender.Client) error {
	return func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendPaidMedia(ctx, req)
		return err
	}
}

func paidReq(items ...sender.InputPaidMedia) sender.SendPaidMediaRequest {
	return sender.SendPaidMediaRequest{ChatID: testutil.TestChatID, StarCount: 1, Media: items}
}

// T-shape-r, T-shape-i
func TestB48B2_Shapes(t *testing.T) {
	t.Run("request", func(t *testing.T) {
		rt := reflect.TypeFor[sender.SendPaidMediaRequest]()
		want := []struct{ name, typ, tag string }{
			{"BusinessConnectionID", "string", "business_connection_id,omitempty"},
			{"ChatID", "interface {}", "chat_id"},
			{"MessageThreadID", "int", "message_thread_id,omitempty"},
			{"DirectMessagesTopicID", "int", "direct_messages_topic_id,omitempty"},
			{"StarCount", "int", "star_count"},
			{"Media", "[]sender.InputPaidMedia", "media"},
			{"Payload", "string", "payload,omitempty"},
			{"Caption", "string", "caption,omitempty"},
			{"ParseMode", "tg.ParseMode", "parse_mode,omitempty"},
			{"CaptionEntities", "[]tg.MessageEntity", "caption_entities,omitempty"},
			{"ShowCaptionAboveMedia", "bool", "show_caption_above_media,omitempty"},
			{"DisableNotification", "bool", "disable_notification,omitempty"},
			{"ProtectContent", "bool", "protect_content,omitempty"},
			{"AllowPaidBroadcast", "bool", "allow_paid_broadcast,omitempty"},
			{"ReplyParameters", "*tg.ReplyParameters", "reply_parameters,omitempty"},
			{"ReplyMarkup", "interface {}", "reply_markup,omitempty"},
		}
		require.Equal(t, len(want), rt.NumField())
		keys := make([]string, 0, rt.NumField())
		for i, w := range want {
			f := rt.Field(i)
			assert.Equal(t, w.name, f.Name, "field %d", i)
			assert.Equal(t, w.typ, f.Type.String(), "field %s", f.Name)
			assert.Equal(t, w.tag, f.Tag.Get("json"), "field %s", f.Name)
			assert.False(t, f.Anonymous, "field %s", f.Name)
			keys = append(keys, strings.TrimSuffix(w.tag, ",omitempty"))
		}
		assert.Equal(t, b48b2RequestKeys, keys)
	})
	t.Run("item", func(t *testing.T) {
		rt := reflect.TypeFor[sender.InputPaidMedia]()
		want := []struct{ name, typ string }{
			{"Type", "string"}, {"Media", "sender.InputFile"}, {"Thumbnail", "*sender.InputFile"},
			{"Cover", "*sender.InputFile"}, {"StartTimestamp", "int"}, {"Width", "int"}, {"Height", "int"},
			{"Duration", "int"}, {"SupportsStreaming", "bool"},
		}
		require.Equal(t, len(want), rt.NumField())
		for i, w := range want {
			f := rt.Field(i)
			assert.Equal(t, w.name, f.Name, "field %d", i)
			assert.Equal(t, w.typ, f.Type.String(), "field %s", f.Name)
			assert.Empty(t, string(f.Tag), "field %s has no tag", f.Name)
			assert.False(t, f.Anonymous, "field %s", f.Name)
		}
		assert.Equal(t, 0, rt.NumMethod(), "no methods (no MarshalJSON, PLAN §4)")
		assert.Equal(t, 0, reflect.PointerTo(rt).NumMethod())
	})
}

// T-ctor
func TestB48B2_Constructors(t *testing.T) {
	photo := sender.FromFileID("P").WithCaption("not used")
	video := sender.FromURL("https://x/v.mp4")
	still := sender.FromFileID("S")

	p := sender.NewInputPaidMediaPhoto(photo)
	assert.Equal(t, sender.InputPaidMedia{Type: "photo", Media: photo}, p)

	v := sender.NewInputPaidMediaVideo(video)
	assert.Equal(t, sender.InputPaidMedia{Type: "video", Media: video}, v)

	l := sender.NewInputPaidMediaLivePhoto(video, still)
	assert.Equal(t, "live_photo", l.Type)
	require.NotNil(t, l.Media.Photo)
	assert.Equal(t, still, *l.Media.Photo)
	wantMedia := video
	wantMedia.Photo = l.Media.Photo
	assert.Equal(t, wantMedia, l.Media, "the video's own fields are kept")
	assert.Nil(t, video.Photo, "the input video is not changed")
	assert.Nil(t, still.Photo)
}

var (
	b48b2PhotoBytes = []byte("paid-photo-bytes")
	b48b2VideoBytes = []byte("paid-video-bytes")
	b48b2ThumbBytes = []byte("paid-thumb-bytes")
	b48b2CoverBytes = []byte("paid-cover-bytes")
	b48b2StillBytes = []byte("paid-still-bytes")
)

// T-w-*
func TestB48B2_Wire(t *testing.T) {
	t.Run("file_ids", func(t *testing.T) {
		mediaType, args, files, err := wireCall(t, "sendPaidMedia", replyMessage, sendPaid(paidReq(
			sender.NewInputPaidMediaPhoto(sender.FromFileID("P1")),
			sender.NewInputPaidMediaVideo(sender.FromFileID("V1")),
			sender.NewInputPaidMediaLivePhoto(sender.FromFileID("LV"), sender.FromFileID("LP")),
		)))
		require.NoError(t, err)
		assert.Equal(t, "multipart/form-data", mediaType)
		require.Contains(t, args, "media")
		assert.JSONEq(t, `[{"type":"photo","media":"P1"},{"type":"video","media":"V1"},`+
			`{"type":"live_photo","media":"LV","photo":"LP"}]`, args["media"])
		delete(args, "media")
		assertArgs(t, map[string]string{"chat_id": wireChatID, "star_count": "1"}, args)
		assertFiles(t, map[string]wireFile{}, files)
	})
	t.Run("urls", func(t *testing.T) {
		mediaType, args, files, err := wireCall(t, "sendPaidMedia", replyMessage, sendPaid(paidReq(
			sender.NewInputPaidMediaPhoto(sender.FromURL("https://x/p.jpg")),
			sender.NewInputPaidMediaVideo(sender.FromURL("https://x/v.mp4")),
		)))
		require.NoError(t, err)
		assert.Equal(t, "multipart/form-data", mediaType)
		assert.JSONEq(t, `[{"type":"photo","media":"https://x/p.jpg"},{"type":"video","media":"https://x/v.mp4"}]`, args["media"])
		assertFiles(t, map[string]wireFile{}, files)
	})
	t.Run("mixed_uploads", func(t *testing.T) {
		// Item 1 uploads all four files, so the attach order media, photo, thumbnail, cover is pinned (U-T1-00).
		coverURL := sender.FromURL("https://x/c.jpg")
		thumb := sender.FromBytes(b48b2ThumbBytes, "t.jpg")
		cover := sender.FromBytes(b48b2CoverBytes, "c.jpg")
		photoByID := sender.NewInputPaidMediaPhoto(sender.FromFileID("PH"))
		photoByID.Cover = &coverURL
		all := sender.NewInputPaidMediaLivePhoto(sender.FromBytes(b48b2VideoBytes, "lv1.mp4"), sender.FromBytes(b48b2StillBytes, "s1.jpg"))
		all.Thumbnail, all.Cover = &thumb, &cover
		video := sender.NewInputPaidMediaVideo(sender.FromBytes(b48b2VideoBytes, "v.mp4"))
		video.Width, video.SupportsStreaming = 3, true
		mediaType, args, files, err := wireCall(t, "sendPaidMedia", replyMessage, sendPaid(paidReq(
			photoByID,
			all,
			video,
			sender.NewInputPaidMediaLivePhoto(sender.FromFileID("LV"), sender.FromBytes(b48b2StillBytes, "s2.jpg")),
			sender.NewInputPaidMediaPhoto(sender.FromURL("https://x/p.jpg")),
		)))
		require.NoError(t, err)
		assert.Equal(t, "multipart/form-data", mediaType)
		assert.JSONEq(t, `[`+
			`{"type":"photo","media":"PH","cover":"https://x/c.jpg"},`+
			`{"type":"live_photo","media":"attach://file0","photo":"attach://file1","thumbnail":"attach://file2","cover":"attach://file3"},`+
			`{"type":"video","media":"attach://file4","width":3,"supports_streaming":true},`+
			`{"type":"live_photo","media":"LV","photo":"attach://file5"},`+
			`{"type":"photo","media":"https://x/p.jpg"}]`, args["media"])
		assertFiles(t, map[string]wireFile{
			"file0": {FileName: "lv1.mp4", Data: b48b2VideoBytes},
			"file1": {FileName: "s1.jpg", Data: b48b2StillBytes},
			"file2": {FileName: "t.jpg", Data: b48b2ThumbBytes},
			"file3": {FileName: "c.jpg", Data: b48b2CoverBytes},
			"file4": {FileName: "v.mp4", Data: b48b2VideoBytes},
			"file5": {FileName: "s2.jpg", Data: b48b2StillBytes},
		}, files)
	})
	t.Run("video_option", func(t *testing.T) {
		th, cv := sender.FromFileID("TH"), sender.FromFileID("CV")
		rows := []struct {
			name string
			set  func(m *sender.InputPaidMedia)
			want string
		}{
			{"none", func(*sender.InputPaidMedia) {}, `{"type":"video","media":"V"}`},
			{"thumbnail", func(m *sender.InputPaidMedia) { m.Thumbnail = &th }, `{"type":"video","media":"V","thumbnail":"TH"}`},
			{"cover", func(m *sender.InputPaidMedia) { m.Cover = &cv }, `{"type":"video","media":"V","cover":"CV"}`},
			{"start_timestamp", func(m *sender.InputPaidMedia) { m.StartTimestamp = 7 }, `{"type":"video","media":"V","start_timestamp":7}`},
			{"width", func(m *sender.InputPaidMedia) { m.Width = 320 }, `{"type":"video","media":"V","width":320}`},
			{"height", func(m *sender.InputPaidMedia) { m.Height = 240 }, `{"type":"video","media":"V","height":240}`},
			{"duration", func(m *sender.InputPaidMedia) { m.Duration = 5 }, `{"type":"video","media":"V","duration":5}`},
			{"supports_streaming", func(m *sender.InputPaidMedia) { m.SupportsStreaming = true }, `{"type":"video","media":"V","supports_streaming":true}`},
			{"all", func(m *sender.InputPaidMedia) {
				m.Thumbnail, m.Cover, m.StartTimestamp, m.Width, m.Height, m.Duration, m.SupportsStreaming = &th, &cv, 7, 320, 240, 5, true
			}, `{"type":"video","media":"V","thumbnail":"TH","cover":"CV","start_timestamp":7,"width":320,"height":240,"duration":5,"supports_streaming":true}`},
			// a set value is sent as it is, also a negative one (the server checks it; O-T1-03)
			{"negative", func(m *sender.InputPaidMedia) {
				m.StartTimestamp, m.Width, m.Height, m.Duration = -1, -2, -3, -4
			}, `{"type":"video","media":"V","start_timestamp":-1,"width":-2,"height":-3,"duration":-4}`},
		}
		for _, r := range rows {
			t.Run(r.name, func(t *testing.T) {
				m := sender.NewInputPaidMediaVideo(sender.FromFileID("V"))
				r.set(&m)
				_, args, _, err := wireCall(t, "sendPaidMedia", replyMessage, sendPaid(paidReq(m)))
				require.NoError(t, err)
				assert.JSONEq(t, `[`+r.want+`]`, args["media"])
			})
		}
	})
	t.Run("options_on_other_kinds", func(t *testing.T) {
		// the video fields are sent for any kind; the client does not check the kind (O-T1-03)
		th, cv := sender.FromFileID("TH"), sender.FromFileID("CV")
		set := func(m sender.InputPaidMedia) sender.InputPaidMedia {
			m.Thumbnail, m.Cover, m.StartTimestamp, m.Width, m.Height, m.Duration, m.SupportsStreaming = &th, &cv, 7, 320, 240, 5, true
			return m
		}
		const opts = `"thumbnail":"TH","cover":"CV","start_timestamp":7,"width":320,"height":240,"duration":5,"supports_streaming":true`
		_, args, _, err := wireCall(t, "sendPaidMedia", replyMessage, sendPaid(paidReq(
			set(sender.NewInputPaidMediaPhoto(sender.FromFileID("P"))),
			set(sender.NewInputPaidMediaLivePhoto(sender.FromFileID("LV"), sender.FromFileID("LP"))),
		)))
		require.NoError(t, err)
		assert.JSONEq(t, `[{"type":"photo","media":"P",`+opts+`},{"type":"live_photo","media":"LV","photo":"LP",`+opts+`}]`, args["media"])
	})
	t.Run("photo_on_photo_item", func(t *testing.T) {
		m := sender.NewInputPaidMediaPhoto(sender.FromFileID("P").WithPhoto(sender.FromFileID("X")))
		_, args, _, err := wireCall(t, "sendPaidMedia", replyMessage, sendPaid(paidReq(m)))
		require.NoError(t, err)
		assert.JSONEq(t, `[{"type":"photo","media":"P","photo":"X"}]`, args["media"])
	})
	t.Run("request_fields", func(t *testing.T) {
		markup := tg.InlineKeyboardMarkup{InlineKeyboard: [][]tg.InlineKeyboardButton{{{Text: "b", CallbackData: "d"}}}}
		markupJSON, err := json.Marshal(markup)
		require.NoError(t, err)
		req := sender.SendPaidMediaRequest{
			BusinessConnectionID: "bc", ChatID: "@chan", MessageThreadID: 7, DirectMessagesTopicID: 8, StarCount: 25,
			Media:   []sender.InputPaidMedia{sender.NewInputPaidMediaPhoto(sender.FromFileID("P"))},
			Payload: "pl", Caption: "cap", ParseMode: tg.ParseModeHTML,
			CaptionEntities:       []tg.MessageEntity{{Type: "bold", Offset: 0, Length: 1}},
			ShowCaptionAboveMedia: true, DisableNotification: true, ProtectContent: true, AllowPaidBroadcast: true,
			ReplyParameters: &tg.ReplyParameters{MessageID: 42},
			ReplyMarkup:     markup,
		}
		_, args, files, err := wireCall(t, "sendPaidMedia", replyMessage, sendPaid(req))
		require.NoError(t, err)
		assertArgs(t, map[string]string{
			"business_connection_id": "bc", "chat_id": "@chan", "message_thread_id": "7", "direct_messages_topic_id": "8",
			"star_count": "25", "media": `[{"media":"P","type":"photo"}]`, "payload": "pl", "caption": "cap",
			"parse_mode": "HTML", "caption_entities": `[{"type":"bold","offset":0,"length":1}]`,
			"show_caption_above_media": "true", "disable_notification": "true", "protect_content": "true",
			"allow_paid_broadcast": "true", "reply_parameters": `{"message_id":42}`, "reply_markup": string(markupJSON),
		}, args)
		assert.Len(t, args, len(b48b2RequestKeys))
		assertFiles(t, map[string]wireFile{}, files)
	})
	t.Run("minimal", func(t *testing.T) {
		_, args, _, err := wireCall(t, "sendPaidMedia", replyMessage, sendPaid(paidReq(sender.NewInputPaidMediaPhoto(sender.FromFileID("P")))))
		require.NoError(t, err)
		assertArgs(t, map[string]string{"chat_id": wireChatID, "star_count": "1", "media": `[{"media":"P","type":"photo"}]`}, args)
	})
}

// b48b2BadItems are the item errors (AC3), each with its message: every file
// check on every Type (photo, video, live_photo), since the checks apply
// whatever the Type (A-T3-01, O-C1-01), and the live photo without its photo.
func b48b2BadItems() []struct {
	name string
	item sender.InputPaidMedia
	text string
} {
	empty := sender.InputFile{}
	v := sender.FromFileID("V")
	lv := sender.FromFileID("LV").WithPhoto(sender.FromFileID("LP")) // a valid live photo's files
	return []struct {
		name string
		item sender.InputPaidMedia
		text string
	}{
		{"empty media", sender.InputPaidMedia{Type: "photo"}, "media is required"},
		{"live photo without photo", sender.InputPaidMedia{Type: "live_photo", Media: v}, "live_photo requires Media.Photo"},
		{"empty photo", sender.InputPaidMedia{Type: "photo", Media: v.WithPhoto(empty)}, "photo is empty"},
		{"empty thumbnail", sender.InputPaidMedia{Type: "video", Media: v, Thumbnail: &empty}, "thumbnail is empty"},
		{"empty cover", sender.InputPaidMedia{Type: "video", Media: v, Cover: &empty}, "cover is empty"},
		{"empty cover on photo", sender.InputPaidMedia{Type: "photo", Media: sender.FromFileID("P"), Cover: &empty}, "cover is empty"},
		{"empty thumbnail on photo", sender.InputPaidMedia{Type: "photo", Media: sender.FromFileID("P"), Thumbnail: &empty}, "thumbnail is empty"},
		{"empty photo on live photo", sender.InputPaidMedia{Type: "live_photo", Media: v.WithPhoto(empty)}, "photo is empty"},
		{"empty media on video", sender.InputPaidMedia{Type: "video"}, "media is required"},
		{"empty media on live photo", sender.InputPaidMedia{Type: "live_photo", Media: empty.WithPhoto(sender.FromFileID("LP"))}, "media is required"},
		{"empty photo on video", sender.InputPaidMedia{Type: "video", Media: v.WithPhoto(empty)}, "photo is empty"},
		{"empty thumbnail on live photo", sender.InputPaidMedia{Type: "live_photo", Media: lv, Thumbnail: &empty}, "thumbnail is empty"},
		{"empty cover on live photo", sender.InputPaidMedia{Type: "live_photo", Media: lv, Cover: &empty}, "cover is empty"},
	}
}

// T-val
func TestB48B2_Validation(t *testing.T) {
	good := sender.NewInputPaidMediaPhoto(sender.FromFileID("P"))
	type row struct {
		name  string
		req   sender.SendPaidMediaRequest
		field string // the *tg.ValidationError field, or "" for a chat id error (a plain error)
		text  string
	}
	rows := []row{
		{"chat id nil", sender.SendPaidMediaRequest{StarCount: 1, Media: []sender.InputPaidMedia{good}}, "", "galigo: chat_id is required"},
		{"chat id empty", sender.SendPaidMediaRequest{ChatID: "", StarCount: 1, Media: []sender.InputPaidMedia{good}}, "", "galigo: chat_id cannot be empty string"},
		{"chat id zero", sender.SendPaidMediaRequest{ChatID: int64(0), StarCount: 1, Media: []sender.InputPaidMedia{good}}, "", "galigo: chat_id cannot be zero"},
		{"star_count 0", sender.SendPaidMediaRequest{ChatID: testutil.TestChatID, Media: []sender.InputPaidMedia{good}}, "star_count", "galigo: validation: star_count - must be positive"},
		{"star_count -1", sender.SendPaidMediaRequest{ChatID: testutil.TestChatID, StarCount: -1, Media: []sender.InputPaidMedia{good}}, "star_count", "galigo: validation: star_count - must be positive"},
		{"media nil", sender.SendPaidMediaRequest{ChatID: testutil.TestChatID, StarCount: 1}, "media", "galigo: validation: media - required"},
		{"media empty", sender.SendPaidMediaRequest{ChatID: testutil.TestChatID, StarCount: 1, Media: []sender.InputPaidMedia{}}, "media", "galigo: validation: media - required"},
		// request-level order (PLAN D4): chat id, then star_count, then the list and its items (LT3-02)
		{"order: chat id before star_count", sender.SendPaidMediaRequest{ChatID: int64(0), Media: []sender.InputPaidMedia{good}}, "", "galigo: chat_id cannot be zero"},
		{"order: star_count before media", sender.SendPaidMediaRequest{ChatID: testutil.TestChatID}, "star_count", "galigo: validation: star_count - must be positive"},
		{"order: star_count before items", sender.SendPaidMediaRequest{ChatID: testutil.TestChatID, Media: []sender.InputPaidMedia{{Type: "photo"}}}, "star_count", "galigo: validation: star_count - must be positive"},
		{"first check wins", paidReq(sender.InputPaidMedia{Type: "live_photo"}), "media", "galigo: validation: media - item 0: media is required"},
		// check order inside one item (PLAN D4): two errors, the earlier check wins (A-T1-03)
		{"order: media before photo", paidReq(sender.InputPaidMedia{Type: "photo", Media: sender.InputFile{}.WithPhoto(sender.InputFile{})}),
			"media", "galigo: validation: media - item 0: media is required"},
		{"order: photo before thumbnail", paidReq(sender.InputPaidMedia{Type: "video", Media: sender.FromFileID("V").WithPhoto(sender.InputFile{}), Thumbnail: &sender.InputFile{}}),
			"media", "galigo: validation: media - item 0: photo is empty"},
		// live_photo is checked before thumbnail and cover (O-T2-03/A-T2-02)
		{"order: live before thumbnail", paidReq(sender.InputPaidMedia{Type: "live_photo", Media: sender.FromFileID("V"), Thumbnail: &sender.InputFile{}}),
			"media", "galigo: validation: media - item 0: live_photo requires Media.Photo"},
		{"order: live before cover", paidReq(sender.InputPaidMedia{Type: "live_photo", Media: sender.FromFileID("V"), Cover: &sender.InputFile{}}),
			"media", "galigo: validation: media - item 0: live_photo requires Media.Photo"},
		{"order: thumbnail before cover", paidReq(sender.InputPaidMedia{Type: "video", Media: sender.FromFileID("V"), Thumbnail: &sender.InputFile{}, Cover: &sender.InputFile{}}),
			"media", "galigo: validation: media - item 0: thumbnail is empty"},
	}
	for _, b := range b48b2BadItems() {
		rows = append(rows,
			row{"item 0: " + b.name, paidReq(b.item), "media", "galigo: validation: media - item 0: " + b.text},
			row{"item 1: " + b.name, paidReq(good, b.item), "media", "galigo: validation: media - item 1: " + b.text},
			// a bad item before a good one: every item is checked, not only the last (A1-T1-04)
			row{"item 0 of 2: " + b.name, paidReq(b.item, good), "media", "galigo: validation: media - item 0: " + b.text})
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			client := testutil.NewTestClient(t, server.BaseURL())
			msg, err := client.SendPaidMedia(context.Background(), r.req)
			require.Error(t, err)
			assert.Nil(t, msg)
			assert.Equal(t, r.text, err.Error())
			var ve *tg.ValidationError
			if r.field == "" {
				assert.False(t, errors.As(err, &ve), "a chat id error is a plain error (AC3)")
			} else {
				require.True(t, errors.As(err, &ve), "want *tg.ValidationError, got %T", err)
				assert.Equal(t, r.field, ve.Field)
			}
			assert.Equal(t, 0, server.CaptureCount(), "no request sent")
		})
	}
}

// b48b2BadRequests are the 17 bad rows of AC3: 4 request rows (the chat id class has two)
// and the 13 item rows of b48b2BadItems, each with its exact error.
func b48b2BadRequests() []struct {
	req  sender.SendPaidMediaRequest
	text string
} {
	good := sender.NewInputPaidMediaPhoto(sender.FromFileID("P"))
	type row = struct {
		req  sender.SendPaidMediaRequest
		text string
	}
	rows := []row{
		{sender.SendPaidMediaRequest{ChatID: "", StarCount: 1, Media: []sender.InputPaidMedia{good}}, "galigo: chat_id cannot be empty string"},
		{sender.SendPaidMediaRequest{ChatID: int64(0), StarCount: 1, Media: []sender.InputPaidMedia{good}}, "galigo: chat_id cannot be zero"},
		{sender.SendPaidMediaRequest{ChatID: testutil.TestChatID, Media: []sender.InputPaidMedia{good}}, "galigo: validation: star_count - must be positive"},
		{sender.SendPaidMediaRequest{ChatID: testutil.TestChatID, StarCount: 1}, "galigo: validation: media - required"},
	}
	for _, b := range b48b2BadItems() {
		rows = append(rows, row{paidReq(b.item), "galigo: validation: media - item 0: " + b.text})
	}
	return rows
}

// T-guard-b, T-guard-l
func TestB48B2_BadCallsTouchNothing(t *testing.T) {
	good := paidReq(sender.NewInputPaidMediaPhoto(sender.FromFileID("P")))
	t.Run("breaker", func(t *testing.T) {
		server := testutil.NewMockServer(t)
		var fail atomic.Bool
		server.On("/bot"+testutil.TestToken+"/sendPaidMedia", func(w http.ResponseWriter, _ *http.Request) {
			if fail.Load() {
				testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway")
				return
			}
			testutil.ReplyMessage(w, 7)
		})
		client := testutil.NewBreakerTestClient(t, server.BaseURL(), sender.WithPerChatRateLimit(1000, 100)) // trips at 2 consecutive failures
		for round := range 3 {
			for i, r := range b48b2BadRequests() {
				_, err := client.SendPaidMedia(context.Background(), r.req)
				require.EqualError(t, err, r.text, "round %d row %d", round, i)
				assert.NotErrorIs(t, err, gobreaker.ErrOpenState, "round %d row %d", round, i)
			}
		}
		assert.Equal(t, 0, server.CaptureCount())
		msg, err := client.SendPaidMedia(context.Background(), good)
		require.NoError(t, err)
		require.NotNil(t, msg)
		assert.Equal(t, 7, msg.MessageID)
		assert.Equal(t, 1, server.CaptureCount())

		// control: the same client's breaker is live
		fail.Store(true)
		for i := range 2 {
			_, err = client.SendPaidMedia(context.Background(), good)
			var apiErr *tg.APIError
			require.True(t, errors.As(err, &apiErr), "control call %d: %v", i, err)
			assert.Equal(t, http.StatusBadGateway, apiErr.Code)
		}
		_, err = client.SendPaidMedia(context.Background(), good)
		assert.ErrorIs(t, err, gobreaker.ErrOpenState, "control: the breaker opens after two 502s (no withRetry, so no ErrCircuitOpen wrap)")
		assert.NotErrorIs(t, err, tg.ErrCircuitOpen, "the open breaker is not wrapped in ErrCircuitOpen (upgrade note; A-C1-01)")
		assert.Equal(t, 3, server.CaptureCount())
	})
	t.Run("limiter", func(t *testing.T) {
		server := testutil.NewMockServer(t)
		client := testutil.NewTestClient(t, server.BaseURL(),
			sender.WithRateLimit(0, 0), sender.WithPerChatRateLimit(0, 0), sender.WithGroupRateLimit(0, 0))
		for i, r := range b48b2BadRequests() {
			_, err := client.SendPaidMedia(context.Background(), r.req)
			require.EqualError(t, err, r.text, "row %d", i)
		}
		assert.Equal(t, 0, client.ChatLimiterCount())
		_, err := client.SendPaidMedia(context.Background(), good)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds limiter's burst 0", "control: the limiter is live")
		var ve *tg.ValidationError
		assert.False(t, errors.As(err, &ve))
		assert.Equal(t, 0, server.CaptureCount())
	})
}

// T-build
func TestB48B2_Builder(t *testing.T) {
	t.Run("nil media is skipped", func(t *testing.T) {
		mr, err := sender.BuildMultipartRequest(sender.SendPaidMediaRequest{ChatID: testutil.TestChatID, StarCount: 1})
		require.NoError(t, err)
		assert.NotContains(t, mr.Params, "media")
	})
	t.Run("empty media is []", func(t *testing.T) {
		mr, err := sender.BuildMultipartRequest(sender.SendPaidMediaRequest{ChatID: testutil.TestChatID, StarCount: 1, Media: []sender.InputPaidMedia{}})
		require.NoError(t, err)
		assert.Equal(t, "[]", mr.Params["media"])
	})
	builderText := map[string]string{
		"empty media":                   "field media: item 1: media: InputFile must have FileID, URL, or Reader set",
		"live photo without photo":      "field media: item 1: live_photo requires Media.Photo",
		"empty photo":                   "field media: item 1: photo: InputFile must have FileID, URL, or Reader set",
		"empty thumbnail":               "field media: item 1: thumbnail: InputFile must have FileID, URL, or Reader set",
		"empty cover":                   "field media: item 1: cover: InputFile must have FileID, URL, or Reader set",
		"empty cover on photo":          "field media: item 1: cover: InputFile must have FileID, URL, or Reader set",
		"empty thumbnail on photo":      "field media: item 1: thumbnail: InputFile must have FileID, URL, or Reader set",
		"empty photo on live photo":     "field media: item 1: photo: InputFile must have FileID, URL, or Reader set",
		"empty media on video":          "field media: item 1: media: InputFile must have FileID, URL, or Reader set",
		"empty media on live photo":     "field media: item 1: media: InputFile must have FileID, URL, or Reader set",
		"empty photo on video":          "field media: item 1: photo: InputFile must have FileID, URL, or Reader set",
		"empty thumbnail on live photo": "field media: item 1: thumbnail: InputFile must have FileID, URL, or Reader set",
		"empty cover on live photo":     "field media: item 1: cover: InputFile must have FileID, URL, or Reader set",
	}
	good := sender.NewInputPaidMediaPhoto(sender.FromFileID("P"))
	for _, b := range b48b2BadItems() {
		t.Run(b.name, func(t *testing.T) {
			_, err := sender.BuildMultipartRequest(paidReq(good, b.item))
			require.Error(t, err)
			assert.Equal(t, builderText[b.name], err.Error())
		})
	}
}

// T-retry
func TestB48B2_NoRetry(t *testing.T) {
	rows := []struct {
		name  string
		code  int
		first func(w http.ResponseWriter)
	}{
		{"429", http.StatusTooManyRequests, func(w http.ResponseWriter) { testutil.ReplyRateLimit(w, 1) }},
		{"502", http.StatusBadGateway, func(w http.ResponseWriter) {
			testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway")
		}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			var paid, plain atomic.Int32
			server.On("/bot"+testutil.TestToken+"/sendPaidMedia", func(w http.ResponseWriter, _ *http.Request) {
				if paid.Add(1) == 1 {
					r.first(w)
					return
				}
				testutil.ReplyMessage(w, 1)
			})
			server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, _ *http.Request) {
				if plain.Add(1) == 1 {
					r.first(w)
					return
				}
				testutil.ReplyMessage(w, 2)
			})
			sleeper := &testutil.FakeSleeper{}
			client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper)
			_, err := client.SendPaidMedia(context.Background(), paidReq(sender.NewInputPaidMediaPhoto(sender.FromFileID("P"))))
			var apiErr *tg.APIError
			require.True(t, errors.As(err, &apiErr), "got %v", err)
			assert.Equal(t, r.code, apiErr.Code)
			assert.Equal(t, int32(1), paid.Load(), "exactly one request: no retry")
			assert.Equal(t, 0, sleeper.CallCount(), "no backoff sleep")

			_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: testutil.TestChatID, Text: "hi"})
			require.NoError(t, err)
			assert.Equal(t, int32(2), plain.Load(), "control: SendMessage is retried once")
			assert.GreaterOrEqual(t, sleeper.CallCount(), 1, "the sleeper is wired")
		})
	}
	t.Run("timeout", func(t *testing.T) {
		rt := &b48b2DrainTransport{}
		sleeper := &testutil.FakeSleeper{}
		client := testutil.NewRetryTestClient(t, "http://galigo.invalid", sleeper, sender.WithHTTPClient(&http.Client{Transport: rt}),
			sender.WithPerChatRateLimit(1000, 100))
		_, err := client.SendPaidMedia(context.Background(), paidReq(sender.NewInputPaidMediaPhoto(sender.FromBytes(b48b2PhotoBytes, "p.jpg"))))
		var ne net.Error
		require.True(t, errors.As(err, &ne) && ne.Timeout(), "error %v", err)
		assert.Equal(t, int32(1), rt.n.Load(), "exactly one round trip")
		assert.Positive(t, rt.bodyBytes.Load(), "the form body was read")
		assert.Equal(t, 0, sleeper.CallCount())

		rt.n.Store(0)
		_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: testutil.TestChatID, Text: "hi"})
		require.Error(t, err)
		assert.GreaterOrEqual(t, rt.n.Load(), int32(2), "control: SendMessage retries a timeout")
	})
}

// b48b2DrainTransport counts round trips, reads and closes the request body (so a multipart encoder goroutine
// ends), and fails with a network timeout. No network is used.
type b48b2DrainTransport struct {
	n         atomic.Int32
	bodyBytes atomic.Int64
}

func (rt *b48b2DrainTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.n.Add(1)
	if req.Body != nil {
		n, _ := io.Copy(io.Discard, req.Body)
		rt.bodyBytes.Add(n)
		_ = req.Body.Close()
	}
	return nil, api101bTimeout{}
}

// T-lim
func TestB48B2_LimiterKey(t *testing.T) {
	call := func(c *sender.Client, chat tg.ChatID) error {
		_, err := c.SendPaidMedia(context.Background(), sender.SendPaidMediaRequest{
			ChatID: chat, StarCount: 1, Media: []sender.InputPaidMedia{sender.NewInputPaidMediaPhoto(sender.FromFileID("P"))},
		})
		return err
	}
	t.Run("int", func(t *testing.T) {
		api100LimiterRow(t, "sendPaidMedia", `{"message_id":1,"date":1,"chat":{"id":-1001234567890,"type":"supergroup"}}`, call)
	})
	// block: the call waits on its chat's limiter, which SendMessage to the same chat shares (O-T1-01, E-0121)
	for _, c := range []struct {
		name        string
		chat, other tg.ChatID
	}{{"block_int", int64(-100), int64(-200)}, {"block_username", "@chan", "@other"}} {
		t.Run(c.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/sendPaidMedia", func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyMessage(w, 1) })
			server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyMessage(w, 2) })
			client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(1000, 100),
				sender.WithPerChatRateLimit(0.001, 1), sender.WithGroupRateLimit(0.001, 1))
			_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: c.chat, Text: "hi"})
			require.NoError(t, err)
			require.Equal(t, 1, server.CaptureCount())
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err = client.SendPaidMedia(ctx, sender.SendPaidMediaRequest{ChatID: c.chat, StarCount: 1,
				Media: []sender.InputPaidMedia{sender.NewInputPaidMediaPhoto(sender.FromFileID("P"))}})
			require.Error(t, err, "same chat: blocked")
			assert.Contains(t, err.Error(), "would exceed context deadline")
			assert.Equal(t, 1, server.CaptureCount(), "no request for the blocked call")
			require.NoError(t, call(client, c.other), "another chat has its own limiter")
			assert.Equal(t, 2, server.CaptureCount())
		})
	}
	t.Run("global", func(t *testing.T) {
		// the call also waits on the global limiter (O-T1-01)
		server := testutil.NewMockServer(t)
		server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyMessage(w, 2) })
		client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(0.001, 1),
			sender.WithPerChatRateLimit(1000, 100), sender.WithGroupRateLimit(1000, 100))
		_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: int64(-300), Text: "hi"})
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err = client.SendPaidMedia(ctx, sender.SendPaidMediaRequest{ChatID: int64(-400), StarCount: 1,
			Media: []sender.InputPaidMedia{sender.NewInputPaidMediaPhoto(sender.FromFileID("P"))}})
		require.Error(t, err, "another chat, but the global token is spent")
		assert.Contains(t, err.Error(), "would exceed context deadline")
		assert.Equal(t, 1, server.CaptureCount(), "no request for the blocked call")
	})
	t.Run("string", func(t *testing.T) {
		server := testutil.NewMockServer(t)
		server.On("/bot"+testutil.TestToken+"/sendPaidMedia", func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyMessage(w, 1) })
		server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyMessage(w, 2) })
		client := testutil.NewTestClient(t, server.BaseURL(), sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
		require.NoError(t, call(client, "@chan"))
		assert.Equal(t, 1, client.ChatLimiterCount(), "after the call on @chan")
		_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: "@chan", Text: "hi"})
		require.NoError(t, err)
		assert.Equal(t, 1, client.ChatLimiterCount(), "SendMessage to @chan shares the entry")
		require.NoError(t, call(client, "@other"))
		assert.Equal(t, 2, client.ChatLimiterCount(), "a second chat has its own entry")
	})
}

// b48b2Reply is the measured reply (E26/lookups/probe-b48b2-raw/03-sendPaidMedia.json) with testutil's bot and chat
// and synthetic file ids; the video's undocumented "thumb" key is kept (a lenient decode ignores it).
const b48b2Reply = `{"message_id":1574,"from":{"id":123456789,"is_bot":true,"first_name":"Test Bot","username":"testbot"},` +
	`"chat":{"id":-1001234567890,"title":"Test Chat","type":"supergroup"},"date":1794440965,` +
	`"paid_media":{"star_count":1,"paid_media":[` +
	`{"type":"photo","photo":[{"file_id":"PS90","file_unique_id":"PS90-u","file_size":341,"width":90,"height":90},` +
	`{"file_id":"PS100","file_unique_id":"PS100-u","file_size":361,"width":100,"height":100}]},` +
	`{"type":"video","video":{"duration":1,"width":320,"height":240,"file_name":"video.mp4","mime_type":"video/mp4",` +
	`"thumbnail":{"file_id":"VT","file_unique_id":"VT-u","file_size":733,"width":320,"height":240},` +
	`"thumb":{"file_id":"VT","file_unique_id":"VT-u","file_size":733,"width":320,"height":240},` +
	`"file_id":"VID","file_unique_id":"VID-u","file_size":663}},` +
	`{"type":"live_photo","live_photo":{"photo":[{"file_id":"PS90","file_unique_id":"PS90-u","file_size":341,"width":90,"height":90},` +
	`{"file_id":"PS100","file_unique_id":"PS100-u","file_size":361,"width":100,"height":100}],` +
	`"duration":1,"width":320,"height":240,"mime_type":"video/mp4","file_id":"LPV","file_unique_id":"LPV-u","file_size":663}}]},` +
	`"caption":"galigo paid media"}`

// T-dec
func TestB48B2_DecodesReply(t *testing.T) {
	var got *tg.Message
	_, _, _, err := wireCall(t, "sendPaidMedia", func(w http.ResponseWriter, _ *http.Request) {
		api96ReplyRaw(w, `{"ok":true,"result":`+b48b2Reply+`}`)
	}, func(ctx context.Context, c *sender.Client) error {
		var err error
		got, err = c.SendPaidMedia(ctx, paidReq(sender.NewInputPaidMediaPhoto(sender.FromFileID("P"))))
		return err
	})
	require.NoError(t, err)
	sizes := []tg.PhotoSize{
		{FileID: "PS90", FileUniqueID: "PS90-u", Width: 90, Height: 90, FileSize: 341},
		{FileID: "PS100", FileUniqueID: "PS100-u", Width: 100, Height: 100, FileSize: 361},
	}
	want := &tg.Message{
		MessageID: 1574, Date: 1794440965, Caption: "galigo paid media",
		From: &tg.User{ID: testutil.TestBotID, IsBot: true, FirstName: "Test Bot", Username: testutil.TestBotUsername},
		Chat: &tg.Chat{ID: -1001234567890, Title: "Test Chat", Type: "supergroup"},
		PaidMedia: &tg.PaidMediaInfo{StarCount: 1, PaidMedia: []tg.PaidMedia{
			{Type: "photo", Photo: []tg.PhotoSize{sizes[0], sizes[1]}},
			{Type: "video", Video: &tg.Video{FileID: "VID", FileUniqueID: "VID-u", Width: 320, Height: 240, Duration: 1,
				FileName: "video.mp4", MimeType: "video/mp4", FileSize: 663,
				Thumbnail: &tg.PhotoSize{FileID: "VT", FileUniqueID: "VT-u", Width: 320, Height: 240, FileSize: 733}}},
			{Type: "live_photo", LivePhoto: &tg.LivePhoto{Photo: []tg.PhotoSize{sizes[0], sizes[1]}, FileID: "LPV",
				FileUniqueID: "LPV-u", Width: 320, Height: 240, Duration: 1, MimeType: "video/mp4", FileSize: 663}},
		}},
	}
	assert.Equal(t, want, got)
}
