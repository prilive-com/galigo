package engine

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.0 testbot steps (B-25 slice 2a, TEST-PLAN §4, T-tb-4 (a) … (h), (g0)).

const api100BotID = int64(7001) // the bot: ≠ admin (987654321) ≠ chat (123456789) ≠ message (4242)

// fake100 is a SenderClient for the 10.0 steps: it embeds the interface (calling any other method panics) and
// overrides only the eight methods the steps use. It counts calls per method, records each request whole (and the
// chat id of each getChat and getChatAdministratorsWithBots call), and returns scripted replies per method in order
// (an unscripted call returns the zero reply).
type fake100 struct {
	SenderClient

	calls   map[string]int
	chatIDs map[string][]int64 // per method: the chat id of each getChat / getChatAdministratorsWithBots call

	memberUserIDs []int64
	deleteReqs    []sender.DeleteMessageReactionRequest
	deleteAllReqs []sender.DeleteAllMessageReactionsRequest
	drafts        []fake100Draft

	replies map[string][]fake100Reply
}

type fake100Draft struct {
	chatID  int64
	draftID int
	text    string
}

type fake100Reply struct {
	chat    *tg.ChatFullInfo
	user    *tg.User
	member  tg.ChatMember
	members []tg.ChatMember
	err     error
}

func newFake100() *fake100 {
	return &fake100{calls: map[string]int{}, chatIDs: map[string][]int64{}, replies: map[string][]fake100Reply{}}
}

func (f *fake100) script(method string, r ...fake100Reply) *fake100 {
	f.replies[method] = append(f.replies[method], r...)
	return f
}

func (f *fake100) next(method string) fake100Reply {
	f.calls[method]++
	q := f.replies[method]
	if len(q) == 0 {
		return fake100Reply{}
	}
	f.replies[method] = q[1:]
	return q[0]
}

func (f *fake100) total() int {
	n := 0
	for _, c := range f.calls {
		n += c
	}
	return n
}

func (f *fake100) GetChat(_ context.Context, chatID int64) (*tg.ChatFullInfo, error) {
	f.chatIDs["getChat"] = append(f.chatIDs["getChat"], chatID)
	r := f.next("getChat")
	return r.chat, r.err
}

func (f *fake100) GetMe(_ context.Context) (*tg.User, error) {
	r := f.next("getMe")
	return r.user, r.err
}

func (f *fake100) GetChatMember(_ context.Context, _ int64, userID int64) (tg.ChatMember, error) {
	f.memberUserIDs = append(f.memberUserIDs, userID)
	r := f.next("getChatMember")
	return r.member, r.err
}

func (f *fake100) GetChatAdministratorsWithBots(_ context.Context, chatID int64) ([]tg.ChatMember, error) {
	f.chatIDs["getChatAdministratorsWithBots"] = append(f.chatIDs["getChatAdministratorsWithBots"], chatID)
	r := f.next("getChatAdministratorsWithBots")
	return r.members, r.err
}

func (f *fake100) GetChatAdministrators(_ context.Context, _ int64) ([]tg.ChatMember, error) {
	r := f.next("getChatAdministrators")
	return r.members, r.err
}

func (f *fake100) DeleteMessageReaction(_ context.Context, req sender.DeleteMessageReactionRequest) error {
	f.deleteReqs = append(f.deleteReqs, req)
	return f.next("deleteMessageReaction").err
}

func (f *fake100) DeleteAllMessageReactions(_ context.Context, req sender.DeleteAllMessageReactionsRequest) error {
	f.deleteAllReqs = append(f.deleteAllReqs, req)
	return f.next("deleteAllMessageReactions").err
}

func (f *fake100) SendMessageDraft(_ context.Context, chatID int64, draftID int, text string) error {
	f.drafts = append(f.drafts, fake100Draft{chatID, draftID, text})
	return f.next("sendMessageDraft").err
}

// api100Writes are the fake's write methods; no read-only step may call one.
var api100Writes = []string{"deleteMessageReaction", "deleteAllMessageReactions", "sendMessageDraft"}

// api100OtherChatID is a second chat id for the fresh-runtime rows: not TestChatID, so a step that reads the
// wrong chat (a stale LastMessage, a fixed id) fails them (X-C2-01).
const api100OtherChatID int64 = -1001000000100

// api100StaleRuntime is a runtime whose other chat sources (LastMessage, ForumChatID, ChatCtx.ChatID,
// TestUserID) all differ from rt.ChatID, so a step that falls back to any of them fails (X-C3-01).
func api100StaleRuntime(f *fake100) *Runtime {
	rt := api100Runtime(f)
	rt.ChatID = api100OtherChatID
	rt.ForumChatID = -1001000000200
	rt.TestUserID = 1000000300
	rt.ChatCtx = &ChatContext{ChatID: testutil.TestChatID, BotIsAdmin: true}
	return rt
}

func api100Runtime(f *fake100) *Runtime {
	rt := NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
	rt.LastMessage = &tg.Message{MessageID: 4242, Chat: &tg.Chat{ID: testutil.TestChatID},
		From: &tg.User{ID: api100BotID, IsBot: true}}
	return rt
}

func api100Admin(id int64, isBot, canDelete bool) tg.ChatMember {
	a := tg.ChatMemberAdministrator{CanDeleteMessages: canDelete}
	a.User = &tg.User{ID: id, IsBot: isBot}
	return a
}

func api100Owner(id int64, isBot bool) tg.ChatMember {
	o := tg.ChatMemberOwner{}
	o.User = &tg.User{ID: id, IsBot: isBot}
	return o
}

func api100Member(id int64, isBot bool) tg.ChatMember {
	m := tg.ChatMemberMember{}
	m.User = &tg.User{ID: id, IsBot: isBot}
	return m
}

// api100Probe scripts the three ProbeChat reads: getChat ok, getMe the bot, getChatMember the given member.
func api100Probe(f *fake100, member tg.ChatMember) *fake100 {
	return f.script("getChat", fake100Reply{chat: &tg.ChatFullInfo{ID: testutil.TestChatID, Type: "supergroup"}}).
		script("getMe", fake100Reply{user: &tg.User{ID: api100BotID, IsBot: true}}).
		script("getChatMember", fake100Reply{member: member})
}

// api100Fields checks the result fields every new step sets on every return path (T-tb-4 (g0)).
func api100Fields(t *testing.T, step Step, res *StepResult, method string) {
	t.Helper()
	require.NotNil(t, res, "a 10.0 step must return a result on every path")
	assert.Equal(t, step.Name(), res.StepName)
	assert.Equal(t, method, res.Method)
}

func api100NoWrites(t *testing.T, f *fake100) {
	t.Helper()
	for _, m := range api100Writes {
		assert.Zero(t, f.calls[m], "write method %s called", m)
	}
}

// T-tb-4 (a): the S51 guard reads chat rights (preset or probed) and never writes.
func TestAPI100Steps_RequireDeleteMessages(t *testing.T) {
	step := &RequireDeleteMessages100Step{}

	t.Run("preset can delete", func(t *testing.T) {
		// API100 T-tb-4 (a)
		f := newFake100()
		rt := api100Runtime(f)
		rt.ChatCtx = &ChatContext{BotIsAdmin: true, CanDeleteMessages: true}
		res, err := step.Execute(context.Background(), rt)
		require.NoError(t, err)
		api100Fields(t, step, res, "probeChat")
		assert.Equal(t, 0, f.total())
	})
	t.Run("probed admin can delete", func(t *testing.T) {
		// API100 T-tb-4 (a)
		f := api100Probe(newFake100(), api100Admin(api100BotID, true, true))
		rt := api100Runtime(f)
		res, err := step.Execute(context.Background(), rt)
		require.NoError(t, err)
		api100Fields(t, step, res, "probeChat")
		assert.Equal(t, map[string]int{"getChat": 1, "getMe": 1, "getChatMember": 1}, f.calls)
		assert.Equal(t, []int64{api100BotID}, f.memberUserIDs)
	})
	skips := []struct {
		name   string
		member tg.ChatMember
	}{
		{"probed admin cannot delete", api100Admin(api100BotID, true, false)},
		{"probed member", api100Member(api100BotID, true)},
	}
	for _, r := range skips {
		t.Run(r.name, func(t *testing.T) {
			// API100 T-tb-4 (a)
			f := api100Probe(newFake100(), r.member)
			rt := api100Runtime(f)
			res, err := step.Execute(context.Background(), rt)
			require.Error(t, err)
			assert.True(t, IsSkip(err), "want a Skip, got %v", err)
			// API100 T-tb-4 (g0): the Skip result keeps StepName and the probeChat marker
			api100Fields(t, step, res, "probeChat")
			api100NoWrites(t, f)
		})
	}
}

// T-tb-4 (b): the delete steps target the last message and its sender, one call each.
func TestAPI100Steps_DeleteReactions(t *testing.T) {
	t.Run("single", func(t *testing.T) {
		// API100 T-tb-4 (b)
		f := newFake100()
		step := &DeleteReactions100Step{Kind: "single"}
		res, err := step.Execute(context.Background(), api100Runtime(f))
		require.NoError(t, err)
		api100Fields(t, step, res, "deleteMessageReaction")
		assert.Empty(t, res.MessageIDs)
		assert.Equal(t, []sender.DeleteMessageReactionRequest{
			{ChatID: testutil.TestChatID, MessageID: 4242, UserID: api100BotID}}, f.deleteReqs)
		assert.Equal(t, map[string]int{"deleteMessageReaction": 1}, f.calls)
	})
	t.Run("all", func(t *testing.T) {
		// API100 T-tb-4 (b)
		f := newFake100()
		step := &DeleteReactions100Step{Kind: "all"}
		res, err := step.Execute(context.Background(), api100Runtime(f))
		require.NoError(t, err)
		api100Fields(t, step, res, "deleteAllMessageReactions")
		assert.Empty(t, res.MessageIDs)
		assert.Equal(t, []sender.DeleteAllMessageReactionsRequest{
			{ChatID: testutil.TestChatID, UserID: api100BotID}}, f.deleteAllReqs)
		assert.Empty(t, f.deleteReqs)
		assert.Equal(t, map[string]int{"deleteAllMessageReactions": 1}, f.calls)
	})
	t.Run("unknown kind", func(t *testing.T) {
		// API100 T-tb-4 (b)
		f := newFake100()
		step := &DeleteReactions100Step{Kind: "some"}
		res, err := step.Execute(context.Background(), api100Runtime(f))
		require.ErrorIs(t, err, errReact100Kind)
		require.NotNil(t, res)
		assert.Equal(t, step.Name(), res.StepName)
		assert.Equal(t, 0, f.total())
	})
}

// T-tb-4 (c): a missing source is the step's own error, never a panic, a call or a Skip.
func TestAPI100Steps_DeleteReactionsGuards(t *testing.T) {
	rows := []struct {
		name string
		last *tg.Message
		want error
	}{
		{"no LastMessage", nil, errReact100NoMessage},
		{"no From", &tg.Message{MessageID: 4242, Chat: &tg.Chat{ID: testutil.TestChatID}}, errReact100NoFrom},
		{"From.ID 0", &tg.Message{MessageID: 4242, Chat: &tg.Chat{ID: testutil.TestChatID}, From: &tg.User{IsBot: true}},
			errReact100NoFromID},
	}
	for _, kind := range []struct{ kind, method string }{
		{"single", "deleteMessageReaction"}, {"all", "deleteAllMessageReactions"}} {
		for _, r := range rows {
			t.Run(kind.kind+" "+r.name, func(t *testing.T) {
				// API100 T-tb-4 (c)
				f := newFake100()
				rt := api100Runtime(f)
				rt.LastMessage = r.last
				step := &DeleteReactions100Step{Kind: kind.kind}
				var (
					res *StepResult
					err error
				)
				require.NotPanics(t, func() { res, err = step.Execute(context.Background(), rt) })
				require.ErrorIs(t, err, r.want)
				assert.False(t, IsSkip(err))
				assert.Equal(t, 0, f.total())
				// API100 T-tb-4 (g0)
				api100Fields(t, step, res, kind.method)
			})
		}
	}
}

// T-tb-4 (d): getChatAdministrators with return_bots finds a bot administrator anywhere in the list.
func TestAPI100Steps_AdminsWithBots(t *testing.T) {
	step := &GetAdminsWithBots100Step{}
	rows := []struct {
		name    string
		members []tg.ChatMember
		want    error
	}{
		{"the bot", []tg.ChatMember{api100Admin(api100BotID, true, false)}, nil},
		{"empty", []tg.ChatMember{}, errAdmins100NoBot},
		{"no bot", []tg.ChatMember{api100Admin(1, false, false), api100Owner(2, false)}, errAdmins100NoBot},
		{"the bot second", []tg.ChatMember{api100Member(3, false), api100Admin(api100BotID, true, false)}, nil},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			// API100 T-tb-4 (d)
			f := newFake100().script("getChatAdministratorsWithBots", fake100Reply{members: r.members})
			rt := api100Runtime(f)
			rt.ChatCtx = &ChatContext{BotIsAdmin: true}
			res, err := step.Execute(context.Background(), rt)
			if r.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, r.want)
			}
			api100Fields(t, step, res, "getChatAdministrators")
			assert.Equal(t, map[string]int{"getChatAdministratorsWithBots": 1}, f.calls)
			// the test chat, not the admin's id (O-C1-01)
			assert.Equal(t, []int64{testutil.TestChatID}, f.chatIDs["getChatAdministratorsWithBots"])
		})
	}
	t.Run("not admin", func(t *testing.T) {
		// API100 T-tb-4 (d)
		f := newFake100()
		rt := api100Runtime(f)
		rt.ChatCtx = &ChatContext{}
		res, err := step.Execute(context.Background(), rt)
		require.Error(t, err)
		assert.True(t, IsSkip(err), "want a Skip, got %v", err)
		assert.Equal(t, 0, f.total())
		// API100 T-tb-4 (g0)
		api100Fields(t, step, res, "getChatAdministrators")
	})
	t.Run("probed", func(t *testing.T) {
		// API100 T-tb-4 (d)
		f := api100Probe(newFake100(), api100Admin(api100BotID, true, false)).
			script("getChatAdministratorsWithBots", fake100Reply{members: []tg.ChatMember{api100Admin(api100BotID, true, false)}})
		res, err := step.Execute(context.Background(), api100Runtime(f))
		require.NoError(t, err)
		api100Fields(t, step, res, "getChatAdministrators")
		assert.Equal(t, map[string]int{"getChat": 1, "getMe": 1, "getChatMember": 1, "getChatAdministratorsWithBots": 1}, f.calls)
		assert.Equal(t, []int64{testutil.TestChatID}, f.chatIDs["getChatAdministratorsWithBots"])
	})
	t.Run("fresh runtime", func(t *testing.T) {
		// API100 T-tb-4 (d): rt.ChatID only; no LastMessage (X-C2-01)
		f := newFake100().script("getChatAdministratorsWithBots", fake100Reply{members: []tg.ChatMember{api100Admin(api100BotID, true, false)}})
		rt := NewRuntime(f, api100OtherChatID, testutil.TestUserID)
		rt.ChatCtx = &ChatContext{BotIsAdmin: true}
		var (
			res *StepResult
			err error
		)
		require.NotPanics(t, func() { res, err = step.Execute(context.Background(), rt) })
		require.NoError(t, err)
		api100Fields(t, step, res, "getChatAdministrators")
		assert.Equal(t, map[string]int{"getChatAdministratorsWithBots": 1}, f.calls)
		assert.Equal(t, []int64{api100OtherChatID}, f.chatIDs["getChatAdministratorsWithBots"])
	})
	t.Run("stale runtime", func(t *testing.T) {
		// API100 T-tb-4 (d): rt.ChatID, not another chat the runtime holds (X-C3-01)
		f := newFake100().script("getChatAdministratorsWithBots", fake100Reply{members: []tg.ChatMember{api100Admin(api100BotID, true, false)}})
		res, err := step.Execute(context.Background(), api100StaleRuntime(f))
		require.NoError(t, err)
		api100Fields(t, step, res, "getChatAdministrators")
		assert.Equal(t, map[string]int{"getChatAdministratorsWithBots": 1}, f.calls)
		assert.Equal(t, []int64{api100OtherChatID}, f.chatIDs["getChatAdministratorsWithBots"])
	})
}

// T-tb-4 (e): getChat must carry permissions with can_react_to_messages set (true or false).
func TestAPI100Steps_ChatPermissionsReact(t *testing.T) {
	step := &ChatPermissionsReact100Step{}
	f, tr := false, true
	rows := []struct {
		name  string
		perms *tg.ChatPermissions
		want  error
	}{
		{"set false", &tg.ChatPermissions{CanReactToMessages: &f}, nil},
		{"set true", &tg.ChatPermissions{CanReactToMessages: &tr}, nil},
		{"no permissions", nil, errPerm100NoPermissions},
		{"no react field", &tg.ChatPermissions{CanSendMessages: &tr}, errPerm100NoReact},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			// API100 T-tb-4 (e)
			fk := newFake100().script("getChat", fake100Reply{chat: &tg.ChatFullInfo{ID: testutil.TestChatID, Type: "supergroup", Permissions: r.perms}})
			var (
				res *StepResult
				err error
			)
			require.NotPanics(t, func() { res, err = step.Execute(context.Background(), api100Runtime(fk)) })
			if r.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, r.want)
			}
			api100Fields(t, step, res, "getChat")
			assert.Equal(t, map[string]int{"getChat": 1}, fk.calls)
			// the test chat, not the admin's id (O-C1-01)
			assert.Equal(t, []int64{testutil.TestChatID}, fk.chatIDs["getChat"])
		})
	}
	t.Run("fresh runtime", func(t *testing.T) {
		// API100 T-tb-4 (e): rt.ChatID only; no LastMessage (X-C2-01)
		fk := newFake100().script("getChat", fake100Reply{chat: &tg.ChatFullInfo{ID: api100OtherChatID, Type: "supergroup", Permissions: &tg.ChatPermissions{CanReactToMessages: &tr}}})
		rt := NewRuntime(fk, api100OtherChatID, testutil.TestUserID)
		var (
			res *StepResult
			err error
		)
		require.NotPanics(t, func() { res, err = step.Execute(context.Background(), rt) })
		require.NoError(t, err)
		api100Fields(t, step, res, "getChat")
		assert.Equal(t, map[string]int{"getChat": 1}, fk.calls)
		assert.Equal(t, []int64{api100OtherChatID}, fk.chatIDs["getChat"])
	})
	t.Run("stale runtime", func(t *testing.T) {
		// API100 T-tb-4 (e): rt.ChatID, not another chat the runtime holds (X-C3-01)
		fk := newFake100().script("getChat", fake100Reply{chat: &tg.ChatFullInfo{ID: api100OtherChatID, Type: "supergroup", Permissions: &tg.ChatPermissions{CanReactToMessages: &tr}}})
		res, err := step.Execute(context.Background(), api100StaleRuntime(fk))
		require.NoError(t, err)
		api100Fields(t, step, res, "getChat")
		assert.Equal(t, map[string]int{"getChat": 1}, fk.calls)
		assert.Equal(t, []int64{api100OtherChatID}, fk.chatIDs["getChat"])
	})
}

// T-tb-4 (f): the empty draft goes to the admin's private chat; no admin is a Skip.
func TestAPI100Steps_EmptyDraft(t *testing.T) {
	step := &EmptyDraft100Step{}
	t.Run("admin", func(t *testing.T) {
		// API100 T-tb-4 (f)
		f := newFake100()
		res, err := step.Execute(context.Background(), api100Runtime(f))
		require.NoError(t, err)
		api100Fields(t, step, res, "sendMessageDraft")
		assert.Equal(t, []fake100Draft{{chatID: testutil.TestUserID, draftID: 100, text: ""}}, f.drafts)
		assert.Equal(t, map[string]int{"sendMessageDraft": 1}, f.calls)
	})
	t.Run("no admin", func(t *testing.T) {
		// API100 T-tb-4 (f)
		f := newFake100()
		rt := api100Runtime(f)
		rt.AdminUserID = 0
		res, err := step.Execute(context.Background(), rt)
		require.Error(t, err)
		assert.True(t, IsSkip(err), "want a Skip, got %v", err)
		assert.Equal(t, 0, f.total())
		// API100 T-tb-4 (g0)
		api100Fields(t, step, res, "sendMessageDraft")
	})
}

// T-tb-4 (g0): a sender error is wrapped (errors.Is finds it) and the result still has StepName and Method.
func TestAPI100Steps_SenderErrorFields(t *testing.T) {
	boom := errors.New("boom")
	rows := []struct {
		name   string
		step   Step
		method string
		setup  func(f *fake100, rt *Runtime)
	}{
		{"guard probe", &RequireDeleteMessages100Step{}, "probeChat",
			func(f *fake100, _ *Runtime) { f.script("getChat", fake100Reply{err: boom}) }},
		{"single", &DeleteReactions100Step{Kind: "single"}, "deleteMessageReaction",
			func(f *fake100, _ *Runtime) { f.script("deleteMessageReaction", fake100Reply{err: boom}) }},
		{"all", &DeleteReactions100Step{Kind: "all"}, "deleteAllMessageReactions",
			func(f *fake100, _ *Runtime) { f.script("deleteAllMessageReactions", fake100Reply{err: boom}) }},
		{"admins", &GetAdminsWithBots100Step{}, "getChatAdministrators",
			func(f *fake100, rt *Runtime) {
				rt.ChatCtx = &ChatContext{BotIsAdmin: true}
				f.script("getChatAdministratorsWithBots", fake100Reply{err: boom})
			}},
		{"permissions", &ChatPermissionsReact100Step{}, "getChat",
			func(f *fake100, _ *Runtime) { f.script("getChat", fake100Reply{err: boom}) }},
		{"draft", &EmptyDraft100Step{}, "sendMessageDraft",
			func(f *fake100, _ *Runtime) { f.script("sendMessageDraft", fake100Reply{err: boom}) }},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			// API100 T-tb-4 (g0)
			f := newFake100()
			rt := api100Runtime(f)
			r.setup(f, rt)
			res, err := r.step.Execute(context.Background(), rt)
			require.ErrorIs(t, err, boom)
			api100Fields(t, r.step, res, r.method)
		})
	}
}

// T-tb-4 (g): one Telegram call per step (the runner repeats a whole step after a 429); a preset ChatCtx means
// the guard makes no call at all.
func TestAPI100Steps_OneCall(t *testing.T) {
	tr := true
	rows := []struct {
		name  string
		step  Step
		setup func(f *fake100, rt *Runtime)
		want  map[string]int
	}{
		{"single", &DeleteReactions100Step{Kind: "single"}, func(*fake100, *Runtime) {},
			map[string]int{"deleteMessageReaction": 1}},
		{"all", &DeleteReactions100Step{Kind: "all"}, func(*fake100, *Runtime) {},
			map[string]int{"deleteAllMessageReactions": 1}},
		{"admins", &GetAdminsWithBots100Step{}, func(f *fake100, rt *Runtime) {
			rt.ChatCtx = &ChatContext{BotIsAdmin: true}
			f.script("getChatAdministratorsWithBots", fake100Reply{members: []tg.ChatMember{api100Admin(api100BotID, true, false)}})
		}, map[string]int{"getChatAdministratorsWithBots": 1}},
		{"permissions", &ChatPermissionsReact100Step{}, func(f *fake100, _ *Runtime) {
			f.script("getChat", fake100Reply{chat: &tg.ChatFullInfo{ID: testutil.TestChatID, Permissions: &tg.ChatPermissions{CanReactToMessages: &tr}}})
		}, map[string]int{"getChat": 1}},
		{"draft", &EmptyDraft100Step{}, func(*fake100, *Runtime) {}, map[string]int{"sendMessageDraft": 1}},
		{"guard preset", &RequireDeleteMessages100Step{}, func(_ *fake100, rt *Runtime) {
			rt.ChatCtx = &ChatContext{BotIsAdmin: true, CanDeleteMessages: true}
		}, map[string]int{}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			// API100 T-tb-4 (g)
			f := newFake100()
			rt := api100Runtime(f)
			r.setup(f, rt)
			_, err := r.step.Execute(context.Background(), rt)
			require.NoError(t, err)
			assert.Equal(t, r.want, f.calls)
		})
	}
}

// T-tb-4 (h): a 429 makes the real runner repeat the delete step once; the step alone does not retry and keeps
// the *tg.APIError for the runner.
func TestAPI100Steps_429Repeat(t *testing.T) {
	rate := func() fake100Reply {
		return fake100Reply{err: &tg.APIError{Code: 429, Description: "Too Many Requests", RetryAfter: time.Millisecond}}
	}
	t.Run("runner repeats single", func(t *testing.T) {
		// API100 T-tb-4 (h)
		f := newFake100().script("deleteMessageReaction", rate(), fake100Reply{})
		rt := api100Runtime(f)
		runner := NewRunner(rt, RunnerConfig{RetryOn429: true, Max429Retries: 1, MaxMessages: 10},
			slog.New(slog.DiscardHandler))
		res := runner.Run(context.Background(), &BaseScenario{
			ScenarioName: "T-tb-4 (h)", ScenarioSteps: []Step{&DeleteReactions100Step{Kind: "single"}}, ScenarioTimeout: 10 * time.Second,
		})
		require.True(t, res.Success, "scenario error: %s", res.Error)
		assert.Equal(t, 2, f.calls["deleteMessageReaction"])
		assert.Equal(t, 2, f.total())
	})
	for _, k := range []struct{ kind, method string }{
		{"single", "deleteMessageReaction"}, {"all", "deleteAllMessageReactions"}} {
		t.Run("step alone "+k.kind, func(t *testing.T) {
			// API100 T-tb-4 (h)
			f := newFake100().script(k.method, rate(), fake100Reply{})
			_, err := (&DeleteReactions100Step{Kind: k.kind}).Execute(context.Background(), api100Runtime(f))
			var apiErr *tg.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, 429, apiErr.Code)
			assert.Equal(t, 1, f.total())
		})
	}
}

// ==================== Bot API 10.0 part b: live photo steps (B-25 10.0b, TEST-PLAN §4, T-tb-2 … T-tb-5, T-tb-7) ====================

type liveSend struct {
	chat         int64
	video, photo MediaInput
	caption      string
}

type liveEdit struct {
	chat  int64
	msgID int
	media sender.InputMedia
}

// fakeLive is a SenderClient for the live photo steps: it embeds the interface (any other method panics),
// records every call with all its arguments, and returns the scripted reply of each method.
type fakeLive struct {
	SenderClient

	sends      []liveSend
	groups     [][]MediaInput
	groupChats []int64 // the chat id of each SendMediaGroup call (C1-06)
	edits      []liveEdit

	sendReply  *tg.Message
	sendErr    error
	groupReply []*tg.Message
	groupErr   error
	editReply  *tg.Message
	editErr    error
}

func (f *fakeLive) SendLivePhoto(_ context.Context, chatID int64, video, photo MediaInput, caption string) (*tg.Message, error) {
	f.sends = append(f.sends, liveSend{chatID, video, photo, caption})
	return f.sendReply, f.sendErr
}

func (f *fakeLive) SendMediaGroup(_ context.Context, chatID int64, media []MediaInput) ([]*tg.Message, error) {
	f.groups = append(f.groups, media)
	f.groupChats = append(f.groupChats, chatID)
	return f.groupReply, f.groupErr
}

func (f *fakeLive) EditMessageMedia(_ context.Context, chatID int64, messageID int, media sender.InputMedia) (*tg.Message, error) {
	f.edits = append(f.edits, liveEdit{chatID, messageID, media})
	return f.editReply, f.editErr
}

func (f *fakeLive) total() int { return len(f.sends) + len(f.groups) + len(f.edits) }

// liveMsg is a full live photo message: the video id, two static sizes (the last is the still id), and the
// backward-compatible top-level photo with the same sizes.
func liveMsg(id int, video, still, caption string) *tg.Message {
	sizes := func() []tg.PhotoSize { return []tg.PhotoSize{{FileID: still + "90"}, {FileID: still}} }
	return &tg.Message{
		MessageID: id, Chat: &tg.Chat{ID: testutil.TestChatID}, Caption: caption,
		LivePhoto: &tg.LivePhoto{FileID: video, Photo: sizes()},
		Photo:     sizes(),
	}
}

// liveBad returns the five broken forms of a live photo reply (T-tb-2).
func liveBad(id int, caption string) map[string]*tg.Message {
	noLive := liveMsg(id, "reply-video-9", "reply-still-9", caption)
	noLive.LivePhoto = nil
	noVideoID := liveMsg(id, "", "reply-still-9", caption)
	noSizes := liveMsg(id, "reply-video-9", "reply-still-9", caption)
	noSizes.LivePhoto.Photo = nil
	noTopPhoto := liveMsg(id, "reply-video-9", "reply-still-9", caption)
	noTopPhoto.Photo = nil
	wrongCaption := liveMsg(id, "reply-video-9", "reply-still-9", caption+"-other")
	return map[string]*tg.Message{
		"no live_photo": noLive, "empty live_photo.file_id": noVideoID, "no live_photo.photo": noSizes,
		"no top-level photo": noTopPhoto, "wrong caption": wrongCaption,
	}
}

func liveRuntime(f *fakeLive, captured bool) *Runtime {
	rt := NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
	if captured {
		rt.CapturedFileIDs["live_video"] = "sent-video"
		rt.CapturedFileIDs["live_still"] = "sent-still"
	}
	return rt
}

func liveTracked(rt *Runtime) []int {
	ids := []int{}
	for _, m := range rt.CreatedMessages {
		ids = append(ids, m.MessageID)
	}
	return ids
}

func requireMediaBytes(t *testing.T, m MediaInput, want []byte) {
	t.Helper()
	require.NotNil(t, m.Reader, "an upload")
	assert.Equal(t, want, m.Reader())
	assert.Empty(t, m.FileID)
	assert.NotEmpty(t, m.FileName)
}

func TestAPI100B_SendLivePhotoUpload(t *testing.T) { // API100B T-tb-2
	t.Run("good reply", func(t *testing.T) {
		f := &fakeLive{sendReply: liveMsg(7, "reply-video-1", "reply-still-1", "lp")}
		rt := liveRuntime(f, false)
		step := &SendLivePhoto100Step{Caption: "lp"}
		res, err := step.Execute(context.Background(), rt)
		require.NoError(t, err)
		require.Len(t, f.sends, 1)
		s := f.sends[0]
		assert.Equal(t, testutil.TestChatID, s.chat)
		assert.Equal(t, "lp", s.caption)
		requireMediaBytes(t, s.video, fixtures.VideoBytes())
		requireMediaBytes(t, s.photo, fixtures.PhotoBytes())
		assert.NotEqual(t, s.video.FileName, s.photo.FileName)
		assert.Equal(t, map[string]string{"live_video": "reply-video-1", "live_still": "reply-still-1"}, rt.CapturedFileIDs)
		assert.Same(t, f.sendReply, rt.LastMessage)
		assert.Equal(t, []int{7}, liveTracked(rt))
		assert.Equal(t, &StepResult{
			StepName: "sendLivePhoto (10.0)", Method: "sendLivePhoto", MessageIDs: []int{7},
			FileIDs: []string{"reply-video-1", "reply-still-1"},
			Evidence: map[string]any{"message_id": 7, "by_file_id": false, "caption": "lp",
				"file_ids": []string{"reply-video-1", "reply-still-1"}},
		}, res)
	})
	for name, reply := range liveBad(7, "lp") {
		t.Run("bad reply: "+name, func(t *testing.T) {
			f := &fakeLive{sendReply: reply}
			rt := liveRuntime(f, false)
			res, err := (&SendLivePhoto100Step{Caption: "lp"}).Execute(context.Background(), rt)
			require.Error(t, err)
			require.NotNil(t, res, "a failed check keeps the result (P3-01)")
			assert.Equal(t, "sendLivePhoto", res.Method)
			assert.Equal(t, []int{7}, res.MessageIDs)
			assert.Equal(t, []int{7}, liveTracked(rt))
			assert.Empty(t, rt.CapturedFileIDs)
		})
	}
	t.Run("send error", func(t *testing.T) {
		f := &fakeLive{sendErr: errors.New("boom")}
		rt := liveRuntime(f, false)
		res, err := (&SendLivePhoto100Step{Caption: "lp"}).Execute(context.Background(), rt)
		require.Error(t, err)
		if res != nil {
			assert.Empty(t, res.MessageIDs)
		}
		assert.Empty(t, rt.CreatedMessages)
	})
}

func TestAPI100B_SendLivePhotoByID(t *testing.T) { // API100B T-tb-3
	t.Run("good reply", func(t *testing.T) {
		f := &fakeLive{sendReply: liveMsg(8, "reply-video-2", "reply-still-2", "")}
		rt := liveRuntime(f, true)
		res, err := (&SendLivePhoto100Step{ByFileID: true}).Execute(context.Background(), rt)
		require.NoError(t, err)
		require.Len(t, f.sends, 1)
		assert.Equal(t, testutil.TestChatID, f.sends[0].chat, "the runtime chat, not the admin (C3-02)")
		assert.Equal(t, MediaInput{FileID: "sent-video"}, f.sends[0].video)
		assert.Equal(t, MediaInput{FileID: "sent-still"}, f.sends[0].photo)
		assert.Equal(t, []string{"sent-video", "sent-still"}, res.FileIDs)
		assert.Equal(t, []int{8}, res.MessageIDs)
		assert.Equal(t, map[string]string{"live_video": "sent-video", "live_still": "sent-still"}, rt.CapturedFileIDs)
		assert.Equal(t, []int{8}, liveTracked(rt))
	})
	for _, key := range []string{"live_video", "live_still"} {
		t.Run("missing "+key, func(t *testing.T) {
			f := &fakeLive{}
			rt := liveRuntime(f, true)
			delete(rt.CapturedFileIDs, key)
			_, err := (&SendLivePhoto100Step{ByFileID: true}).Execute(context.Background(), rt)
			require.Error(t, err)
			assert.Contains(t, err.Error(), key)
			assert.Equal(t, 0, f.total())
		})
	}
	// T3-04: the reply checks run on the by-id branch too, and a failed check keeps the result.
	for name, reply := range liveBad(8, "lp") {
		t.Run("bad reply by id: "+name, func(t *testing.T) {
			f := &fakeLive{sendReply: reply}
			rt := liveRuntime(f, true)
			res, err := (&SendLivePhoto100Step{ByFileID: true, Caption: "lp"}).Execute(context.Background(), rt)
			require.Error(t, err)
			require.Len(t, f.sends, 1)
			assert.Equal(t, testutil.TestChatID, f.sends[0].chat, "the runtime chat, not the admin (C3-02)")
			assert.Equal(t, "lp", f.sends[0].caption)
			require.NotNil(t, res, "a failed check keeps the result (P3-01)")
			assert.Equal(t, "sendLivePhoto", res.Method)
			assert.Equal(t, []int{8}, res.MessageIDs)
			assert.Equal(t, []int{8}, liveTracked(rt))
			assert.Equal(t, map[string]string{"live_video": "sent-video", "live_still": "sent-still"}, rt.CapturedFileIDs)
		})
	}
}

func TestAPI100B_LivePhotoAlbum(t *testing.T) { // API100B T-tb-4
	good := func() []*tg.Message {
		return []*tg.Message{liveMsg(21, "reply-video-1", "reply-still-1", ""), liveMsg(22, "reply-video-2", "reply-still-2", "")}
	}
	t.Run("upload", func(t *testing.T) {
		f := &fakeLive{groupReply: good()}
		rt := liveRuntime(f, false)
		res, err := (&LivePhotoAlbum100Step{}).Execute(context.Background(), rt)
		require.NoError(t, err)
		require.Len(t, f.groups, 1)
		assert.Equal(t, []int64{testutil.TestChatID}, f.groupChats, "the runtime chat, not the admin (C1-06)")
		require.Len(t, f.groups[0], 2)
		for i, item := range f.groups[0] {
			assert.Equal(t, "live_photo", item.Type, "item %d", i)
			requireMediaBytes(t, item, fixtures.VideoBytes())
			require.NotNil(t, item.Photo, "item %d", i)
			requireMediaBytes(t, *item.Photo, fixtures.PhotoBytes())
			assert.NotEqual(t, item.FileName, item.Photo.FileName)
		}
		assert.Equal(t, []int{21, 22}, res.MessageIDs)
		assert.Equal(t, []string{"reply-video-1", "reply-still-1", "reply-video-2", "reply-still-2"}, res.FileIDs)
		assert.Equal(t, []int{21, 22}, liveTracked(rt))
	})
	t.Run("by id", func(t *testing.T) {
		f := &fakeLive{groupReply: good()}
		rt := liveRuntime(f, true)
		res, err := (&LivePhotoAlbum100Step{ByFileID: true}).Execute(context.Background(), rt)
		require.NoError(t, err)
		require.Len(t, f.groups, 1)
		assert.Equal(t, []int64{testutil.TestChatID}, f.groupChats, "the runtime chat, not the admin (C1-06)")
		item := MediaInput{FileID: "sent-video", Type: "live_photo", Photo: &MediaInput{FileID: "sent-still"}}
		assert.Equal(t, []MediaInput{item, item}, f.groups[0])
		assert.Equal(t, []int{21, 22}, res.MessageIDs)
		assert.Equal(t, []string{"sent-video", "sent-still"}, res.FileIDs)
	})
	bad := liveMsg(31, "reply-video-x", "reply-still-x", "")
	bad.LivePhoto = nil
	bad2 := liveMsg(32, "reply-video-y", "reply-still-y", "")
	bad2.LivePhoto = nil
	failures := []struct {
		name  string
		reply []*tg.Message
		ids   []int
	}{
		{"0 messages", []*tg.Message{}, []int{}},
		{"1 message", []*tg.Message{liveMsg(41, "v", "s", "")}, []int{41}},
		{"3 messages", []*tg.Message{liveMsg(41, "v", "s", ""), liveMsg(42, "v2", "s2", ""), liveMsg(43, "v3", "s3", "")}, []int{41, 42, 43}},
		{"1st invalid", []*tg.Message{bad, liveMsg(42, "v2", "s2", "")}, []int{31, 42}},
		{"2nd invalid", []*tg.Message{liveMsg(41, "v", "s", ""), bad2}, []int{41, 32}},
	}
	for _, r := range failures {
		for _, byID := range []bool{false, true} { // C1-01: the checks hold in both modes
			name := "bad reply: " + r.name
			if byID {
				name = "bad reply by id: " + r.name
			}
			t.Run(name, func(t *testing.T) {
				f := &fakeLive{groupReply: r.reply}
				rt := liveRuntime(f, byID)
				res, err := (&LivePhotoAlbum100Step{ByFileID: byID}).Execute(context.Background(), rt)
				require.Error(t, err)
				require.Len(t, f.groups, 1)
				require.NotNil(t, res, "a failed check keeps the result (P3-01)")
				assert.Equal(t, "sendMediaGroup", res.Method)
				got := res.MessageIDs
				if got == nil {
					got = []int{}
				}
				assert.Equal(t, r.ids, got)
				assert.Equal(t, r.ids, liveTracked(rt))
			})
		}
	}
	t.Run("send error", func(t *testing.T) {
		f := &fakeLive{groupErr: errors.New("boom")}
		rt := liveRuntime(f, false)
		res, err := (&LivePhotoAlbum100Step{}).Execute(context.Background(), rt)
		require.Error(t, err)
		if res != nil {
			assert.Empty(t, res.MessageIDs)
		}
		assert.Empty(t, rt.CreatedMessages)
	})
	// T3-05: by id, each captured key absent or empty -> an error naming it, no call, no panic.
	for _, key := range []string{"live_video", "live_still"} {
		for _, empty := range []bool{false, true} {
			name := "absent " + key
			if empty {
				name = "empty " + key
			}
			t.Run(name, func(t *testing.T) {
				f := &fakeLive{groupReply: good()}
				rt := liveRuntime(f, true)
				if empty {
					rt.CapturedFileIDs[key] = ""
				} else {
					delete(rt.CapturedFileIDs, key)
				}
				_, err := (&LivePhotoAlbum100Step{ByFileID: true}).Execute(context.Background(), rt)
				require.Error(t, err)
				assert.Contains(t, err.Error(), key)
				assert.Equal(t, 0, f.total())
			})
		}
	}
}

func TestAPI100B_EditToLivePhoto(t *testing.T) { // API100B T-tb-5
	withLast := func(f *fakeLive) *Runtime {
		rt := liveRuntime(f, true)
		rt.LastMessage = &tg.Message{MessageID: 42, Chat: &tg.Chat{ID: testutil.TestChatID}, Photo: []tg.PhotoSize{{FileID: "plain"}}}
		return rt
	}
	t.Run("good reply", func(t *testing.T) {
		f := &fakeLive{editReply: liveMsg(42, "reply-video-3", "reply-still-3", "")}
		rt := withLast(f)
		res, err := (&EditToLivePhoto100Step{}).Execute(context.Background(), rt)
		require.NoError(t, err)
		require.Len(t, f.edits, 1)
		assert.Equal(t, liveEdit{testutil.TestChatID, 42, sender.InputMedia{Type: "live_photo",
			Media: sender.InputFile{FileID: "sent-video", Photo: &sender.InputFile{FileID: "sent-still"}}}}, f.edits[0])
		assert.Equal(t, "editMessageMedia", res.Method)
		assert.Equal(t, []string{"sent-video", "sent-still"}, res.FileIDs)
		assert.Empty(t, res.MessageIDs)
		assert.Empty(t, rt.CreatedMessages)
		assert.Same(t, f.editReply, rt.LastMessage)
	})
	t.Run("reply with another message id", func(t *testing.T) {
		f := &fakeLive{editReply: liveMsg(43, "reply-video-3", "reply-still-3", "")}
		_, err := (&EditToLivePhoto100Step{}).Execute(context.Background(), withLast(f))
		require.Error(t, err)
	})
	t.Run("reply not a live photo", func(t *testing.T) {
		reply := liveMsg(42, "reply-video-3", "reply-still-3", "")
		reply.LivePhoto = nil
		f := &fakeLive{editReply: reply}
		_, err := (&EditToLivePhoto100Step{}).Execute(context.Background(), withLast(f))
		require.Error(t, err)
	})
	t.Run("no last message", func(t *testing.T) {
		f := &fakeLive{}
		_, err := (&EditToLivePhoto100Step{}).Execute(context.Background(), liveRuntime(f, true))
		require.Error(t, err)
		assert.Equal(t, 0, f.total())
	})
	for _, key := range []string{"live_video", "live_still"} {
		t.Run("missing "+key, func(t *testing.T) {
			f := &fakeLive{}
			rt := withLast(f)
			delete(rt.CapturedFileIDs, key)
			_, err := (&EditToLivePhoto100Step{}).Execute(context.Background(), rt)
			require.Error(t, err)
			assert.Contains(t, err.Error(), key)
			assert.Equal(t, 0, f.total())
		})
	}
}

func TestAPI100B_HasMediaKindLive(t *testing.T) { // API100B T-tb-7
	full := liveMsg(1, "v", "s", "")
	noLive := liveMsg(1, "v", "s", "")
	noLive.LivePhoto = nil
	noID := liveMsg(1, "", "s", "")
	noSizes := liveMsg(1, "v", "s", "")
	noSizes.LivePhoto.Photo = nil
	noTop := liveMsg(1, "v", "s", "")
	noTop.Photo = nil
	rows := []struct {
		name string
		msg  *tg.Message
		kind string
		want bool
	}{
		{"full live photo", full, "live_photo", true},
		{"no live_photo", noLive, "live_photo", false},
		{"empty file_id", noID, "live_photo", false},
		{"no static sizes", noSizes, "live_photo", false},
		{"no top-level photo", noTop, "live_photo", false},
		{"nil message", nil, "live_photo", false},
		{"photo kind unchanged", full, "photo", true},
		{"video kind unchanged", full, "video", false},
		{"document kind unchanged", &tg.Message{Document: &tg.Document{FileID: "d"}}, "document", true},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) { assert.Equal(t, r.want, hasMediaKind(r.msg, r.kind)) })
	}
}
