package sender_test

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// b48c1SpecMethods are the methods whose spec takes suggested_post_parameters (Bot API 9.2), in the spec's order
// (E27/lookups/spec-suggested-post.txt, core.telegram.org/bots/api fetched 2026-11-12).
var b48c1SpecMethods = []string{
	"sendMessage", "forwardMessage", "copyMessage", "sendPhoto", "sendLivePhoto", "sendAudio", "sendDocument",
	"sendVideo", "sendAnimation", "sendVoice", "sendVideoNote", "sendPaidMedia", "sendLocation", "sendVenue",
	"sendContact", "sendDice", "sendSticker", "sendRichMessage", "sendInvoice",
}

// b48c1Full is the full value the wire rows send, and b48c1FullJSON its JSON (amount 10^13: a TON price in
// nanograms that does not fit a 32-bit int).
// b48c1ChannelChat is a channel-style (negative) chat id: the client must not refuse the parameter for a chat kind
// (AC5: the server checks the chat) [C1-02].
const b48c1ChannelChat = int64(-1001234567890)

var b48c1Full = &tg.SuggestedPostParameters{Price: &tg.SuggestedPostPrice{Currency: "TON", Amount: 10000000000000}, SendDate: 1900000000}

const b48c1FullJSON = `{"price":{"currency":"TON","amount":10000000000000},"send_date":1900000000}`

// b48c1Call is one of the 19 methods: req is its request type (zero value, for the shape rows); send sends one
// request with the two new fields set to spp and topic, by upload (form path) or by file id / no file (JSON path).
type b48c1Call struct {
	method string
	req    any
	paths  []string // the paths the method has: "json" (no upload), "upload" (form), "fileid" (form without an upload: paid
	// media), "upload-ptr" (form, the rich media upload as a *InputMedia), "thumb-upload" (form: the file by id, only its
	// thumbnail uploaded) [C1-03]
	send func(ctx context.Context, c *sender.Client, path string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error
}

func b48c1File(path, name string) sender.InputFile {
	if strings.HasPrefix(path, "upload") {
		return sender.FromBytes([]byte("data-"+name), name)
	}
	return sender.FromFileID("ID-" + name)
}

// b48c1Thumb is a thumbnail: uploaded on the upload paths and on "thumb-upload", else by file id.
func b48c1Thumb(path, name string) sender.InputFile {
	if path == "thumb-upload" {
		path = "upload"
	}
	return b48c1File(path, name)
}

func b48c1Err[T any](_ T, err error) error { return err }

func b48c1Calls() []b48c1Call {
	both, jsonOnly := []string{"json", "upload"}, []string{"json"}
	thumb := []string{"json", "upload", "thumb-upload"} // the five types with a thumbnail [C1-03]
	return []b48c1Call{
		{"sendMessage", sender.SendMessageRequest{}, jsonOnly, func(ctx context.Context, c *sender.Client, _ string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			return b48c1Err(c.SendMessage(ctx, sender.SendMessageRequest{ChatID: chat, Text: "t", DirectMessagesTopicID: topic, SuggestedPostParameters: spp}))
		}},
		{"forwardMessage", sender.ForwardMessageRequest{}, jsonOnly, func(ctx context.Context, c *sender.Client, _ string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			return b48c1Err(c.ForwardMessage(ctx, sender.ForwardMessageRequest{ChatID: chat, FromChatID: int64(5), MessageID: 6, DirectMessagesTopicID: topic, SuggestedPostParameters: spp}))
		}},
		{"copyMessage", sender.CopyMessageRequest{}, jsonOnly, func(ctx context.Context, c *sender.Client, _ string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			return b48c1Err(c.CopyMessage(ctx, sender.CopyMessageRequest{ChatID: chat, FromChatID: int64(5), MessageID: 6, DirectMessagesTopicID: topic, SuggestedPostParameters: spp}))
		}},
		{"sendPhoto", sender.SendPhotoRequest{}, both, func(ctx context.Context, c *sender.Client, up string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			return b48c1Err(c.SendPhoto(ctx, sender.SendPhotoRequest{ChatID: chat, Photo: b48c1File(up, "p.jpg"), DirectMessagesTopicID: topic, SuggestedPostParameters: spp}))
		}},
		{"sendLivePhoto", sender.SendLivePhotoRequest{}, both, func(ctx context.Context, c *sender.Client, up string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			return b48c1Err(c.SendLivePhoto(ctx, sender.SendLivePhotoRequest{ChatID: chat, LivePhoto: b48c1File(up, "l.mp4"), Photo: b48c1File(up, "l.jpg"), DirectMessagesTopicID: int(topic), SuggestedPostParameters: spp}))
		}},
		{"sendAudio", sender.SendAudioRequest{}, thumb, func(ctx context.Context, c *sender.Client, up string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			th := b48c1Thumb(up, "t.jpg") // a second file: the thumbnail
			return b48c1Err(c.SendAudio(ctx, sender.SendAudioRequest{ChatID: chat, Audio: b48c1File(up, "a.mp3"), Thumbnail: &th, DirectMessagesTopicID: topic, SuggestedPostParameters: spp}))
		}},
		{"sendDocument", sender.SendDocumentRequest{}, thumb, func(ctx context.Context, c *sender.Client, up string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			th := b48c1Thumb(up, "t.jpg")
			return b48c1Err(c.SendDocument(ctx, sender.SendDocumentRequest{ChatID: chat, Document: b48c1File(up, "d.txt"), Thumbnail: &th, DirectMessagesTopicID: topic, SuggestedPostParameters: spp}))
		}},
		{"sendVideo", sender.SendVideoRequest{}, thumb, func(ctx context.Context, c *sender.Client, up string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			th := b48c1Thumb(up, "t.jpg")
			return b48c1Err(c.SendVideo(ctx, sender.SendVideoRequest{ChatID: chat, Video: b48c1File(up, "v.mp4"), Thumbnail: &th, DirectMessagesTopicID: topic, SuggestedPostParameters: spp}))
		}},
		{"sendAnimation", sender.SendAnimationRequest{}, thumb, func(ctx context.Context, c *sender.Client, up string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			th := b48c1Thumb(up, "t.jpg")
			return b48c1Err(c.SendAnimation(ctx, sender.SendAnimationRequest{ChatID: chat, Animation: b48c1File(up, "g.gif"), Thumbnail: &th, DirectMessagesTopicID: topic, SuggestedPostParameters: spp}))
		}},
		{"sendVoice", sender.SendVoiceRequest{}, both, func(ctx context.Context, c *sender.Client, up string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			return b48c1Err(c.SendVoice(ctx, sender.SendVoiceRequest{ChatID: chat, Voice: b48c1File(up, "v.ogg"), DirectMessagesTopicID: topic, SuggestedPostParameters: spp}))
		}},
		{"sendVideoNote", sender.SendVideoNoteRequest{}, thumb, func(ctx context.Context, c *sender.Client, up string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			th := b48c1Thumb(up, "t.jpg")
			return b48c1Err(c.SendVideoNote(ctx, sender.SendVideoNoteRequest{ChatID: chat, VideoNote: b48c1File(up, "n.mp4"), Thumbnail: &th, DirectMessagesTopicID: topic, SuggestedPostParameters: spp}))
		}},
		{"sendPaidMedia", sender.SendPaidMediaRequest{}, []string{"fileid", "upload"}, func(ctx context.Context, c *sender.Client, up string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			return b48c1Err(c.SendPaidMedia(ctx, sender.SendPaidMediaRequest{ChatID: chat, StarCount: 1,
				Media: []sender.InputPaidMedia{sender.NewInputPaidMediaPhoto(b48c1File(up, "m.jpg"))}, DirectMessagesTopicID: topic, SuggestedPostParameters: spp}))
		}},
		{"sendLocation", sender.SendLocationRequest{}, jsonOnly, func(ctx context.Context, c *sender.Client, _ string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			return b48c1Err(c.SendLocation(ctx, sender.SendLocationRequest{ChatID: chat, Latitude: 1.5, Longitude: 2.5, DirectMessagesTopicID: topic, SuggestedPostParameters: spp}))
		}},
		{"sendVenue", sender.SendVenueRequest{}, jsonOnly, func(ctx context.Context, c *sender.Client, _ string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			return b48c1Err(c.SendVenue(ctx, sender.SendVenueRequest{ChatID: chat, Latitude: 1.5, Longitude: 2.5, Title: "t", Address: "a", DirectMessagesTopicID: topic, SuggestedPostParameters: spp}))
		}},
		{"sendContact", sender.SendContactRequest{}, jsonOnly, func(ctx context.Context, c *sender.Client, _ string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			return b48c1Err(c.SendContact(ctx, sender.SendContactRequest{ChatID: chat, PhoneNumber: "+1000", FirstName: "f", DirectMessagesTopicID: topic, SuggestedPostParameters: spp}))
		}},
		{"sendDice", sender.SendDiceRequest{}, jsonOnly, func(ctx context.Context, c *sender.Client, _ string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			// SendDice takes options; SendDiceOption is func(*SendDiceRequest), so a caller sets the fields in its own option.
			return b48c1Err(c.SendDice(ctx, chat, func(r *sender.SendDiceRequest) { r.DirectMessagesTopicID, r.SuggestedPostParameters = topic, spp }))
		}},
		{"sendSticker", sender.SendStickerRequest{}, both, func(ctx context.Context, c *sender.Client, up string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			return b48c1Err(c.SendSticker(ctx, sender.SendStickerRequest{ChatID: chat, Sticker: b48c1File(up, "s.webp"), DirectMessagesTopicID: topic, SuggestedPostParameters: spp}))
		}},
		{"sendRichMessage", sender.SendRichMessageRequest{}, []string{"json", "upload", "upload-ptr"}, func(ctx context.Context, c *sender.Client, up string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			rich := tg.InputRichMessage{Markdown: "m"}
			switch m := sender.NewInputMediaPhoto(b48c1File("upload", "r.jpg")); up { // an upload in the media list: the form path (B-61)
			case "upload":
				rich.Media = []tg.InputRichMessageMedia{{ID: "p", Media: m}}
			case "upload-ptr":
				rich.Media = []tg.InputRichMessageMedia{{ID: "p", Media: &m}}
			}
			return b48c1Err(c.SendRichMessage(ctx, sender.SendRichMessageRequest{ChatID: chat, RichMessage: rich, DirectMessagesTopicID: int(topic), SuggestedPostParameters: spp}))
		}},
		{"sendInvoice", sender.SendInvoiceRequest{}, jsonOnly, func(ctx context.Context, c *sender.Client, _ string, chat int64, spp *tg.SuggestedPostParameters, topic int64) error {
			return b48c1Err(c.SendInvoice(ctx, sender.SendInvoiceRequest{ChatID: chat, Title: "t", Description: "d", Payload: "p", Currency: "XTR",
				Prices: []tg.LabeledPrice{{Label: "l", Amount: 1}}, DirectMessagesTopicID: topic, SuggestedPostParameters: spp}))
		}},
	}
}

// b48c1TopicType is the Go type of DirectMessagesTopicID: int64, except the two fields released as int in v0.10.2
// (B-84 asks the owner whether to change them).
func b48c1TopicType(method string) string {
	if method == "sendLivePhoto" || method == "sendRichMessage" {
		return "int"
	}
	return "int64"
}

// b48c1BigTopic is the largest test topic id the field holds on every platform: 2^40 for int64 (the spec allows up
// to 52 significant bits), 2^31-1 for an int.
func b48c1BigTopic(method string) int64 {
	if b48c1TopicType(method) == "int" {
		return 1<<31 - 1
	}
	return 1 << 40
}

// T-list: the call table is the spec's method list, in order.
func TestB48C1_CallTableIsTheSpecList(t *testing.T) {
	var got []string
	for _, c := range b48c1Calls() {
		got = append(got, c.method)
	}
	assert.Equal(t, b48c1SpecMethods, got)
	assert.Len(t, b48c1SpecMethods, 19)
}

// T-type: the Go shape of the two new types (E-0126).
func TestB48C1_TypeShapes(t *testing.T) {
	type field struct{ name, typ, tag string }
	shape := func(v any) []field {
		rt := reflect.TypeOf(v)
		var out []field
		for i := range rt.NumField() {
			f := rt.Field(i)
			assert.False(t, f.Anonymous, "%s.%s is embedded", rt.Name(), f.Name)
			out = append(out, field{f.Name, f.Type.String(), f.Tag.Get("json")})
		}
		return out
	}
	assert.Equal(t, []field{{"Currency", "string", "currency"}, {"Amount", "int64", "amount"}}, shape(tg.SuggestedPostPrice{}))
	assert.Equal(t, []field{{"Price", "*tg.SuggestedPostPrice", "price,omitempty"}, {"SendDate", "int64", "send_date,omitempty"}},
		shape(tg.SuggestedPostParameters{}))
}

// T-type-json: the JSON of the two types, both ways.
func TestB48C1_TypeJSON(t *testing.T) {
	for _, r := range []struct {
		name string
		v    tg.SuggestedPostParameters
		want string
	}{
		{"empty", tg.SuggestedPostParameters{}, `{}`},
		{"zero price keeps its required keys", tg.SuggestedPostParameters{Price: &tg.SuggestedPostPrice{}}, `{"price":{"currency":"","amount":0}}`},
		{"stars", tg.SuggestedPostParameters{Price: &tg.SuggestedPostPrice{Currency: "XTR", Amount: 5}}, `{"price":{"currency":"XTR","amount":5}}`},
		{"send date only", tg.SuggestedPostParameters{SendDate: 1900000000}, `{"send_date":1900000000}`},
		{"full", *b48c1Full, b48c1FullJSON},
	} {
		t.Run(r.name, func(t *testing.T) {
			b, err := json.Marshal(r.v)
			require.NoError(t, err)
			assert.Equal(t, r.want, string(b)) // exact text: key order and absent keys
			var back tg.SuggestedPostParameters
			require.NoError(t, json.Unmarshal([]byte(r.want), &back))
			assert.Equal(t, r.v, back)
		})
	}
}

// T-req: each of the 19 request types has both fields with the exact Go type and tag (E-0132, E-0126).
func TestB48C1_RequestFields(t *testing.T) {
	for _, c := range b48c1Calls() {
		t.Run(c.method, func(t *testing.T) {
			rt := reflect.TypeOf(c.req)
			for _, want := range []struct{ name, typ, tag string }{
				{"SuggestedPostParameters", "*tg.SuggestedPostParameters", "suggested_post_parameters,omitempty"},
				{"DirectMessagesTopicID", b48c1TopicType(c.method), "direct_messages_topic_id,omitempty"},
			} {
				f, ok := rt.FieldByName(want.name)
				require.True(t, ok, "%s has no %s", rt.Name(), want.name)
				assert.False(t, f.Anonymous)
				assert.Len(t, f.Index, 1, "a direct field, not promoted")
				assert.Equal(t, want.typ, f.Type.String())
				assert.Equal(t, want.tag, f.Tag.Get("json"))
			}
		})
	}
}

// T-census: in the sender package source, exactly the 19 request types carry each of the two keys (E-0105: the
// whole class, not the instances a reviewer named). A type also carries a key that it gets through an embedded field
// or from the type it is defined as (type X Y), so a key promoted into another request type fails too.
func TestB48C1_Census(t *testing.T) {
	want := make([]string, 0, 19)
	for _, c := range b48c1Calls() {
		want = append(want, reflect.TypeOf(c.req).Name())
	}
	slices.Sort(want)
	direct := map[string]map[string]bool{} // type -> keys written on it
	from := map[string][]string{}          // type -> the sender types it embeds or is defined as
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	fset := token.NewFileSet()
	files := 0
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		files++
		ast.Inspect(f, func(node ast.Node) bool {
			ts, ok := node.(*ast.TypeSpec)
			if !ok {
				return true
			}
			name := ts.Name.Name
			if direct[name] == nil {
				direct[name] = map[string]bool{}
			}
			st, ok := b48c1Struct(ts.Type) // also type X (struct{…}) [O-T2-05] and type X = *struct{…} [O-T3-05]
			if !ok {
				if base := b48c1LocalType(ts.Type); base != "" {
					from[name] = append(from[name], base)
				}
				return true
			}
			for _, fl := range st.Fields.List {
				if len(fl.Names) == 0 {
					if base := b48c1LocalType(fl.Type); base != "" {
						from[name] = append(from[name], base)
					}
				}
				if fl.Tag == nil {
					continue
				}
				tag, err := strconv.Unquote(fl.Tag.Value)
				require.NoError(t, err)
				key, _, _ := strings.Cut(reflect.StructTag(tag).Get("json"), ",")
				if key == "suggested_post_parameters" || key == "direct_messages_topic_id" {
					direct[name][key] = true
				}
			}
			return true
		})
	}
	require.Positive(t, files)
	for changed := true; changed; { // carry keys through embedding and type definitions to a fixed point
		changed = false
		for name, bases := range from {
			for _, b := range bases {
				for key := range direct[b] {
					if !direct[name][key] {
						direct[name][key], changed = true, true
					}
				}
			}
		}
	}
	for _, key := range []string{"suggested_post_parameters", "direct_messages_topic_id"} {
		var got []string
		for name, keys := range direct {
			if keys[key] {
				got = append(got, name)
			}
		}
		slices.Sort(got)
		assert.Equal(t, want, got, "types with %s", key)
	}
}

// b48c1Struct is the struct type that expr spells, through parentheses and pointers, or false.
func b48c1Struct(expr ast.Expr) (*ast.StructType, bool) {
	for {
		switch e := ast.Unparen(expr).(type) {
		case *ast.StarExpr:
			expr = e.X
		case *ast.StructType:
			return e, true
		default:
			return nil, false
		}
	}
}

// b48c1LocalType is the name of a type of this package that expr names (T, *T, T[…], also in parentheses), or "".
func b48c1LocalType(expr ast.Expr) string {
	for {
		switch e := ast.Unparen(expr).(type) {
		case *ast.StarExpr:
			expr = e.X
			continue
		case *ast.IndexExpr: // a generic instantiation T[A] [O-T3-05]
			expr = e.X
			continue
		case *ast.IndexListExpr: // T[A, B]
			expr = e.X
			continue
		}
		break
	}
	if id, ok := ast.Unparen(expr).(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// b48c1Bad is a parameter value the server must refuse (unknown currency, negative amount, a date in the past): the
// client sends it as it is (AC5).
var b48c1Bad = &tg.SuggestedPostParameters{Price: &tg.SuggestedPostPrice{Currency: "ABC", Amount: -1}, SendDate: 1}

const b48c1BadJSON = `{"price":{"currency":"ABC","amount":-1},"send_date":1}`

// b48c1PriceOnly (1 Star: below the Stars minimum, with no date) and b48c1DateOnly (a date with no price) send one
// inner field each: the client sends each as it is, and the other inner field stays absent [O-T2-01, O-T2-02].
var (
	b48c1PriceOnly = &tg.SuggestedPostParameters{Price: &tg.SuggestedPostPrice{Currency: "XTR", Amount: 1}}
	b48c1DateOnly  = &tg.SuggestedPostParameters{SendDate: 1900000000}
)

const (
	b48c1PriceOnlyJSON = `{"price":{"currency":"XTR","amount":1}}`
	b48c1DateOnlyJSON  = `{"send_date":1900000000}`
)

// The other bounds the server checks (spec: XTR 5-100000, TON 10^7-10^13, a date 300 s to 30 days ahead), each sent
// unchanged: TON below its minimum and above its maximum, XTR above its maximum, a zero amount (an explicit price, not
// an absent one), and a date one minute ahead [A-T3-03 = O-T3-01, O-T3-02, O-T3-03, C1-01].
var (
	b48c1TONLow   = &tg.SuggestedPostParameters{Price: &tg.SuggestedPostPrice{Currency: "TON", Amount: 1}}
	b48c1TONHigh  = &tg.SuggestedPostParameters{Price: &tg.SuggestedPostPrice{Currency: "TON", Amount: 10000000000001}} // [C1-01]
	b48c1XTRHigh  = &tg.SuggestedPostParameters{Price: &tg.SuggestedPostPrice{Currency: "XTR", Amount: 100001}}
	b48c1ZeroCost = &tg.SuggestedPostParameters{Price: &tg.SuggestedPostPrice{Currency: "XTR", Amount: 0}}
)

const (
	b48c1TONLowJSON   = `{"price":{"currency":"TON","amount":1}}`
	b48c1TONHighJSON  = `{"price":{"currency":"TON","amount":10000000000001}}`
	b48c1XTRHighJSON  = `{"price":{"currency":"XTR","amount":100001}}`
	b48c1ZeroCostJSON = `{"price":{"currency":"XTR","amount":0}}`
)

// T-wire: each method sends the two new keys on each of its paths, each on its own: the parameter as one JSON object
// (the JSON body's raw value, or the form field's text), the topic id as a number (the JSON body's raw value is the
// unquoted digits); nil and 0 send neither key; a non-nil empty value is {}; a value the server must refuse goes out
// unchanged, also a price alone, a date alone, each other bound the server checks, and a date one minute ahead.
func TestB48C1_Wire(t *testing.T) {
	soon := time.Now().Unix() + 60 // inside the server's 300 s minimum: the client must still send it
	soonJSON := `{"send_date":` + strconv.FormatInt(soon, 10) + `}`
	for _, c := range b48c1Calls() {
		for _, up := range c.paths {
			wantType := "application/json"
			if up != "json" {
				wantType = "multipart/form-data"
			}
			for _, r := range []struct {
				name  string
				spp   *tg.SuggestedPostParameters
				topic int64  // 0 = the key is absent
				want  string // the parameter's JSON; "" = the key is absent
				chat  int64  // 0 = testutil.TestChatID
			}{
				{"full", b48c1Full, b48c1BigTopic(c.method), b48c1FullJSON, 0},
				{"empty", &tg.SuggestedPostParameters{}, 7, `{}`, 0},
				{"none", nil, 0, "", 0},
				{"spp_only", b48c1Bad, 0, b48c1BadJSON, 0},
				{"topic_only", nil, b48c1BigTopic(c.method), "", 0},
				{"price_only", b48c1PriceOnly, 0, b48c1PriceOnlyJSON, 0},
				{"date_only", b48c1DateOnly, 0, b48c1DateOnlyJSON, 0},
				{"ton_low", b48c1TONLow, 0, b48c1TONLowJSON, 0},
				{"xtr_high", b48c1XTRHigh, 0, b48c1XTRHighJSON, 0},
				{"zero_price", b48c1ZeroCost, 0, b48c1ZeroCostJSON, 0},
				{"near_date", &tg.SuggestedPostParameters{SendDate: soon}, 0, soonJSON, 0},
				{"ton_high", b48c1TONHigh, 0, b48c1TONHighJSON, 0},                                    // above the TON maximum [C1-01]
				{"channel_chat", b48c1Full, b48c1BigTopic(c.method), b48c1FullJSON, b48c1ChannelChat}, // a channel chat id [C1-02]
			} {
				t.Run(c.method+"/"+up+"/"+r.name, func(t *testing.T) {
					chat := r.chat
					if chat == 0 {
						chat = testutil.TestChatID
					}
					mediaType, args, files, raw, err := b48c1WireCall(t, c.method, replyAny(c.method),
						func(ctx context.Context, cl *sender.Client) error { return c.send(ctx, cl, up, chat, r.spp, r.topic) })
					require.NoError(t, err)
					assert.Equal(t, strconv.FormatInt(chat, 10), args["chat_id"], "the chat id unchanged")
					assert.Equal(t, wantType, mediaType)
					assert.Len(t, files, b48c1Files(c.method, up), "file parts")
					got, ok := args["suggested_post_parameters"]
					if r.want == "" {
						assert.False(t, ok, "suggested_post_parameters sent: %q", got)
					} else if assert.True(t, ok, "suggested_post_parameters missing; args %v", args) {
						assert.JSONEq(t, r.want, got)
						if up == "json" { // the raw value is an object, not a string holding one
							assert.JSONEq(t, r.want, string(raw["suggested_post_parameters"]))
						}
					}
					topic, tok := args["direct_messages_topic_id"]
					if r.topic == 0 {
						assert.False(t, tok, "direct_messages_topic_id sent: %q", topic)
					} else if assert.True(t, tok, "direct_messages_topic_id missing; args %v", args) {
						assert.Equal(t, strconv.FormatInt(r.topic, 10), topic)
						if up == "json" { // the raw value is a number, not a quoted one
							assert.Equal(t, strconv.FormatInt(r.topic, 10), string(raw["direct_messages_topic_id"]))
						}
					}
				})
			}
		}
	}
}

// b48c1WireCall is wireCall that also returns the raw top-level values of a JSON body (nil for a form body).
func b48c1WireCall(t *testing.T, method string, reply http.HandlerFunc, call func(ctx context.Context, c *sender.Client) error) (string, map[string]string, map[string]wireFile, map[string]json.RawMessage, error) {
	t.Helper()
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/"+method, reply)
	callErr := call(context.Background(), testutil.NewTestClient(t, server.BaseURL()))
	require.Equal(t, 1, server.CaptureCount(), "requests sent (call error: %v)", callErr)
	c := server.LastCapture()
	require.Equal(t, "/bot"+testutil.TestToken+"/"+method, c.Path)
	require.Empty(t, c.Query, "no argument in the URL query")
	mediaType, args, files := wireArgs(t, capturedRequest(c))
	var raw map[string]json.RawMessage
	if mediaType == "application/json" {
		require.NoError(t, json.Unmarshal(c.Body, &raw))
	}
	return mediaType, args, files, raw, callErr
}

// b48c1Files is the number of file parts a call sends on a path.
func b48c1Files(method, path string) int {
	if path == "thumb-upload" {
		return 1 // the thumbnail only [C1-03]
	}
	if !strings.HasPrefix(path, "upload") {
		return 0
	}
	switch method {
	case "sendLivePhoto", "sendAudio", "sendDocument", "sendVideo", "sendAnimation", "sendVideoNote":
		return 2 // the file and its thumbnail, or the video and the photo
	}
	return 1
}

// replyAny answers a method with what its client decodes: a MessageId for copyMessage, a Message otherwise.
func replyAny(method string) http.HandlerFunc {
	if method == "copyMessage" {
		return func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyOK(w, tg.MessageID{MessageID: 42}) }
	}
	return replyMessage
}
