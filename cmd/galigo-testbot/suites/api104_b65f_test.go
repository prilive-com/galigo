package suites

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/evidence"
	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
)

// Bot API 10.4 widgets, S76 (B-65 65f, TEST-PLAN M1-M3, M6).

// b65fS76 returns the one 10.4 scenario; a missing scenario fails the row, it does not panic the binary.
func b65fS76(t *testing.T) engine.Scenario {
	t.Helper()
	all := AllAPI104Scenarios()
	require.Len(t, all, 1, "AllAPI104Scenarios")
	return all[0]
}

const (
	b65fGroup = int64(-1001234567890)  // the fake test group
	b65fHTML  = "<b>galigo widget</b>" // S76's html, written out here
)

// M1: S76's name, covered methods, timeout and exact steps, every value written out here.
func TestB65F_S76Steps(t *testing.T) {
	s := b65fS76(t)
	assert.Equal(t, "S76-Widgets", s.Name())
	assert.Equal(t, []string{"sendWidget", "sendSharedWidget", "answerWidgetQuery", "editMessageWidgetData",
		"editSharedWidgetData", "sendMessage"}, s.Covers())
	assert.Equal(t, 60*time.Second, s.Timeout())
	want := []engine.Step{
		&engine.WidgetRefusalStep{Method: "sendWidget", WantError: "INPUT_MEDIA_WIDGET_NOT_ALLOWED"},
		&engine.SendMessageStep{Text: "galigo 10.4 widget target"},
		&engine.WidgetRefusalStep{Method: "sendSharedWidget", WantError: "invalid widget identifier specified"},
		&engine.WidgetRefusalStep{Method: "answerWidgetQuery", WantError: "query is too old"},
		&engine.WidgetRefusalStep{Method: "editMessageWidgetData", Variant: "target", WantError: "MESSAGE_ID_INVALID"},
		&engine.WidgetRefusalStep{Method: "editMessageWidgetData", Variant: "missing", WantError: "message with widget to edit not found"},
		&engine.WidgetRefusalStep{Method: "editMessageWidgetData", Variant: "inline", WantError: "invalid inline message identifier specified"},
		&engine.WidgetRefusalStep{Method: "editSharedWidgetData", WantError: "invalid widget identifier specified"},
		&engine.CleanupStep{},
	}
	assert.Equal(t, want, s.Steps())
	names := map[string]bool{}
	var got []string
	for _, st := range s.Steps() {
		if _, ok := st.(*engine.WidgetRefusalStep); ok {
			names[st.Name()] = true
			got = append(got, st.Name())
		}
	}
	assert.Equal(t, []string{"sendWidget (10.4)", "sendSharedWidget (10.4)", "answerWidgetQuery (10.4)",
		"editMessageWidgetData (10.4), target message", "editMessageWidgetData (10.4), missing message",
		"editMessageWidgetData (10.4), inline message", "editSharedWidgetData (10.4)"}, got)
	assert.Len(t, names, 7, "distinct step names")
}

// b65fTargetID is the target message id: odd and above 2^53 where int has 64 bits, else 70.
func b65fTargetID() int {
	if strconv.IntSize == 64 {
		return int(b65fHugeTarget) // odd, above 2^53: a float64 step changes its digits; +1000000 still fits
	}
	return 70
}

// b65fHugeTarget is a variable, so the file builds where int has 32 bits (a constant would overflow there).
var b65fHugeTarget int64 = 9007199254740993

// b65fWidgetBodies are S76's planned widget bodies, in order, for target id id.
func b65fWidgetBodies(id int) []struct{ method, body, refusal string } {
	g := strconv.FormatInt(b65fGroup, 10)
	return []struct{ method, body, refusal string }{
		{"sendWidget", `{"chat_id":` + g + `,"html":"<b>galigo widget</b>"}`, "INPUT_MEDIA_WIDGET_NOT_ALLOWED"},
		{"sendSharedWidget", `{"chat_id":` + g + `,"widget_id":"galigo-fake-widget"}`, "invalid widget identifier specified"},
		{"answerWidgetQuery", `{"widget_query_id":"galigo-fake-query","data":"x"}`,
			"query is too old and response timeout expired or query ID is invalid"},
		{"editMessageWidgetData", `{"chat_id":` + g + `,"message_id":` + strconv.Itoa(id) + `,"chat_data":"c"}`, "MESSAGE_ID_INVALID"},
		{"editMessageWidgetData", `{"chat_id":` + g + `,"message_id":` + strconv.Itoa(id+1000000) + `,"chat_data":"c"}`,
			"message with widget to edit not found"},
		{"editMessageWidgetData", `{"inline_message_id":"galigo-fake-inline","public_data":"p"}`, "invalid inline message identifier specified"},
		{"editSharedWidgetData", `{"widget_id":"galigo-fake-widget","public_data":""}`, "invalid widget identifier specified"},
	}
}

// b65fCanon decodes a JSON body with exact numbers and encodes it again, so equal bodies give equal strings.
func b65fCanon(t *testing.T, body []byte) string {
	t.Helper()
	var v any
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	require.NoError(t, d.Decode(&v))
	out, err := json.Marshal(v)
	require.NoError(t, err)
	return string(out)
}

// b65fMock is a mock Bot API for S76: the probe answers with chat type chatType (per chat id in types), the target
// send with message id, deleteMessage with true; a widget body equal to a planned body gets its refusal, any other
// body (and a sendWidget when takeSend is set) is taken: sendWidget / sendSharedWidget reply a message, the rest true.
type b65fMock struct {
	server   *testutil.MockTelegramServer
	mu       sync.Mutex
	methods  []string
	refusals map[string]string // canonical body -> refusal text
}

func newB65FMock(t *testing.T, types map[int64]string, id int, takeSend bool) *b65fMock {
	t.Helper()
	m := &b65fMock{server: testutil.NewMockServer(t), refusals: map[string]string{}}
	for _, b := range b65fWidgetBodies(id) {
		m.refusals[b.method+" "+b65fCanon(t, []byte(b.body))] = b.refusal
	}
	path := func(method string) string { return "/bot" + testutil.TestToken + "/" + method }
	on := func(method string, h func(w http.ResponseWriter, body []byte)) {
		m.server.On(path(method), func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			m.mu.Lock()
			m.methods = append(m.methods, method)
			m.mu.Unlock()
			h(w, body)
		})
	}
	chatOf := func(body []byte) int64 {
		var req struct {
			ChatID int64 `json:"chat_id"`
		}
		_ = json.Unmarshal(body, &req)
		return req.ChatID
	}
	on("getChat", func(w http.ResponseWriter, body []byte) {
		c := chatOf(body)
		testutil.ReplyOK(w, map[string]any{"id": c, "type": types[c]})
	})
	on("getMe", func(w http.ResponseWriter, _ []byte) { testutil.ReplyUser(w) })
	on("getChatMember", func(w http.ResponseWriter, _ []byte) {
		testutil.ReplyOK(w, map[string]any{"status": "member", "user": map[string]any{"id": testutil.TestBotID, "is_bot": true, "first_name": "b"}})
	})
	on("sendMessage", func(w http.ResponseWriter, body []byte) { testutil.ReplyMessageWithChat(w, id, chatOf(body)) })
	on("deleteMessage", func(w http.ResponseWriter, _ []byte) { testutil.ReplyBool(w, true) })
	for _, method := range []string{"sendWidget", "sendSharedWidget", "answerWidgetQuery", "editMessageWidgetData", "editSharedWidgetData"} {
		on(method, func(w http.ResponseWriter, body []byte) {
			if refusal, ok := m.refusals[method+" "+canonOrRaw(body)]; ok && (!takeSend || method != "sendWidget") {
				testutil.ReplyBadRequest(w, refusal)
				return
			}
			if method == "sendWidget" || method == "sendSharedWidget" {
				testutil.ReplyMessageWithChat(w, 4242, chatOf(body))
				return
			}
			testutil.ReplyBool(w, true)
		})
	}
	return m
}

func canonOrRaw(body []byte) string {
	var v any
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	if d.Decode(&v) != nil {
		return string(body)
	}
	out, _ := json.Marshal(v)
	return string(out)
}

// b65fRuntime is a runtime in chat over the mock, with fast limiters.
func b65fRuntime(t *testing.T, m *b65fMock, chat int64) *engine.Runtime {
	t.Helper()
	client := testutil.NewTestClient(t, m.server.BaseURL(), sender.WithRateLimit(1000, 100),
		sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
	return engine.NewRuntime(engine.NewSenderAdapter(client), chat, testutil.TestUserID)
}

// b65fSaved runs the report writer on res and returns the saved JSON, decoded with exact numbers.
func b65fSaved(t *testing.T, res *engine.ScenarioResult) map[string]any {
	t.Helper()
	report := evidence.NewReport()
	report.AddScenario(res)
	report.Finalize()
	path, err := report.Save(t.TempDir())
	require.NoError(t, err)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var out map[string]any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	require.NoError(t, d.Decode(&out))
	return out
}

func b65fNum(n int) json.Number { return json.Number(strconv.Itoa(n)) }

// M2: S76 on the real runner, adapter and report writer in a group: the planned bodies, the method order, the
// cleanup of the target, and the saved report as written.
func TestB65F_S76RealRunnerGroup(t *testing.T) {
	id := b65fTargetID()
	m := newB65FMock(t, map[int64]string{b65fGroup: "group"}, id, false)
	rt := b65fRuntime(t, m, b65fGroup)
	runner := engine.NewRunner(rt, engine.RunnerConfig{MaxMessages: 50}, slog.New(slog.DiscardHandler))
	res := runner.Run(context.Background(), b65fS76(t))
	require.True(t, res.Success, "%+v", res)

	// the widget bodies, in order, and the delete
	var widget []string
	var del []byte
	for _, c := range m.server.Captures() {
		switch method := c.Path[len("/bot"+testutil.TestToken+"/"):]; method {
		case "sendWidget", "sendSharedWidget", "answerWidgetQuery", "editMessageWidgetData", "editSharedWidgetData":
			widget = append(widget, method+" "+b65fCanon(t, c.Body))
		case "deleteMessage":
			require.Nil(t, del, "one deleteMessage")
			del = c.Body
		}
	}
	var want []string
	for _, b := range b65fWidgetBodies(id) {
		want = append(want, b.method+" "+b65fCanon(t, []byte(b.body)))
	}
	assert.Equal(t, want, widget)
	require.NotNil(t, del)
	assert.Equal(t, b65fCanon(t, []byte(`{"chat_id":`+strconv.FormatInt(b65fGroup, 10)+`,"message_id":`+strconv.Itoa(id)+`}`)), b65fCanon(t, del))
	assert.Equal(t, []string{"getChat", "getMe", "getChatMember", "sendWidget", "sendMessage", "sendSharedWidget",
		"answerWidgetQuery", "editMessageWidgetData", "editMessageWidgetData", "editMessageWidgetData",
		"editSharedWidgetData", "deleteMessage"}, m.methods)

	// the saved report
	saved := b65fSaved(t, res)
	scs, ok := saved["scenarios"].([]any)
	require.True(t, ok)
	require.Len(t, scs, 1)
	sc := scs[0].(map[string]any)
	assert.Equal(t, "S76-Widgets", sc["scenario_name"])
	assert.Equal(t, true, sc["success"])
	assert.Nil(t, sc["skipped"])
	steps := sc["steps"].([]any)
	pairs := [][2]string{
		{"sendWidget (10.4)", "sendWidget"}, {"", "sendMessage"}, {"sendSharedWidget (10.4)", "sendSharedWidget"},
		{"answerWidgetQuery (10.4)", "answerWidgetQuery"},
		{"editMessageWidgetData (10.4), target message", "editMessageWidgetData"},
		{"editMessageWidgetData (10.4), missing message", "editMessageWidgetData"},
		{"editMessageWidgetData (10.4), inline message", "editMessageWidgetData"},
		{"editSharedWidgetData (10.4)", "editSharedWidgetData"}, {"", "deleteMessage"},
	}
	require.Len(t, steps, len(pairs))
	refusals := b65fWidgetBodies(id)
	ri := 0
	for i, raw := range steps {
		st := raw.(map[string]any)
		name, _ := st["step_name"].(string)
		assert.Equal(t, pairs[i], [2]string{name, st["method"].(string)}, "step %d", i)
		assert.Equal(t, true, st["success"], "step %d", i)
		ev, _ := st["evidence"].(map[string]any)
		switch i {
		case 1:
			assert.Equal(t, []any{b65fNum(id)}, st["message_ids"])
			assert.Equal(t, b65fNum(id), ev["message_id"])
		case 8:
			assert.Equal(t, b65fNum(1), ev["deleted_count"])
			assert.NotContains(t, ev, "failed")
			assert.Nil(t, st["message_ids"])
		default:
			r := refusals[ri]
			ri++
			assert.Equal(t, b65fNum(400), ev["error_code"], "step %d", i)
			assert.Equal(t, "Bad Request: "+r.refusal, ev["description"], "step %d", i)
			assert.Nil(t, st["message_ids"], "step %d", i)
			switch i {
			case 4:
				assert.Equal(t, b65fNum(id), ev["message_id"])
			case 5:
				assert.Equal(t, b65fNum(id+1000000), ev["message_id"])
			default:
				assert.NotContains(t, ev, "message_id", "step %d", i)
			}
		}
	}
	sum := saved["summary"].(map[string]any)
	assert.Equal(t, b65fNum(1), sum["total_scenarios"])
	assert.Equal(t, b65fNum(1), sum["passed_scenarios"])
	assert.Equal(t, b65fNum(0), sum["failed_scenarios"])
	assert.Equal(t, b65fNum(9), sum["total_steps"])
	assert.Equal(t, b65fNum(9), sum["passed_steps"])
	assert.Equal(t, b65fNum(0), sum["failed_steps"])
}

// M3: a taken send fails S76 at that step; the cleanup after the stop deletes that message, and no later widget
// call is made.
func TestB65F_S76RealRunnerTakenSend(t *testing.T) {
	m := newB65FMock(t, map[int64]string{b65fGroup: "group"}, 70, true)
	rt := b65fRuntime(t, m, b65fGroup)
	runner := engine.NewRunner(rt, engine.RunnerConfig{MaxMessages: 50}, slog.New(slog.DiscardHandler))
	res := runner.Run(context.Background(), b65fS76(t))
	require.False(t, res.Success)
	require.Len(t, res.Steps, 2)
	assert.Equal(t, "sendWidget (10.4)", res.Steps[0].StepName)
	assert.Equal(t, []int{4242}, res.Steps[0].MessageIDs)
	assert.Equal(t, "cleanup after stop", res.Steps[1].StepName)
	assert.Equal(t, map[string]any{"deleted_count": 1}, res.Steps[1].Evidence)
	assert.Equal(t, []string{"getChat", "getMe", "getChatMember", "sendWidget", "deleteMessage"}, m.methods)
	last := m.server.LastCapture()
	assert.Equal(t, b65fCanon(t, []byte(`{"chat_id":-1001234567890,"message_id":4242}`)), b65fCanon(t, last.Body))
	saved := b65fSaved(t, res)
	assert.Equal(t, false, saved["success"])
}

// m6First is a scenario that probes the chat (private, cached) and sends one tracked message.
func m6First() engine.Scenario {
	return &engine.BaseScenario{ScenarioName: "b65f-m6-first", ScenarioTimeout: 10 * time.Second,
		ScenarioSteps: []engine.Step{m6Probe{}, &engine.SendMessageStep{Text: "m6"}}}
}

type m6Probe struct{}

func (m6Probe) Name() string { return "probe" }
func (m6Probe) Execute(ctx context.Context, rt *engine.Runtime) (*engine.StepResult, error) {
	return &engine.StepResult{StepName: "probe"}, rt.ProbeChat(ctx)
}

// M6: an earlier scenario leaves the chat type cached (private) and a tracked message; S76 then skips on the cached
// type with no probe and no widget call, and the stop cleanup deletes the earlier message only.
func TestB65F_S76RetainedState(t *testing.T) {
	const private = int64(77)
	m := newB65FMock(t, map[int64]string{private: "private"}, 700, false)
	rt := b65fRuntime(t, m, private)
	runner := engine.NewRunner(rt, engine.RunnerConfig{MaxMessages: 50}, slog.New(slog.DiscardHandler))
	first := runner.Run(context.Background(), m6First())
	require.True(t, first.Success, "%+v", first)
	m.mu.Lock()
	m.methods = nil
	m.mu.Unlock()
	res := runner.Run(context.Background(), b65fS76(t))
	assert.True(t, res.Skipped, "%+v", res)
	assert.Equal(t, []string{"deleteMessage"}, m.methods, "no probe, no widget call, one delete")
	last := m.server.LastCapture()
	assert.Equal(t, b65fCanon(t, []byte(`{"chat_id":77,"message_id":700}`)), b65fCanon(t, last.Body))
}
