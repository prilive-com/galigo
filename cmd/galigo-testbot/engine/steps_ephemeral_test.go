package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-65 slice 65a: the ephemeral message steps, their cleanup and the message budget (E65/TEST-PLAN.md §2.4).

// b65aWidths are user ids that a narrowing copy changes (E-0178).
var b65aWidths = []int64{1<<31 + 1, 1<<32 + 1, 1<<53 + 1, math.MaxInt64}

const (
	b65aAdmin   = int64(5551)         // the configured admin (fake)
	b65aOther   = int64(4242)         // another receiver the server could name
	b65aChat2   = -(int64(1)<<40 + 1) // a chat the reply names, not the runtime chat; wider than 32 bits (register O2-04)
	b65aRefused = "Bad Request: query is too old and response timeout expired or query ID is invalid"
)

// b65aSend is one send the fake saw.
type b65aSend struct {
	method string
	chatID int64
	opts   SendOptions
	photo  MediaInput
}

// b65aResp is a scripted answer to a send.
type b65aResp struct {
	msg *tg.Message
	err error
}

// b65aFake is the B-30 fake (deletes of messages, sets and tags, in one ordered log) plus the ephemeral calls:
// SendMessage and SendPhoto answer from sends (a queue; empty = an ephemeral message 9 for the requested receiver),
// SendEphemeralReply answers reply, DeleteEphemeralMessage logs "eph:<chat>/<receiver>/<id>" and takes its error from
// the same script as the other deletes.
type b65aFake struct {
	*b30Fake
	sent    []b65aSend
	sends   []b65aResp
	replies []string // callback query ids of SendEphemeralReply
	reply   b65aResp
	nextEph int
}

func newB65aFake() *b65aFake { return &b65aFake{b30Fake: newB30Fake(), nextEph: 9} }

func (f *b65aFake) answer(method string, chatID int64, photo MediaInput, opts []SendOption) (*tg.Message, error) {
	var o SendOptions
	for _, opt := range opts {
		opt(&o)
	}
	f.sent = append(f.sent, b65aSend{method, chatID, o, photo})
	if len(f.sends) > 0 {
		r := f.sends[0]
		f.sends = f.sends[1:]
		return r.msg, r.err
	}
	id := f.nextEph
	f.nextEph++
	var recv int64
	if o.EphemeralMessageParameters != nil {
		recv = o.EphemeralMessageParameters.ReceiverUserID
	}
	return &tg.Message{EphemeralMessageID: id, Chat: &tg.Chat{ID: chatID}, ReceiverUser: &tg.User{ID: recv}}, nil
}

func (f *b65aFake) SendMessage(_ context.Context, chatID int64, _ string, opts ...SendOption) (*tg.Message, error) {
	return f.answer("sendMessage", chatID, MediaInput{}, opts)
}

func (f *b65aFake) SendPhoto(_ context.Context, chatID int64, photo MediaInput, opts ...SendOption) (*tg.Message, error) {
	return f.answer("sendPhoto", chatID, photo, opts)
}

func (f *b65aFake) SendEphemeralReply(_ context.Context, callbackQueryID, _ string) (*tg.Message, error) {
	f.replies = append(f.replies, callbackQueryID)
	return f.reply.msg, f.reply.err
}

func (f *b65aFake) DeleteEphemeralMessage(ctx context.Context, req sender.DeleteEphemeralMessageRequest) error {
	return f.call(ctx, fmt.Sprintf("eph:%v/%d/%d", req.ChatID, req.ReceiverUserID, req.EphemeralMessageID))
}

func ephKey(chat, recv int64, id int) string { return fmt.Sprintf("eph:%d/%d/%d", chat, recv, id) }

// b65aRuntime is a runtime for the fake: the B-30 chat, the given admin, and the bot an administrator.
func b65aRuntime(f *b65aFake, admin int64) *Runtime {
	rt := NewRuntime(f, b30Chat, admin)
	rt.ChatCtx = &ChatContext{BotIsAdmin: true}
	return rt
}

func b65aRunner(f *b65aFake, cfg RunnerConfig) (*Runner, *Runtime) {
	if cfg.MaxMessages == 0 {
		cfg.MaxMessages = 100
	}
	rt := b65aRuntime(f, b65aAdmin)
	return NewRunner(rt, cfg, slog.New(slog.DiscardHandler)), rt
}

func b65a429(retryAfter time.Duration) error {
	return tg.NewAPIErrorWithRetry("x", 429, "Too Many Requests: retry later", retryAfter)
}

func b65aCallbackRefusal() error { return tg.NewAPIError("sendMessage", 400, b65aRefused) }

func ephTargets(rt *Runtime) []ephemeralMessage {
	return append([]ephemeralMessage(nil), rt.ephemeralMessages...)
}

// noReceiver fails if a receiver id (decimal) appears in any text (A2-04).
func noReceiver(t *testing.T, text string, ids ...int64) {
	t.Helper()
	for _, id := range ids {
		assert.NotContains(t, text, strconv.FormatInt(id, 10), "a receiver id in %q", text)
	}
}

// T-opt
func TestB65A_WithEphemeralMessageParameters(t *testing.T) {
	p := &tg.EphemeralMessageParameters{ReceiverUserID: 7}
	var o SendOptions
	WithEphemeralMessageParameters(p)(&o)
	assert.Same(t, p, o.EphemeralMessageParameters)
	WithEphemeralMessageParameters(nil)(&o)
	assert.Nil(t, o.EphemeralMessageParameters)
}

// b65aArgs returns the arguments of a captured request: raw JSON values, or form field texts.
func b65aArgs(t *testing.T, c *testutil.Capture) map[string]string {
	t.Helper()
	mt, params, err := mime.ParseMediaType(c.ContentType)
	require.NoError(t, err)
	out := map[string]string{}
	if mt == "application/json" {
		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(c.Body, &raw))
		for k, v := range raw {
			out[k] = string(v)
		}
		return out
	}
	require.Equal(t, "multipart/form-data", mt)
	r := multipart.NewReader(bytes.NewReader(c.Body), params["boundary"])
	for {
		part, err := r.NextPart()
		if err != nil {
			break
		}
		if part.FileName() != "" {
			out["file:"+part.FormName()] = part.FileName()
			continue
		}
		b, err := io.ReadAll(part)
		require.NoError(t, err)
		out[part.FormName()] = string(b)
	}
	return out
}

func b65aAdapter(t *testing.T, method string, reply http.HandlerFunc) (*SenderAdapter, *testutil.MockTelegramServer) {
	t.Helper()
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/"+method, reply)
	return NewSenderAdapter(testutil.NewTestClient(t, server.BaseURL())), server
}

func replyMsg(w http.ResponseWriter, _ *http.Request) { testutil.ReplyMessage(w, 1) }

// T-ad: the adapter passes the parameter into sendMessage (JSON) and sendPhoto (upload, form) unchanged.
func TestB65A_AdapterPassThrough(t *testing.T) {
	for _, method := range []string{"sendMessage", "sendPhoto"} {
		send := func(a *SenderAdapter, opts ...SendOption) (*tg.Message, error) {
			if method == "sendMessage" {
				return a.SendMessage(context.Background(), b30Chat, "x", opts...)
			}
			return a.SendPhoto(context.Background(), b30Chat, MediaFromBytes([]byte("img"), "p.jpg", "photo"), opts...)
		}
		for _, w := range b65aWidths {
			t.Run(method+"/"+strconv.FormatInt(w, 10), func(t *testing.T) {
				// The server answers an ephemeral message; the adapter returns it unchanged (register O3-04).
				a, s := b65aAdapter(t, method, func(rw http.ResponseWriter, _ *http.Request) {
					testutil.ReplyOK(rw, map[string]any{"message_id": 0, "ephemeral_message_id": 9, "date": 1234567890,
						"chat": map[string]any{"id": b30Chat, "type": "group"}, "receiver_user": map[string]any{"id": w, "is_bot": false, "first_name": "r"}})
				})
				msg, err := send(a, WithEphemeralMessageParameters(&tg.EphemeralMessageParameters{
					ReceiverUserID: w, CallbackQueryID: "cq", ReplaceCallbackQueryMessage: true}))
				require.NoError(t, err)
				require.NotNil(t, msg)
				assert.Equal(t, 0, msg.MessageID)
				assert.Equal(t, 9, msg.EphemeralMessageID)
				require.NotNil(t, msg.ReceiverUser)
				assert.Equal(t, w, msg.ReceiverUser.ID)
				require.Equal(t, 1, s.CaptureCount())
				args := b65aArgs(t, s.LastCapture())
				assert.Equal(t, `{"receiver_user_id":`+strconv.FormatInt(w, 10)+`,"callback_query_id":"cq","replace_callback_query_message":true}`,
					args["ephemeral_message_parameters"])
				assert.Equal(t, strconv.FormatInt(b30Chat, 10), args["chat_id"])
			})
		}
		t.Run(method+"/none", func(t *testing.T) {
			a, s := b65aAdapter(t, method, replyMsg)
			_, err := send(a)
			require.NoError(t, err)
			require.Equal(t, 1, s.CaptureCount())
			assert.NotContains(t, b65aArgs(t, s.LastCapture()), "ephemeral_message_parameters")
		})
	}
}

// T-ad-reply: SendEphemeralReply sends one sendMessage with no chat_id and only callback_query_id, and returns the
// server's message or its error (register O1-01).
func TestB65A_AdapterEphemeralReply(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		a, s := b65aAdapter(t, "sendMessage", replyMsg)
		msg, err := a.SendEphemeralReply(context.Background(), "cq-65a", "x")
		require.NoError(t, err)
		require.NotNil(t, msg)
		assert.Equal(t, 1, msg.MessageID)
		require.Equal(t, 1, s.CaptureCount())
		assert.Equal(t, map[string]string{"text": `"x"`, "ephemeral_message_parameters": `{"callback_query_id":"cq-65a"}`},
			b65aArgs(t, s.LastCapture()))
	})
	t.Run("refused", func(t *testing.T) {
		a, s := b65aAdapter(t, "sendMessage", func(w http.ResponseWriter, _ *http.Request) {
			testutil.ReplyError(w, 400, b65aRefused, nil)
		})
		msg, err := a.SendEphemeralReply(context.Background(), "cq-65a", "x")
		assert.Nil(t, msg)
		var apiErr *tg.APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, 400, apiErr.Code)
		assert.ErrorIs(t, err, tg.ErrCallbackExpired)
		assert.Equal(t, 1, s.CaptureCount(), "a 400 is not retried")
	})
}

// T-ad-del: DeleteEphemeralMessage sends the three ids as given, and returns the server's error (register O1-01).
func TestB65A_AdapterDeleteEphemeral(t *testing.T) {
	t.Run("429", func(t *testing.T) {
		a, s := b65aAdapter(t, "deleteEphemeralMessage", func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyRateLimit(w, 1) })
		err := a.DeleteEphemeralMessage(context.Background(),
			sender.DeleteEphemeralMessageRequest{ChatID: b30Chat, ReceiverUserID: b65aAdmin, EphemeralMessageID: 9})
		var apiErr *tg.APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, 429, apiErr.Code)
		assert.Equal(t, 1, s.CaptureCount())
	})
	// The real adapter under CleanupStep: a refused delete keeps the target and is not counted.
	t.Run("cleanup_429", func(t *testing.T) {
		a, s := b65aAdapter(t, "deleteEphemeralMessage", func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyRateLimit(w, 1) })
		rt := NewRuntime(a, b30Chat, b65aAdmin)
		rt.recordEphemeral(b30Chat, b65aAdmin, 9)
		res, err := (&CleanupStep{}).Execute(context.Background(), rt)
		require.Error(t, err)
		var apiErr *tg.APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, 429, apiErr.Code)
		assert.Equal(t, []ephemeralMessage{{b30Chat, b65aAdmin, 9}}, ephTargets(rt))
		require.NotNil(t, res)
		ev, ok := res.Evidence.(map[string]any)
		require.True(t, ok, "evidence %T", res.Evidence)
		assert.NotContains(t, ev, "ephemeral_deleted")
		assert.Equal(t, 1, s.CaptureCount())
	})
	for _, w := range b65aWidths {
		t.Run(strconv.FormatInt(w, 10), func(t *testing.T) {
			a, s := b65aAdapter(t, "deleteEphemeralMessage", func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyBool(w, true) })
			require.NoError(t, a.DeleteEphemeralMessage(context.Background(),
				sender.DeleteEphemeralMessageRequest{ChatID: b30Chat, ReceiverUserID: w, EphemeralMessageID: 9}))
			require.Equal(t, 1, s.CaptureCount())
			assert.Equal(t, map[string]string{"chat_id": strconv.FormatInt(b30Chat, 10), "receiver_user_id": strconv.FormatInt(w, 10),
				"ephemeral_message_id": "9"}, b65aArgs(t, s.LastCapture()))
		})
	}
}

// T-st: SendEphemeralStep, per method.
func TestB65A_SendEphemeralStep(t *testing.T) {
	ephMsg := func(chat *tg.Chat, recv *tg.User) *tg.Message {
		return &tg.Message{EphemeralMessageID: 9, Chat: chat, ReceiverUser: recv}
	}
	type row struct {
		name      string
		resp      *b65aResp // nil: the fake's default answer
		admin     int64
		notAdmin  bool
		wantErr   bool
		wantSkip  bool
		wantSend  bool
		wantEph   []ephemeralMessage
		wantTrack []CreatedMessage
		wantEphID []int
		wantMsgID []int
	}
	var rows []row
	for _, w := range b65aWidths {
		rows = append(rows, row{name: "ok_" + strconv.FormatInt(w, 10), admin: w, wantSend: true,
			wantEph: []ephemeralMessage{{b30Chat, w, 9}}, wantEphID: []int{9}})
	}
	rows = append(rows,
		row{name: "public", resp: &b65aResp{msg: &tg.Message{MessageID: 77, Chat: &tg.Chat{ID: b30Chat}}}, wantErr: true, wantSend: true,
			wantTrack: []CreatedMessage{{ChatID: b30Chat, MessageID: 77}}, wantMsgID: []int{77}},
		row{name: "public_other_chat", resp: &b65aResp{msg: &tg.Message{MessageID: 77, Chat: &tg.Chat{ID: b65aChat2}}}, wantErr: true, wantSend: true,
			wantTrack: []CreatedMessage{{ChatID: b65aChat2, MessageID: 77}}, wantMsgID: []int{77}},
		row{name: "no_eph_id", resp: &b65aResp{msg: &tg.Message{Chat: &tg.Chat{ID: b30Chat}}}, wantErr: true, wantSend: true},
		row{name: "wrong_receiver", resp: &b65aResp{msg: ephMsg(&tg.Chat{ID: b30Chat}, &tg.User{ID: b65aOther})}, wantErr: true, wantSend: true,
			wantEph: []ephemeralMessage{{b30Chat, b65aOther, 9}}, wantEphID: []int{9}},
		row{name: "nil_receiver", resp: &b65aResp{msg: ephMsg(&tg.Chat{ID: b30Chat}, nil)}, wantErr: true, wantSend: true,
			wantEph: []ephemeralMessage{{b30Chat, b65aAdmin, 9}}, wantEphID: []int{9}},
		row{name: "zero_receiver", resp: &b65aResp{msg: ephMsg(&tg.Chat{ID: b30Chat}, &tg.User{})}, wantErr: true, wantSend: true,
			wantEph: []ephemeralMessage{{b30Chat, b65aAdmin, 9}}, wantEphID: []int{9}},
		row{name: "other_chat", resp: &b65aResp{msg: ephMsg(&tg.Chat{ID: b65aChat2}, &tg.User{ID: b65aAdmin})}, wantSend: true,
			wantEph: []ephemeralMessage{{b65aChat2, b65aAdmin, 9}}, wantEphID: []int{9}},
		row{name: "zero_chat", resp: &b65aResp{msg: ephMsg(&tg.Chat{}, &tg.User{ID: b65aAdmin})}, wantSend: true,
			wantEph: []ephemeralMessage{{b30Chat, b65aAdmin, 9}}, wantEphID: []int{9}},
		row{name: "no_chat", resp: &b65aResp{msg: ephMsg(nil, &tg.User{ID: b65aAdmin})}, wantSend: true,
			wantEph: []ephemeralMessage{{b30Chat, b65aAdmin, 9}}, wantEphID: []int{9}},
		row{name: "send_fails", resp: &b65aResp{err: b65a429(time.Second)}, wantErr: true, wantSend: true},
		row{name: "nil_msg", resp: &b65aResp{}, wantErr: true, wantSend: true},
		row{name: "not_admin", notAdmin: true, wantErr: true, wantSkip: true},
		row{name: "no_admin_id", admin: -1, wantErr: true},
	)
	for _, method := range []string{"sendMessage", "sendPhoto"} {
		for _, r := range rows {
			t.Run(method+"/"+r.name, func(t *testing.T) {
				f := newB65aFake()
				if r.resp != nil {
					f.sends = []b65aResp{*r.resp}
				}
				admin := r.admin
				switch admin {
				case 0:
					admin = b65aAdmin
				case -1:
					admin = 0
				}
				rt := b65aRuntime(f, admin)
				if r.notAdmin {
					rt.ChatCtx = &ChatContext{BotIsAdmin: false}
				}
				res, err := (&SendEphemeralStep{Method: method}).Execute(context.Background(), rt)
				require.NotNil(t, res)
				if r.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				assert.Equal(t, r.wantSkip, IsSkip(err), "skip")
				if r.name == "send_fails" {
					var apiErr *tg.APIError
					require.True(t, errors.As(err, &apiErr))
					assert.Equal(t, 429, apiErr.Code, "the 429 stays visible, so the runner repeats the step")
				}
				if !r.wantSend {
					assert.Empty(t, f.sent)
				} else {
					require.Len(t, f.sent, 1)
					s := f.sent[0]
					assert.Equal(t, method, s.method)
					assert.Equal(t, b30Chat, s.chatID)
					assert.Equal(t, &tg.EphemeralMessageParameters{ReceiverUserID: admin}, s.opts.EphemeralMessageParameters)
					if method == "sendPhoto" {
						require.NotNil(t, s.photo.Reader)
						assert.Equal(t, fixtures.PhotoBytes(), s.photo.Reader())
						assert.Equal(t, "galigo-ephemeral.jpg", s.photo.FileName)
						assert.Equal(t, "photo", s.photo.Type)
					}
				}
				assert.Empty(t, f.log, "no delete inside the step")
				assert.Equal(t, r.wantEph, ephTargets(rt))
				if len(r.wantTrack) == 0 {
					assert.Empty(t, rt.CreatedMessages)
				} else {
					assert.Equal(t, r.wantTrack, rt.CreatedMessages)
				}
				assert.Equal(t, r.wantEphID, res.EphemeralMessageIDs)
				assert.Equal(t, r.wantMsgID, res.MessageIDs)
				if strings.HasPrefix(r.name, "ok_") || r.name == "other_chat" {
					assert.Equal(t, map[string]any{"message_id": 0, "ephemeral_message_id": 9, "receiver_is_admin": true}, res.Evidence)
				}
				if r.name == "wrong_receiver" {
					assert.Equal(t, false, res.Evidence.(map[string]any)["receiver_is_admin"])
				}
				if err != nil {
					noReceiver(t, err.Error(), admin, b65aOther)
				}
			})
		}
	}
	t.Run("unknown_method", func(t *testing.T) {
		f := newB65aFake()
		_, err := (&SendEphemeralStep{Method: "sendVoice"}).Execute(context.Background(), b65aRuntime(f, b65aAdmin))
		require.Error(t, err)
		assert.Empty(t, f.sent)
	})
}

// T-st-reply: EphemeralReplyStep.
func TestB65A_EphemeralReplyStep(t *testing.T) {
	type row struct {
		name      string
		resp      b65aResp
		wantErr   bool
		wantTrack []CreatedMessage
		wantEph   []ephemeralMessage
		wantMsgID []int
		wantEphID []int
		noDelete  bool // the error says it cannot be deleted
	}
	pub := func(chat *tg.Chat) *tg.Message { return &tg.Message{MessageID: 5, Chat: chat} }
	eph := func(chat *tg.Chat, recv *tg.User) *tg.Message {
		return &tg.Message{EphemeralMessageID: 8, Chat: chat, ReceiverUser: recv}
	}
	other403 := tg.NewAPIError("sendMessage", 403, b65aRefused)
	require.ErrorIs(t, other403, tg.ErrCallbackExpired, "fixture: the 403 row must carry the sentinel")
	rows := []row{
		{name: "refused", resp: b65aResp{err: b65aCallbackRefusal()}},
		{name: "refused_wrapped", resp: b65aResp{err: fmt.Errorf("adapter: %w", b65aCallbackRefusal())}},
		{name: "other_400", resp: b65aResp{err: tg.NewAPIError("sendMessage", 400, "Bad Request: chat_id is empty")}, wantErr: true},
		// other_code carries the sentinel (checked below), so only the code check refuses it (register O1-04).
		{name: "other_code", resp: b65aResp{err: other403}, wantErr: true},
		// 400_no_sentinel has the code and the text but no sentinel: only the sentinel check refuses it.
		{name: "400_no_sentinel", resp: b65aResp{err: &tg.APIError{Code: 400, Description: b65aRefused, Method: "sendMessage"}}, wantErr: true},
		{name: "not_api", resp: b65aResp{err: errors.New(b65aRefused)}, wantErr: true},
		{name: "taken_nil", resp: b65aResp{}, wantErr: true},
		{name: "taken", resp: b65aResp{msg: pub(&tg.Chat{ID: b65aChat2})}, wantErr: true,
			wantTrack: []CreatedMessage{{ChatID: b65aChat2, MessageID: 5}}, wantMsgID: []int{5}},
		// taken_no_ids: a reply with a chat and a receiver but no id of either kind (register O2-03).
		{name: "taken_no_ids", resp: b65aResp{msg: &tg.Message{Chat: &tg.Chat{ID: b65aChat2}, ReceiverUser: &tg.User{ID: b65aOther}}}, wantErr: true},
		// taken_both_ids: a reply with both ids is handled as the public message, so it is deleted (register A3-02).
		{name: "taken_both_ids", resp: b65aResp{msg: &tg.Message{MessageID: 5, EphemeralMessageID: 8, Chat: &tg.Chat{ID: b65aChat2},
			ReceiverUser: &tg.User{ID: b65aOther}}}, wantErr: true,
			wantTrack: []CreatedMessage{{ChatID: b65aChat2, MessageID: 5}}, wantMsgID: []int{5}},
		{name: "taken_no_chat", resp: b65aResp{msg: pub(nil)}, wantErr: true, wantMsgID: []int{5}, noDelete: true},
		{name: "taken_zero_chat", resp: b65aResp{msg: pub(&tg.Chat{})}, wantErr: true, wantMsgID: []int{5}, noDelete: true},
		{name: "taken_ephemeral", resp: b65aResp{msg: eph(&tg.Chat{ID: b65aChat2}, &tg.User{ID: b65aOther})}, wantErr: true,
			wantEph: []ephemeralMessage{{b65aChat2, b65aOther, 8}}, wantEphID: []int{8}},
		{name: "taken_ephemeral_no_chat", resp: b65aResp{msg: eph(nil, &tg.User{ID: b65aOther})}, wantErr: true, wantEphID: []int{8}, noDelete: true},
		{name: "taken_ephemeral_zero_chat", resp: b65aResp{msg: eph(&tg.Chat{}, &tg.User{ID: b65aOther})}, wantErr: true, wantEphID: []int{8}, noDelete: true},
		{name: "taken_ephemeral_no_receiver", resp: b65aResp{msg: eph(&tg.Chat{ID: b65aChat2}, nil)}, wantErr: true, wantEphID: []int{8}, noDelete: true},
		{name: "taken_ephemeral_zero_receiver", resp: b65aResp{msg: eph(&tg.Chat{ID: b65aChat2}, &tg.User{})}, wantErr: true, wantEphID: []int{8}, noDelete: true},
	}
	for _, w := range b65aWidths { // the receiver copied from the reply into the recorded target (E-0178, register A1-08)
		rows = append(rows, row{name: "taken_ephemeral_" + strconv.FormatInt(w, 10),
			resp: b65aResp{msg: eph(&tg.Chat{ID: b65aChat2}, &tg.User{ID: w})}, wantErr: true,
			wantEph: []ephemeralMessage{{b65aChat2, w, 8}}, wantEphID: []int{8}})
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			f := newB65aFake()
			f.reply = r.resp
			rt := b65aRuntime(f, b65aAdmin)
			res, err := (&EphemeralReplyStep{CallbackQueryID: "cq-x"}).Execute(context.Background(), rt)
			require.NotNil(t, res)
			assert.Equal(t, []string{"cq-x"}, f.replies)
			if !r.wantErr {
				require.NoError(t, err)
				assert.Equal(t, map[string]any{"error_code": 400, "description": b65aRefused}, res.Evidence)
				return
			}
			require.Error(t, err)
			assert.False(t, IsSkip(err))
			if len(r.wantTrack) == 0 {
				assert.Empty(t, rt.CreatedMessages)
			} else {
				assert.Equal(t, r.wantTrack, rt.CreatedMessages)
			}
			assert.Equal(t, r.wantEph, ephTargets(rt))
			assert.Equal(t, r.wantMsgID, res.MessageIDs)
			assert.Equal(t, r.wantEphID, res.EphemeralMessageIDs)
			if r.noDelete {
				assert.Contains(t, err.Error(), "cannot be deleted")
			}
			noReceiver(t, err.Error(), b65aOther)
		})
	}
	// Through the runner: an incomplete target is counted once and never deleted (O3-03).
	for _, name := range []string{"taken_no_chat", "taken_ephemeral_no_receiver"} {
		t.Run("runner/"+name, func(t *testing.T) {
			f := newB65aFake()
			for _, r := range rows {
				if r.name == name {
					f.reply = r.resp
				}
			}
			r, _ := b65aRunner(f, RunnerConfig{})
			res := r.Run(context.Background(), b30Scenario(0, &EphemeralReplyStep{CallbackQueryID: "cq-x"}, &CleanupStep{}))
			assert.False(t, res.Success)
			assert.Equal(t, 1, r.messageCount)
			assert.Empty(t, f.log, "no delete call")
		})
	}
}

// b65aRecord records ephemeral targets (and tracks messages), reports them, and returns err.
type b65aRecord struct {
	eph  []ephemeralMessage
	msgs []int
	err  error
}

func (s *b65aRecord) Name() string { return "record" }
func (s *b65aRecord) Execute(_ context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: "record"}
	for _, e := range s.eph {
		rt.recordEphemeral(e.chatID, e.receiverUserID, e.messageID)
		res.EphemeralMessageIDs = append(res.EphemeralMessageIDs, e.messageID)
	}
	for _, id := range s.msgs {
		rt.TrackMessage(rt.ChatID, id)
		res.MessageIDs = append(res.MessageIDs, id)
	}
	return res, s.err
}

// T-clean: deleteTracked's ephemeral loop.
func TestB65A_DeleteTrackedEphemeral(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		f := newB65aFake()
		rt := b65aRuntime(f, b65aAdmin)
		var want []string
		for i, w := range b65aWidths {
			rt.recordEphemeral(b65aChat2, w, 10+i)
			want = append(want, ephKey(b65aChat2, w, 10+i))
		}
		ev, failed := deleteTracked(context.Background(), rt, false)
		assert.Empty(t, failed)
		assert.Equal(t, want, f.keys())
		assert.Equal(t, map[string]any{"deleted_count": 0, "ephemeral_deleted": 4}, ev)
		assert.Empty(t, rt.ephemeralMessages)
		for _, c := range f.log { // each call is bounded (register A3-03, O3-03)
			require.True(t, c.hasDeadline, "%s: no deadline", c.key)
			assert.LessOrEqual(t, c.deadline.Sub(c.at), 15*time.Second, c.key)
		}
	})
	for _, c := range []struct {
		name     string
		err      error
		dropPerm bool
		kept     bool
	}{
		{"transient_429", b65a429(time.Second), false, true},
		{"transient_500", tg.NewAPIError("x", 500, "Internal Server Error"), true, true},
		{"permanent_kept", tg.NewAPIError("x", 400, "Bad Request: message not found"), false, true},
		{"permanent_dropped", tg.NewAPIError("x", 400, "Bad Request: message not found"), true, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newB65aFake()
			rt := b65aRuntime(f, b65aAdmin)
			rt.recordEphemeral(b30Chat, b65aOther, 12)
			f.always[ephKey(b30Chat, b65aOther, 12)] = c.err
			ev, failed := deleteTracked(context.Background(), rt, c.dropPerm)
			require.Len(t, failed, 1)
			assert.Equal(t, fmt.Sprintf("ephemeral message %d/12: %v", b30Chat, c.err), failed[0].Error())
			assert.ErrorIs(t, failed[0], c.err)
			assert.Equal(t, []string{ephKey(b30Chat, b65aOther, 12)}, f.keys(), "one delete, the full target (register O2-06)")
			assert.Equal(t, []string{fmt.Sprintf("ephemeral message %d/12: %v", b30Chat, c.err)}, ev["failed"])
			noReceiver(t, failed[0].Error(), b65aOther)
			assert.NotContains(t, ev, "ephemeral_deleted")
			if c.kept {
				assert.Equal(t, []ephemeralMessage{{b30Chat, b65aOther, 12}}, ephTargets(rt))
			} else {
				assert.Empty(t, rt.ephemeralMessages)
			}
		})
	}
}

// T-mixed: CleanupStep over all four kinds, with the first ephemeral delete answered 429 (A2-02, O2-03, E-0176).
func TestB65A_CleanupMixedKinds(t *testing.T) {
	f := newB65aFake()
	f.script[ephKey(b30Chat, b65aAdmin, 20)] = []error{b65a429(time.Second)}
	r, rt := b65aRunner(f, RunnerConfig{RetryOn429: true, Max429Retries: 1})
	setup := &b65aRecord{msgs: []int{501, 502}, eph: []ephemeralMessage{{b30Chat, b65aAdmin, 20}, {b65aChat2, b65aOther, 21}}}
	res := r.Run(context.Background(), b30Scenario(0, setup, &b30Set{name: "s1"}, &b65aTag{user: 600}, &CleanupStep{}))
	require.True(t, res.Success, res.Error)
	assert.Equal(t, []string{
		"msg:-1001/501", "msg:-1001/502", "set:s1", "tag:-1001/600/", ephKey(b30Chat, b65aAdmin, 20), ephKey(b65aChat2, b65aOther, 21),
		ephKey(b30Chat, b65aAdmin, 20), // the repeat: only the failed item
	}, f.keys())
	assert.Equal(t, map[string]any{"deleted_count": 2, "sticker_sets_deleted": 1, "tags_cleared": 1, "ephemeral_deleted": 2},
		evMap(t, lastStep(res)))
	assert.Empty(t, rt.ephemeralMessages)

	t.Run("stop", func(t *testing.T) {
		f := newB65aFake()
		err429, err400 := b65a429(time.Second), tg.NewAPIError("x", 400, "Bad Request: message not found")
		f.always[ephKey(b30Chat, b65aAdmin, 30)] = err429
		f.always[ephKey(b30Chat, b65aOther, 31)] = err400
		r, rt := b65aRunner(f, RunnerConfig{})
		res := r.Run(context.Background(), b30Scenario(0,
			&b65aRecord{msgs: []int{501}, eph: []ephemeralMessage{{b30Chat, b65aAdmin, 30}, {b30Chat, b65aOther, 31}}, err: errB30}))
		require.False(t, res.Success)
		s := lastStep(res)
		require.Equal(t, "cleanup after stop", s.StepName)
		assert.Equal(t, []string{"msg:-1001/501", ephKey(b30Chat, b65aAdmin, 30), ephKey(b30Chat, b65aOther, 31)}, f.keys())
		assert.Equal(t, []ephemeralMessage{{b30Chat, b65aAdmin, 30}}, ephTargets(rt), "transient kept, permanent dropped")
		assert.Empty(t, rt.CreatedMessages, "done items gone")
		assert.Equal(t, []string{fmt.Sprintf("ephemeral message %d/30: %v", b30Chat, err429),
			fmt.Sprintf("ephemeral message %d/31: %v", b30Chat, err400)}, evMap(t, s)["failed"], "in order (register O2-06)")
		assert.Contains(t, s.Error, fmt.Sprintf("ephemeral message %d/30", b30Chat))
		assert.Contains(t, s.Error, fmt.Sprintf("ephemeral message %d/31", b30Chat))
		noReceiver(t, s.Error, b65aAdmin, b65aOther)
		noReceiver(t, res.Error, b65aAdmin, b65aOther)
		b, err := json.Marshal(s.Evidence)
		require.NoError(t, err)
		noReceiver(t, string(b), b65aAdmin, b65aOther)
	})
}

type b65aTag struct{ user int64 }

func (s *b65aTag) Name() string { return "tag" }
func (s *b65aTag) Execute(_ context.Context, rt *Runtime) (*StepResult, error) {
	rt.recordTag(rt.ChatID, s.user, true)
	return &StepResult{StepName: "tag"}, nil
}

// b65aAttempts reports attempts[i] on its i-th run (the last one again after that).
type b65aAttempts struct {
	attempts []b65aRecord
	runs     int
	cancel   func() // called as the first attempt returns, if set
}

func (s *b65aAttempts) Name() string { return "attempts" }
func (s *b65aAttempts) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	a := s.attempts[min(s.runs, len(s.attempts)-1)]
	s.runs++
	if s.runs == 1 && s.cancel != nil {
		defer s.cancel()
	}
	return a.Execute(ctx, rt)
}

// T-budget: the message count includes the sends of failed steps and of 429 attempts; a 429 step is not repeated
// once the budget is used (O2-01, A3-02, O3-01).
func TestB65A_RunnerBudgetCountsFailedSteps(t *testing.T) {
	t.Run("failed_steps", func(t *testing.T) {
		f := newB65aFake()
		r, _ := b65aRunner(f, RunnerConfig{MaxMessages: 3})
		res := r.Run(context.Background(), b30Scenario(0, &b65aRecord{msgs: []int{1}, err: errB30}))
		require.False(t, res.Success)
		assert.Equal(t, 1, r.messageCount)
		res = r.Run(context.Background(), b30Scenario(0, &b65aRecord{eph: []ephemeralMessage{{b30Chat, b65aAdmin, 2}}, err: errB30}))
		require.False(t, res.Success)
		assert.Equal(t, 2, r.messageCount)
		res = r.Run(context.Background(), b30Scenario(0, &b65aRecord{msgs: []int{3}, eph: []ephemeralMessage{{b30Chat, b65aAdmin, 4}}}))
		assert.Equal(t, 4, r.messageCount, "a step with both: +2, counted once")
		require.True(t, res.Success)
		step := &b65aRecord{msgs: []int{5}}
		res = r.Run(context.Background(), b30Scenario(0, step))
		assert.Equal(t, "message budget exceeded (3)", res.Error)
		assert.Equal(t, 4, r.messageCount, "the step did not start")
	})
	attempts := func(a ...b65aRecord) *b65aAttempts { return &b65aAttempts{attempts: a} }
	t.Run("429_with_send", func(t *testing.T) {
		f := newB65aFake()
		r, _ := b65aRunner(f, RunnerConfig{MaxMessages: 10, RetryOn429: true, Max429Retries: 1})
		s := attempts(b65aRecord{msgs: []int{1}, err: b65a429(time.Millisecond)}, b65aRecord{msgs: []int{2}})
		res := r.Run(context.Background(), b30Scenario(0, s))
		require.True(t, res.Success, res.Error)
		assert.Equal(t, 2, s.runs)
		assert.Equal(t, 2, r.messageCount)
	})
	// One below the budget after a 429 attempt that sent: the attempt's sends are counted once, so it is repeated
	// (register A3-01).
	t.Run("429_with_send_below_budget", func(t *testing.T) {
		f := newB65aFake()
		r, _ := b65aRunner(f, RunnerConfig{MaxMessages: 2, RetryOn429: true, Max429Retries: 1})
		s := attempts(b65aRecord{msgs: []int{1}, err: b65a429(time.Millisecond)}, b65aRecord{msgs: []int{2}})
		res := r.Run(context.Background(), b30Scenario(0, s))
		require.True(t, res.Success, res.Error)
		assert.Equal(t, 2, s.runs)
		assert.Equal(t, 2, r.messageCount)
	})
	// At the budget, a 429 attempt is not repeated. The context stays live and the retry wait is 1 ms, so a runner
	// that retries runs the second attempt after about 501 ms (1 ms plus the runner's 500 ms margin), and the attempt
	// count shows it: no timing oracle (register O1-05, A1-05, O2-01, A3-05). Once with an ordinary message, once with
	// an ephemeral one.
	for _, c := range []struct {
		name string
		rec  b65aRecord
		keys []string
	}{
		{"429_at_budget", b65aRecord{msgs: []int{1}, err: b65a429(time.Millisecond)}, msgKeys(1)},
		{"429_at_budget_ephemeral", b65aRecord{eph: []ephemeralMessage{{b30Chat, b65aAdmin, 7}}, err: b65a429(time.Millisecond)},
			[]string{ephKey(b30Chat, b65aAdmin, 7)}},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newB65aFake()
			r, rt := b65aRunner(f, RunnerConfig{MaxMessages: 1, RetryOn429: true, Max429Retries: 1})
			s := attempts(c.rec, b65aRecord{msgs: []int{2}})
			res := r.Run(context.Background(), b30Scenario(0, s))
			require.False(t, res.Success)
			assert.Equal(t, 1, s.runs)
			assert.Equal(t, 1, r.messageCount)
			assert.Contains(t, res.Error, "429")
			assert.Equal(t, c.keys, f.keys(), "the stop cleanup deletes what was sent")
			assert.Empty(t, rt.CreatedMessages)
			assert.Empty(t, rt.ephemeralMessages)
		})
	}
	// At the budget with a cancelled context: the attempt cancels the scenario's context as it returns, and the retry
	// wait is 1 hour. A runner that stops only after a wait returns the context error, not the 429 (register O3-01).
	t.Run("429_at_budget_cancelled", func(t *testing.T) {
		f := newB65aFake()
		r, _ := b65aRunner(f, RunnerConfig{MaxMessages: 1, RetryOn429: true, Max429Retries: 1})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		s := attempts(b65aRecord{msgs: []int{1}, err: b65a429(time.Hour)}, b65aRecord{msgs: []int{2}})
		s.cancel = cancel
		res := r.Run(ctx, b30Scenario(0, s))
		require.False(t, res.Success)
		assert.Equal(t, 1, s.runs)
		assert.Equal(t, 1, r.messageCount)
		assert.Contains(t, res.Error, "429")
		assert.NotContains(t, res.Error, context.Canceled.Error())
	})
	t.Run("429_no_send_under_budget", func(t *testing.T) {
		f := newB65aFake()
		r, _ := b65aRunner(f, RunnerConfig{MaxMessages: 1, RetryOn429: true, Max429Retries: 1})
		s := attempts(b65aRecord{err: b65a429(time.Millisecond)}, b65aRecord{})
		res := r.Run(context.Background(), b30Scenario(0, s))
		require.True(t, res.Success, res.Error)
		assert.Equal(t, 2, s.runs)
		assert.Equal(t, 0, r.messageCount)
	})
}

// T-carry: an ephemeral target never outlives its scenario; the other kinds still carry (O2-04, A3-01, O3-05).
func TestB65A_NoEphemeralCarry(t *testing.T) {
	t.Run("after_stop", func(t *testing.T) {
		f := newB65aFake()
		e1 := ephemeralMessage{b30Chat, b65aAdmin, 40}
		f.script[ephKey(e1.chatID, e1.receiverUserID, e1.messageID)] = []error{b65a429(time.Second)}
		f.script["msg:-1001/501"] = []error{b65a429(time.Second)}
		r, rt := b65aRunner(f, RunnerConfig{})
		res := r.Run(context.Background(), b30Scenario(0, &b65aRecord{msgs: []int{501}, eph: []ephemeralMessage{e1}, err: errB30}))
		require.False(t, res.Success)
		require.Equal(t, []ephemeralMessage{e1}, ephTargets(rt), "kept and named in scenario 1")
		f.log = nil
		e2 := ephemeralMessage{b30Chat, b65aAdmin, 41}
		res = r.Run(context.Background(), b30Scenario(0, &b65aRecord{eph: []ephemeralMessage{e2}}, &CleanupStep{}))
		require.True(t, res.Success, res.Error)
		assert.Equal(t, []string{"msg:-1001/501", ephKey(e2.chatID, e2.receiverUserID, e2.messageID)}, f.keys())
		assert.Equal(t, map[string]any{"deleted_count": 1, "ephemeral_deleted": 1}, evMap(t, lastStep(res)))
	})
	t.Run("after_pass", func(t *testing.T) {
		f := newB65aFake()
		r, rt := b65aRunner(f, RunnerConfig{})
		res := r.Run(context.Background(), b30Scenario(0, &b65aRecord{eph: []ephemeralMessage{{b30Chat, b65aAdmin, 42}}}))
		require.True(t, res.Success)
		require.Len(t, rt.ephemeralMessages, 1)
		res = r.Run(context.Background(), b30Scenario(0, &CleanupStep{}))
		require.True(t, res.Success)
		assert.Empty(t, f.log, "dropped at the start of scenario 2, never deleted")
		assert.Empty(t, rt.ephemeralMessages)
	})
}

// b65aS73 runs the real S73 steps (the suites package imports engine, so the scenario is rebuilt here; T-sc checks
// that suites.S73_EphemeralMessages has exactly these steps).
func b65aS73() Scenario {
	return &BaseScenario{ScenarioName: "S73", ScenarioTimeout: 60 * time.Second, ScenarioSteps: []Step{
		&EphemeralReplyStep{CallbackQueryID: "65000000000000001"},
		&SendEphemeralStep{Method: "sendMessage"},
		&SendEphemeralStep{Method: "sendPhoto"},
		&CleanupStep{},
	}}
}

// T-run: S73 through the real runner with a fake sender.
func TestB65A_RunnerS73(t *testing.T) {
	newFake := func() *b65aFake {
		f := newB65aFake()
		f.reply = b65aResp{err: b65aCallbackRefusal()}
		return f
	}
	t.Run("pass", func(t *testing.T) {
		f := newFake()
		r, rt := b65aRunner(f, RunnerConfig{})
		res := r.Run(context.Background(), b65aS73())
		require.True(t, res.Success, res.Error)
		assert.Equal(t, []string{"65000000000000001"}, f.replies)
		assert.Equal(t, []string{ephKey(b30Chat, b65aAdmin, 9), ephKey(b30Chat, b65aAdmin, 10)}, f.keys())
		assert.Equal(t, map[string]any{"deleted_count": 0, "ephemeral_deleted": 2}, evMap(t, lastStep(res)))
		assert.Equal(t, 2, r.messageCount)
		assert.Empty(t, rt.ephemeralMessages)
		// The report key the live checker reads (register O1-07): the two sends name their ids, the other steps
		// have no key.
		require.Len(t, res.Steps, 4)
		for i, want := range []string{"", "[9]", "[10]", ""} {
			b, err := json.Marshal(res.Steps[i])
			require.NoError(t, err)
			var raw map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(b, &raw))
			if want == "" {
				assert.NotContains(t, raw, "ephemeral_message_ids", "step %d", i)
			} else {
				assert.Equal(t, want, string(raw["ephemeral_message_ids"]), "step %d", i)
			}
		}
	})
	t.Run("delete_429", func(t *testing.T) {
		f := newFake()
		f.script[ephKey(b30Chat, b65aAdmin, 9)] = []error{b65a429(time.Second)}
		r, rt := b65aRunner(f, RunnerConfig{RetryOn429: true, Max429Retries: 1})
		res := r.Run(context.Background(), b65aS73())
		require.True(t, res.Success, res.Error)
		assert.Len(t, f.sent, 2, "one send per method")
		assert.Equal(t, []string{ephKey(b30Chat, b65aAdmin, 9), ephKey(b30Chat, b65aAdmin, 10), ephKey(b30Chat, b65aAdmin, 9)}, f.keys())
		assert.Equal(t, map[string]any{"deleted_count": 0, "ephemeral_deleted": 2}, evMap(t, lastStep(res)))
		assert.Empty(t, rt.ephemeralMessages)
	})
	t.Run("budget_1", func(t *testing.T) {
		f := newFake()
		r, _ := b65aRunner(f, RunnerConfig{MaxMessages: 1})
		res := r.Run(context.Background(), b65aS73())
		assert.False(t, res.Success)
		assert.Contains(t, res.Error, "message budget exceeded (1)")
		require.Len(t, f.sent, 1)
		assert.Equal(t, "sendMessage", f.sent[0].method)
		assert.Equal(t, []string{ephKey(b30Chat, b65aAdmin, 9)}, f.keys())
		assert.Equal(t, map[string]any{"deleted_count": 0, "ephemeral_deleted": 1}, evMap(t, lastStep(res)))
	})
	t.Run("budget_2", func(t *testing.T) {
		f := newFake()
		r, _ := b65aRunner(f, RunnerConfig{MaxMessages: 2})
		res := r.Run(context.Background(), b65aS73())
		assert.False(t, res.Success)
		assert.Len(t, f.sent, 2)
		s := lastStep(res)
		require.Equal(t, "cleanup after stop", s.StepName)
		assert.Equal(t, map[string]any{"deleted_count": 0, "ephemeral_deleted": 2}, evMap(t, s))
	})
	t.Run("send_fail_after_first", func(t *testing.T) {
		f := newFake()
		f.sends = []b65aResp{{msg: &tg.Message{EphemeralMessageID: 9, Chat: &tg.Chat{ID: b30Chat}, ReceiverUser: &tg.User{ID: b65aAdmin}}},
			{err: errB30}}
		r, _ := b65aRunner(f, RunnerConfig{})
		res := r.Run(context.Background(), b65aS73())
		assert.False(t, res.Success)
		assert.Equal(t, []string{ephKey(b30Chat, b65aAdmin, 9)}, f.keys())
	})
	t.Run("not_admin", func(t *testing.T) {
		f := newFake()
		r, rt := b65aRunner(f, RunnerConfig{})
		rt.ChatCtx = &ChatContext{BotIsAdmin: false}
		res := r.Run(context.Background(), b65aS73())
		assert.True(t, res.Skipped)
		assert.Equal(t, []string{"65000000000000001"}, f.replies)
		assert.Empty(t, f.sent)
		assert.False(t, hasCleanupAfterStop(res))
	})
	t.Run("wrong_receiver", func(t *testing.T) {
		f := newFake()
		f.sends = []b65aResp{{msg: &tg.Message{EphemeralMessageID: 9, Chat: &tg.Chat{ID: b30Chat}, ReceiverUser: &tg.User{ID: b65aOther}}}}
		r, _ := b65aRunner(f, RunnerConfig{})
		res := r.Run(context.Background(), b65aS73())
		assert.False(t, res.Success)
		assert.Equal(t, 1, r.messageCount, "the failed step's send counts")
		assert.Equal(t, []string{ephKey(b30Chat, b65aOther, 9)}, f.keys(), "deleted for the receiver the server named")
	})
}

var _ SenderClient = (*b65aFake)(nil)
