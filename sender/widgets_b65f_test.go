package sender_test

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"reflect"
	"sort"
	"strconv"
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

// B-65 65f: the five widget methods (Bot API 10.4), TEST-PLAN rows U1-U11 and V0-V3. Shared helpers: api101bPath,
// api101bOK, api101bFailTransport (sender/joinreq101b_test.go), api96ReplyRaw (sender/managed_bots_test.go),
// api100Chat (sender/chat_info_test.go: -1001234567890), b48c2OnlySentinel (sender/suggested_post_b48c2_test.go).

const b65fHTML = "<b>a & b</b> <script>x()</script>"

func b65fPtr(s string) *string { return &s }

// b65fBigID is a message id above 2^31 where int has 64 bits (70 on 32-bit builds).
func b65fBigID() int {
	if strconv.IntSize == 64 {
		return 3000000000
	}
	return 70
}

// b65fHugeID is an odd message id above 2^53 where int has 64 bits: a float64 step on the way changes its digits.
func b65fHugeID() int {
	if strconv.IntSize == 64 {
		return 9007199254740993
	}
	return 71
}

// b65fFast are the limiter options of every row that is not about the limiter.
func b65fFast() []sender.Option {
	return []sender.Option{sender.WithRateLimit(1000, 100), sender.WithGroupRateLimit(1000, 100),
		sender.WithPerChatRateLimit(1000, 100)}
}

// b65fEntry is one widget method path; the two editMessageWidgetData forms are separate entries (E-0189).
type b65fEntry struct {
	name   string // the row label
	method string // the API method
	chat   bool   // keyed on a chat id
	send   bool   // returns a message
	call   func(ctx context.Context, c *sender.Client, chat tg.ChatID) (*tg.Message, error)
}

func b65fNoMsg(err error) (*tg.Message, error) { return nil, err }

var b65fMethods = [...]b65fEntry{
	{"sendWidget", "sendWidget", true, true, func(ctx context.Context, c *sender.Client, chat tg.ChatID) (*tg.Message, error) {
		return c.SendWidget(ctx, sender.SendWidgetRequest{ChatID: chat, HTML: "h"})
	}},
	{"sendSharedWidget", "sendSharedWidget", true, true, func(ctx context.Context, c *sender.Client, chat tg.ChatID) (*tg.Message, error) {
		return c.SendSharedWidget(ctx, sender.SendSharedWidgetRequest{ChatID: chat, WidgetID: "w"})
	}},
	{"answerWidgetQuery", "answerWidgetQuery", false, false, func(ctx context.Context, c *sender.Client, _ tg.ChatID) (*tg.Message, error) {
		return b65fNoMsg(c.AnswerWidgetQuery(ctx, sender.AnswerWidgetQueryRequest{WidgetQueryID: "q"}))
	}},
	{"editMessageWidgetData chat", "editMessageWidgetData", true, false, func(ctx context.Context, c *sender.Client, chat tg.ChatID) (*tg.Message, error) {
		return b65fNoMsg(c.EditMessageWidgetData(ctx, sender.EditMessageWidgetDataRequest{ChatID: chat, MessageID: 42}))
	}},
	{"editMessageWidgetData inline", "editMessageWidgetData", false, false, func(ctx context.Context, c *sender.Client, _ tg.ChatID) (*tg.Message, error) {
		return b65fNoMsg(c.EditMessageWidgetData(ctx, sender.EditMessageWidgetDataRequest{InlineMessageID: "i"}))
	}},
	{"editSharedWidgetData", "editSharedWidgetData", false, false, func(ctx context.Context, c *sender.Client, _ tg.ChatID) (*tg.Message, error) {
		return b65fNoMsg(c.EditSharedWidgetData(ctx, sender.EditSharedWidgetDataRequest{WidgetID: "w"}))
	}},
}

// b65fOK answers the two sends with a message and the other methods, sendMessage and unbanChatMember with true.
func b65fOK(server *testutil.MockTelegramServer) {
	for _, m := range []string{"sendWidget", "sendSharedWidget", "sendMessage"} {
		server.On(api101bPath(m), func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyMessage(w, 42) })
	}
	api101bOK(server, "answerWidgetQuery", "editMessageWidgetData", "editSharedWidgetData", "unbanChatMember")
}

func b65fClient(t *testing.T, opts ...sender.Option) (*testutil.MockTelegramServer, *sender.Client) {
	t.Helper()
	server := testutil.NewMockServer(t)
	b65fOK(server)
	return server, testutil.NewTestClient(t, server.BaseURL(), append(b65fFast(), opts...)...)
}

// b65fOne asserts exactly one request to method and returns it.
func b65fOne(t *testing.T, server *testutil.MockTelegramServer, method string) *testutil.Capture {
	t.Helper()
	require.Equal(t, 1, server.CaptureCount(), "exactly one request")
	c := server.LastCapture()
	c.AssertPath(t, api101bPath(method))
	c.AssertMethod(t, http.MethodPost)
	assert.Contains(t, c.ContentType, "application/json")
	return c
}

// b65fNumbers decodes a body with exact numbers (JSONEq compares numbers as float64).
func b65fNumbers(t *testing.T, body []byte) map[string]any {
	t.Helper()
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	var m map[string]any
	require.NoError(t, d.Decode(&m))
	return m
}

// The method list has exactly six entries: a compile-time check (the index is out of range for any other length).
var _ = [1]struct{}{}[len(b65fMethods)-6]

// U1 minimal: only the required fields.
func TestB65F_U1_Minimal(t *testing.T) {
	id := b65fBigID()
	rows := []struct {
		name, method, want string
		call               func(*sender.Client) error
	}{
		{"sendWidget", "sendWidget", `{"chat_id":-1001234567890,"html":"<b>a & b</b> <script>x()</script>"}`, func(c *sender.Client) error {
			_, err := c.SendWidget(context.Background(), sender.SendWidgetRequest{ChatID: api100Chat, HTML: b65fHTML})
			return err
		}},
		{"sendSharedWidget", "sendSharedWidget", `{"chat_id":-1001234567890,"widget_id":"w"}`, func(c *sender.Client) error {
			_, err := c.SendSharedWidget(context.Background(), sender.SendSharedWidgetRequest{ChatID: api100Chat, WidgetID: "w"})
			return err
		}},
		{"answerWidgetQuery", "answerWidgetQuery", `{"widget_query_id":"q"}`, func(c *sender.Client) error {
			return c.AnswerWidgetQuery(context.Background(), sender.AnswerWidgetQueryRequest{WidgetQueryID: "q"})
		}},
		{"editMessageWidgetData chat", "editMessageWidgetData", `{"chat_id":-1001234567890,"message_id":` + strconv.Itoa(id) + `}`, func(c *sender.Client) error {
			return c.EditMessageWidgetData(context.Background(), sender.EditMessageWidgetDataRequest{ChatID: api100Chat, MessageID: id})
		}},
		{"editMessageWidgetData inline", "editMessageWidgetData", `{"inline_message_id":"i"}`, func(c *sender.Client) error {
			return c.EditMessageWidgetData(context.Background(), sender.EditMessageWidgetDataRequest{InlineMessageID: "i"})
		}},
		{"editSharedWidgetData", "editSharedWidgetData", `{"widget_id":"w","public_data":""}`, func(c *sender.Client) error {
			return c.EditSharedWidgetData(context.Background(), sender.EditSharedWidgetDataRequest{WidgetID: "w"})
		}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server, client := b65fClient(t)
			require.NoError(t, r.call(client))
			assert.JSONEq(t, r.want, b65fOne(t, server, r.method).BodyString())
		})
	}
}

// b65fMarkups are the four reply_markup forms of the two sends, with their exact wire value.
var b65fMarkups = []struct {
	name string
	v    any
	want string
}{
	{"inline", tg.InlineKeyboardMarkup{InlineKeyboard: [][]tg.InlineKeyboardButton{{{Text: "a", CallbackData: "b"}}}},
		`{"inline_keyboard":[[{"text":"a","callback_data":"b"}]]}`},
	{"reply", tg.ReplyKeyboardMarkup{Keyboard: [][]tg.KeyboardButton{{{Text: "k"}}}, ResizeKeyboard: true},
		`{"keyboard":[[{"text":"k"}]],"resize_keyboard":true}`},
	{"remove", tg.ReplyKeyboardRemove{RemoveKeyboard: true}, `{"remove_keyboard":true}`},
	{"force_reply", map[string]any{"force_reply": true}, `{"force_reply":true}`},
}

// U1 markup: every reply_markup form reaches the wire as given, on both sends.
func TestB65F_U1_Markup(t *testing.T) {
	for _, mk := range b65fMarkups {
		t.Run("sendWidget/"+mk.name, func(t *testing.T) {
			server, client := b65fClient(t)
			_, err := client.SendWidget(context.Background(), sender.SendWidgetRequest{ChatID: api100Chat, HTML: "h", ReplyMarkup: mk.v})
			require.NoError(t, err)
			got, _ := json.Marshal(b65fOne(t, server, "sendWidget").BodyMap(t)["reply_markup"])
			assert.JSONEq(t, mk.want, string(got))
		})
		t.Run("sendSharedWidget/"+mk.name, func(t *testing.T) {
			server, client := b65fClient(t)
			_, err := client.SendSharedWidget(context.Background(), sender.SendSharedWidgetRequest{ChatID: api100Chat, WidgetID: "w", ReplyMarkup: mk.v})
			require.NoError(t, err)
			got, _ := json.Marshal(b65fOne(t, server, "sendSharedWidget").BodyMap(t)["reply_markup"])
			assert.JSONEq(t, mk.want, string(got))
		})
	}
}

func b65fEntities() []tg.MessageEntity {
	return []tg.MessageEntity{{Type: "bold", Offset: 0, Length: 2}, {Type: "italic", Offset: 2, Length: 3}}
}

// U1 full: every field set, each with its own value.
func TestB65F_U1_Full(t *testing.T) {
	markup := tg.InlineKeyboardMarkup{InlineKeyboard: [][]tg.InlineKeyboardButton{{{Text: "a", CallbackData: "b"}}}}
	const ents = `[{"type":"bold","offset":0,"length":2},{"type":"italic","offset":2,"length":3}]`
	const mk = `{"inline_keyboard":[[{"text":"a","callback_data":"b"}]]}`
	rows := []struct {
		name, method, want string
		call               func(*sender.Client) error
	}{
		{"sendWidget", "sendWidget", `{"chat_id":"@chan","message_thread_id":11,"html":"<b>a & b</b> <script>x()</script>",` +
			`"width":300,"height":200,"public_data":"pub-1","chat_data":"chat-2","caption":"  Привет  ","parse_mode":"HTML",` +
			`"caption_entities":` + ents + `,"show_caption_above_media":true,"allow_queries":true,"is_shared":true,` +
			`"disable_notification":true,"protect_content":true,"allow_paid_broadcast":true,"message_effect_id":"eff-3",` +
			`"reply_parameters":{"message_id":9},"reply_markup":` + mk + `}`, func(c *sender.Client) error {
			_, err := c.SendWidget(context.Background(), sender.SendWidgetRequest{ChatID: "@chan", MessageThreadID: 11,
				HTML: b65fHTML, Width: 300, Height: 200, PublicData: "pub-1", ChatData: "chat-2", Caption: "  Привет  ",
				ParseMode: tg.ParseModeHTML, CaptionEntities: b65fEntities(), ShowCaptionAboveMedia: true, AllowQueries: true,
				IsShared: true, DisableNotification: true, ProtectContent: true, AllowPaidBroadcast: true,
				MessageEffectID: "eff-3", ReplyParameters: &tg.ReplyParameters{MessageID: 9}, ReplyMarkup: markup})
			return err
		}},
		{"sendSharedWidget", "sendSharedWidget", `{"chat_id":5000000001,"message_thread_id":12,"widget_id":"wid-4",` +
			`"chat_data":"chat-2","caption":"  Привет  ","parse_mode":"MarkdownV2","caption_entities":` + ents + `,` +
			`"show_caption_above_media":true,"disable_notification":true,"protect_content":true,"allow_paid_broadcast":true,` +
			`"message_effect_id":"eff-5","reply_parameters":{"message_id":8},"reply_markup":` + mk + `}`, func(c *sender.Client) error {
			_, err := c.SendSharedWidget(context.Background(), sender.SendSharedWidgetRequest{ChatID: int64(5000000001),
				MessageThreadID: 12, WidgetID: "wid-4", ChatData: "chat-2", Caption: "  Привет  ",
				ParseMode: tg.ParseModeMarkdownV2, CaptionEntities: b65fEntities(), ShowCaptionAboveMedia: true,
				DisableNotification: true, ProtectContent: true, AllowPaidBroadcast: true, MessageEffectID: "eff-5",
				ReplyParameters: &tg.ReplyParameters{MessageID: 8}, ReplyMarkup: markup})
			return err
		}},
		{"answerWidgetQuery", "answerWidgetQuery", `{"widget_query_id":"q-6","data":"x"}`, func(c *sender.Client) error {
			return c.AnswerWidgetQuery(context.Background(), sender.AnswerWidgetQueryRequest{WidgetQueryID: "q-6", Data: "x"})
		}},
		{"editSharedWidgetData", "editSharedWidgetData", `{"widget_id":"wid-4","public_data":"pub-1","cache_time":5}`, func(c *sender.Client) error {
			return c.EditSharedWidgetData(context.Background(), sender.EditSharedWidgetDataRequest{WidgetID: "wid-4", PublicData: "pub-1", CacheTime: 5})
		}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server, client := b65fClient(t)
			require.NoError(t, r.call(client))
			c := b65fOne(t, server, r.method)
			assert.JSONEq(t, r.want, c.BodyString())
			if r.method == "sendWidget" {
				body := c.BodyMap(t)
				assert.Equal(t, b65fHTML, body["html"], "html is sent as given")
				assert.Equal(t, "  Привет  ", body["caption"], "the caption is not trimmed")
			}
		})
	}
}

// U1 bools: each bool alone sends exactly its own key.
func TestB65F_U1_Bools(t *testing.T) {
	widget := map[string]func(*sender.SendWidgetRequest){
		"show_caption_above_media": func(r *sender.SendWidgetRequest) { r.ShowCaptionAboveMedia = true },
		"allow_queries":            func(r *sender.SendWidgetRequest) { r.AllowQueries = true },
		"is_shared":                func(r *sender.SendWidgetRequest) { r.IsShared = true },
		"disable_notification":     func(r *sender.SendWidgetRequest) { r.DisableNotification = true },
		"protect_content":          func(r *sender.SendWidgetRequest) { r.ProtectContent = true },
		"allow_paid_broadcast":     func(r *sender.SendWidgetRequest) { r.AllowPaidBroadcast = true },
	}
	for key, set := range widget {
		t.Run("sendWidget/"+key, func(t *testing.T) {
			server, client := b65fClient(t)
			req := sender.SendWidgetRequest{ChatID: api100Chat, HTML: "h"}
			set(&req)
			_, err := client.SendWidget(context.Background(), req)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"chat_id": float64(api100Chat), "html": "h", key: true}, b65fOne(t, server, "sendWidget").BodyMap(t))
		})
	}
	shared := map[string]func(*sender.SendSharedWidgetRequest){
		"show_caption_above_media": func(r *sender.SendSharedWidgetRequest) { r.ShowCaptionAboveMedia = true },
		"disable_notification":     func(r *sender.SendSharedWidgetRequest) { r.DisableNotification = true },
		"protect_content":          func(r *sender.SendSharedWidgetRequest) { r.ProtectContent = true },
		"allow_paid_broadcast":     func(r *sender.SendSharedWidgetRequest) { r.AllowPaidBroadcast = true },
	}
	for key, set := range shared {
		t.Run("sendSharedWidget/"+key, func(t *testing.T) {
			server, client := b65fClient(t)
			req := sender.SendSharedWidgetRequest{ChatID: api100Chat, WidgetID: "w"}
			set(&req)
			_, err := client.SendSharedWidget(context.Background(), req)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"chat_id": float64(api100Chat), "widget_id": "w", key: true}, b65fOne(t, server, "sendSharedWidget").BodyMap(t))
		})
	}
}

// U1 keys: the JSON keys of each new type are the spec's keys in spec order, and only its optional keys have
// omitempty (public_data of editSharedWidgetData is required). Copied from E65f/lookups/widget-methods-spec.txt.
func TestB65F_U1_Keys(t *testing.T) {
	rows := []struct {
		typ            reflect.Type
		keys, required []string
	}{
		{reflect.TypeFor[sender.SendWidgetRequest](), []string{"chat_id", "message_thread_id", "html", "width", "height",
			"public_data", "chat_data", "caption", "parse_mode", "caption_entities", "show_caption_above_media", "allow_queries",
			"is_shared", "disable_notification", "protect_content", "allow_paid_broadcast", "message_effect_id",
			"reply_parameters", "reply_markup"}, []string{"chat_id", "html"}},
		{reflect.TypeFor[sender.SendSharedWidgetRequest](), []string{"chat_id", "message_thread_id", "widget_id", "chat_data",
			"caption", "parse_mode", "caption_entities", "show_caption_above_media", "disable_notification", "protect_content",
			"allow_paid_broadcast", "message_effect_id", "reply_parameters", "reply_markup"}, []string{"chat_id", "widget_id"}},
		{reflect.TypeFor[sender.AnswerWidgetQueryRequest](), []string{"widget_query_id", "data"}, []string{"widget_query_id"}},
		{reflect.TypeFor[sender.EditMessageWidgetDataRequest](), []string{"chat_id", "message_id", "inline_message_id",
			"public_data", "chat_data"}, nil},
		{reflect.TypeFor[sender.EditSharedWidgetDataRequest](), []string{"widget_id", "public_data", "cache_time"},
			[]string{"widget_id", "public_data"}},
		{reflect.TypeFor[tg.InputWidgetMessageContent](), []string{"html", "width", "height", "public_data", "chat_data",
			"caption", "parse_mode", "caption_entities", "show_caption_above_media", "allow_queries", "is_shared"},
			[]string{"html"}},
		{reflect.TypeFor[tg.InputSharedWidgetMessageContent](), []string{"widget_id", "chat_data", "caption", "parse_mode",
			"caption_entities", "show_caption_above_media"}, []string{"widget_id"}},
	}
	for _, r := range rows {
		t.Run(r.typ.Name(), func(t *testing.T) {
			var keys, required []string
			for i := range r.typ.NumField() {
				key, opts, _ := strings.Cut(r.typ.Field(i).Tag.Get("json"), ",")
				keys = append(keys, key)
				if opts != "omitempty" {
					required = append(required, key)
				}
			}
			assert.Equal(t, r.keys, keys)
			assert.Equal(t, r.required, required, "keys without omitempty")
		})
	}
}

// U2: the two data fields of editMessageWidgetData have three states each, on both target forms.
func TestB65F_U2_ThreeStates(t *testing.T) {
	states := []struct {
		name string
		v    *string
	}{{"nil", nil}, {"empty", b65fPtr("")}, {"set", b65fPtr("x")}}
	forms := []struct {
		name string
		req  sender.EditMessageWidgetDataRequest
		base map[string]any
	}{
		{"chat", sender.EditMessageWidgetDataRequest{ChatID: api100Chat, MessageID: 42}, map[string]any{"chat_id": float64(api100Chat), "message_id": float64(42)}},
		{"inline", sender.EditMessageWidgetDataRequest{InlineMessageID: "i"}, map[string]any{"inline_message_id": "i"}},
	}
	for _, f := range forms {
		for _, pub := range states {
			for _, chat := range states {
				t.Run(f.name+"/public_"+pub.name+"/chat_"+chat.name, func(t *testing.T) {
					server, client := b65fClient(t)
					req := f.req
					req.PublicData, req.ChatData = pub.v, chat.v
					require.NoError(t, client.EditMessageWidgetData(context.Background(), req))
					want := map[string]any{}
					for k, v := range f.base {
						want[k] = v
					}
					if pub.v != nil {
						want["public_data"] = *pub.v
					}
					if chat.v != nil {
						want["chat_data"] = *chat.v
					}
					assert.Equal(t, want, b65fOne(t, server, "editMessageWidgetData").BodyMap(t))
				})
			}
		}
	}
}

// U3: cache_time is sent as given; the server checks its range.
func TestB65F_U3_CacheTime(t *testing.T) {
	for _, ct := range []int{5, 3600, 1, 3601, -1} {
		t.Run(strconv.Itoa(ct), func(t *testing.T) {
			server, client := b65fClient(t)
			require.NoError(t, client.EditSharedWidgetData(context.Background(), sender.EditSharedWidgetDataRequest{WidgetID: "w", CacheTime: ct}))
			assert.JSONEq(t, fmt.Sprintf(`{"widget_id":"w","public_data":"","cache_time":%d}`, ct), b65fOne(t, server, "editSharedWidgetData").BodyString())
		})
	}
}

// b65fZlibHTML is a fixed html within the raw limit whose zlib size (best compression) is above 4096 bytes.
func b65fZlibHTML(t *testing.T) string {
	t.Helper()
	r := rand.New(rand.NewPCG(65, 6))
	raw := make([]byte, 16000)
	for i := range raw {
		raw[i] = byte(r.Uint32())
	}
	html := hex.EncodeToString(raw)
	var buf bytes.Buffer
	zw, err := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	require.NoError(t, err)
	_, err = zw.Write([]byte(html))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.LessOrEqual(t, len(html), 32768, "the fixture is within the raw limit")
	require.Greater(t, buf.Len(), 4096, "the fixture is above the zlib limit")
	return html
}

// b65fKeysWith returns the sorted top-level keys of body whose value is the string v.
func b65fKeysWith(body map[string]any, v string) []string {
	var keys []string
	for k, x := range body {
		if x == v {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// U4b: galigo has no local size limits: over-limit values are sent unchanged.
func TestB65F_U4b_NoLocalLimits(t *testing.T) {
	big := strings.Repeat("d", 1025)
	rows := []struct {
		name, method, key, want string
		call                    func(*sender.Client) error
	}{
		{"html 32769", "sendWidget", "html", strings.Repeat("a", 32769), func(c *sender.Client) error {
			_, err := c.SendWidget(context.Background(), sender.SendWidgetRequest{ChatID: api100Chat, HTML: strings.Repeat("a", 32769)})
			return err
		}},
		{"html zlib", "sendWidget", "html", b65fZlibHTML(t), func(c *sender.Client) error {
			_, err := c.SendWidget(context.Background(), sender.SendWidgetRequest{ChatID: api100Chat, HTML: b65fZlibHTML(t)})
			return err
		}},
		{"sendWidget public_data", "sendWidget", "public_data", big, func(c *sender.Client) error {
			_, err := c.SendWidget(context.Background(), sender.SendWidgetRequest{ChatID: api100Chat, HTML: "h", PublicData: big})
			return err
		}},
		{"sendWidget chat_data", "sendWidget", "chat_data", big, func(c *sender.Client) error {
			_, err := c.SendWidget(context.Background(), sender.SendWidgetRequest{ChatID: api100Chat, HTML: "h", ChatData: big})
			return err
		}},
		{"sendSharedWidget chat_data", "sendSharedWidget", "chat_data", big, func(c *sender.Client) error {
			_, err := c.SendSharedWidget(context.Background(), sender.SendSharedWidgetRequest{ChatID: api100Chat, WidgetID: "w", ChatData: big})
			return err
		}},
		{"edit chat public_data", "editMessageWidgetData", "public_data", big, func(c *sender.Client) error {
			return c.EditMessageWidgetData(context.Background(), sender.EditMessageWidgetDataRequest{ChatID: api100Chat, MessageID: 42, PublicData: &big})
		}},
		{"edit chat chat_data", "editMessageWidgetData", "chat_data", big, func(c *sender.Client) error {
			return c.EditMessageWidgetData(context.Background(), sender.EditMessageWidgetDataRequest{ChatID: api100Chat, MessageID: 42, ChatData: &big})
		}},
		{"edit inline public_data", "editMessageWidgetData", "public_data", big, func(c *sender.Client) error {
			return c.EditMessageWidgetData(context.Background(), sender.EditMessageWidgetDataRequest{InlineMessageID: "i", PublicData: &big})
		}},
		{"edit inline chat_data", "editMessageWidgetData", "chat_data", big, func(c *sender.Client) error {
			return c.EditMessageWidgetData(context.Background(), sender.EditMessageWidgetDataRequest{InlineMessageID: "i", ChatData: &big})
		}},
		{"editSharedWidgetData public_data", "editSharedWidgetData", "public_data", big, func(c *sender.Client) error {
			return c.EditSharedWidgetData(context.Background(), sender.EditSharedWidgetDataRequest{WidgetID: "w", PublicData: big})
		}},
		{"answerWidgetQuery data", "answerWidgetQuery", "data", big, func(c *sender.Client) error {
			return c.AnswerWidgetQuery(context.Background(), sender.AnswerWidgetQueryRequest{WidgetQueryID: "q", Data: big})
		}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server, client := b65fClient(t)
			require.NoError(t, r.call(client))
			assert.Equal(t, []string{r.key}, b65fKeysWith(b65fOne(t, server, r.method).BodyMap(t), r.want), "the value is sent once, in its own key")
		})
	}
}

// U4c: only the empty string is refused; blank and one-byte values are sent unchanged.
func TestB65F_U4c_BlankValues(t *testing.T) {
	rows := []struct {
		name, method, key, want string
		call                    func(*sender.Client) error
	}{
		{"html blank", "sendWidget", "html", " \t\n", func(c *sender.Client) error {
			_, err := c.SendWidget(context.Background(), sender.SendWidgetRequest{ChatID: api100Chat, HTML: " \t\n"})
			return err
		}},
		{"html one byte", "sendWidget", "html", "x", func(c *sender.Client) error {
			_, err := c.SendWidget(context.Background(), sender.SendWidgetRequest{ChatID: api100Chat, HTML: "x"})
			return err
		}},
		{"sendSharedWidget widget_id blank", "sendSharedWidget", "widget_id", " ", func(c *sender.Client) error {
			_, err := c.SendSharedWidget(context.Background(), sender.SendSharedWidgetRequest{ChatID: api100Chat, WidgetID: " "})
			return err
		}},
		{"editSharedWidgetData widget_id blank", "editSharedWidgetData", "widget_id", " ", func(c *sender.Client) error {
			return c.EditSharedWidgetData(context.Background(), sender.EditSharedWidgetDataRequest{WidgetID: " "})
		}},
		{"widget_query_id blank", "answerWidgetQuery", "widget_query_id", " ", func(c *sender.Client) error {
			return c.AnswerWidgetQuery(context.Background(), sender.AnswerWidgetQueryRequest{WidgetQueryID: " "})
		}},
		{"inline_message_id blank", "editMessageWidgetData", "inline_message_id", " ", func(c *sender.Client) error {
			return c.EditMessageWidgetData(context.Background(), sender.EditMessageWidgetDataRequest{InlineMessageID: " "})
		}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server, client := b65fClient(t)
			require.NoError(t, r.call(client))
			assert.Equal(t, []string{r.key}, b65fKeysWith(b65fOne(t, server, r.method).BodyMap(t), r.want), "the value is sent once, in its own key")
		})
	}
}

// U5: the sends return the decoded message; a result that is not a message is an error.
func TestB65F_U5_Decode(t *testing.T) {
	id := b65fBigID()
	msg := `{"ok":true,"result":{"message_id":` + strconv.Itoa(id) + `,"date":1,"chat":{"id":-1001234567890,"type":"supergroup"},` +
		`"widget":{"id":"wid-1","html":"<b>h</b>","width":300,"chat_data":"c"}}}`
	for _, m := range b65fMethods[:2] {
		t.Run(m.name+"/message", func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On(api101bPath(m.method), func(w http.ResponseWriter, _ *http.Request) { api96ReplyRaw(w, msg) })
			client := testutil.NewTestClient(t, server.BaseURL())
			got, err := m.call(context.Background(), client, api100Chat)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, id, got.MessageID)
			require.NotNil(t, got.Chat)
			assert.Equal(t, api100Chat, got.Chat.ID)
			require.NotNil(t, got.Widget)
			assert.Equal(t, tg.Widget{ID: "wid-1", HTML: "<b>h</b>", Width: 300, ChatData: "c"}, *got.Widget)
		})
		t.Run(m.name+"/true", func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On(api101bPath(m.method), func(w http.ResponseWriter, _ *http.Request) { api96ReplyRaw(w, `{"ok":true,"result":true}`) })
			client := testutil.NewTestClient(t, server.BaseURL())
			got, err := m.call(context.Background(), client, api100Chat)
			require.Error(t, err)
			assert.Nil(t, got)
			assert.Equal(t, 1, server.CaptureCount(), "the error is the result decode, after the request")
			var apiErr *tg.APIError
			assert.False(t, errors.As(err, &apiErr), "not an API error")
		})
	}
}

// U6: the True methods do not decode the result; a broken envelope is an error.
func TestB65F_U6_ResultNotDecoded(t *testing.T) {
	bodies := []struct {
		name, body string
		wantErr    bool
	}{
		{"true", `{"ok":true,"result":true}`, false},
		{"false", `{"ok":true,"result":false}`, false},
		{"string", `{"ok":true,"result":"x"}`, false},
		{"broken", `{"ok":true,"result":`, true},
	}
	for _, m := range b65fMethods[2:] {
		for _, b := range bodies {
			t.Run(m.name+"/"+b.name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				server.On(api101bPath(m.method), func(w http.ResponseWriter, _ *http.Request) { api96ReplyRaw(w, b.body) })
				client := testutil.NewTestClient(t, server.BaseURL())
				_, err := m.call(context.Background(), client, api100Chat)
				if b.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				assert.Equal(t, 1, server.CaptureCount())
			})
		}
	}
}

// U7: a refusal is an *tg.APIError with the method and the server's text; the measured texts are used.
func TestB65F_U7_APIError(t *testing.T) {
	texts := map[string][]string{
		"sendWidget":                   {"INPUT_MEDIA_WIDGET_NOT_ALLOWED"},
		"sendSharedWidget":             {"invalid widget identifier specified"},
		"answerWidgetQuery":            {"query is too old and response timeout expired or query ID is invalid"},
		"editMessageWidgetData chat":   {"MESSAGE_ID_INVALID", "message with widget to edit not found"},
		"editMessageWidgetData inline": {"invalid inline message identifier specified"},
		"editSharedWidgetData":         {"invalid widget identifier specified"},
	}
	for _, m := range b65fMethods {
		require.NotEmpty(t, texts[m.name], m.name)
		for _, text := range texts[m.name] {
			t.Run(m.name+"/"+text, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				server.On(api101bPath(m.method), func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyBadRequest(w, text) })
				client := testutil.NewTestClient(t, server.BaseURL())
				got, err := m.call(context.Background(), client, api100Chat)
				assert.Nil(t, got, "no message with an error")
				var apiErr *tg.APIError
				require.True(t, errors.As(err, &apiErr), "error %v", err)
				assert.Equal(t, 400, apiErr.Code)
				assert.Equal(t, m.method, apiErr.Method)
				assert.Equal(t, "Bad Request: "+text, apiErr.Description)
				switch {
				case strings.HasPrefix(text, "query is too old"):
					b48c2OnlySentinel(t, err, tg.ErrCallbackExpired)
				case text == "message with widget to edit not found":
					b48c2OnlySentinel(t, err, nil)
				}
			})
		}
	}
}

// U8: none of the methods is retried (429, 500, network timeout); SendMessage is the control.
func TestB65F_U8_NoRetry(t *testing.T) {
	sets := []struct {
		name  string
		first func(http.ResponseWriter)
		code  int
	}{
		{"429", func(w http.ResponseWriter) { testutil.ReplyRateLimit(w, 1) }, 429},
		{"500", func(w http.ResponseWriter) { testutil.ReplyServerError(w, 500, "Internal Server Error") }, 500},
	}
	for _, set := range sets {
		t.Run(set.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			for _, p := range []string{"sendMessage", "sendWidget", "sendSharedWidget", "answerWidgetQuery", "editMessageWidgetData", "editSharedWidgetData"} {
				var n atomic.Int32
				server.On(api101bPath(p), func(w http.ResponseWriter, _ *http.Request) {
					if n.Add(1) == 1 {
						set.first(w)
						return
					}
					testutil.ReplyMessage(w, 4242)
				})
			}
			sleeper := &testutil.FakeSleeper{}
			client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, b65fFast()...)
			for _, m := range b65fMethods {
				server.ResetCaptures()
				got, err := m.call(context.Background(), client, api100Chat)
				assert.Nil(t, got, m.name)
				var apiErr *tg.APIError
				require.True(t, errors.As(err, &apiErr), "%s: error %v", m.name, err)
				assert.Equal(t, set.code, apiErr.Code, m.name)
				if set.code == 429 {
					assert.Equal(t, time.Second, apiErr.RetryAfter, m.name)
				}
				assert.Equal(t, 1, server.CaptureCount(), "%s: exactly one request", m.name)
				if m.name == "editMessageWidgetData chat" { // the inline entry shares the path: answer it with an error again
					server.On(api101bPath(m.method), func(w http.ResponseWriter, _ *http.Request) { set.first(w) })
				}
			}
			assert.Equal(t, 0, sleeper.CallCount(), "no backoff sleep")
			server.ResetCaptures()
			_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api100Chat, Text: "hi"})
			require.NoError(t, err)
			assert.Equal(t, 2, server.CaptureCount(), "the control call is retried once")
		})
	}
	t.Run("timeout", func(t *testing.T) {
		rt := &api101bFailTransport{}
		sleeper := &testutil.FakeSleeper{}
		client := testutil.NewRetryTestClient(t, "http://galigo.invalid", sleeper,
			append(b65fFast(), sender.WithHTTPClient(&http.Client{Transport: rt}))...)
		for _, m := range b65fMethods {
			rt.n.Store(0)
			got, err := m.call(context.Background(), client, api100Chat)
			assert.Nil(t, got, m.name)
			var ne net.Error
			require.True(t, errors.As(err, &ne) && ne.Timeout(), "%s: error %v", m.name, err)
			assert.Equal(t, int32(1), rt.n.Load(), "%s: exactly one round trip", m.name)
		}
		assert.Equal(t, 0, sleeper.CallCount())
		rt.n.Store(0)
		_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api100Chat, Text: "hi"})
		require.Error(t, err)
		assert.GreaterOrEqual(t, rt.n.Load(), int32(2), "the control call is retried")
	})
}

// U9: every call waits on the global limiter; the chat paths are keyed on the chat id like the existing methods;
// the chat-less paths take no per-chat limiter. Every call has a 5 s deadline and a 10 s guard (b65fBound), so a
// wrong path that waits on a slow bucket fails the row instead of hanging the test binary.
func TestB65F_U9_Limiter(t *testing.T) {
	t.Run("global", func(t *testing.T) {
		server := testutil.NewMockServer(t)
		b65fOK(server)
		client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(0, 0))
		for _, m := range b65fMethods {
			err := b65fBound(t, func(ctx context.Context) error { _, err := m.call(ctx, client, api100Chat); return err })
			require.Error(t, err, m.name)
			assert.Contains(t, err.Error(), "exceeds limiter's burst", m.name)
		}
		assert.Equal(t, 0, server.CaptureCount())
	})
	slow := func(t *testing.T) (*testutil.MockTelegramServer, *sender.Client) {
		server := testutil.NewMockServer(t)
		b65fOK(server)
		return server, testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(1000, 100),
			sender.WithPerChatRateLimit(0.001, 1), sender.WithGroupRateLimit(0.001, 1))
	}
	blocked := func(t *testing.T, call func(context.Context) error) {
		t.Helper()
		err := b65fBound(t, call)
		require.Error(t, err, "the chat's bucket is empty")
		assert.Contains(t, err.Error(), "would exceed context deadline")
	}
	keys := []struct {
		name                string
		first, again, other tg.ChatID
	}{
		{"group", int64(-100), int64(-100), int64(-200)},
		{"per_chat", int64(100), int64(100), int64(200)},
		{"string", "@chan", "@chan", "@other"},
	}
	for _, m := range b65fMethods {
		if !m.chat {
			t.Run(m.name+"/chat-less", func(t *testing.T) {
				server, client := b65fClient(t)
				require.NoError(t, b65fBound(t, func(ctx context.Context) error { _, err := m.call(ctx, client, nil); return err }))
				assert.Equal(t, 1, server.CaptureCount())
				assert.Equal(t, 0, client.ChatLimiterCount(), "no per-chat limiter")
			})
			continue
		}
		for _, k := range keys {
			t.Run(m.name+"/key_"+k.name, func(t *testing.T) {
				server, client := slow(t)
				require.NoError(t, b65fBound(t, func(ctx context.Context) error { _, err := m.call(ctx, client, k.first); return err }))
				blocked(t, func(ctx context.Context) error { _, err := m.call(ctx, client, k.again); return err })
				assert.Equal(t, 1, server.CaptureCount(), "no request for the blocked call")
				require.NoError(t, b65fBound(t, func(ctx context.Context) error { _, err := m.call(ctx, client, k.other); return err }),
					"another chat has its own limiter")
				assert.Equal(t, 2, server.CaptureCount())
			})
		}
		for _, chat := range []int64{-300, 300, api100Chat, 5000000001} {
			t.Run(fmt.Sprintf("%s/shared_%d", m.name, chat), func(t *testing.T) {
				server, client := slow(t)
				require.NoError(t, b65fBound(t, func(ctx context.Context) error { return client.UnbanChatMember(ctx, chat, 7) }))
				blocked(t, func(ctx context.Context) error { _, err := m.call(ctx, client, chat); return err })
				assert.Equal(t, 1, server.CaptureCount())
			})
		}
		t.Run(m.name+"/shared_int", func(t *testing.T) {
			server, client := slow(t)
			require.NoError(t, b65fBound(t, func(ctx context.Context) error { return client.UnbanChatMember(ctx, 300, 7) }))
			blocked(t, func(ctx context.Context) error { _, err := m.call(ctx, client, int(300)); return err })
			assert.Equal(t, 1, server.CaptureCount())
		})
		t.Run(m.name+"/shared_string", func(t *testing.T) {
			server, client := slow(t)
			require.NoError(t, b65fBound(t, func(ctx context.Context) error {
				_, err := client.SendMessage(ctx, sender.SendMessageRequest{ChatID: "@chan", Text: "hi"})
				return err
			}))
			blocked(t, func(ctx context.Context) error { _, err := m.call(ctx, client, "@chan"); return err })
			assert.Equal(t, 1, server.CaptureCount())
		})
		t.Run(m.name+"/group_limiter", func(t *testing.T) {
			server, client := b65fClient(t, sender.WithGroupRateLimit(0.001, 1))
			require.NoError(t, b65fBound(t, func(ctx context.Context) error { _, err := m.call(ctx, client, int64(-400)); return err }))
			blocked(t, func(ctx context.Context) error { _, err := m.call(ctx, client, int64(-400)); return err })
			assert.Equal(t, 1, server.CaptureCount())
		})
	}
}

// b65fBound runs call with a 5 s deadline and fails the test if it has not returned after 10 s (a call that ignores
// its context and waits on a slow bucket must fail the row, not hang the binary).
func b65fBound(t *testing.T, call func(context.Context) error) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- call(ctx) }()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("the call did not return within 10 s")
	}
	return nil
}

// b65fHeldCancel runs call while the server holds the reply, cancels the context after the request arrives and
// returns the call's error (prompt: within 5 s).
func b65fHeldCancel(t *testing.T, server *testutil.MockTelegramServer, method string, call func(context.Context) error) error {
	t.Helper()
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	server.On(api101bPath(method), func(w http.ResponseWriter, _ *http.Request) {
		arrived <- struct{}{}
		<-release
		api96ReplyRaw(w, `{"ok":true,"result":true}`)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- call(ctx) }()
	select {
	case <-arrived:
	case err := <-done:
		t.Fatalf("the call returned before the request arrived: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("the request did not arrive")
	}
	cancel()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the call did not return after the cancel")
	}
	return nil
}

// U10: every path goes through the client's breaker; 4xx, a result decode error and a cancel do not count.
func TestB65F_U10_Breaker(t *testing.T) {
	const chat = int64(777)
	newClient := func(t *testing.T, server *testutil.MockTelegramServer) *sender.Client {
		return testutil.NewBreakerTestClient(t, server.BaseURL(), b65fFast()...)
	}
	reply := func(server *testutil.MockTelegramServer, h func(http.ResponseWriter)) {
		for _, p := range []string{"sendWidget", "sendSharedWidget", "answerWidgetQuery", "editMessageWidgetData", "editSharedWidgetData"} {
			server.On(api101bPath(p), func(w http.ResponseWriter, _ *http.Request) { h(w) })
		}
	}
	classes := []struct {
		name  string
		h     func(http.ResponseWriter)
		trips bool
		sends bool // only for the two sends
	}{
		{"502", func(w http.ResponseWriter) { testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway") }, true, false},
		{"400", func(w http.ResponseWriter) { testutil.ReplyBadRequest(w, "x") }, false, false},
		{"429", func(w http.ResponseWriter) { testutil.ReplyRateLimit(w, 1) }, false, false},
		{"broken envelope", func(w http.ResponseWriter) { api96ReplyRaw(w, `{"ok":true,"result":`) }, true, false},
		{"result not a message", func(w http.ResponseWriter) { api96ReplyRaw(w, `{"ok":true,"result":true}`) }, false, true},
	}
	for _, m := range b65fMethods {
		for _, cl := range classes {
			if cl.sends && !m.send {
				continue
			}
			t.Run(m.name+"/"+cl.name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				reply(server, cl.h)
				client := newClient(t, server)
				for i := 1; i <= 2; i++ {
					_, err := m.call(context.Background(), client, chat)
					require.Error(t, err)
					assert.False(t, errors.Is(err, gobreaker.ErrOpenState), "call %d: %v", i, err)
				}
				_, err := m.call(context.Background(), client, chat)
				if cl.trips {
					assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "call 3: %v", err)
					assert.Equal(t, 2, server.CaptureCount(), "call 3 sends no request")
				} else {
					assert.False(t, errors.Is(err, gobreaker.ErrOpenState), "call 3: %v", err)
					assert.Equal(t, 3, server.CaptureCount(), "call 3 is sent")
				}
			})
		}
		t.Run(m.name+"/cancel", func(t *testing.T) {
			server := testutil.NewMockServer(t)
			client := newClient(t, server)
			for range 2 {
				err := b65fHeldCancel(t, server, m.method, func(ctx context.Context) error { _, err := m.call(ctx, client, chat); return err })
				require.ErrorIs(t, err, context.Canceled)
			}
			b65fOK(server)
			_, err := m.call(context.Background(), client, chat)
			require.NoError(t, err, "a cancel does not count as a failure")
			assert.Equal(t, 3, server.CaptureCount())
		})
	}
	t.Run("shared", func(t *testing.T) {
		server := testutil.NewMockServer(t)
		reply(server, classes[0].h)
		client := newClient(t, server)
		for range 2 {
			_, err := b65fMethods[0].call(context.Background(), client, chat)
			require.Error(t, err)
		}
		for _, m := range b65fMethods[1:] {
			_, err := m.call(context.Background(), client, chat)
			assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "%s: %v", m.name, err)
		}
		assert.Equal(t, 2, server.CaptureCount(), "no request while the breaker is open")
	})
}

// U11: the caller's context is used: cancelled before the call, or while the server holds the reply.
func TestB65F_U11_Context(t *testing.T) {
	for _, m := range b65fMethods {
		t.Run(m.name+"/before", func(t *testing.T) {
			server, client := b65fClient(t)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			got, err := m.call(ctx, client, api100Chat)
			assert.Nil(t, got)
			assert.ErrorIs(t, err, context.Canceled)
			assert.Equal(t, 0, server.CaptureCount())
		})
		t.Run(m.name+"/held", func(t *testing.T) {
			server := testutil.NewMockServer(t)
			client := testutil.NewTestClient(t, server.BaseURL(), b65fFast()...)
			err := b65fHeldCancel(t, server, m.method, func(ctx context.Context) error { _, err := m.call(ctx, client, api100Chat); return err })
			assert.ErrorIs(t, err, context.Canceled)
		})
	}
}

// b65fRefusal is one local validation row: a typed error (field, reason) or a plain one.
type b65fRefusal struct {
	name          string
	call          func(context.Context, *sender.Client) error
	field, reason string // typed *tg.ValidationError
	plain         string // the exact plain error text
}

func b65fCheckRefusals(t *testing.T, rows []b65fRefusal) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			check := func(t *testing.T, err error) {
				t.Helper()
				require.Error(t, err)
				if r.plain != "" {
					assert.EqualError(t, err, r.plain)
					var ve *tg.ValidationError
					assert.False(t, errors.As(err, &ve), "a plain error")
					return
				}
				var ve *tg.ValidationError
				require.True(t, errors.As(err, &ve), "error %v", err)
				assert.Equal(t, r.field, ve.Field)
				assert.Equal(t, r.reason, ve.Message)
				assert.EqualError(t, err, "galigo: validation: "+r.field+" - "+r.reason)
			}
			server, client := b65fClient(t)
			check(t, r.call(context.Background(), client))
			assert.Equal(t, 0, server.CaptureCount(), "no request")
			assert.Equal(t, 0, client.ChatLimiterCount(), "no chat bucket taken")
			_, zero := b65fClient(t, sender.WithRateLimit(0, 0))
			check(t, r.call(context.Background(), zero)) // refused before the limiter
		})
	}
}

type b65fAccept struct {
	name   string
	method string
	call   func(context.Context, *sender.Client) error
}

func b65fCheckAccepts(t *testing.T, rows []b65fAccept) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server, client := b65fClient(t)
			require.NoError(t, r.call(context.Background(), client))
			b65fOne(t, server, r.method)
		})
	}
}

// b65fChatPaths call each method that takes a chat target with a valid other input.
var b65fChatPaths = []struct {
	name, method string
	call         func(context.Context, *sender.Client, tg.ChatID) error
}{
	{"sendWidget", "sendWidget", func(ctx context.Context, c *sender.Client, chat tg.ChatID) error {
		_, err := c.SendWidget(ctx, sender.SendWidgetRequest{ChatID: chat, HTML: "h"})
		return err
	}},
	{"sendSharedWidget", "sendSharedWidget", func(ctx context.Context, c *sender.Client, chat tg.ChatID) error {
		_, err := c.SendSharedWidget(ctx, sender.SendSharedWidgetRequest{ChatID: chat, WidgetID: "w"})
		return err
	}},
	{"editMessageWidgetData", "editMessageWidgetData", func(ctx context.Context, c *sender.Client, chat tg.ChatID) error {
		return c.EditMessageWidgetData(ctx, sender.EditMessageWidgetDataRequest{ChatID: chat, MessageID: 42})
	}},
}

// V0: the chat target uses the repo's validateChatID texts.
func TestB65F_V0_Chat(t *testing.T) {
	bad := []struct {
		name  string
		chat  tg.ChatID
		plain string
	}{
		{"nil", nil, "galigo: chat_id is required"},
		{"int64 0", int64(0), "galigo: chat_id cannot be zero"},
		{"int 0", int(0), "galigo: chat_id cannot be zero"},
		{"empty", "", "galigo: chat_id cannot be empty string"},
		{"typed nil", (*int64)(nil), "galigo: chat_id must be int64, int, or string, got *int64"},
		{"float64", float64(1), "galigo: chat_id must be int64, int, or string, got float64"},
	}
	good := []tg.ChatID{int64(5), int(5), "@chan"}
	for _, p := range b65fChatPaths {
		var refusals []b65fRefusal
		for _, b := range bad {
			refusals = append(refusals, b65fRefusal{name: p.name + "/" + b.name, plain: b.plain,
				call: func(ctx context.Context, c *sender.Client) error { return p.call(ctx, c, b.chat) }})
		}
		b65fCheckRefusals(t, refusals)
		var accepts []b65fAccept
		for _, g := range good {
			accepts = append(accepts, b65fAccept{name: fmt.Sprintf("%s/ok %T", p.name, g), method: p.method,
				call: func(ctx context.Context, c *sender.Client) error { return p.call(ctx, c, g) }})
		}
		b65fCheckAccepts(t, accepts)
	}
}

// V1, V2: the required strings, and the send chat check first.
func TestB65F_V1V2_Required(t *testing.T) {
	b65fCheckRefusals(t, []b65fRefusal{
		{name: "sendWidget html", field: "html", reason: "required", call: func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendWidget(ctx, sender.SendWidgetRequest{ChatID: api100Chat})
			return err
		}},
		{name: "sendSharedWidget widget_id", field: "widget_id", reason: "required", call: func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendSharedWidget(ctx, sender.SendSharedWidgetRequest{ChatID: api100Chat})
			return err
		}},
		{name: "sendWidget chat first", plain: "galigo: chat_id is required", call: func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendWidget(ctx, sender.SendWidgetRequest{})
			return err
		}},
		{name: "sendSharedWidget chat first", plain: "galigo: chat_id is required", call: func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendSharedWidget(ctx, sender.SendSharedWidgetRequest{})
			return err
		}},
		{name: "answerWidgetQuery widget_query_id", field: "widget_query_id", reason: "required", call: func(ctx context.Context, c *sender.Client) error {
			return c.AnswerWidgetQuery(ctx, sender.AnswerWidgetQueryRequest{Data: "x"})
		}},
		{name: "editSharedWidgetData widget_id", field: "widget_id", reason: "required", call: func(ctx context.Context, c *sender.Client) error {
			return c.EditSharedWidgetData(ctx, sender.EditSharedWidgetDataRequest{PublicData: "p", CacheTime: 5})
		}},
	})
	b65fCheckAccepts(t, []b65fAccept{
		{"editSharedWidgetData empty public_data and cache_time -1", "editSharedWidgetData", func(ctx context.Context, c *sender.Client) error {
			return c.EditSharedWidgetData(ctx, sender.EditSharedWidgetDataRequest{WidgetID: "w", CacheTime: -1})
		}},
	})
}

// V3: the editMessageWidgetData target rule, in its order (PLAN AC7).
func TestB65F_V3_EditTarget(t *testing.T) {
	const combined = "cannot be combined with chat_id or message_id"
	edit := func(req sender.EditMessageWidgetDataRequest) func(context.Context, *sender.Client) error {
		return func(ctx context.Context, c *sender.Client) error { return c.EditMessageWidgetData(ctx, req) }
	}
	b65fCheckRefusals(t, []b65fRefusal{
		{name: "inline + chat 5", field: "inline_message_id", reason: combined, call: edit(sender.EditMessageWidgetDataRequest{InlineMessageID: "i", ChatID: int64(5)})},
		{name: "inline + chat empty", field: "inline_message_id", reason: combined, call: edit(sender.EditMessageWidgetDataRequest{InlineMessageID: "i", ChatID: ""})},
		{name: "inline + chat 0", field: "inline_message_id", reason: combined, call: edit(sender.EditMessageWidgetDataRequest{InlineMessageID: "i", ChatID: int64(0)})},
		{name: "inline + chat typed nil", field: "inline_message_id", reason: combined, call: edit(sender.EditMessageWidgetDataRequest{InlineMessageID: "i", ChatID: (*int64)(nil)})},
		{name: "inline + message 7", field: "inline_message_id", reason: combined, call: edit(sender.EditMessageWidgetDataRequest{InlineMessageID: "i", MessageID: 7})},
		{name: "inline + message -1", field: "inline_message_id", reason: combined, call: edit(sender.EditMessageWidgetDataRequest{InlineMessageID: "i", MessageID: -1})},
		{name: "inline + chat 5 + message 7", field: "inline_message_id", reason: combined, call: edit(sender.EditMessageWidgetDataRequest{InlineMessageID: "i", ChatID: int64(5), MessageID: 7})},
		{name: "inline blank + chat 5 + message 7", field: "inline_message_id", reason: combined, call: edit(sender.EditMessageWidgetDataRequest{InlineMessageID: " ", ChatID: int64(5), MessageID: 7})},
		{name: "nothing", field: "inline_message_id", reason: "required if chat_id and message_id are not specified", call: edit(sender.EditMessageWidgetDataRequest{PublicData: b65fPtr("p")})},
		{name: "chat 5 + message 0", plain: "galigo: message_id must be positive, got 0", call: edit(sender.EditMessageWidgetDataRequest{ChatID: int64(5)})},
		{name: "chat 5 + message -1", plain: "galigo: message_id must be positive, got -1", call: edit(sender.EditMessageWidgetDataRequest{ChatID: int64(5), MessageID: -1})},
		{name: "chat nil + message 5", plain: "galigo: chat_id is required", call: edit(sender.EditMessageWidgetDataRequest{MessageID: 5})},
		{name: "chat nil + message -1", plain: "galigo: chat_id is required", call: edit(sender.EditMessageWidgetDataRequest{MessageID: -1})},
		{name: "chat 0 + message 0", plain: "galigo: chat_id cannot be zero", call: edit(sender.EditMessageWidgetDataRequest{ChatID: int64(0)})},
		{name: "chat empty + message 0", plain: "galigo: chat_id cannot be empty string", call: edit(sender.EditMessageWidgetDataRequest{ChatID: ""})},
		{name: "chat 0 + message -1", plain: "galigo: chat_id cannot be zero", call: edit(sender.EditMessageWidgetDataRequest{ChatID: int64(0), MessageID: -1})},
	})
	b65fCheckAccepts(t, []b65fAccept{
		{"inline only", "editMessageWidgetData", edit(sender.EditMessageWidgetDataRequest{InlineMessageID: "i"})},
		{"chat 5 + message 7", "editMessageWidgetData", edit(sender.EditMessageWidgetDataRequest{ChatID: int64(5), MessageID: 7})},
	})
}

// U1 (exact digits): an id above 2^53 reaches the wire digit for digit.
func TestB65F_U1_HugeID(t *testing.T) {
	server, client := b65fClient(t)
	require.NoError(t, client.EditMessageWidgetData(context.Background(), sender.EditMessageWidgetDataRequest{ChatID: api100Chat, MessageID: b65fHugeID()}))
	body := b65fNumbers(t, b65fOne(t, server, "editMessageWidgetData").Body)
	assert.Equal(t, json.Number(strconv.Itoa(b65fHugeID())), body["message_id"])
}
