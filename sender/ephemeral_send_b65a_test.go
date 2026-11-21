package sender_test

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-65 slice 65a: ephemeral_message_parameters (Bot API 10.3) on the 14 send methods, and chat_id optional for an
// ephemeral reply to a callback query (10.4). Rows of E65/TEST-PLAN.md §2.2.

// b65aSpecMethods are the methods whose spec takes ephemeral_message_parameters, in the spec's order
// (core.telegram.org/bots/api fetched 2026-11-18, E65/lookups/spec-65a-methods.txt).
var b65aSpecMethods = []string{
	"sendMessage", "sendPhoto", "sendLivePhoto", "sendAudio", "sendDocument", "sendVideo", "sendAnimation",
	"sendVoice", "sendVideoNote", "sendLocation", "sendVenue", "sendContact", "sendSticker", "sendRichMessage",
}

// b65aChecking are the 4 methods that check chat_id before a request (the other 10 do not, PLAN §4).
var b65aChecking = []string{"sendMessage", "sendPhoto", "sendLivePhoto", "sendRichMessage"}

// b65aWidths are receiver ids that a narrowing copy changes (E-0178): above int32, above uint32, above float64's exact
// integers, and the largest int64.
var b65aWidths = []int64{1<<31 + 1, 1<<32 + 1, 1<<53 + 1, math.MaxInt64}

const b65aCallback = "cq-65a"

// b65aCall is one of the 14 methods: req is its request type (zero value); send sends one request with chat and p, by
// upload (path "upload": the form body) or with no upload ("json").
type b65aCall struct {
	method string
	req    any
	paths  []string
	send   func(ctx context.Context, c *sender.Client, path string, chat tg.ChatID, p *tg.EphemeralMessageParameters) error
}

func b65aCalls() []b65aCall {
	both, jsonOnly := []string{"json", "upload"}, []string{"json"}
	return []b65aCall{
		{"sendMessage", sender.SendMessageRequest{}, jsonOnly, func(ctx context.Context, c *sender.Client, _ string, chat tg.ChatID, p *tg.EphemeralMessageParameters) error {
			return b48c1Err(c.SendMessage(ctx, sender.SendMessageRequest{ChatID: chat, Text: "t", EphemeralMessageParameters: p}))
		}},
		{"sendPhoto", sender.SendPhotoRequest{}, both, func(ctx context.Context, c *sender.Client, up string, chat tg.ChatID, p *tg.EphemeralMessageParameters) error {
			return b48c1Err(c.SendPhoto(ctx, sender.SendPhotoRequest{ChatID: chat, Photo: b48c1File(up, "p.jpg"), EphemeralMessageParameters: p}))
		}},
		{"sendLivePhoto", sender.SendLivePhotoRequest{}, both, func(ctx context.Context, c *sender.Client, up string, chat tg.ChatID, p *tg.EphemeralMessageParameters) error {
			return b48c1Err(c.SendLivePhoto(ctx, sender.SendLivePhotoRequest{ChatID: chat, LivePhoto: b48c1File(up, "l.mp4"), Photo: b48c1File(up, "l.jpg"), EphemeralMessageParameters: p}))
		}},
		{"sendAudio", sender.SendAudioRequest{}, both, func(ctx context.Context, c *sender.Client, up string, chat tg.ChatID, p *tg.EphemeralMessageParameters) error {
			th := b48c1Thumb(up, "t.jpg")
			return b48c1Err(c.SendAudio(ctx, sender.SendAudioRequest{ChatID: chat, Audio: b48c1File(up, "a.mp3"), Thumbnail: &th, EphemeralMessageParameters: p}))
		}},
		{"sendDocument", sender.SendDocumentRequest{}, both, func(ctx context.Context, c *sender.Client, up string, chat tg.ChatID, p *tg.EphemeralMessageParameters) error {
			th := b48c1Thumb(up, "t.jpg")
			return b48c1Err(c.SendDocument(ctx, sender.SendDocumentRequest{ChatID: chat, Document: b48c1File(up, "d.txt"), Thumbnail: &th, EphemeralMessageParameters: p}))
		}},
		{"sendVideo", sender.SendVideoRequest{}, both, func(ctx context.Context, c *sender.Client, up string, chat tg.ChatID, p *tg.EphemeralMessageParameters) error {
			th := b48c1Thumb(up, "t.jpg")
			return b48c1Err(c.SendVideo(ctx, sender.SendVideoRequest{ChatID: chat, Video: b48c1File(up, "v.mp4"), Thumbnail: &th, EphemeralMessageParameters: p}))
		}},
		{"sendAnimation", sender.SendAnimationRequest{}, both, func(ctx context.Context, c *sender.Client, up string, chat tg.ChatID, p *tg.EphemeralMessageParameters) error {
			th := b48c1Thumb(up, "t.jpg")
			return b48c1Err(c.SendAnimation(ctx, sender.SendAnimationRequest{ChatID: chat, Animation: b48c1File(up, "g.gif"), Thumbnail: &th, EphemeralMessageParameters: p}))
		}},
		{"sendVoice", sender.SendVoiceRequest{}, both, func(ctx context.Context, c *sender.Client, up string, chat tg.ChatID, p *tg.EphemeralMessageParameters) error {
			return b48c1Err(c.SendVoice(ctx, sender.SendVoiceRequest{ChatID: chat, Voice: b48c1File(up, "v.ogg"), EphemeralMessageParameters: p}))
		}},
		{"sendVideoNote", sender.SendVideoNoteRequest{}, both, func(ctx context.Context, c *sender.Client, up string, chat tg.ChatID, p *tg.EphemeralMessageParameters) error {
			th := b48c1Thumb(up, "t.jpg")
			return b48c1Err(c.SendVideoNote(ctx, sender.SendVideoNoteRequest{ChatID: chat, VideoNote: b48c1File(up, "n.mp4"), Thumbnail: &th, EphemeralMessageParameters: p}))
		}},
		{"sendLocation", sender.SendLocationRequest{}, jsonOnly, func(ctx context.Context, c *sender.Client, _ string, chat tg.ChatID, p *tg.EphemeralMessageParameters) error {
			return b48c1Err(c.SendLocation(ctx, sender.SendLocationRequest{ChatID: chat, Latitude: 1.5, Longitude: 2.5, EphemeralMessageParameters: p}))
		}},
		{"sendVenue", sender.SendVenueRequest{}, jsonOnly, func(ctx context.Context, c *sender.Client, _ string, chat tg.ChatID, p *tg.EphemeralMessageParameters) error {
			return b48c1Err(c.SendVenue(ctx, sender.SendVenueRequest{ChatID: chat, Latitude: 1.5, Longitude: 2.5, Title: "t", Address: "a", EphemeralMessageParameters: p}))
		}},
		{"sendContact", sender.SendContactRequest{}, jsonOnly, func(ctx context.Context, c *sender.Client, _ string, chat tg.ChatID, p *tg.EphemeralMessageParameters) error {
			return b48c1Err(c.SendContact(ctx, sender.SendContactRequest{ChatID: chat, PhoneNumber: "+1000", FirstName: "f", EphemeralMessageParameters: p}))
		}},
		{"sendSticker", sender.SendStickerRequest{}, both, func(ctx context.Context, c *sender.Client, up string, chat tg.ChatID, p *tg.EphemeralMessageParameters) error {
			return b48c1Err(c.SendSticker(ctx, sender.SendStickerRequest{ChatID: chat, Sticker: b48c1File(up, "s.webp"), EphemeralMessageParameters: p}))
		}},
		{"sendRichMessage", sender.SendRichMessageRequest{}, both, func(ctx context.Context, c *sender.Client, up string, chat tg.ChatID, p *tg.EphemeralMessageParameters) error {
			rich := tg.InputRichMessage{Markdown: "m"}
			if up == "upload" { // an upload in the media list: the form path (B-61)
				rich.Media = []tg.InputRichMessageMedia{{ID: "p", Media: sender.NewInputMediaPhoto(b48c1File("upload", "r.jpg"))}}
			}
			return b48c1Err(c.SendRichMessage(ctx, sender.SendRichMessageRequest{ChatID: chat, RichMessage: rich, EphemeralMessageParameters: p}))
		}},
	}
}

func b65aCallByName(t *testing.T, method string) b65aCall {
	t.Helper()
	for _, c := range b65aCalls() {
		if c.method == method {
			return c
		}
	}
	t.Fatalf("no call %s", method)
	return b65aCall{}
}

// T-list: the call table is the spec's 14 methods, in order.
func TestB65A_CallTableIsTheSpecList(t *testing.T) {
	var got []string
	for _, c := range b65aCalls() {
		got = append(got, c.method)
	}
	assert.Equal(t, b65aSpecMethods, got)
	assert.Len(t, b65aSpecMethods, 14)
}

// T-req: each of the 14 request types has the new field (direct, exact type and tag) and an omitempty chat_id.
func TestB65A_RequestFields(t *testing.T) {
	for _, c := range b65aCalls() {
		t.Run(c.method, func(t *testing.T) {
			rt := reflect.TypeOf(c.req)
			f, ok := rt.FieldByName("EphemeralMessageParameters")
			require.True(t, ok, "%s has no EphemeralMessageParameters", rt.Name())
			assert.False(t, f.Anonymous)
			assert.Len(t, f.Index, 1, "a direct field, not promoted")
			assert.Equal(t, "*tg.EphemeralMessageParameters", f.Type.String())
			assert.Equal(t, "ephemeral_message_parameters,omitempty", f.Tag.Get("json"))
			chat, ok := rt.FieldByName("ChatID")
			require.True(t, ok)
			assert.Equal(t, "chat_id,omitempty", chat.Tag.Get("json"))
		})
	}
}

// b65aBaseChatTypes are the sender types that had a chat_id key at 8d762ce (generated from the source by the census
// walker below), and b65aBaseChatOmit the ones among them whose key was already chat_id,omitempty.
var (
	b65aBaseChatTypes = []string{
		"ApproveSuggestedPostRequest", "BanChatMemberRequest", "BanChatSenderChatRequest", "CopyMessageRequest",
		"CopyMessagesRequest", "CreateChatSubscriptionInviteLinkRequest", "CreateForumTopicRequest",
		"DeclineSuggestedPostRequest", "DeleteAllMessageReactionsRequest", "DeleteChatPhotoRequest",
		"DeleteEphemeralMessageRequest", "DeleteMessageReactionRequest", "DeleteMessageRequest", "DeleteMessagesRequest",
		"EditChatSubscriptionInviteLinkRequest", "EditEphemeralMessageCaptionRequest", "EditEphemeralMessageMediaRequest",
		"EditEphemeralMessageReplyMarkupRequest", "EditEphemeralMessageTextRequest", "EditForumTopicRequest",
		"EditMessageCaptionRequest", "EditMessageChecklistRequest", "EditMessageMediaRequest",
		"EditMessageReplyMarkupRequest", "EditMessageTextRequest", "ForumTopicRequest", "ForwardMessageRequest",
		"ForwardMessagesRequest", "GetChatMemberRequest", "GetChatRequest", "GetGameHighScoresRequest",
		"GetUserChatBoostsRequest", "LeaveChatRequest", "PinChatMessageRequest", "PromoteChatMemberRequest",
		"RemoveChatVerificationRequest", "RestrictChatMemberRequest", "SendAnimationRequest", "SendAudioRequest",
		"SendChatActionRequest", "SendChecklistRequest", "SendContactRequest", "SendDiceRequest", "SendDocumentRequest",
		"SendGameRequest", "SendInvoiceRequest", "SendLivePhotoRequest", "SendLocationRequest", "SendMediaGroupRequest",
		"SendMessageDraftRequest", "SendMessageRequest", "SendPaidMediaRequest", "SendPhotoRequest", "SendPollRequest",
		"SendRichMessageDraftRequest", "SendRichMessageRequest", "SendStickerRequest", "SendVenueRequest",
		"SendVideoNoteRequest", "SendVideoRequest", "SendVoiceRequest", "SetChatAdministratorCustomTitleRequest",
		"SetChatDescriptionRequest", "SetChatMemberTagRequest", "SetChatPermissionsRequest", "SetChatPhotoRequest",
		"SetChatTitleRequest", "SetGameScoreRequest", "SetMessageReactionRequest", "StopPollRequest",
		"UnbanChatMemberRequest", "UnbanChatSenderChatRequest", "UnpinAllChatMessagesRequest", "UnpinChatMessageRequest",
		"VerifyChatRequest", "chatJoinRequestDecision", "getChatAdministratorsRequest",
	}
	b65aBaseChatOmit = []string{
		"EditMessageCaptionRequest", "EditMessageMediaRequest", "EditMessageReplyMarkupRequest", "EditMessageTextRequest",
		"GetGameHighScoresRequest", "SetGameScoreRequest",
	}
)

// b65fChatTags are the chat_id tags of the request types B-65 65f added after 8d762ce (a named addition: the 77-name
// list above stays the historical one).
var b65fChatTags = map[string]string{
	"SendWidgetRequest":            "chat_id",
	"SendSharedWidgetRequest":      "chat_id",
	"EditMessageWidgetDataRequest": "chat_id,omitempty",
}

// T-census: in the sender package source, ephemeral_message_parameters is carried by exactly the 14 request types
// (also through embedding or a type definition), and every type that had a chat_id key at 8d762ce keeps its tag,
// except the 14, which become chat_id,omitempty: an extra omitempty on any other type fails.
func TestB65A_Census(t *testing.T) {
	want := make([]string, 0, 14)
	for _, c := range b65aCalls() {
		want = append(want, reflect.TypeOf(c.req).Name())
	}
	slices.Sort(want)
	require.Len(t, b65aBaseChatTypes, 77)
	carries := map[string]bool{}   // type -> written ephemeral_message_parameters key
	chatTag := map[string]string{} // type -> its chat_id json tag
	from := map[string][]string{}  // type -> the sender types it embeds or is defined as
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
			if _, ok := node.(*ast.FuncDecl); ok {
				return false
			}
			ts, ok := node.(*ast.TypeSpec)
			if !ok {
				return true
			}
			name := ts.Name.Name
			st, ok := b48c1Struct(ts.Type)
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
				j := reflect.StructTag(tag).Get("json")
				switch key, _, _ := strings.Cut(j, ","); key {
				case "ephemeral_message_parameters":
					carries[name] = true
				case "chat_id":
					chatTag[name] = j
				}
			}
			return true
		})
	}
	require.Positive(t, files)
	for changed := true; changed; {
		changed = false
		for name, bases := range from {
			for _, b := range bases {
				if carries[b] && !carries[name] {
					carries[name], changed = true, true
				}
			}
		}
	}
	var got []string
	for name := range carries {
		got = append(got, name)
	}
	slices.Sort(got)
	assert.Equal(t, want, got, "types with ephemeral_message_parameters")
	var tagged []string // every type with a direct chat_id tag: no type gains one (register O3-06)
	for name := range chatTag {
		tagged = append(tagged, name)
	}
	slices.Sort(tagged)
	base := slices.Clone(b65aBaseChatTypes)
	for name, tag := range b65fChatTags {
		base = append(base, name)
		assert.Equal(t, tag, chatTag[name], "chat_id tag of %s (65f)", name)
	}
	slices.Sort(base)
	assert.Equal(t, base, tagged, "types with a chat_id key")
	for _, name := range b65aBaseChatTypes {
		wantTag := "chat_id"
		if slices.Contains(want, name) || slices.Contains(b65aBaseChatOmit, name) {
			wantTag = "chat_id,omitempty"
		}
		assert.Equal(t, wantTag, chatTag[name], "chat_id tag of %s", name)
	}
}

// b65aJSON is the exact JSON text of p (exact digits: JSONEq would compare 2^53+1 as a float64).
func b65aReceiverJSON(id int64) string {
	return `{"receiver_user_id":` + strconv.FormatInt(id, 10) + `}`
}

// T-wire (and T-null): each method sends the parameter as one JSON object on each of its paths; no parameter sends no
// key; an ephemeral reply with a nil chat sends no chat_id key at all (not null) and uses no per-chat limiter.
func TestB65A_Wire(t *testing.T) {
	type row struct {
		name string
		chat tg.ChatID // nil: no chat_id key
		p    *tg.EphemeralMessageParameters
		want string // exact text of the parameter; "" = the key is absent
	}
	rows := []row{{"none", testutil.TestChatID, nil, ""}}
	for _, w := range b65aWidths {
		rows = append(rows, row{"receiver_" + strconv.FormatInt(w, 10), testutil.TestChatID,
			&tg.EphemeralMessageParameters{ReceiverUserID: w}, b65aReceiverJSON(w)})
	}
	rows = append(rows,
		row{"receiver_replace", testutil.TestChatID,
			&tg.EphemeralMessageParameters{ReceiverUserID: 1<<53 + 1, CallbackQueryID: b65aCallback, ReplaceCallbackQueryMessage: true},
			`{"receiver_user_id":9007199254740993,"callback_query_id":"cq-65a","replace_callback_query_message":true}`},
		row{"reply", nil, &tg.EphemeralMessageParameters{CallbackQueryID: b65aCallback}, `{"callback_query_id":"cq-65a"}`},
		row{"reply_replace", nil, &tg.EphemeralMessageParameters{CallbackQueryID: b65aCallback, ReplaceCallbackQueryMessage: true},
			`{"callback_query_id":"cq-65a","replace_callback_query_message":true}`},
		// A chat the caller gives is sent as given, also with callback-only parameters (register O1-06).
		row{"reply_with_chat", testutil.TestChatID, &tg.EphemeralMessageParameters{CallbackQueryID: b65aCallback}, `{"callback_query_id":"cq-65a"}`},
		row{"string_chat", "@b65a_chan", &tg.EphemeralMessageParameters{ReceiverUserID: 7}, b65aReceiverJSON(7)},
		// A plain Go int chat id is a valid chat class too (validateChatID; register O-C1-01).
		row{"int_chat", int(-100123), &tg.EphemeralMessageParameters{ReceiverUserID: 7}, b65aReceiverJSON(7)},
	)
	for _, c := range b65aCalls() {
		for _, up := range c.paths {
			wantType := "application/json"
			if up != "json" {
				wantType = "multipart/form-data"
			}
			for _, r := range rows {
				t.Run(c.method+"/"+up+"/"+r.name, func(t *testing.T) {
					var cl *sender.Client
					mediaType, args, files, raw, err := b48c1WireCall(t, c.method, replyAny(c.method),
						func(ctx context.Context, client *sender.Client) error {
							cl = client
							return c.send(ctx, client, up, r.chat, r.p)
						})
					require.NoError(t, err)
					assert.Equal(t, wantType, mediaType)
					assert.Len(t, files, b48c1Files(c.method, up), "file parts")
					chat, ok := args["chat_id"]
					if r.chat == nil {
						assert.False(t, ok, "chat_id sent: %q", chat)
						if up == "json" {
							assert.NotContains(t, raw, "chat_id", "no chat_id key, not null")
						}
						assert.Equal(t, 0, cl.ChatLimiterCount(), "no per-chat limiter without a chat")
					} else {
						// The exact value: the form text, and on the JSON path the raw JSON (a number stays a number,
						// a string a string; register O1-06).
						wantText, wantRaw := "@b65a_chan", `"@b65a_chan"`
						switch id := r.chat.(type) {
						case int64:
							wantText = strconv.FormatInt(id, 10)
							wantRaw = wantText
						case int:
							wantText = strconv.Itoa(id)
							wantRaw = wantText
						}
						assert.Equal(t, wantText, chat)
						if up == "json" {
							assert.Equal(t, wantRaw, string(raw["chat_id"]))
						}
						assert.Equal(t, 1, cl.ChatLimiterCount())
					}
					got, ok := args["ephemeral_message_parameters"]
					if r.want == "" {
						assert.False(t, ok, "ephemeral_message_parameters sent: %q", got)
						return
					}
					require.True(t, ok, "ephemeral_message_parameters missing; args %v", args)
					assert.Equal(t, r.want, got)
					if up == "json" { // the raw value is an object, not a string holding one
						assert.Equal(t, r.want, string(raw["ephemeral_message_parameters"]))
					}
				})
			}
		}
	}
}

// b65aNoRequest runs send against a mock server and checks it sent nothing.
func b65aNoRequest(t *testing.T, method string, send func(ctx context.Context, c *sender.Client) error) error {
	t.Helper()
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/"+method, replyAny(method))
	err := send(context.Background(), testutil.NewTestClient(t, server.BaseURL()))
	assert.Equal(t, 0, server.CaptureCount(), "requests sent")
	return err
}

func b65aIsParamError(t *testing.T, err error) {
	t.Helper()
	var ve *tg.ValidationError
	require.True(t, errors.As(err, &ve), "want *tg.ValidationError, got %T: %v", err, err)
	assert.Equal(t, "ephemeral_message_parameters", ve.Field)
}

// T-refuse: parameters with neither ReceiverUserID nor CallbackQueryID are refused before any request, by all 14, on
// each of their paths (an upload too, register O1-02).
func TestB65A_Refuse(t *testing.T) {
	for _, c := range b65aCalls() {
		for _, up := range c.paths {
			for _, p := range []struct {
				name string
				v    *tg.EphemeralMessageParameters
			}{{"empty", &tg.EphemeralMessageParameters{}}, {"replace_only", &tg.EphemeralMessageParameters{ReplaceCallbackQueryMessage: true}}} {
				t.Run(c.method+"/"+up+"/"+p.name, func(t *testing.T) {
					err := b65aNoRequest(t, c.method, func(ctx context.Context, cl *sender.Client) error {
						return c.send(ctx, cl, up, testutil.TestChatID, p.v)
					})
					b65aIsParamError(t, err)
				})
			}
		}
	}
}

// T-chat: the 4 checking methods accept a nil chat only for an ephemeral reply to a callback query, and keep every
// other refusal they make today.
func TestB65A_ChatRule(t *testing.T) {
	reply := &tg.EphemeralMessageParameters{CallbackQueryID: b65aCallback}
	replyReplace := &tg.EphemeralMessageParameters{CallbackQueryID: b65aCallback, ReplaceCallbackQueryMessage: true}
	type row struct {
		name    string
		chat    tg.ChatID
		p       *tg.EphemeralMessageParameters
		wantErr string // "" = the parameter error
	}
	rows := []row{
		{"nil_chat_receiver", nil, &tg.EphemeralMessageParameters{ReceiverUserID: 5}, "galigo: chat_id is required"},
		{"nil_chat_both_ids", nil, &tg.EphemeralMessageParameters{ReceiverUserID: 5, CallbackQueryID: b65aCallback}, "galigo: chat_id is required"},
		// A negative receiver is still a receiver: a nil chat is refused (register O2-05).
		{"nil_chat_negative_receiver", nil, &tg.EphemeralMessageParameters{ReceiverUserID: -1, CallbackQueryID: b65aCallback}, "galigo: chat_id is required"},
		{"nil_chat_empty", nil, &tg.EphemeralMessageParameters{}, ""},
		// An invalid parameter is refused before the chat check, also with a chat that check refuses (A-C1-01).
		{"int64_0_empty", int64(0), &tg.EphemeralMessageParameters{}, ""},
		{"nil_chat_no_param", nil, nil, "galigo: chat_id is required"},
	}
	for _, p := range []struct {
		name string
		v    *tg.EphemeralMessageParameters
	}{{"reply", reply}, {"reply_replace", replyReplace}} {
		rows = append(rows,
			row{p.name + "/int64_0", int64(0), p.v, "galigo: chat_id cannot be zero"},
			row{p.name + "/int_0", 0, p.v, "galigo: chat_id cannot be zero"},
			row{p.name + "/empty_string", "", p.v, "galigo: chat_id cannot be empty string"},
			row{p.name + "/float64", 1.5, p.v, "galigo: chat_id must be int64, int, or string, got float64"},
			row{p.name + "/bool", true, p.v, "galigo: chat_id must be int64, int, or string, got bool"},
			row{p.name + "/struct", struct{}{}, p.v, "galigo: chat_id must be int64, int, or string, got struct {}"},
		)
	}
	for _, m := range b65aChecking {
		c := b65aCallByName(t, m)
		for _, r := range rows {
			t.Run(m+"/"+r.name, func(t *testing.T) {
				err := b65aNoRequest(t, m, func(ctx context.Context, cl *sender.Client) error {
					return c.send(ctx, cl, "json", r.chat, r.p)
				})
				if r.wantErr == "" {
					b65aIsParamError(t, err)
					return
				}
				require.Error(t, err)
				assert.EqualError(t, err, r.wantErr)
			})
		}
	}
}

// T-nochk: the 10 methods that do not check chat_id still send a request for a nil chat with no parameter; the key
// is now absent instead of null.
func TestB65A_NoChatCheckOnTheTen(t *testing.T) {
	for _, c := range b65aCalls() {
		if slices.Contains(b65aChecking, c.method) {
			continue
		}
		t.Run(c.method, func(t *testing.T) {
			_, args, _, raw, err := b48c1WireCall(t, c.method, replyAny(c.method),
				func(ctx context.Context, cl *sender.Client) error { return c.send(ctx, cl, "json", nil, nil) })
			require.NoError(t, err)
			assert.NotContains(t, args, "chat_id")
			assert.NotContains(t, raw, "chat_id")
		})
	}
}

// b65aLimServer answers the three limiter classes' methods.
func b65aLimServer(t *testing.T) *testutil.MockTelegramServer {
	t.Helper()
	s := testutil.NewMockServer(t)
	for _, m := range []string{"sendMessage", "sendVoice", "sendRichMessage"} {
		s.On(b51Path(m), func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyMessage(w, 1) })
	}
	return s
}

// b65aReplies are the limiter classes of an ephemeral reply with a nil chat: withRetry (sendMessage), a methods.go
// send (sendVoice) and callJSON (sendRichMessage).
var b65aReplies = []b51Class{
	{"reply sendMessage", func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendMessage(ctx, sender.SendMessageRequest{Text: "t", EphemeralMessageParameters: &tg.EphemeralMessageParameters{CallbackQueryID: b65aCallback}})
		return err
	}},
	{"reply sendVoice", func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendVoice(ctx, sender.SendVoiceRequest{Voice: sender.FromFileID("v"), EphemeralMessageParameters: &tg.EphemeralMessageParameters{CallbackQueryID: b65aCallback}})
		return err
	}},
	{"reply sendRichMessage", func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendRichMessage(ctx, sender.SendRichMessageRequest{RichMessage: tg.InputRichMessage{Markdown: "m"},
			EphemeralMessageParameters: &tg.EphemeralMessageParameters{CallbackQueryID: b65aCallback}})
		return err
	}},
}

// T-lim (B-76, E-0121): a reply with a nil chat creates no per-chat limiter and takes the one global token that chat
// calls take, in both orders; the same call with a real chat creates one per-chat limiter.
func TestB65A_Limiter(t *testing.T) {
	chat := b51Classes[4] // C5: sendMessage to a real chat
	for _, cl := range b65aReplies {
		t.Run(cl.name+"/no per-chat limiter", func(t *testing.T) {
			s := b65aLimServer(t)
			c := b51Client(t, s, 1000, 100)
			require.NoError(t, b51Call(t, b51Ctx(t), c, cl))
			assert.Equal(t, 0, c.ChatLimiterCount())
			require.NoError(t, b51Call(t, b51Ctx(t), c, chat))
			assert.Equal(t, 1, c.ChatLimiterCount())
		})
		for _, pair := range [][2]b51Class{{chat, cl}, {cl, chat}} {
			t.Run(pair[0].name+" then "+pair[1].name, func(t *testing.T) {
				s := b65aLimServer(t)
				c := b51Client(t, s, 0.001, 1)
				require.NoError(t, b51Call(t, b51Ctx(t), c, pair[0]))
				err := b51Call(t, b51Ctx(t), c, pair[1])
				require.Error(t, err)
				assert.EqualError(t, err, b51Refused)
				assert.Equal(t, 1, s.CaptureCount())
			})
		}
	}
}

// T-retry: SendMessage and SendPhoto (with an upload) still retry an ephemeral reply; every attempt has no chat_id
// (register O3-02).
func TestB65A_ReplyRetried(t *testing.T) {
	p := &tg.EphemeralMessageParameters{CallbackQueryID: b65aCallback}
	for _, c := range []struct {
		method string
		send   func(c *sender.Client) error
	}{
		{"sendMessage", func(c *sender.Client) error {
			return b48c1Err(c.SendMessage(context.Background(), sender.SendMessageRequest{Text: "t", EphemeralMessageParameters: p}))
		}},
		{"sendPhoto", func(c *sender.Client) error {
			return b48c1Err(c.SendPhoto(context.Background(), sender.SendPhotoRequest{Photo: b48c1File("upload", "p.jpg"), EphemeralMessageParameters: p}))
		}},
	} {
		t.Run(c.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			calls := 0
			server.On(b51Path(c.method), func(w http.ResponseWriter, _ *http.Request) {
				calls++
				if calls == 1 {
					testutil.ReplyServerError(w, 500, "Internal Server Error")
					return
				}
				testutil.ReplyMessage(w, 1)
			})
			client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRetries(1), sender.WithSleeper(&testutil.FakeSleeper{}))
			require.NoError(t, c.send(client))
			caps := server.Captures()
			require.Len(t, caps, 2)
			for i, cp := range caps {
				_, args, files := wireArgs(t, capturedRequest(&cp))
				assert.NotContains(t, args, "chat_id", "attempt %d", i+1)
				assert.Equal(t, `{"callback_query_id":"cq-65a"}`, args["ephemeral_message_parameters"], "attempt %d", i+1)
				if c.method == "sendPhoto" {
					require.Contains(t, files, "photo", "attempt %d", i+1)
					assert.Equal(t, "p.jpg", files["photo"].FileName, "attempt %d", i+1)
					assert.Equal(t, "data-p.jpg", string(files["photo"].Data), "attempt %d", i+1)
				}
			}
		})
	}
}
