package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 9.6 testbot steps (B-25 slice 1, TEST-PLAN §4, T-tb-4 (a) … (f)).

// fake96 is a SenderClient for the 9.6 steps: it embeds the interface (calling any other method panics) and
// overrides only the five methods the steps use. It counts calls, records each request, and returns scripted
// replies in order.
type fake96 struct {
	SenderClient

	calls int

	pollReqs []sender.SendPollRequest
	quizArgs []fake96Quiz
	stops    [][2]int64
	messages []fake96Message
	buttons  []fake96Button

	replies []fake96Reply
}

type fake96Quiz struct {
	chatID   int64
	question string
	options  []string
	index    int
}

type fake96Message struct {
	chatID int64
	text   string
	opts   SendOptions
}

type fake96Button struct {
	userID int64
	button tg.KeyboardButton
}

type fake96Reply struct {
	msg  *tg.Message
	poll *tg.Poll
	btn  *tg.PreparedKeyboardButton
	err  error
}

func (f *fake96) next() fake96Reply {
	f.calls++
	if len(f.replies) == 0 {
		panic("fake96: no scripted reply")
	}
	r := f.replies[0]
	f.replies = f.replies[1:]
	return r
}

func (f *fake96) SendPoll(_ context.Context, req sender.SendPollRequest) (*tg.Message, error) {
	f.pollReqs = append(f.pollReqs, req)
	r := f.next()
	return r.msg, r.err
}

func (f *fake96) SendQuiz(_ context.Context, chatID int64, question string, options []string, index int) (*tg.Message, error) {
	f.quizArgs = append(f.quizArgs, fake96Quiz{chatID, question, options, index})
	r := f.next()
	return r.msg, r.err
}

func (f *fake96) StopPoll(_ context.Context, chatID int64, messageID int) (*tg.Poll, error) {
	f.stops = append(f.stops, [2]int64{chatID, int64(messageID)})
	r := f.next()
	return r.poll, r.err
}

func (f *fake96) SendMessage(_ context.Context, chatID int64, text string, opts ...SendOption) (*tg.Message, error) {
	o := SendOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	f.messages = append(f.messages, fake96Message{chatID, text, o})
	r := f.next()
	return r.msg, r.err
}

func (f *fake96) SavePreparedKeyboardButton(_ context.Context, userID int64, button tg.KeyboardButton) (*tg.PreparedKeyboardButton, error) {
	f.buttons = append(f.buttons, fake96Button{userID, button})
	r := f.next()
	return r.btn, r.err
}

const api96MsgID = 4242

func api96Runtime(f *fake96) *Runtime {
	return NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
}

// options with persistent ids that are not index strings (E-0013)
func api96Opts() []tg.PollOption {
	return []tg.PollOption{{Text: "A", PersistentID: "pa"}, {Text: "B", PersistentID: "pb"}, {Text: "C", PersistentID: "pc"}}
}

func api96PollMsg(p *tg.Poll) *tg.Message {
	return &tg.Message{MessageID: api96MsgID, Chat: &tg.Chat{ID: testutil.TestChatID, Type: "supergroup"}, Poll: p}
}

func api96F() *bool { f := false; return &f }

// S47's and S48's requests, as in suites/api96.go (T-tb-1 pins the suite values; package engine cannot import suites).
func api96S47Req() sender.SendPollRequest {
	return sender.SendPollRequest{
		Question: "galigo 9.6 quiz", Options: []sender.InputPollOption{{Text: "A"}, {Text: "B"}, {Text: "C"}},
		Type: "quiz", CorrectOptionIDs: []int{0, 2}, AllowsMultipleAnswers: true, IsAnonymous: api96F(),
	}
}

func api96S48Req() sender.SendPollRequest {
	return sender.SendPollRequest{
		Question: "galigo 9.6 poll", Options: []sender.InputPollOption{{Text: "A"}, {Text: "B"}, {Text: "C"}},
		IsAnonymous: api96F(), AllowsRevoting: api96F(), ShuffleOptions: true, AllowAddingOptions: true,
		HideResultsUntilCloses: true, Description: "galigo 9.6 description", OpenPeriod: 2628000,
	}
}

// the replies that match S47's and S48's requests
func api96S47Poll() *tg.Poll {
	return &tg.Poll{Type: "quiz", Options: api96Opts(), CorrectOptionIDs: []int{0, 2}, AllowsMultipleAnswers: true}
}

func api96S48Poll() *tg.Poll {
	return &tg.Poll{Type: "regular", Options: api96Opts(), AllowsRevoting: false, Description: "galigo 9.6 description"}
}

func api96WithChat(req sender.SendPollRequest) sender.SendPollRequest {
	req.ChatID = testutil.TestChatID
	return req
}

func api96RequestUsersButton() tg.KeyboardButton {
	return tg.KeyboardButton{Text: "galigo 9.6 users", RequestUsers: &tg.KeyboardButtonRequestUsers{RequestID: 96}}
}

// api96Exec runs one step and turns a panic into a test failure (the runner has no recover).
func api96Exec(t *testing.T, step Step, rt *Runtime) (res *StepResult, err error) {
	t.Helper()
	func() {
		defer func() {
			if p := recover(); p != nil {
				t.Errorf("%s panicked: %v", step.Name(), p)
			}
		}()
		res, err = step.Execute(context.Background(), rt)
	}()
	return res, err
}

func api96Tracked() []CreatedMessage {
	return []CreatedMessage{{ChatID: testutil.TestChatID, MessageID: api96MsgID}}
}

// T-tb-4 (a), (c): pass rows. Each step makes exactly one call, records the expected whole request, tracks the
// message, keeps its id, sets LastMessage, StepName and the exact Method.
func TestAPI96Steps_Pass(t *testing.T) {
	t.Run("T-tb-4 (a) SendPoll96Step S47", func(t *testing.T) {
		f := &fake96{replies: []fake96Reply{{msg: api96PollMsg(api96S47Poll())}}}
		rt := api96Runtime(f)
		step := &SendPoll96Step{Req: api96S47Req()}
		res, err := api96Exec(t, step, rt)
		require.NoError(t, err)
		assert.Equal(t, []sender.SendPollRequest{api96WithChat(api96S47Req())}, f.pollReqs)
		assert.Equal(t, &StepResult{StepName: step.Name(), Method: "sendPoll", MessageIDs: []int{api96MsgID}}, api96NoEvidence(res))
		assert.Equal(t, api96Tracked(), rt.CreatedMessages)
		assert.Equal(t, api96PollMsg(api96S47Poll()), rt.LastMessage)
		assert.Equal(t, 1, f.calls, "T-tb-4 (c) one call")
	})
	t.Run("T-tb-4 (a) SendPoll96Step S48", func(t *testing.T) {
		f := &fake96{replies: []fake96Reply{{msg: api96PollMsg(api96S48Poll())}}}
		rt := api96Runtime(f)
		step := &SendPoll96Step{Req: api96S48Req()}
		res, err := api96Exec(t, step, rt)
		require.NoError(t, err)
		assert.Equal(t, []sender.SendPollRequest{api96WithChat(api96S48Req())}, f.pollReqs)
		assert.Equal(t, &StepResult{StepName: step.Name(), Method: "sendPoll", MessageIDs: []int{api96MsgID}}, api96NoEvidence(res))
		assert.Equal(t, api96Tracked(), rt.CreatedMessages)
		assert.Equal(t, api96PollMsg(api96S48Poll()), rt.LastMessage)
		assert.Equal(t, 1, f.calls, "T-tb-4 (c) one call")
	})
	t.Run("T-tb-4 (a) StopPoll96Step", func(t *testing.T) {
		f := &fake96{replies: []fake96Reply{{poll: api96S47Poll()}}}
		rt := api96Runtime(f)
		rt.LastMessage = api96PollMsg(api96S47Poll())
		step := &StopPoll96Step{}
		res, err := api96Exec(t, step, rt)
		require.NoError(t, err)
		assert.Equal(t, [][2]int64{{testutil.TestChatID, api96MsgID}}, f.stops, "T-tb-4 (e) stops LastMessage")
		assert.Equal(t, &StepResult{StepName: step.Name(), Method: "stopPoll"}, api96NoEvidence(res))
		assert.Equal(t, 1, f.calls, "T-tb-4 (c) one call")
	})
	t.Run("T-tb-4 (a) SendQuizIDsStep", func(t *testing.T) {
		quiz := &tg.Poll{Type: "quiz", Options: api96Opts(), CorrectOptionIDs: []int{1}}
		f := &fake96{replies: []fake96Reply{{msg: api96PollMsg(quiz)}}}
		rt := api96Runtime(f)
		step := &SendQuizIDsStep{Index: 1}
		res, err := api96Exec(t, step, rt)
		require.NoError(t, err)
		require.Len(t, f.quizArgs, 1)
		assert.Equal(t, testutil.TestChatID, f.quizArgs[0].chatID)
		assert.Equal(t, 1, f.quizArgs[0].index)
		assert.Equal(t, &StepResult{StepName: step.Name(), Method: "sendPoll", MessageIDs: []int{api96MsgID}}, api96NoEvidence(res))
		assert.Equal(t, api96Tracked(), rt.CreatedMessages)
		assert.Equal(t, api96PollMsg(quiz), rt.LastMessage)
		assert.Equal(t, 1, f.calls, "T-tb-4 (c) one call")
	})
	t.Run("T-tb-4 (a) ReplyToPollOptionStep", func(t *testing.T) {
		reply := &tg.Message{MessageID: api96MsgID, Chat: &tg.Chat{ID: testutil.TestChatID}, ReplyToPollOptionID: "pb"}
		f := &fake96{replies: []fake96Reply{{msg: reply}}}
		rt := api96Runtime(f)
		rt.LastMessage = api96PollMsg(api96S48Poll())
		step := &ReplyToPollOptionStep{}
		res, err := api96Exec(t, step, rt)
		require.NoError(t, err)
		require.Len(t, f.messages, 1)
		assert.Equal(t, testutil.TestChatID, f.messages[0].chatID)
		assert.Equal(t, &tg.ReplyParameters{MessageID: api96MsgID, PollOptionID: "pb"}, f.messages[0].opts.ReplyParameters,
			"T-tb-4 (e) replies to LastMessage's option 1 by its persistent id")
		assert.Equal(t, &StepResult{StepName: step.Name(), Method: "sendMessage", MessageIDs: []int{api96MsgID}}, api96NoEvidence(res))
		assert.Equal(t, api96Tracked(), rt.CreatedMessages)
		assert.Equal(t, reply, rt.LastMessage)
		assert.Equal(t, 1, f.calls, "T-tb-4 (c) one call")
	})
	t.Run("T-tb-4 (a) SavePreparedKeyboardButtonStep", func(t *testing.T) {
		f := &fake96{replies: []fake96Reply{{btn: &tg.PreparedKeyboardButton{ID: "abc"}}}}
		rt := api96Runtime(f)
		step := &SavePreparedKeyboardButtonStep{Button: api96RequestUsersButton()}
		res, err := api96Exec(t, step, rt)
		require.NoError(t, err)
		assert.Equal(t, []fake96Button{{testutil.TestUserID, api96RequestUsersButton()}}, f.buttons,
			"T-tb-4 (e) the admin user (not the chat) and the whole button")
		assert.Equal(t, &StepResult{StepName: step.Name(), Method: "savePreparedKeyboardButton"}, api96NoEvidence(res))
		assert.Equal(t, 1, f.calls, "T-tb-4 (c) one call")
	})
}

// api96NoEvidence returns a copy of res without Evidence and Duration, so a whole-value compare checks the rest.
func api96NoEvidence(res *StepResult) *StepResult {
	if res == nil {
		return nil
	}
	c := *res
	c.Evidence = nil
	c.Duration = 0
	return &c
}

// T-tb-4 (b): one sub-row per check, each changing exactly one field of the matching reply. Each gives that
// check's own named error; the send steps keep the message id in a non-nil result and in the tracking list.
func TestAPI96Steps_Checks(t *testing.T) {
	type row struct {
		name string
		step Step
		last *tg.Message
		rep  fake96Reply
		want error
		send bool
	}
	s47 := func(ids ...int) *tg.Message { p := api96S47Poll(); p.CorrectOptionIDs = ids; return api96PollMsg(p) }
	s48 := func(change func(*tg.Poll)) *tg.Message { p := api96S48Poll(); change(p); return api96PollMsg(p) }
	quiz := func(ids ...int) *tg.Message {
		return api96PollMsg(&tg.Poll{Type: "quiz", Options: api96Opts(), CorrectOptionIDs: ids})
	}
	stopped := func(ids ...int) *tg.Poll { p := api96S47Poll(); p.CorrectOptionIDs = ids; return p }
	rows := []row{
		// S47 wants [0,2]: [0] passes a first-element-only and a subset-only check, [0,1] a length-only check,
		// [0,1,2] a superset-only check.
		{"S47 send [0]", &SendPoll96Step{Req: api96S47Req()}, nil, fake96Reply{msg: s47(0)}, errPoll96CorrectIDs, true},
		{"S47 send [0,1]", &SendPoll96Step{Req: api96S47Req()}, nil, fake96Reply{msg: s47(0, 1)}, errPoll96CorrectIDs, true},
		{"S47 send [0,1,2]", &SendPoll96Step{Req: api96S47Req()}, nil, fake96Reply{msg: s47(0, 1, 2)}, errPoll96CorrectIDs, true},
		{"S48 send allows_revoting true", &SendPoll96Step{Req: api96S48Req()}, nil,
			fake96Reply{msg: s48(func(p *tg.Poll) { p.AllowsRevoting = true })}, errPoll96AllowsRevoting, true},
		{"S48 send other description", &SendPoll96Step{Req: api96S48Req()}, nil,
			fake96Reply{msg: s48(func(p *tg.Poll) { p.Description = "other text" })}, errPoll96Description, true},
		{"S48 send option pb blank", &SendPoll96Step{Req: api96S48Req()}, nil,
			fake96Reply{msg: s48(func(p *tg.Poll) { p.Options[1].PersistentID = "" })}, errPoll96PersistentID, true},
		{"stop [0]", &StopPoll96Step{}, api96PollMsg(api96S47Poll()), fake96Reply{poll: stopped(0)}, errPoll96CorrectIDs, false},
		{"stop [0,1]", &StopPoll96Step{}, api96PollMsg(api96S47Poll()), fake96Reply{poll: stopped(0, 1)}, errPoll96CorrectIDs, false},
		{"stop [0,1,2]", &StopPoll96Step{}, api96PollMsg(api96S47Poll()), fake96Reply{poll: stopped(0, 1, 2)}, errPoll96CorrectIDs, false},
		// SendQuizIDsStep wants [1]: [0] passes a length-only check, [1,2] a first-element-only and a
		// superset-only check, [] a subset-only check.
		{"quiz ids [0]", &SendQuizIDsStep{Index: 1}, nil, fake96Reply{msg: quiz(0)}, errPoll96CorrectIDs, true},
		{"quiz ids [1,2]", &SendQuizIDsStep{Index: 1}, nil, fake96Reply{msg: quiz(1, 2)}, errPoll96CorrectIDs, true},
		{"quiz ids []", &SendQuizIDsStep{Index: 1}, nil, fake96Reply{msg: quiz()}, errPoll96CorrectIDs, true},
		// another non-empty option id, so a non-empty-only check fails
		{"reply option pc", &ReplyToPollOptionStep{}, api96PollMsg(api96S48Poll()),
			fake96Reply{msg: &tg.Message{MessageID: api96MsgID, Chat: &tg.Chat{ID: testutil.TestChatID}, ReplyToPollOptionID: "pc"}},
			errPoll96ReplyOption, true},
	}
	for _, r := range rows {
		t.Run("T-tb-4 (b) "+r.name, func(t *testing.T) {
			f := &fake96{replies: []fake96Reply{r.rep}}
			rt := api96Runtime(f)
			rt.LastMessage = r.last
			res, err := api96Exec(t, r.step, rt)
			require.ErrorIs(t, err, r.want)
			if r.send {
				require.NotNil(t, res, "the result keeps the sent id")
				assert.Equal(t, []int{api96MsgID}, res.MessageIDs)
				assert.Equal(t, api96Tracked(), rt.CreatedMessages)
			}
		})
	}
}

// T-tb-4 (d): a 429 on SendPoll96Step makes the real runner repeat the step once; the report keeps the id and
// the message is tracked once.
func TestAPI96Steps_429Repeat(t *testing.T) {
	f := &fake96{replies: []fake96Reply{
		{err: &tg.APIError{Code: 429, Description: "Too Many Requests", RetryAfter: time.Millisecond}},
		{msg: api96PollMsg(api96S48Poll())},
	}}
	rt := api96Runtime(f)
	runner := NewRunner(rt, RunnerConfig{RetryOn429: true, Max429Retries: 1, MaxMessages: 10},
		slog.New(slog.DiscardHandler))
	res := runner.Run(context.Background(), &BaseScenario{
		ScenarioName: "T-tb-4 (d)", ScenarioSteps: []Step{&SendPoll96Step{Req: api96S48Req()}}, ScenarioTimeout: 10 * time.Second,
	})
	require.True(t, res.Success, "scenario error: %s", res.Error)
	require.Len(t, res.Steps, 1)
	assert.Equal(t, []int{api96MsgID}, res.Steps[0].MessageIDs)
	assert.Equal(t, 2, f.calls)
	assert.Equal(t, api96Tracked(), rt.CreatedMessages)
}

// T-tb-4 (e): sources and guards. A guard gives its own named error, no panic and no call; a send step whose
// reply has no poll keeps and tracks the id; the button step uses the admin user and skips without one.
func TestAPI96Steps_Guards(t *testing.T) {
	noPoll := func() *tg.Message { return api96PollMsg(nil) }
	guards := []struct {
		name string
		step Step
		last *tg.Message
		want error
	}{
		{"stop: no LastMessage", &StopPoll96Step{}, nil, errPoll96NoSource},
		{"stop: LastMessage without poll", &StopPoll96Step{}, noPoll(), errPoll96NoSource},
		{"reply: no LastMessage", &ReplyToPollOptionStep{}, nil, errPoll96NoSource},
		{"reply: LastMessage without poll", &ReplyToPollOptionStep{}, noPoll(), errPoll96NoSource},
		{"reply: 0 options", &ReplyToPollOptionStep{}, api96PollMsg(&tg.Poll{Type: "regular"}), errPoll96FewOptions},
		{"reply: 1 option", &ReplyToPollOptionStep{}, api96PollMsg(&tg.Poll{Type: "regular", Options: api96Opts()[:1]}), errPoll96FewOptions},
	}
	for _, g := range guards {
		t.Run("T-tb-4 (e) "+g.name, func(t *testing.T) {
			f := &fake96{}
			rt := api96Runtime(f)
			rt.LastMessage = g.last
			_, err := api96Exec(t, g.step, rt)
			require.ErrorIs(t, err, g.want)
			assert.Equal(t, 0, f.calls, "no call")
		})
	}

	sends := []struct {
		name string
		step Step
	}{
		{"SendPoll96Step reply without poll", &SendPoll96Step{Req: api96S48Req()}},
		{"SendQuizIDsStep reply without poll", &SendQuizIDsStep{Index: 1}},
	}
	for _, s := range sends {
		t.Run("T-tb-4 (e) "+s.name, func(t *testing.T) {
			f := &fake96{replies: []fake96Reply{{msg: noPoll()}}}
			rt := api96Runtime(f)
			res, err := api96Exec(t, s.step, rt)
			require.ErrorIs(t, err, errPoll96NoPoll)
			require.NotNil(t, res)
			assert.Equal(t, []int{api96MsgID}, res.MessageIDs)
			assert.Equal(t, api96Tracked(), rt.CreatedMessages, "tracked before the poll guard")
		})
	}

	t.Run("T-tb-4 (e) SavePreparedKeyboardButtonStep empty id", func(t *testing.T) {
		f := &fake96{replies: []fake96Reply{{btn: &tg.PreparedKeyboardButton{ID: ""}}}}
		rt := api96Runtime(f)
		_, err := api96Exec(t, &SavePreparedKeyboardButtonStep{Button: api96RequestUsersButton()}, rt)
		require.ErrorIs(t, err, errPreparedButtonNoID)
		assert.Equal(t, 1, f.calls)
	})
	t.Run("T-tb-4 (e) SavePreparedKeyboardButtonStep no admin", func(t *testing.T) {
		f := &fake96{}
		rt := NewRuntime(f, testutil.TestChatID, 0)
		_, err := api96Exec(t, &SavePreparedKeyboardButtonStep{Button: api96RequestUsersButton()}, rt)
		assert.True(t, IsSkip(err), "want a Skip, got %v", err)
		assert.Equal(t, 0, f.calls)
	})
}

// T-tb-4 (f): the checks come from each step's input, not from the suite's constants, and a check runs only
// when its request field is set.
func TestAPI96Steps_ChecksFromInput(t *testing.T) {
	tr := true
	other := sender.SendPollRequest{
		Question: "other", Options: []sender.InputPollOption{{Text: "A"}, {Text: "B"}, {Text: "C"}},
		CorrectOptionIDs: []int{1}, AllowsRevoting: &tr, Description: "other d",
	}
	match := func() *tg.Poll {
		return &tg.Poll{Options: api96Opts(), CorrectOptionIDs: []int{1}, AllowsRevoting: true, Description: "other d"}
	}
	t.Run("T-tb-4 (f) SendPoll96Step other request, matching reply", func(t *testing.T) {
		f := &fake96{replies: []fake96Reply{{msg: api96PollMsg(match())}}}
		_, err := api96Exec(t, &SendPoll96Step{Req: other}, api96Runtime(f))
		assert.NoError(t, err)
	})
	suiteValues := []struct {
		name   string
		change func(*tg.Poll)
		want   error
	}{
		{"correct_option_ids S47's [0,2]", func(p *tg.Poll) { p.CorrectOptionIDs = []int{0, 2} }, errPoll96CorrectIDs},
		{"allows_revoting S48's false", func(p *tg.Poll) { p.AllowsRevoting = false }, errPoll96AllowsRevoting},
		{"description S48's", func(p *tg.Poll) { p.Description = "galigo 9.6 description" }, errPoll96Description},
	}
	for _, sv := range suiteValues {
		t.Run("T-tb-4 (f) SendPoll96Step other request, reply "+sv.name, func(t *testing.T) {
			p := match()
			sv.change(p)
			f := &fake96{replies: []fake96Reply{{msg: api96PollMsg(p)}}}
			_, err := api96Exec(t, &SendPoll96Step{Req: other}, api96Runtime(f))
			assert.ErrorIs(t, err, sv.want)
		})
	}
	t.Run("T-tb-4 (f) SendPoll96Step nothing set, non-default reply", func(t *testing.T) {
		plain := sender.SendPollRequest{Question: "plain", Options: []sender.InputPollOption{{Text: "A"}, {Text: "B"}}}
		reply := &tg.Poll{Options: api96Opts()[:2], CorrectOptionIDs: []int{0}, AllowsRevoting: true, Description: "x"}
		f := &fake96{replies: []fake96Reply{{msg: api96PollMsg(reply)}}}
		_, err := api96Exec(t, &SendPoll96Step{Req: plain}, api96Runtime(f))
		assert.NoError(t, err)
	})
	for _, c := range []struct {
		stop []int
		want error
	}{{[]int{1}, nil}, {[]int{0, 2}, errPoll96CorrectIDs}} {
		t.Run(fmt.Sprintf("T-tb-4 (f) StopPoll96Step LastMessage [1], stop reply %v", c.stop), func(t *testing.T) {
			f := &fake96{replies: []fake96Reply{{poll: &tg.Poll{Options: api96Opts(), CorrectOptionIDs: c.stop}}}}
			rt := api96Runtime(f)
			rt.LastMessage = api96PollMsg(&tg.Poll{Type: "quiz", Options: api96Opts(), CorrectOptionIDs: []int{1}})
			_, err := api96Exec(t, &StopPoll96Step{}, rt)
			api96AssertErr(t, err, c.want)
		})
	}
	for _, c := range []struct {
		ids  []int
		want error
	}{{[]int{2}, nil}, {[]int{1}, errPoll96CorrectIDs}} {
		t.Run(fmt.Sprintf("T-tb-4 (f) SendQuizIDsStep Index 2, reply %v", c.ids), func(t *testing.T) {
			f := &fake96{replies: []fake96Reply{{msg: api96PollMsg(&tg.Poll{Type: "quiz", Options: api96Opts(), CorrectOptionIDs: c.ids})}}}
			rt := api96Runtime(f)
			_, err := api96Exec(t, &SendQuizIDsStep{Index: 2}, rt)
			api96AssertErr(t, err, c.want)
			if assert.Len(t, f.quizArgs, 1) {
				assert.Equal(t, 2, f.quizArgs[0].index)
			}
		})
	}
}

func api96AssertErr(t *testing.T, err, want error) {
	t.Helper()
	if want == nil {
		assert.NoError(t, err)
		return
	}
	assert.True(t, errors.Is(err, want), "want %v, got %v", want, err)
}
