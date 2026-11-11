package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-30 (+B-55): the runner cleans up what a stopped or panicking scenario left, and CleanupStep keeps and reports
// failed items. Rows of the B-30 TEST-PLAN §2 (R, O rows) and §3 (C, D rows).

const b30Chat = int64(-1001) // a group chat (fake id)

// b30Call is one cleanup call the fake saw, with its context state at the call (E-0123).
type b30Call struct {
	key         string // "msg:<chat>/<id>", "set:<name>", "tag:<chat>/<user>/<tag>"
	at          time.Time
	ctxErr      error
	deadline    time.Time
	hasDeadline bool
}

// b30Fake records every DeleteMessage, DeleteStickerSet and SetChatMemberTag call in one ordered log. Errors are
// scripted per item key: script[key] is a queue (each call pops one; empty = success), always[key] fails every call.
// It also answers SendDice (message 77) and SendPhoto (message 702 without caption entities). Any other call panics
// on the nil embedded interface.
type b30Fake struct {
	SenderClient
	log    []b30Call
	script map[string][]error
	always map[string]error
}

func newB30Fake() *b30Fake { return &b30Fake{script: map[string][]error{}, always: map[string]error{}} }

func (f *b30Fake) call(ctx context.Context, key string) error {
	c := b30Call{key: key, at: time.Now(), ctxErr: ctx.Err()}
	c.deadline, c.hasDeadline = ctx.Deadline()
	f.log = append(f.log, c)
	if err, ok := f.always[key]; ok {
		return err
	}
	if q := f.script[key]; len(q) > 0 {
		f.script[key] = q[1:]
		return q[0]
	}
	return nil
}

func (f *b30Fake) DeleteMessage(ctx context.Context, chatID int64, messageID int) error {
	return f.call(ctx, fmt.Sprintf("msg:%d/%d", chatID, messageID))
}

func (f *b30Fake) DeleteStickerSet(ctx context.Context, name string) error {
	return f.call(ctx, "set:"+name)
}

func (f *b30Fake) SetChatMemberTag(ctx context.Context, chatID, userID int64, tag string) error {
	return f.call(ctx, fmt.Sprintf("tag:%d/%d/%s", chatID, userID, tag))
}

func (f *b30Fake) SendDice(_ context.Context, chatID int64, _ string) (*tg.Message, error) {
	return &tg.Message{MessageID: 77, Chat: &tg.Chat{ID: chatID}}, nil
}

func (f *b30Fake) SendPhoto(_ context.Context, chatID int64, _ MediaInput, _ ...SendOption) (*tg.Message, error) {
	return &tg.Message{MessageID: 702, Chat: &tg.Chat{ID: chatID}, Caption: "c"}, nil
}

// b30FakeEnt answers SendPhoto with message 703 that has a bold caption entity (the S36 success path).
type b30FakeEnt struct{ *b30Fake }

func (f b30FakeEnt) SendPhoto(_ context.Context, chatID int64, _ MediaInput, _ ...SendOption) (*tg.Message, error) {
	return &tg.Message{MessageID: 703, Chat: &tg.Chat{ID: chatID}, Caption: "c",
		CaptionEntities: []tg.MessageEntity{{Type: "bold", Offset: 0, Length: 1}}}, nil
}

func (f *b30Fake) keys() []string {
	out := make([]string, len(f.log))
	for i, c := range f.log {
		out[i] = c.key
	}
	return out
}

func msgKeys(ids ...int) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = fmt.Sprintf("msg:%d/%d", b30Chat, id)
	}
	return out
}

// b30Track tracks ids in rt.ChatID, sets LastMessage to the last one, calls cancel when set, and returns err.
type b30Track struct {
	ids    []int
	err    error
	cancel context.CancelFunc
}

func (s *b30Track) Name() string { return "track" }
func (s *b30Track) Execute(_ context.Context, rt *Runtime) (*StepResult, error) {
	for _, id := range s.ids {
		rt.TrackMessage(rt.ChatID, id)
		rt.LastMessage = &tg.Message{MessageID: id, Chat: &tg.Chat{ID: rt.ChatID}}
	}
	if s.cancel != nil {
		s.cancel()
	}
	return &StepResult{StepName: "track", MessageIDs: s.ids}, s.err
}

type b30Set struct{ name string }

func (s *b30Set) Name() string { return "set" }
func (s *b30Set) Execute(_ context.Context, rt *Runtime) (*StepResult, error) {
	rt.TrackStickerSet(s.name)
	return &StepResult{StepName: "set"}, nil
}

type b30Wait struct{}

func (s *b30Wait) Name() string { return "wait" }
func (s *b30Wait) Execute(ctx context.Context, _ *Runtime) (*StepResult, error) {
	<-ctx.Done()
	return &StepResult{StepName: "wait"}, nil
}

type b30Panic struct{ id int }

func (s *b30Panic) Name() string { return "panic" }
func (s *b30Panic) Execute(_ context.Context, rt *Runtime) (*StepResult, error) {
	rt.TrackMessage(rt.ChatID, s.id)
	panic("b30-boom")
}

var errB30 = errors.New("boom")

func b30Scenario(timeout time.Duration, steps ...Step) Scenario {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &BaseScenario{ScenarioName: "b30", ScenarioSteps: steps, ScenarioTimeout: timeout}
}

func b30Runner(f *b30Fake, cfg RunnerConfig) (*Runner, *Runtime) {
	if cfg.MaxMessages == 0 {
		cfg.MaxMessages = 100
	}
	rt := NewRuntime(f, b30Chat, 555)
	return NewRunner(rt, cfg, slog.New(slog.DiscardHandler)), rt
}

func lastStep(res *ScenarioResult) StepResult { return res.Steps[len(res.Steps)-1] }

func evMap(t *testing.T, s StepResult) map[string]any {
	t.Helper()
	ev, ok := s.Evidence.(map[string]any)
	require.True(t, ok, "evidence is %T", s.Evidence)
	return ev
}

func hasCleanupAfterStop(res *ScenarioResult) bool {
	for _, s := range res.Steps {
		if s.StepName == "cleanup after stop" {
			return true
		}
	}
	return false
}

// requireCleanOK checks the last step is a successful "cleanup after stop" that deleted n messages and nothing else.
func requireCleanOK(t *testing.T, res *ScenarioResult, n int) {
	t.Helper()
	s := lastStep(res)
	require.Equal(t, "cleanup after stop", s.StepName)
	assert.True(t, s.Success)
	assert.Empty(t, s.Error)
	assert.Equal(t, map[string]any{"deleted_count": n}, evMap(t, s))
}

// ---- §2 runner stop paths ----

func TestB30_R1_FailedStep(t *testing.T) {
	f := newB30Fake()
	r, rt := b30Runner(f, RunnerConfig{})
	res := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{501}, err: errB30}, &b30Track{ids: []int{502}}))
	assert.Equal(t, msgKeys(501), f.keys())
	require.Len(t, res.Steps, 2)
	requireCleanOK(t, res, 1)
	assert.False(t, res.Success)
	assert.Equal(t, `step "track" failed: boom`, res.Error)
	assert.Empty(t, rt.CreatedMessages)
	assert.Nil(t, rt.LastMessage)
}

func TestB30_R2_Skip(t *testing.T) {
	f := newB30Fake()
	r, _ := b30Runner(f, RunnerConfig{})
	res := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{501}, err: Skip("x")}, &b30Track{ids: []int{502}}))
	assert.Equal(t, msgKeys(501), f.keys())
	requireCleanOK(t, res, 1)
	assert.True(t, res.Success)
	assert.True(t, res.Skipped)
	assert.Equal(t, "skipped: x", res.SkipReason)
	assert.Empty(t, res.Error)
}

func TestB30_R3_Budget(t *testing.T) {
	f := newB30Fake()
	r, _ := b30Runner(f, RunnerConfig{MaxMessages: 1})
	res := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{501}}, &b30Track{ids: []int{502}}))
	assert.Equal(t, msgKeys(501), f.keys(), "step 2 must not run; 501 deleted once")
	requireCleanOK(t, res, 1)
	assert.Equal(t, "message budget exceeded (1)", res.Error)
}

func TestB30_R4_ScenarioTimeout(t *testing.T) {
	f := newB30Fake()
	r, _ := b30Runner(f, RunnerConfig{})
	res := r.Run(context.Background(), b30Scenario(20*time.Millisecond, &b30Track{ids: []int{501}}, &b30Wait{}, &b30Track{ids: []int{503}}))
	require.Equal(t, msgKeys(501), f.keys())
	c := f.log[0]
	assert.NoError(t, c.ctxErr, "the cleanup must not use the expired scenario context")
	require.True(t, c.hasDeadline, "each cleanup call has its own bound")
	d := c.deadline.Sub(c.at)
	assert.True(t, d <= 15*time.Second && d > 14*time.Second, "per-call bound %s", d)
	requireCleanOK(t, res, 1)
	assert.Equal(t, context.DeadlineExceeded.Error(), res.Error)
}

func TestB30_R5_CancelledParent(t *testing.T) {
	f := newB30Fake()
	r, _ := b30Runner(f, RunnerConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	res := r.Run(ctx, b30Scenario(0, &b30Track{ids: []int{501}}, &b30Track{cancel: cancel}, &b30Track{ids: []int{503}}))
	require.Equal(t, msgKeys(501), f.keys())
	assert.NoError(t, f.log[0].ctxErr)
	requireCleanOK(t, res, 1)
	assert.Equal(t, "context canceled", res.Error)
}

func TestB30_R6_PaceError(t *testing.T) {
	f := newB30Fake()
	r, _ := b30Runner(f, RunnerConfig{BaseDelay: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	start := time.Now()
	res := r.Run(ctx, b30Scenario(0, &b30Track{ids: []int{501}, cancel: cancel}))
	assert.Less(t, time.Since(start), 5*time.Second)
	require.Equal(t, msgKeys(501), f.keys())
	assert.NoError(t, f.log[0].ctxErr)
	requireCleanOK(t, res, 1)
	assert.Equal(t, "context canceled", res.Error)
}

// Guard (passes on the base): a scenario whose steps all succeed is not cleaned (AC2).
func TestB30_R7_NormalRunLeavesItem(t *testing.T) {
	f := newB30Fake()
	r, rt := b30Runner(f, RunnerConfig{})
	res := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{501}}))
	assert.Empty(t, f.log)
	assert.Len(t, res.Steps, 1)
	assert.True(t, res.Success)
	assert.Equal(t, []CreatedMessage{{ChatID: b30Chat, MessageID: 501}}, rt.CreatedMessages)
}

// Guard: a stopped scenario with nothing tracked gets no call and no extra step (AC2).
func TestB30_R8_StopNothingTracked(t *testing.T) {
	f := newB30Fake()
	r, _ := b30Runner(f, RunnerConfig{})
	res := r.Run(context.Background(), b30Scenario(0, &b30Track{err: errB30}))
	assert.Empty(t, f.log)
	assert.Len(t, res.Steps, 1)
	assert.False(t, hasCleanupAfterStop(res))
}

func TestB30_R9_CleanupFailsAfterFailedStep(t *testing.T) {
	f := newB30Fake()
	apiErr := &tg.APIError{Code: 502, Description: "del-501"}
	f.always["msg:-1001/501"] = apiErr
	r, rt := b30Runner(f, RunnerConfig{})
	res := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{501}, err: errB30}))
	line := "message -1001/501: " + apiErr.Error()
	assert.Equal(t, msgKeys(501), f.keys(), "one cleanup attempt only")
	assert.False(t, res.Success)
	assert.Equal(t, `step "track" failed: boom; cleanup after stop: `+line, res.Error)
	s := lastStep(res)
	require.Equal(t, "cleanup after stop", s.StepName)
	assert.False(t, s.Success)
	assert.Equal(t, line, s.Error)
	assert.Equal(t, map[string]any{"deleted_count": 0, "failed": []string{line}}, evMap(t, s))
	assert.Equal(t, []CreatedMessage{{ChatID: b30Chat, MessageID: 501}}, rt.CreatedMessages, "transient: kept")
}

func TestB30_R10_CleanupFailsAfterSkip(t *testing.T) {
	f := newB30Fake()
	f.always["msg:-1001/501"] = errors.New("net-501")
	r, _ := b30Runner(f, RunnerConfig{})
	res := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{501}, err: Skip("x")}))
	assert.False(t, res.Success)
	assert.True(t, res.Skipped)
	assert.Equal(t, "skipped: x", res.SkipReason)
	assert.Equal(t, "cleanup after stop: message -1001/501: net-501", res.Error)
}

func TestB30_R11_EarlierScenarioLeftovers(t *testing.T) {
	f := newB30Fake()
	r, _ := b30Runner(f, RunnerConfig{})
	r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{500}}))
	res := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{501}, err: errB30}))
	assert.Equal(t, msgKeys(500, 501), f.keys())
	requireCleanOK(t, res, 2)
}

func TestB30_R13_Panic(t *testing.T) {
	f := newB30Fake()
	r, rt := b30Runner(f, RunnerConfig{})
	require.PanicsWithValue(t, "b30-boom", func() {
		r.Run(context.Background(), b30Scenario(0, &b30Panic{id: 501}))
	})
	assert.Equal(t, msgKeys(501), f.keys())
	assert.Empty(t, rt.CreatedMessages)
}

func api429(d time.Duration) error {
	return &tg.APIError{Code: 429, Description: "Too Many Requests", RetryAfter: d}
}

func TestB30_R14_429OnCleanupStep(t *testing.T) {
	f := newB30Fake()
	f.script["msg:-1001/501"] = []error{api429(time.Millisecond)}
	r, _ := b30Runner(f, RunnerConfig{RetryOn429: true, Max429Retries: 1})
	res := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{500, 501}}, &CleanupStep{}))
	assert.Equal(t, msgKeys(500, 501, 501), f.keys())
	assert.True(t, res.Success)
	require.Len(t, res.Steps, 2)
	assert.False(t, hasCleanupAfterStop(res))
	assert.Equal(t, map[string]any{"deleted_count": 2}, evMap(t, res.Steps[1]))
}

func apiNotFound() error {
	return &tg.APIError{Code: 400, Description: "Bad Request: message to delete not found"}
}

func TestB30_R15_PermanentDropped(t *testing.T) {
	f := newB30Fake()
	f.always["msg:-1001/1"] = apiNotFound()
	r, rt := b30Runner(f, RunnerConfig{})
	a := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{1}}, &CleanupStep{}))
	assert.False(t, a.Success)
	s := lastStep(a)
	require.Equal(t, "cleanup after stop", s.StepName)
	failed, _ := evMap(t, s)["failed"].([]string)
	require.Len(t, failed, 1)
	assert.Contains(t, failed[0], "message -1001/1: ")
	assert.Empty(t, rt.CreatedMessages)
	b := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{2}}, &CleanupStep{}))
	assert.True(t, b.Success)
	assert.Len(t, b.Steps, 2)
	assert.Equal(t, msgKeys(1, 1, 2), f.keys())
}

func TestB30_R15b_PermanentDroppedSetAndTag(t *testing.T) {
	f := newB30Fake()
	f.always["msg:-1001/1"] = apiNotFound()
	f.always["set:a"] = &tg.APIError{Code: 400, Description: "Bad Request: STICKERSET_INVALID"}
	f.always["tag:-1001/555/"] = &tg.APIError{Code: 400, Description: "Bad Request: USER_NOT_PARTICIPANT"}
	r, rt := b30Runner(f, RunnerConfig{})
	a := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{1}}, &b30Set{name: "a"},
		&SetChatMemberTagStep{Tag: "t"}, &CleanupStep{}))
	assert.False(t, a.Success)
	failed, _ := evMap(t, lastStep(a))["failed"].([]string)
	require.Len(t, failed, 3)
	assert.Contains(t, failed[0], "message -1001/1: ")
	assert.Contains(t, failed[1], "sticker set a: ")
	assert.Contains(t, failed[2], "member tag -1001/555: ")
	assert.Equal(t, strings.Join(failed, "\n"), lastStep(a).Error, "the errors.Join text, one item per line")
	assert.Empty(t, rt.CreatedMessages)
	assert.Empty(t, rt.CreatedStickerSets)
	n := len(f.log)
	b := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{2}}, &CleanupStep{}))
	assert.True(t, b.Success)
	assert.Equal(t, msgKeys(2), f.keys()[n:], "nothing dropped is tried again")
}

func TestB30_R17_TransientKept(t *testing.T) {
	for name, err := range map[string]error{
		"non-API": errors.New("circuit breaker is open"),
		"502":     &tg.APIError{Code: 502, Description: "Bad Gateway"},
		"429":     api429(time.Millisecond),
	} {
		t.Run(name, func(t *testing.T) {
			f := newB30Fake()
			f.script["msg:-1001/1"] = []error{err}
			r, rt := b30Runner(f, RunnerConfig{})
			a := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{1}, err: errB30}))
			assert.False(t, a.Success)
			assert.Equal(t, []CreatedMessage{{ChatID: b30Chat, MessageID: 1}}, rt.CreatedMessages)
			b := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{2}}, &CleanupStep{}))
			assert.True(t, b.Success)
			assert.Equal(t, msgKeys(1, 1, 2), f.keys())
		})
	}
}

// [t3a] O-T3-01: a failed after-stop sticker-set delete or tag clear stays tracked; the next CleanupStep retries it.
func TestB30_R17_TransientKeptSetAndTag(t *testing.T) {
	for name, c := range map[string]struct {
		key  string
		step Step
		ev   map[string]any
	}{
		"set": {"set:a", &b30Set{name: "a"}, map[string]any{"deleted_count": 0, "sticker_sets_deleted": 1}},
		"tag": {"tag:-1001/555/", &SetChatMemberTagStep{Tag: "t"}, map[string]any{"deleted_count": 0, "tags_cleared": 1}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newB30Fake()
			f.script[c.key] = []error{errors.New("temporary")}
			r, _ := b30Runner(f, RunnerConfig{})
			a := r.Run(context.Background(), b30Scenario(0, c.step, &b30Track{err: errB30}))
			assert.False(t, a.Success)
			assert.False(t, lastStep(a).Success, "the after-stop call failed")
			n := len(f.log)
			b := r.Run(context.Background(), b30Scenario(0, &CleanupStep{}))
			require.True(t, b.Success, b.Error)
			assert.Equal(t, []string{c.key}, f.keys()[n:], "the kept item is retried once")
			assert.Equal(t, c.ev, evMap(t, lastStep(b)))
		})
	}
}

// [t3a] A-T3-01: the carry adds up over two failed attempts (TESTBOT_MAX_429_RETRIES defaults to 2).
func TestB30_R23_CarryOverTwoFailedAttempts(t *testing.T) {
	f := newB30Fake()
	f.script["msg:-1001/2"] = []error{api429(time.Millisecond), api429(time.Millisecond)}
	r, _ := b30Runner(f, RunnerConfig{RetryOn429: true, Max429Retries: 2})
	res := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{1, 2, 3}}, &CleanupStep{}))
	require.True(t, res.Success, res.Error)
	assert.Equal(t, msgKeys(1, 2, 3, 2, 2), f.keys())
	assert.Equal(t, map[string]any{"deleted_count": 3}, evMap(t, res.Steps[1]))
}

func TestB30_R18_CarryAcrossRepeat(t *testing.T) {
	f := newB30Fake()
	f.script["msg:-1001/3"] = []error{api429(time.Millisecond)}
	r, _ := b30Runner(f, RunnerConfig{RetryOn429: true, Max429Retries: 1})
	a := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{1, 2, 3}}, &b30Set{name: "a"},
		&SetChatMemberTagStep{Tag: "t"}, &CleanupStep{}))
	require.True(t, a.Success, a.Error)
	assert.Equal(t, map[string]any{"deleted_count": 3, "sticker_sets_deleted": 1, "tags_cleared": 1}, evMap(t, lastStep(a)))
	b := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{4}}, &CleanupStep{}))
	require.True(t, b.Success, b.Error)
	assert.Equal(t, map[string]any{"deleted_count": 1}, evMap(t, lastStep(b)))
}

func TestB30_R20_FailedCleanupThenNextScenario(t *testing.T) {
	f := newB30Fake()
	f.script["msg:-1001/2"] = []error{errors.New("net-2")}
	r, _ := b30Runner(f, RunnerConfig{})
	a := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{1, 2}}, &CleanupStep{}))
	assert.False(t, a.Success)
	requireCleanOK(t, a, 1)
	b := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{4}}, &CleanupStep{}))
	require.True(t, b.Success, b.Error)
	assert.Equal(t, map[string]any{"deleted_count": 1}, evMap(t, lastStep(b)), "the carry of A's failed CleanupStep must not reach B")
	assert.Equal(t, msgKeys(1, 2, 2, 4), f.keys())
}

func TestB30_R21_OnlyStickerSetTracked(t *testing.T) {
	f := newB30Fake()
	r, rt := b30Runner(f, RunnerConfig{})
	res := r.Run(context.Background(), b30Scenario(0, &b30Set{name: "a"}, &b30Track{err: errB30}))
	assert.Equal(t, []string{"set:a"}, f.keys())
	assert.Empty(t, rt.CreatedStickerSets)
	s := lastStep(res)
	require.Equal(t, "cleanup after stop", s.StepName)
	assert.True(t, s.Success)
	assert.Equal(t, map[string]any{"deleted_count": 0, "sticker_sets_deleted": 1}, evMap(t, s))
}

// R19: the real sender.Client with its default per-chat limiter (a group gets about one call per 3 s) against a
// local server: every one of 15 tracked messages gets its request (a single bound for all calls would cut it off).
func TestB30_R19_RealSenderGroupLimiter(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/deleteMessage") {
			n.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()
	c, err := sender.New("123456:FAKE-test-token-not-real", sender.WithBaseURL(srv.URL))
	require.NoError(t, err)
	defer c.Close()
	chat := int64(-1001234567890)
	rt := NewRuntime(NewSenderAdapter(c), chat, 0)
	ids := make([]int, 15)
	for i := range ids {
		ids[i] = 1000 + i
	}
	r := NewRunner(rt, RunnerConfig{MaxMessages: 100}, slog.New(slog.DiscardHandler))
	res := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: ids, err: errB30}))
	assert.Equal(t, int32(15), n.Load())
	requireCleanOK(t, res, 15)
	assert.Empty(t, rt.CreatedMessages)
}

func TestB30_R16_S36PhotoCleaned(t *testing.T) {
	f := newB30Fake()
	r, rt := b30Runner(f, RunnerConfig{})
	step := &SendPhotoWithParseModeStep{Photo: MediaInput{FileID: "fake-photo", Type: "photo"}, Caption: "<b>c</b>", ParseMode: tg.ParseMode("HTML")}
	res := r.Run(context.Background(), b30Scenario(0, step))
	require.Len(t, res.Steps, 2)
	s := res.Steps[0]
	assert.False(t, s.Success)
	assert.Equal(t, "sendPhoto", s.StepName)
	assert.Equal(t, []int{702}, s.MessageIDs)
	assert.Equal(t, msgKeys(702), f.keys())
	requireCleanOK(t, res, 1)
	assert.Empty(t, rt.CreatedMessages)
}

// ---- O rows: 429-first ordering of CleanupStep's joined errors (AC5, O-P3-01) ----

// [t3a] A-T3-03: the S36 success path keeps LastMessage and tracks the photo after the move.
func TestB30_R16b_S36PhotoSuccess(t *testing.T) {
	f := newB30Fake()
	rt := NewRuntime(b30FakeEnt{f}, b30Chat, 555)
	step := &SendPhotoWithParseModeStep{Photo: MediaInput{FileID: "fake-photo", Type: "photo"}, Caption: "<b>c</b>", ParseMode: tg.ParseMode("HTML")}
	_, err := step.Execute(context.Background(), rt)
	require.NoError(t, err)
	require.NotNil(t, rt.LastMessage)
	assert.Equal(t, 703, rt.LastMessage.MessageID)
	assert.Equal(t, []CreatedMessage{{ChatID: b30Chat, MessageID: 703}}, rt.CreatedMessages)
}

func TestB30_O1_500Then429(t *testing.T) {
	f := newB30Fake()
	f.script["msg:-1001/1"] = []error{&tg.APIError{Code: 500, Description: "Internal Server Error"}}
	f.script["msg:-1001/2"] = []error{api429(time.Millisecond)}
	r, _ := b30Runner(f, RunnerConfig{RetryOn429: true, Max429Retries: 1})
	res := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{1, 2}}, &CleanupStep{}))
	assert.True(t, res.Success, res.Error)
	assert.Equal(t, msgKeys(1, 2, 1, 2), f.keys())
	assert.False(t, hasCleanupAfterStop(res))

	// the same script, one direct call: the joined error yields the 429 first; the evidence keeps item order
	f2 := newB30Fake()
	f2.script["msg:-1001/1"] = []error{&tg.APIError{Code: 500, Description: "Internal Server Error"}}
	f2.script["msg:-1001/2"] = []error{api429(time.Millisecond)}
	rt := NewRuntime(f2, b30Chat, 0)
	rt.TrackMessage(b30Chat, 1)
	rt.TrackMessage(b30Chat, 2)
	sr, err := (&CleanupStep{}).Execute(context.Background(), rt)
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 429, apiErr.Code)
	failed, _ := evMap(t, *sr)["failed"].([]string)
	require.Len(t, failed, 2)
	assert.True(t, strings.HasPrefix(failed[0], "message -1001/1: "))
	assert.True(t, strings.HasPrefix(failed[1], "message -1001/2: "))
}

func TestB30_O2_429Then500(t *testing.T) {
	f := newB30Fake()
	f.script["msg:-1001/1"] = []error{api429(time.Millisecond)}
	f.script["msg:-1001/2"] = []error{&tg.APIError{Code: 500, Description: "Internal Server Error"}}
	r, _ := b30Runner(f, RunnerConfig{RetryOn429: true, Max429Retries: 1})
	res := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{1, 2}}, &CleanupStep{}))
	assert.True(t, res.Success, res.Error)
	assert.Equal(t, msgKeys(1, 2, 1, 2), f.keys())
}

func TestB30_O3_400Then429(t *testing.T) {
	f := newB30Fake()
	f.always["msg:-1001/1"] = &tg.APIError{Code: 400, Description: "Bad Request: message can't be deleted"}
	f.script["msg:-1001/2"] = []error{api429(time.Millisecond)}
	r, rt := b30Runner(f, RunnerConfig{RetryOn429: true, Max429Retries: 1})
	res := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{1, 2}}, &CleanupStep{}))
	assert.Equal(t, msgKeys(1, 2, 1, 2, 1), f.keys())
	assert.False(t, res.Success)
	s := lastStep(res)
	require.Equal(t, "cleanup after stop", s.StepName)
	failed, _ := evMap(t, s)["failed"].([]string)
	require.Len(t, failed, 1)
	assert.Contains(t, failed[0], "message -1001/1: ")
	assert.Empty(t, rt.CreatedMessages)
}

func TestB30_O4_LongestRetryAfterFirst(t *testing.T) {
	f := newB30Fake()
	f.script["msg:-1001/1"] = []error{api429(time.Millisecond)}
	f.script["msg:-1001/2"] = []error{api429(300 * time.Millisecond)}
	r, _ := b30Runner(f, RunnerConfig{RetryOn429: true, Max429Retries: 1})
	res := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{1, 2}}, &CleanupStep{}))
	assert.True(t, res.Success, res.Error)
	require.Equal(t, msgKeys(1, 2, 1, 2), f.keys())
	assert.GreaterOrEqual(t, f.log[2].at.Sub(f.log[0].at), 800*time.Millisecond, "the runner must wait the longest RetryAfter")

	f2 := newB30Fake()
	f2.script["msg:-1001/1"] = []error{api429(time.Millisecond)}
	f2.script["msg:-1001/2"] = []error{api429(300 * time.Millisecond)}
	_, err := (&CleanupStep{}).Execute(context.Background(), b30RT(f2, 1, 2))
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 300*time.Millisecond, apiErr.RetryAfter)
}

// ---- §3 CleanupStep and deleteTracked ----

// [t3] A-T2-02: a 429 without RetryAfter (sender builds it when retry_after is missing) still goes first. Guard: the
// base returns the last error, which is this 429.
func TestB30_O5_429WithoutRetryAfterFirst(t *testing.T) {
	f := newB30Fake()
	f.script["msg:-1001/1"] = []error{&tg.APIError{Code: 500, Description: "Internal Server Error"}}
	f.script["msg:-1001/2"] = []error{&tg.APIError{Code: 429, Description: "Too Many Requests"}}
	rt := NewRuntime(f, b30Chat, 0)
	rt.TrackMessage(b30Chat, 1)
	rt.TrackMessage(b30Chat, 2)
	_, err := (&CleanupStep{}).Execute(context.Background(), rt)
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 429, apiErr.Code)
}

// [t3] A-T2-03: two CleanupSteps in one scenario; the first is repeated after a 429. The second counts only its own work.
func TestB30_R22_TwoCleanupStepsOneScenario(t *testing.T) {
	f := newB30Fake()
	f.script["msg:-1001/2"] = []error{api429(time.Millisecond)}
	r, _ := b30Runner(f, RunnerConfig{RetryOn429: true, Max429Retries: 1})
	res := r.Run(context.Background(), b30Scenario(0, &b30Track{ids: []int{1, 2}}, &CleanupStep{}, &b30Track{ids: []int{3}}, &CleanupStep{}))
	require.True(t, res.Success, res.Error)
	require.Len(t, res.Steps, 4)
	assert.Equal(t, map[string]any{"deleted_count": 2}, evMap(t, res.Steps[1]))
	assert.Equal(t, map[string]any{"deleted_count": 1}, evMap(t, res.Steps[3]))
	assert.Equal(t, msgKeys(1, 2, 2, 3), f.keys())
}

func b30RT(f *b30Fake, ids ...int) *Runtime {
	rt := NewRuntime(f, b30Chat, 555)
	for _, id := range ids {
		rt.TrackMessage(b30Chat, id)
	}
	return rt
}

func TestB30_C1_PartialFailureKept(t *testing.T) {
	f := newB30Fake()
	f.always["msg:-1001/501"] = errors.New("del-501")
	rt := b30RT(f, 500, 501)
	rt.LastMessage = &tg.Message{MessageID: 501, Chat: &tg.Chat{ID: b30Chat}}
	sr, err := (&CleanupStep{}).Execute(context.Background(), rt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message -1001/501: del-501")
	assert.Equal(t, []CreatedMessage{{ChatID: b30Chat, MessageID: 501}}, rt.CreatedMessages)
	require.NotNil(t, rt.LastMessage)
	assert.Equal(t, 501, rt.LastMessage.MessageID)
	assert.Equal(t, map[string]any{"deleted_count": 1, "failed": []string{"message -1001/501: del-501"}}, evMap(t, *sr))
}

// Guard: the success path is unchanged.
func TestB30_C2_AllSucceed(t *testing.T) {
	f := newB30Fake()
	rt := b30RT(f, 500, 501)
	rt.LastMessage = &tg.Message{MessageID: 501, Chat: &tg.Chat{ID: b30Chat}}
	sr, err := (&CleanupStep{}).Execute(context.Background(), rt)
	require.NoError(t, err)
	assert.Empty(t, rt.CreatedMessages)
	assert.Nil(t, rt.LastMessage)
	assert.Equal(t, map[string]any{"deleted_count": 2}, evMap(t, *sr))
}

func TestB30_C3_KeptInOrder(t *testing.T) {
	f := newB30Fake()
	e500, e502 := errors.New("e500"), errors.New("e502")
	f.always["msg:-1001/500"] = e500
	f.always["msg:-1001/502"] = e502
	rt := b30RT(f, 500, 501, 502)
	sr, err := (&CleanupStep{}).Execute(context.Background(), rt)
	require.Error(t, err)
	// [t3] O-T2-01: the returned error wraps every item's error, one line per item
	assert.ErrorIs(t, err, e500)
	assert.ErrorIs(t, err, e502)
	assert.Equal(t, "cleanup: message -1001/500: e500\nmessage -1001/502: e502", err.Error())
	assert.Equal(t, []CreatedMessage{{ChatID: b30Chat, MessageID: 500}, {ChatID: b30Chat, MessageID: 502}}, rt.CreatedMessages)
	assert.Equal(t, []string{"message -1001/500: e500", "message -1001/502: e502"}, evMap(t, *sr)["failed"])
}

func TestB30_C4_StickerSetFailureKept(t *testing.T) {
	f := newB30Fake()
	f.always["set:a"] = errors.New("set-a")
	f.always["set:c"] = errors.New("set-c") // [t3a] A-T3-04: two failures, kept in order
	rt := b30RT(f)
	rt.TrackStickerSet("a")
	rt.TrackStickerSet("b")
	rt.TrackStickerSet("c")
	sr, err := (&CleanupStep{}).Execute(context.Background(), rt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sticker set a: set-a")
	assert.Contains(t, err.Error(), "sticker set c: set-c")
	assert.Equal(t, []string{"a", "c"}, rt.CreatedStickerSets)
	assert.Equal(t, 1, evMap(t, *sr)["sticker_sets_deleted"])
}

func TestB30_C5_RepeatRetriesOnlyFailures(t *testing.T) {
	f := newB30Fake()
	f.script["msg:-1001/501"] = []error{errors.New("del-501")}
	rt := b30RT(f, 500, 501)
	_, err := (&CleanupStep{}).Execute(context.Background(), rt)
	require.Error(t, err)
	_, err = (&CleanupStep{}).Execute(context.Background(), rt)
	require.NoError(t, err)
	assert.Equal(t, msgKeys(500, 501, 501), f.keys())
	assert.Empty(t, rt.CreatedMessages)
}

// Guard: nothing tracked, no call.
func TestB30_C6_NothingTracked(t *testing.T) {
	f := newB30Fake()
	sr, err := (&CleanupStep{}).Execute(context.Background(), b30RT(f))
	require.NoError(t, err)
	assert.Empty(t, f.log)
	assert.Equal(t, map[string]any{"deleted_count": 0}, evMap(t, *sr))
}

func TestB30_C7_APIErrorKept(t *testing.T) {
	f := newB30Fake()
	f.script["msg:-1001/501"] = []error{api429(time.Second)}
	_, err := (&CleanupStep{}).Execute(context.Background(), b30RT(f, 500, 501))
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr), "err %v", err)
	assert.Equal(t, 429, apiErr.Code)
}

func TestB30_C8_LastMessage(t *testing.T) {
	t.Run("deleted", func(t *testing.T) { // guard
		f := newB30Fake()
		f.always["msg:-1001/501"] = errors.New("x")
		rt := b30RT(f, 500, 501)
		rt.LastMessage = &tg.Message{MessageID: 500, Chat: &tg.Chat{ID: b30Chat}}
		_, _ = (&CleanupStep{}).Execute(context.Background(), rt)
		assert.Nil(t, rt.LastMessage)
	})
	t.Run("kept", func(t *testing.T) {
		f := newB30Fake()
		f.always["msg:-1001/501"] = errors.New("x")
		rt := b30RT(f, 500, 501)
		rt.LastMessage = &tg.Message{MessageID: 501, Chat: &tg.Chat{ID: b30Chat}}
		_, _ = (&CleanupStep{}).Execute(context.Background(), rt)
		require.NotNil(t, rt.LastMessage)
		assert.Equal(t, 501, rt.LastMessage.MessageID)
	})
	t.Run("other chat", func(t *testing.T) { // guard: chat and id must both match
		f := newB30Fake()
		f.always["msg:-1001/501"] = errors.New("x")
		rt := b30RT(f, 501)
		rt.LastMessage = &tg.Message{MessageID: 501, Chat: &tg.Chat{ID: -2002}}
		_, _ = (&CleanupStep{}).Execute(context.Background(), rt)
		assert.Nil(t, rt.LastMessage)
	})
	t.Run("nil chat", func(t *testing.T) {
		f := newB30Fake()
		f.always["msg:-1001/501"] = errors.New("x")
		rt := b30RT(f, 501)
		rt.LastMessage = &tg.Message{MessageID: 501}
		require.NotPanics(t, func() { _, _ = (&CleanupStep{}).Execute(context.Background(), rt) })
		require.NotNil(t, rt.LastMessage)
	})
}

func TestB30_C9_AllKindsCounted(t *testing.T) {
	f := newB30Fake()
	rt := b30RT(f, 500)
	rt.TrackStickerSet("a")
	_, err := (&SetChatMemberTagStep{Tag: "t"}).Execute(context.Background(), rt)
	require.NoError(t, err)
	sr, err := (&CleanupStep{}).Execute(context.Background(), rt)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"deleted_count": 1, "sticker_sets_deleted": 1, "tags_cleared": 1}, evMap(t, *sr))
	assert.Equal(t, []string{"tag:-1001/555/t", "msg:-1001/500", "set:a", "tag:-1001/555/"}, f.keys())
}

func TestB30_C10_TagClearFailureKept(t *testing.T) {
	f := newB30Fake()
	f.script["tag:-1001/555/"] = []error{errors.New("tag-x")}
	f.script["tag:-1001/777/"] = []error{errors.New("tag-z")} // [t3a] A-T3-04: two failures, kept in order
	rt := b30RT(f)
	for _, u := range []int64{555, 666, 777} {
		rt.TestUserID = u
		_, err := (&SetChatMemberTagStep{Tag: "t"}).Execute(context.Background(), rt)
		require.NoError(t, err)
	}
	_, err := (&CleanupStep{}).Execute(context.Background(), rt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "member tag -1001/555: tag-x")
	assert.Contains(t, err.Error(), "member tag -1001/777: tag-z")
	_, err = (&CleanupStep{}).Execute(context.Background(), rt)
	require.NoError(t, err)
	assert.Equal(t, []string{"tag:-1001/555/t", "tag:-1001/666/t", "tag:-1001/777/t",
		"tag:-1001/555/", "tag:-1001/666/", "tag:-1001/777/", "tag:-1001/555/", "tag:-1001/777/"}, f.keys())
}

// Guard: each message is deleted in the chat it was tracked in (forward/copy track their target chat).
func TestB30_C11_TwoChats(t *testing.T) {
	f := newB30Fake()
	rt := b30RT(f)
	rt.TrackMessage(-1001, 501)
	rt.TrackMessage(-2002, 501)
	_, err := (&CleanupStep{}).Execute(context.Background(), rt)
	require.NoError(t, err)
	assert.Equal(t, []string{"msg:-1001/501", "msg:-2002/501"}, f.keys())
	assert.Empty(t, rt.CreatedMessages)
}

// [t3] O-T2-02, S-T2-01: tag records are kept per (chat, user) pair; the real SetChatMemberTagStep writes them.
func TestB30_C12_TagPairs(t *testing.T) {
	tag := func(t *testing.T, rt *Runtime, chat, user int64, v string) {
		t.Helper()
		rt.ChatID, rt.TestUserID = chat, user
		_, err := (&SetChatMemberTagStep{Tag: v}).Execute(context.Background(), rt)
		require.NoError(t, err)
	}
	t.Run("same chat two users", func(t *testing.T) {
		f := newB30Fake()
		rt := b30RT(f)
		tag(t, rt, b30Chat, 555, "t")
		tag(t, rt, b30Chat, 777, "t")
		sr, err := (&CleanupStep{}).Execute(context.Background(), rt)
		require.NoError(t, err)
		assert.Equal(t, []string{"tag:-1001/555/t", "tag:-1001/777/t", "tag:-1001/555/", "tag:-1001/777/"}, f.keys())
		assert.Equal(t, map[string]any{"deleted_count": 0, "tags_cleared": 2}, evMap(t, *sr))
	})
	t.Run("two chats one user", func(t *testing.T) {
		f := newB30Fake()
		rt := b30RT(f)
		tag(t, rt, -1001, 555, "t")
		tag(t, rt, -2002, 555, "t")
		sr, err := (&CleanupStep{}).Execute(context.Background(), rt)
		require.NoError(t, err)
		assert.Equal(t, []string{"tag:-1001/555/t", "tag:-2002/555/t", "tag:-1001/555/", "tag:-2002/555/"}, f.keys())
		assert.Equal(t, map[string]any{"deleted_count": 0, "tags_cleared": 2}, evMap(t, *sr))
	})
	t.Run("one cleared one kept", func(t *testing.T) {
		f := newB30Fake()
		rt := b30RT(f)
		tag(t, rt, b30Chat, 555, "t")
		tag(t, rt, b30Chat, 777, "t")
		tag(t, rt, b30Chat, 555, "")
		sr, err := (&CleanupStep{}).Execute(context.Background(), rt)
		require.NoError(t, err)
		assert.Equal(t, []string{"tag:-1001/555/t", "tag:-1001/777/t", "tag:-1001/555/", "tag:-1001/777/"}, f.keys())
		assert.Equal(t, map[string]any{"deleted_count": 0, "tags_cleared": 1}, evMap(t, *sr))
	})
	t.Run("second cleared first kept", func(t *testing.T) { // [t3a] A-T3-02 = O-T3-02
		f := newB30Fake()
		rt := b30RT(f)
		tag(t, rt, b30Chat, 555, "t")
		tag(t, rt, b30Chat, 777, "t")
		tag(t, rt, b30Chat, 777, "")
		sr, err := (&CleanupStep{}).Execute(context.Background(), rt)
		require.NoError(t, err)
		assert.Equal(t, []string{"tag:-1001/555/t", "tag:-1001/777/t", "tag:-1001/777/", "tag:-1001/555/"}, f.keys())
		assert.Equal(t, map[string]any{"deleted_count": 0, "tags_cleared": 1}, evMap(t, *sr))
	})
	t.Run("repeat write one record", func(t *testing.T) { // [t3a] O-T3-03
		f := newB30Fake()
		rt := b30RT(f)
		tag(t, rt, b30Chat, 555, "t")
		tag(t, rt, b30Chat, 555, "new")
		sr, err := (&CleanupStep{}).Execute(context.Background(), rt)
		require.NoError(t, err)
		assert.Equal(t, []string{"tag:-1001/555/t", "tag:-1001/555/new", "tag:-1001/555/"}, f.keys())
		assert.Equal(t, map[string]any{"deleted_count": 0, "tags_cleared": 1}, evMap(t, *sr))
	})
	t.Run("clear only", func(t *testing.T) { // guard: a clear with no record adds none
		f := newB30Fake()
		rt := b30RT(f)
		tag(t, rt, b30Chat, 555, "")
		sr, err := (&CleanupStep{}).Execute(context.Background(), rt)
		require.NoError(t, err)
		assert.Equal(t, []string{"tag:-1001/555/"}, f.keys())
		assert.Equal(t, map[string]any{"deleted_count": 0}, evMap(t, *sr))
	})
}

// ---- D rows: dice in a private chat are not tracked (AC9) ----

func TestB30_D_Dice(t *testing.T) {
	for _, tc := range []struct {
		name    string
		chat    int64
		tracked bool
	}{{"private", 42, false}, {"group", b30Chat, true}, {"zero", 0, true}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newB30Fake()
			rt := NewRuntime(f, tc.chat, 0)
			r := NewRunner(rt, RunnerConfig{MaxMessages: 100}, slog.New(slog.DiscardHandler))
			var tracked []CreatedMessage
			probe := stepFunc(func(_ context.Context, rt *Runtime) (*StepResult, error) {
				tracked = append([]CreatedMessage(nil), rt.CreatedMessages...)
				return nil, errB30
			})
			res := r.Run(context.Background(), b30Scenario(0, &SendDiceStep{}, probe))
			require.GreaterOrEqual(t, len(res.Steps), 2)
			assert.Equal(t, []int{77}, res.Steps[0].MessageIDs)
			_, notTracked := evMap(t, res.Steps[0])["not_tracked"]
			if tc.tracked {
				assert.Equal(t, []CreatedMessage{{ChatID: tc.chat, MessageID: 77}}, tracked)
				assert.False(t, notTracked)
				assert.Equal(t, []string{fmt.Sprintf("msg:%d/77", tc.chat)}, f.keys())
			} else {
				assert.Empty(t, tracked)
				assert.True(t, notTracked)
				assert.Empty(t, f.log)
				assert.False(t, hasCleanupAfterStop(res))
			}
		})
	}
}

type stepFunc func(context.Context, *Runtime) (*StepResult, error)

func (s stepFunc) Name() string { return "probe" }
func (s stepFunc) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	return s(ctx, rt)
}
