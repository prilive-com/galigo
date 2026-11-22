package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-65 65f: WidgetRefusalStep (TEST-PLAN S1-S7, S5b) and the five adapter methods (A1).

// fakeWidget records each widget request and its context and returns the scripted result; the chat probe methods
// are scripted and counted.
type fakeWidget struct {
	SenderClient
	reqs     []any
	ctxs     []context.Context
	msg      *tg.Message
	err      error
	chatType string
	chatErr  error
	meErr    error
	memErr   error
	chats    []int64    // GetChat arguments
	mes      int        // GetMe calls
	members  [][2]int64 // GetChatMember arguments
}

func (f *fakeWidget) record(ctx context.Context, req any) {
	f.ctxs = append(f.ctxs, ctx)
	f.reqs = append(f.reqs, req)
}

func (f *fakeWidget) SendWidget(ctx context.Context, req sender.SendWidgetRequest) (*tg.Message, error) {
	f.record(ctx, req)
	return f.msg, f.err
}

func (f *fakeWidget) SendSharedWidget(ctx context.Context, req sender.SendSharedWidgetRequest) (*tg.Message, error) {
	f.record(ctx, req)
	return f.msg, f.err
}

func (f *fakeWidget) AnswerWidgetQuery(ctx context.Context, req sender.AnswerWidgetQueryRequest) error {
	f.record(ctx, req)
	return f.err
}

func (f *fakeWidget) EditMessageWidgetData(ctx context.Context, req sender.EditMessageWidgetDataRequest) error {
	f.record(ctx, req)
	return f.err
}

func (f *fakeWidget) EditSharedWidgetData(ctx context.Context, req sender.EditSharedWidgetDataRequest) error {
	f.record(ctx, req)
	return f.err
}

func (f *fakeWidget) GetChat(_ context.Context, chatID int64) (*tg.ChatFullInfo, error) {
	f.chats = append(f.chats, chatID)
	if f.chatErr != nil {
		return nil, f.chatErr
	}
	return &tg.ChatFullInfo{ID: chatID, Type: f.chatType}, nil
}

func (f *fakeWidget) GetMe(context.Context) (*tg.User, error) {
	f.mes++
	if f.meErr != nil {
		return nil, f.meErr
	}
	return &tg.User{ID: 4242, IsBot: true}, nil
}

func (f *fakeWidget) GetChatMember(_ context.Context, chatID, userID int64) (tg.ChatMember, error) {
	f.members = append(f.members, [2]int64{chatID, userID})
	if f.memErr != nil {
		return nil, f.memErr
	}
	return tg.ChatMemberMember{}, nil
}

const b65fChat = int64(-1001234567890)

// b65fStepTargets are the target ids of S1 and S2: above 2^31, 2^32 and 2^53 (odd) where int has 64 bits.
func b65fStepTargets() []int {
	if strconv.IntSize == 64 {
		return []int{int(b48c2BigTarget), int(b48c2BigTarget) + 2000000000, int(b65fHugeTarget)}
	}
	return []int{70}
}

// b65fHugeTarget is odd and above 2^53; a variable, so the file builds where int has 32 bits.
var b65fHugeTarget int64 = 9007199254740993

// b65fAllSteps are the seven S76 rows (the scenario's values; M1 pins the scenario itself).
func b65fAllSteps() []*WidgetRefusalStep {
	return []*WidgetRefusalStep{
		{Method: "sendWidget", WantError: WidgetNotAllowed},
		{Method: "sendSharedWidget", WantError: WidgetIDInvalid},
		{Method: "answerWidgetQuery", WantError: WidgetQueryInvalid},
		{Method: "editMessageWidgetData", Variant: "target", WantError: WidgetMessageInvalid},
		{Method: "editMessageWidgetData", Variant: "missing", WantError: WidgetMessageNotFound},
		{Method: "editMessageWidgetData", Variant: "inline", WantError: WidgetInlineInvalid},
		{Method: "editSharedWidgetData", WantError: WidgetIDInvalid},
	}
}

func b65fPtrS(s string) *string { return &s }

// b65fRT is a runtime in the group with the given target (0: none) and chat type cache ("": no cache).
func b65fRT(f *fakeWidget, target int, cached string) *Runtime {
	rt := NewRuntime(f, b65fChat, 1)
	if target != 0 {
		rt.LastMessage = &tg.Message{MessageID: target, Chat: &tg.Chat{ID: b65fChat}}
	}
	if cached != "" {
		rt.ChatCtx = &ChatContext{ChatID: b65fChat, ChatType: cached}
	}
	return rt
}

// b65fWantReq is the request each row must send for target id tgt.
func b65fWantReq(s *WidgetRefusalStep, tgt int) any {
	switch s.Method + "/" + s.Variant {
	case "sendWidget/":
		return sender.SendWidgetRequest{ChatID: b65fChat, HTML: "<b>galigo widget</b>"}
	case "sendSharedWidget/":
		return sender.SendSharedWidgetRequest{ChatID: b65fChat, WidgetID: "galigo-fake-widget"}
	case "answerWidgetQuery/":
		return sender.AnswerWidgetQueryRequest{WidgetQueryID: "galigo-fake-query", Data: "x"}
	case "editMessageWidgetData/target":
		return sender.EditMessageWidgetDataRequest{ChatID: b65fChat, MessageID: tgt, ChatData: b65fPtrS("c")}
	case "editMessageWidgetData/missing":
		return sender.EditMessageWidgetDataRequest{ChatID: b65fChat, MessageID: tgt + 1000000, ChatData: b65fPtrS("c")}
	case "editMessageWidgetData/inline":
		return sender.EditMessageWidgetDataRequest{InlineMessageID: "galigo-fake-inline", PublicData: b65fPtrS("p")}
	case "editSharedWidgetData/":
		return sender.EditSharedWidgetDataRequest{WidgetID: "galigo-fake-widget", PublicData: ""}
	}
	panic("unknown row " + s.Name())
}

// S1, S2: each row sends one planned request with the caller's context, and accepts the measured refusal.
func TestB65F_Step_RequestsAccept(t *testing.T) {
	for _, tgt := range b65fStepTargets() {
		for _, s := range b65fAllSteps() {
			t.Run(fmt.Sprintf("%s/%d", s.Name(), tgt), func(t *testing.T) {
				f := &fakeWidget{err: b48c2Refusal(s.WantError)}
				ctx := context.WithValue(context.Background(), b48c2CtxKey{}, "caller")
				res, err := s.Execute(ctx, b65fRT(f, tgt, "supergroup"))
				require.NoError(t, err)
				require.Len(t, f.reqs, 1, "one widget call")
				assert.Equal(t, b65fWantReq(s, tgt), f.reqs[0])
				assert.Equal(t, "caller", f.ctxs[0].Value(b48c2CtxKey{}), "the caller's context is passed on")
				assert.Equal(t, s.Name(), res.StepName)
				assert.Equal(t, s.Method, res.Method)
				want := map[string]any{"error_code": 400, "description": "Bad Request: " + s.WantError}
				switch s.Variant {
				case "target":
					want["message_id"] = tgt
				case "missing":
					want["message_id"] = tgt + 1000000
				}
				assert.Equal(t, want, res.Evidence)
			})
		}
	}
}

// S2: the wanted text is the step's own WantError, matched anywhere in the description.
func TestB65F_Step_AlternateWant(t *testing.T) {
	for _, s := range b65fAllSteps() {
		t.Run(s.Name(), func(t *testing.T) {
			alt := *s
			alt.WantError = "alt refusal text"
			f := &fakeWidget{err: &tg.APIError{Code: 400, Description: "Bad Request: before alt refusal text after"}}
			_, err := alt.Execute(context.Background(), b65fRT(f, 70, "group"))
			require.NoError(t, err)
			f.err = b48c2Refusal(s.WantError)
			_, err = alt.Execute(context.Background(), b65fRT(f, 70, "group"))
			require.Error(t, err, "the S76 literal is not the wanted text here")
		})
	}
}

// S3: anything but a 400 *tg.APIError with the wanted text in its own description fails the step.
func TestB65F_Step_Reject(t *testing.T) {
	for _, s := range b65fAllSteps() {
		want := s.WantError
		rows := []struct {
			name string
			err  error
		}{
			{"403", &tg.APIError{Code: 403, Description: "Forbidden: " + want}},
			{"429", &tg.APIError{Code: 429, Description: "Too Many Requests: " + want}},
			{"other text", b48c2Refusal("something else")},
			{"plain", errors.New("plain " + want)},
			{"text in wrapper", fmt.Errorf("%s: %w", want, b48c2Refusal("something else"))},
			{"nil", nil},
		}
		for _, r := range rows {
			t.Run(s.Name()+"/"+r.name, func(t *testing.T) {
				f := &fakeWidget{err: r.err}
				res, err := s.Execute(context.Background(), b65fRT(f, 70, "group"))
				require.Error(t, err)
				assert.False(t, IsSkip(err))
				assert.Contains(t, err.Error(), s.Method)
				assert.Contains(t, err.Error(), want)
				assert.Nil(t, res.Evidence)
				assert.Equal(t, s.Name(), res.StepName)
				assert.Equal(t, s.Method, res.Method)
			})
		}
		t.Run(s.Name()+"/wrapped accepted", func(t *testing.T) {
			f := &fakeWidget{err: fmt.Errorf("x: %w", b48c2Refusal(want))}
			_, err := s.Execute(context.Background(), b65fRT(f, 70, "group"))
			require.NoError(t, err)
		})
	}
}

// S4: a taken send fails the step; its message is tracked in the test chat and reported.
func TestB65F_Step_TakenSend(t *testing.T) {
	for _, s := range b65fAllSteps()[:2] {
		t.Run(s.Name(), func(t *testing.T) {
			f := &fakeWidget{msg: &tg.Message{MessageID: 9001, Chat: &tg.Chat{ID: b65fChat}}}
			rt := b65fRT(f, 0, "group")
			res, err := s.Execute(context.Background(), rt)
			require.Error(t, err)
			assert.Equal(t, []CreatedMessage{{ChatID: b65fChat, MessageID: 9001}}, rt.CreatedMessages)
			assert.Equal(t, []int{9001}, res.MessageIDs)
		})
	}
}

// S5: the step runs only in a group or supergroup, checked on every step (cached or probed once).
func TestB65F_Step_GroupCheck(t *testing.T) {
	for _, s := range b65fAllSteps() {
		for _, typ := range []string{"group", "supergroup"} {
			t.Run(s.Name()+"/probe_"+typ, func(t *testing.T) {
				f := &fakeWidget{chatType: typ, err: b48c2Refusal(s.WantError)}
				_, err := s.Execute(context.Background(), b65fRT(f, 70, ""))
				require.NoError(t, err)
				assert.Len(t, f.reqs, 1)
				assert.Equal(t, []int64{b65fChat}, f.chats, "getChat with the test chat")
				assert.Equal(t, [][2]int64{{b65fChat, 4242}}, f.members, "getChatMember with the bot id")
			})
			t.Run(s.Name()+"/cached_"+typ, func(t *testing.T) {
				f := &fakeWidget{err: b48c2Refusal(s.WantError)}
				_, err := s.Execute(context.Background(), b65fRT(f, 70, typ))
				require.NoError(t, err)
				assert.Len(t, f.reqs, 1)
				assert.Empty(t, f.chats, "no probe")
			})
		}
		for _, typ := range []string{"private", "channel", ""} {
			t.Run(s.Name()+"/probe_skip_"+typ, func(t *testing.T) {
				f := &fakeWidget{chatType: typ}
				_, err := s.Execute(context.Background(), b65fRT(f, 70, ""))
				assert.True(t, IsSkip(err), "error %v", err)
				assert.Empty(t, f.reqs, "no widget call")
			})
			t.Run(s.Name()+"/cached_skip_"+typ, func(t *testing.T) {
				f := &fakeWidget{}
				rt := b65fRT(f, 70, "")
				rt.ChatCtx = &ChatContext{ChatID: b65fChat, ChatType: typ}
				_, err := s.Execute(context.Background(), rt)
				assert.True(t, IsSkip(err), "error %v", err)
				assert.Empty(t, f.reqs, "no widget call")
				assert.Empty(t, f.chats, "no probe")
				assert.Zero(t, f.mes)
			})
		}
		t.Run(s.Name()+"/getChat error", func(t *testing.T) {
			f := &fakeWidget{chatErr: errors.New("chat-net")}
			_, err := s.Execute(context.Background(), b65fRT(f, 70, ""))
			require.Error(t, err)
			assert.False(t, IsSkip(err))
			assert.ErrorIs(t, err, f.chatErr)
			assert.Len(t, f.chats, 1)
			assert.Empty(t, f.reqs)
		})
		t.Run(s.Name()+"/getMe error", func(t *testing.T) {
			f := &fakeWidget{chatType: "group", meErr: errors.New("me-net")}
			_, err := s.Execute(context.Background(), b65fRT(f, 70, ""))
			require.Error(t, err)
			assert.False(t, IsSkip(err))
			assert.ErrorIs(t, err, f.meErr)
			assert.Equal(t, 1, f.mes)
			assert.Empty(t, f.reqs)
		})
	}
	for _, typ := range []string{"group", "supergroup"} {
		t.Run("one probe for all steps/"+typ, func(t *testing.T) {
			f := &fakeWidget{chatType: typ}
			rt := b65fRT(f, 70, "")
			for _, s := range b65fAllSteps() {
				f.err = b48c2Refusal(s.WantError)
				_, err := s.Execute(context.Background(), rt)
				require.NoError(t, err, s.Name())
			}
			assert.Len(t, f.chats, 1, "one probe")
			assert.Len(t, f.reqs, 7)
		})
	}
}

// S5b: the runner's retry after a failed probe uses the cached chat type.
func TestB65F_Step_ProbeRetry(t *testing.T) {
	tooMany := &tg.APIError{Code: 429, Description: "Too Many Requests: retry after 1", RetryAfter: 1}
	for _, s := range b65fAllSteps() {
		t.Run(s.Name()+"/group", func(t *testing.T) {
			f := &fakeWidget{chatType: "group", meErr: tooMany}
			rt := b65fRT(f, 70, "")
			_, err := s.Execute(context.Background(), rt)
			require.ErrorIs(t, err, tooMany)
			assert.Empty(t, f.reqs)
			f.meErr, f.err = nil, b48c2Refusal(s.WantError)
			_, err = s.Execute(context.Background(), rt)
			require.NoError(t, err)
			assert.Len(t, f.chats, 1, "no new getChat")
			assert.Len(t, f.reqs, 1)
		})
		t.Run(s.Name()+"/private", func(t *testing.T) {
			f := &fakeWidget{chatType: "private", meErr: tooMany}
			rt := b65fRT(f, 70, "")
			_, err := s.Execute(context.Background(), rt)
			require.ErrorIs(t, err, tooMany)
			f.meErr = nil
			_, err = s.Execute(context.Background(), rt)
			assert.True(t, IsSkip(err), "error %v", err)
			assert.Empty(t, f.reqs, "no widget call in total")
		})
		t.Run(s.Name()+"/getChatMember error", func(t *testing.T) {
			f := &fakeWidget{chatType: "group", memErr: errors.New("member-net"), err: b48c2Refusal(s.WantError)}
			_, err := s.Execute(context.Background(), b65fRT(f, 70, ""))
			require.NoError(t, err)
			assert.Len(t, f.reqs, 1)
		})
	}
}

// S6: the two target rows need a target message.
func TestB65F_Step_NoTarget(t *testing.T) {
	for _, s := range b65fAllSteps()[3:5] {
		t.Run(s.Name(), func(t *testing.T) {
			f := &fakeWidget{}
			_, err := s.Execute(context.Background(), b65fRT(f, 0, "group"))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no target message")
			assert.Empty(t, f.reqs)
		})
	}
}

// S6b: the missing row's id (target + 1000000) must fit in an int. A target too close to the int maximum stops the
// step with a named error before any call; the largest target that fits is used.
func TestB65F_Step_MissingIDRange(t *testing.T) {
	s := b65fAllSteps()[4]
	require.Equal(t, "missing", s.Variant)
	f := &fakeWidget{err: b48c2Refusal(s.WantError)}
	_, err := s.Execute(context.Background(), b65fRT(f, math.MaxInt-999999, "group"))
	require.Error(t, err)
	assert.Equal(t, fmt.Sprintf("editMessageWidgetData: target message id %d is too large for the missing message id (target + 1000000)",
		math.MaxInt-999999), err.Error())
	assert.Empty(t, f.reqs)
	f = &fakeWidget{err: b48c2Refusal(s.WantError)}
	res, err := s.Execute(context.Background(), b65fRT(f, math.MaxInt-1000000, "group"))
	require.NoError(t, err)
	require.Len(t, f.reqs, 1)
	assert.Equal(t, b65fWantReq(s, math.MaxInt-1000000), f.reqs[0])
	ev, ok := res.Evidence.(map[string]any)
	require.True(t, ok, "evidence %T", res.Evidence)
	assert.Equal(t, math.MaxInt, ev["message_id"])
}

// S6c: the range check runs before the chat probe, so a cold runtime makes no call at all; it applies only to the
// missing row: the target row sends a target id above the bound as it is.
func TestB65F_Step_MissingIDRangeScope(t *testing.T) {
	steps := b65fAllSteps()
	missing, target := steps[4], steps[3]
	require.Equal(t, "missing", missing.Variant)
	require.Equal(t, "target", target.Variant)
	t.Run("cold missing row", func(t *testing.T) {
		f := &fakeWidget{chatType: "group", err: b48c2Refusal(missing.WantError)}
		_, err := missing.Execute(context.Background(), b65fRT(f, math.MaxInt-999999, ""))
		require.Error(t, err)
		assert.Equal(t, fmt.Sprintf("editMessageWidgetData: target message id %d is too large for the missing message id (target + 1000000)",
			math.MaxInt-999999), err.Error())
		assert.Empty(t, f.reqs)
		assert.Empty(t, f.chats)
		assert.Zero(t, f.mes)
		assert.Empty(t, f.members)
	})
	for _, tgt := range []int{math.MaxInt - 999999, math.MaxInt} {
		t.Run(fmt.Sprintf("target row %d", tgt), func(t *testing.T) {
			f := &fakeWidget{err: b48c2Refusal(target.WantError)}
			res, err := target.Execute(context.Background(), b65fRT(f, tgt, "group"))
			require.NoError(t, err)
			require.Len(t, f.reqs, 1)
			assert.Equal(t, b65fWantReq(target, tgt), f.reqs[0])
			ev, ok := res.Evidence.(map[string]any)
			require.True(t, ok, "evidence %T", res.Evidence)
			assert.Equal(t, tgt, ev["message_id"])
		})
	}
}

// S7: the refusal constants are the measured texts.
func TestB65F_Step_Constants(t *testing.T) {
	assert.Equal(t, "INPUT_MEDIA_WIDGET_NOT_ALLOWED", WidgetNotAllowed)
	assert.Equal(t, "invalid widget identifier specified", WidgetIDInvalid)
	assert.Equal(t, "query is too old", WidgetQueryInvalid)
	assert.Equal(t, "MESSAGE_ID_INVALID", WidgetMessageInvalid)
	assert.Equal(t, "message with widget to edit not found", WidgetMessageNotFound)
	assert.Equal(t, "invalid inline message identifier specified", WidgetInlineInvalid)
}

func b65fPath(method string) string { return "/bot" + testutil.TestToken + "/" + method }

// b65fAdapter is a SenderAdapter over a mock server that answers the widget methods.
func b65fAdapter(t *testing.T) (*testutil.MockTelegramServer, *SenderAdapter) {
	t.Helper()
	server := testutil.NewMockServer(t)
	for _, m := range []string{"sendWidget", "sendSharedWidget"} {
		server.On(b65fPath(m), func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyMessage(w, 42) })
	}
	for _, m := range []string{"answerWidgetQuery", "editMessageWidgetData", "editSharedWidgetData"} {
		server.On(b65fPath(m), func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyBool(w, true) })
	}
	client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(1000, 100),
		sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
	return server, NewSenderAdapter(client)
}

// b65fAdapterCalls call each adapter method with a full request; want is the exact body.
func b65fAdapterCalls() []struct {
	name, method, want string
	call               func(context.Context, *SenderAdapter) error
} {
	huge := strconv.FormatInt(b65fHugeTarget, 10)
	if strconv.IntSize != 64 {
		huge = "71"
	}
	hugeID, _ := strconv.Atoi(huge)
	return []struct {
		name, method, want string
		call               func(context.Context, *SenderAdapter) error
	}{
		{"sendWidget", "sendWidget", `{"chat_id":5000000001,"message_thread_id":11,"html":"<b>h</b>","width":300,` +
			`"height":200,"public_data":"pub-1","chat_data":"chat-2","caption":"  cap  ","parse_mode":"HTML",` +
			`"caption_entities":[{"type":"bold","offset":0,"length":2}],"message_effect_id":"eff-3",` +
			`"reply_parameters":{"message_id":9}}`, func(ctx context.Context, a *SenderAdapter) error {
			_, err := a.SendWidget(ctx, sender.SendWidgetRequest{ChatID: int64(5000000001), MessageThreadID: 11, HTML: "<b>h</b>",
				Width: 300, Height: 200, PublicData: "pub-1", ChatData: "chat-2", Caption: "  cap  ", ParseMode: tg.ParseModeHTML,
				CaptionEntities: []tg.MessageEntity{{Type: "bold", Length: 2}}, MessageEffectID: "eff-3",
				ReplyParameters: &tg.ReplyParameters{MessageID: 9}})
			return err
		}},
		{"sendSharedWidget", "sendSharedWidget", `{"chat_id":"@chan","message_thread_id":12,"widget_id":"wid-4",` +
			`"chat_data":"chat-2","caption":"  cap  ","parse_mode":"MarkdownV2","message_effect_id":"eff-5",` +
			`"reply_parameters":{"message_id":8}}`, func(ctx context.Context, a *SenderAdapter) error {
			_, err := a.SendSharedWidget(ctx, sender.SendSharedWidgetRequest{ChatID: "@chan", MessageThreadID: 12, WidgetID: "wid-4",
				ChatData: "chat-2", Caption: "  cap  ", ParseMode: tg.ParseModeMarkdownV2, MessageEffectID: "eff-5",
				ReplyParameters: &tg.ReplyParameters{MessageID: 8}})
			return err
		}},
		{"answerWidgetQuery", "answerWidgetQuery", `{"widget_query_id":"q-6","data":"x"}`, func(ctx context.Context, a *SenderAdapter) error {
			return a.AnswerWidgetQuery(ctx, sender.AnswerWidgetQueryRequest{WidgetQueryID: "q-6", Data: "x"})
		}},
		{"editMessageWidgetData chat", "editMessageWidgetData", `{"chat_id":5000000001,"message_id":` + huge + `,"public_data":"","chat_data":"chat-2"}`,
			func(ctx context.Context, a *SenderAdapter) error {
				return a.EditMessageWidgetData(ctx, sender.EditMessageWidgetDataRequest{ChatID: int64(5000000001), MessageID: hugeID,
					PublicData: b65fPtrS(""), ChatData: b65fPtrS("chat-2")})
			}},
		{"editMessageWidgetData inline", "editMessageWidgetData", `{"inline_message_id":"inl-7","public_data":"pub-1"}`,
			func(ctx context.Context, a *SenderAdapter) error {
				return a.EditMessageWidgetData(ctx, sender.EditMessageWidgetDataRequest{InlineMessageID: "inl-7", PublicData: b65fPtrS("pub-1")})
			}},
		{"editMessageWidgetData chat both data", "editMessageWidgetData", `{"chat_id":5000000001,"message_id":7,"public_data":"pub-x","chat_data":"chat-y"}`,
			func(ctx context.Context, a *SenderAdapter) error {
				return a.EditMessageWidgetData(ctx, sender.EditMessageWidgetDataRequest{ChatID: int64(5000000001), MessageID: 7,
					PublicData: b65fPtrS("pub-x"), ChatData: b65fPtrS("chat-y")})
			}},
		{"editMessageWidgetData inline both data", "editMessageWidgetData", `{"inline_message_id":"inl-7","public_data":"pub-x","chat_data":"chat-y"}`,
			func(ctx context.Context, a *SenderAdapter) error {
				return a.EditMessageWidgetData(ctx, sender.EditMessageWidgetDataRequest{InlineMessageID: "inl-7",
					PublicData: b65fPtrS("pub-x"), ChatData: b65fPtrS("chat-y")})
			}},
		{"editSharedWidgetData", "editSharedWidgetData", `{"widget_id":"wid-4","public_data":"pub-1","cache_time":5}`,
			func(ctx context.Context, a *SenderAdapter) error {
				return a.EditSharedWidgetData(ctx, sender.EditSharedWidgetDataRequest{WidgetID: "wid-4", PublicData: "pub-1", CacheTime: 5})
			}},
	}
}

// b65fExact compares a body with exact numbers (JSONEq would accept 2^53+1 as 2^53).
func b65fExact(t *testing.T, want string, got []byte) {
	t.Helper()
	dec := func(b []byte) any {
		d := json.NewDecoder(bytes.NewReader(b))
		d.UseNumber()
		var v any
		require.NoError(t, d.Decode(&v))
		return v
	}
	assert.Equal(t, dec([]byte(want)), dec(got))
}

// A1: the adapter passes each request, its result and its error on unchanged, and the caller's context.
func TestB65F_A1_Adapter(t *testing.T) {
	for _, c := range b65fAdapterCalls() {
		t.Run(c.name+"/body", func(t *testing.T) {
			server, a := b65fAdapter(t)
			require.NoError(t, c.call(context.Background(), a))
			require.Equal(t, 1, server.CaptureCount())
			server.LastCapture().AssertPath(t, b65fPath(c.method))
			b65fExact(t, c.want, server.LastCapture().Body)
		})
		t.Run(c.name+"/api error", func(t *testing.T) {
			server, a := b65fAdapter(t)
			server.On(b65fPath(c.method), func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyBadRequest(w, "nope") })
			err := c.call(context.Background(), a)
			var apiErr *tg.APIError
			require.True(t, errors.As(err, &apiErr), "error %v", err)
			assert.Equal(t, 400, apiErr.Code)
			assert.Equal(t, "Bad Request: nope", apiErr.Description)
		})
		t.Run(c.name+"/held context", func(t *testing.T) {
			server, a := b65fAdapter(t)
			arrived := make(chan struct{}, 1)
			release := make(chan struct{})
			t.Cleanup(func() { close(release) })
			server.On(b65fPath(c.method), func(w http.ResponseWriter, _ *http.Request) {
				arrived <- struct{}{}
				<-release
				testutil.ReplyBool(w, true)
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- c.call(ctx, a) }()
			select {
			case <-arrived:
			case <-time.After(5 * time.Second):
				t.Fatal("the request did not arrive")
			}
			cancel()
			select {
			case err := <-done:
				assert.ErrorIs(t, err, context.Canceled)
			case <-time.After(5 * time.Second):
				t.Fatal("the call did not return after the cancel")
			}
		})
	}
	t.Run("sendWidget message", func(t *testing.T) {
		_, a := b65fAdapter(t)
		msg, err := a.SendWidget(context.Background(), sender.SendWidgetRequest{ChatID: b65fChat, HTML: "h"})
		require.NoError(t, err)
		assert.Equal(t, 42, msg.MessageID)
	})
	// The adapter sends width and height as given, also outside the range the app clamps them to and when negative,
	// and each one alone (a zero size is left out).
	for _, z := range []struct{ w, h int }{{100, 10}, {199, 601}, {5000, 700}, {-1, -2}, {300, 0}, {0, 450}} {
		t.Run(fmt.Sprintf("sendWidget size %dx%d", z.w, z.h), func(t *testing.T) {
			server, a := b65fAdapter(t)
			_, err := a.SendWidget(context.Background(), sender.SendWidgetRequest{ChatID: b65fChat, HTML: "h", Width: z.w, Height: z.h})
			require.NoError(t, err)
			require.Equal(t, 1, server.CaptureCount())
			want := fmt.Sprintf(`{"chat_id":%d,"html":"h"`, b65fChat)
			if z.w != 0 {
				want += fmt.Sprintf(`,"width":%d`, z.w)
			}
			if z.h != 0 {
				want += fmt.Sprintf(`,"height":%d`, z.h)
			}
			b65fExact(t, want+"}", server.LastCapture().Body)
		})
	}
}

// A1 (bools and markup): each bool alone and each reply_markup form reach the wire through the adapter.
func TestB65F_A1_BoolsMarkup(t *testing.T) {
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
			server, a := b65fAdapter(t)
			req := sender.SendWidgetRequest{ChatID: b65fChat, HTML: "h"}
			set(&req)
			_, err := a.SendWidget(context.Background(), req)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"chat_id": float64(b65fChat), "html": "h", key: true}, server.LastCapture().BodyMap(t))
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
			server, a := b65fAdapter(t)
			req := sender.SendSharedWidgetRequest{ChatID: b65fChat, WidgetID: "w"}
			set(&req)
			_, err := a.SendSharedWidget(context.Background(), req)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"chat_id": float64(b65fChat), "widget_id": "w", key: true}, server.LastCapture().BodyMap(t))
		})
	}
	markups := []struct {
		name string
		v    any
		want string
	}{
		{"inline", tg.InlineKeyboardMarkup{InlineKeyboard: [][]tg.InlineKeyboardButton{{{Text: "a", CallbackData: "b"}}}},
			`{"inline_keyboard":[[{"text":"a","callback_data":"b"}]]}`},
		{"reply", tg.ReplyKeyboardMarkup{Keyboard: [][]tg.KeyboardButton{{{Text: "k"}}}}, `{"keyboard":[[{"text":"k"}]]}`},
		{"remove", tg.ReplyKeyboardRemove{RemoveKeyboard: true}, `{"remove_keyboard":true}`},
		{"force_reply", map[string]any{"force_reply": true}, `{"force_reply":true}`},
	}
	for _, mk := range markups {
		t.Run("markup/"+mk.name, func(t *testing.T) {
			server, a := b65fAdapter(t)
			_, err := a.SendWidget(context.Background(), sender.SendWidgetRequest{ChatID: b65fChat, HTML: "h", ReplyMarkup: mk.v})
			require.NoError(t, err)
			got, _ := json.Marshal(server.LastCapture().BodyMap(t)["reply_markup"])
			assert.JSONEq(t, mk.want, string(got))
			_, err = a.SendSharedWidget(context.Background(), sender.SendSharedWidgetRequest{ChatID: b65fChat, WidgetID: "w", ReplyMarkup: mk.v})
			require.NoError(t, err)
			got, _ = json.Marshal(server.LastCapture().BodyMap(t)["reply_markup"])
			assert.JSONEq(t, mk.want, string(got))
		})
	}
}
