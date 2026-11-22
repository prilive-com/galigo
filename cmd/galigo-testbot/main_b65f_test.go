package main

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/config"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/suites"
	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-65 65f: the 10.4 routes, lists and the bot /run of S76 (TEST-PLAN §2.6, M4, M5).

// T-routes: the 10.4 labels select exactly S76 and S77 (B-65 65g), in that order, and all holds each once; the names
// are written out here.
func TestB65F_SuiteRoutesAPI104(t *testing.T) {
	for _, in := range []string{"api104", "api-104", "10.4"} {
		got, ok := suiteScenarios(in)
		require.True(t, ok, in)
		assert.Equal(t, []string{"S76-Widgets", "S77-RichQuote"}, b49Names(got), in)
	}
	got, ok := suiteScenarios("all")
	require.True(t, ok)
	for _, name := range []string{"S76-Widgets", "S77-RichQuote"} {
		assert.Equal(t, 1, strings.Count(","+strings.Join(b49Names(got), ",")+",", ","+name+","), name)
	}
}

// T-once: suites.AllAPI104Scenarios() is called exactly once in suiteScenarios' all case and once in each status
// function (the B-48c1 test proves only that all is a subset of the status lists).
func TestB65F_MainRoutesOnce(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, parser.SkipObjectResolution)
	require.NoError(t, err)
	funcs := map[string]*ast.FuncDecl{}
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			funcs[fd.Name.Name] = fd
		}
	}
	calls := func(n ast.Node) int {
		c := 0
		ast.Inspect(n, func(m ast.Node) bool {
			if call, ok := m.(*ast.CallExpr); ok && len(call.Args) == 0 {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if id, ok := sel.X.(*ast.Ident); ok && id.Name == "suites" && sel.Sel.Name == "AllAPI104Scenarios" {
						c++
					}
				}
			}
			return true
		})
		return c
	}
	fd := funcs["suiteScenarios"]
	require.NotNil(t, fd)
	var all []ast.Stmt
	found := 0
	ast.Inspect(fd, func(n ast.Node) bool {
		if cc, ok := n.(*ast.CaseClause); ok {
			for _, e := range cc.List {
				if lit, ok := e.(*ast.BasicLit); ok && lit.Value == `"all"` {
					all = cc.Body
					found++
				}
			}
		}
		return true
	})
	require.Equal(t, 1, found)
	assert.Equal(t, 1, calls(&ast.BlockStmt{List: all}), "suiteScenarios: case \"all\"")
	for _, name := range []string{"showCoverageStatus", "handleStatus"} {
		require.NotNil(t, funcs[name], name)
		assert.Equal(t, 1, calls(funcs[name].Body), "%s counts the group", name)
	}
}

const b65fGroup = int64(-1001234567890)

// b65fBot is a mock Bot API for the bot's /run of S76 and S77: getChat answers by chat_id (77 private, the group a
// group), the widget paths refuse with their measured texts, sendMessage replies a message in the request's chat
// (760), sendRichMessage a rich message (770), and a sendMessage with quote_rich_texts a reply to it (771, 772, ...)
// that quotes the sent value, or the measured 400 for "absent text" (B-65 65g); every method is recorded.
type b65fBot struct {
	server  *testutil.MockTelegramServer
	mu      sync.Mutex
	methods []string
	bodies  []map[string]any
}

func newB65FBot(t *testing.T) *b65fBot {
	t.Helper()
	b := &b65fBot{server: testutil.NewMockServer(t)}
	refusal := map[string]string{
		"sendWidget": "INPUT_MEDIA_WIDGET_NOT_ALLOWED", "sendSharedWidget": "invalid widget identifier specified",
		"answerWidgetQuery":    "query is too old and response timeout expired or query ID is invalid",
		"editSharedWidgetData": "invalid widget identifier specified",
	}
	on := func(method string, h func(http.ResponseWriter, map[string]any)) {
		b.server.On("/bot"+testutil.TestToken+"/"+method, func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			b.mu.Lock()
			b.methods = append(b.methods, method)
			b.bodies = append(b.bodies, body)
			b.mu.Unlock()
			h(w, body)
		})
	}
	chat := func(body map[string]any) int64 { f, _ := body["chat_id"].(float64); return int64(f) }
	on("getChat", func(w http.ResponseWriter, body map[string]any) {
		typ := map[int64]string{77: "private", b65fGroup: "group"}[chat(body)]
		testutil.ReplyOK(w, map[string]any{"id": chat(body), "type": typ})
	})
	on("getMe", func(w http.ResponseWriter, _ map[string]any) { testutil.ReplyUser(w) })
	on("getChatMember", func(w http.ResponseWriter, _ map[string]any) {
		testutil.ReplyOK(w, map[string]any{"status": "member", "user": map[string]any{"id": testutil.TestBotID, "is_bot": true, "first_name": "b"}})
	})
	quoteID := 770
	on("sendRichMessage", func(w http.ResponseWriter, body map[string]any) {
		testutil.ReplyOK(w, map[string]any{"message_id": 770, "date": 1, "chat": map[string]any{"id": chat(body), "type": "group"},
			"rich_message": map[string]any{"blocks": []any{map[string]any{"type": "paragraph", "text": "a"}, map[string]any{"type": "paragraph", "text": "b"}}}})
	})
	on("sendMessage", func(w http.ResponseWriter, body map[string]any) {
		rp, _ := body["reply_parameters"].(map[string]any)
		if rp == nil || rp["quote_rich_texts"] == nil {
			testutil.ReplyMessageWithChat(w, 760, chat(body))
			return
		}
		texts, _ := json.Marshal(rp["quote_rich_texts"])
		if string(texts) == `["absent text"]` {
			testutil.ReplyBadRequest(w, "QUOTE_TEXT_INVALID")
			return
		}
		pos, _ := rp["quote_position"].(float64)
		quoteID++
		testutil.ReplyOK(w, map[string]any{"message_id": quoteID, "date": 1, "chat": map[string]any{"id": chat(body), "type": "group"},
			"text": body["text"], "reply_to_message": map[string]any{"message_id": rp["message_id"], "date": 1, "chat": map[string]any{"id": chat(body), "type": "group"}},
			"quote": map[string]any{"text": "", "rich_texts": rp["quote_rich_texts"], "position": pos, "is_manual": true}})
	})
	on("deleteMessage", func(w http.ResponseWriter, _ map[string]any) { testutil.ReplyBool(w, true) })
	for m, text := range refusal {
		on(m, func(w http.ResponseWriter, _ map[string]any) { testutil.ReplyBadRequest(w, text) })
	}
	on("editMessageWidgetData", func(w http.ResponseWriter, body map[string]any) {
		switch {
		case body["inline_message_id"] != nil:
			testutil.ReplyBadRequest(w, "invalid inline message identifier specified")
		case body["message_id"] == float64(760):
			testutil.ReplyBadRequest(w, "MESSAGE_ID_INVALID")
		default:
			testutil.ReplyBadRequest(w, "message with widget to edit not found")
		}
	})
	return b
}

// b65fRunBot runs the bot's /run api104 from chat (budget 50, config chat the group) and returns the saved report.
func b65fRunBot(t *testing.T, b *b65fBot, chat int64) map[string]any {
	t.Helper()
	require.Len(t, suites.AllAPI104Scenarios(), 2, "AllAPI104Scenarios")
	client := testutil.NewTestClient(t, b.server.BaseURL(), sender.WithRateLimit(1000, 100),
		sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
	cfg := &config.Config{Admins: []int64{1}, StorageDir: t.TempDir(), MaxMessagesPerRun: 50, ChatID: b65fGroup}
	updates := make(chan tg.Update)
	defer close(updates)
	handleRun(context.Background(), cfg, client, engine.NewSenderAdapter(client), b49Discard(), chat, "api104", updates)
	entries, err := os.ReadDir(filepath.Join(cfg.StorageDir, "reports"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	raw, err := os.ReadFile(filepath.Join(cfg.StorageDir, "reports", entries[0].Name()))
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

// M4: /run api104 from a private chat makes no widget call and no target send; the report shows S76 skipped. S77
// runs in the private chat (B-65 65g): the rich message, three quote replies, the refused reply, four deletes.
func TestB65F_BotRunPrivate(t *testing.T) {
	b := newB65FBot(t)
	report := b65fRunBot(t, b, 77)
	require.Equal(t, []string{"sendMessage", "getChat", "getMe", "getChatMember", "sendRichMessage", "sendMessage", "sendMessage",
		"sendMessage", "sendMessage", "deleteMessage", "deleteMessage", "deleteMessage", "deleteMessage", "sendMessage"}, b.methods)
	require.Len(t, b.bodies, len(b.methods))
	assert.Equal(t, float64(77), b.bodies[0]["chat_id"])
	assert.Equal(t, "Running 2 scenario(s)...", b.bodies[0]["text"])
	assert.Equal(t, float64(77), b.bodies[1]["chat_id"])
	for i := 4; i < len(b.bodies); i++ {
		assert.Equal(t, float64(77), b.bodies[i]["chat_id"], "call %d (%s) in the private chat", i, b.methods[i])
	}
	b65gCheckS77(t, b, 77)
	assert.Contains(t, b.bodies[13]["text"], "Scenarios: 2/2 passed")
	scs, _ := report["scenarios"].([]any)
	require.Len(t, scs, 2)
	sc := scs[0].(map[string]any)
	assert.Equal(t, "S76-Widgets", sc["scenario_name"])
	assert.Equal(t, true, sc["skipped"])
	assert.Contains(t, sc["skip_reason"], "group")
	sc = scs[1].(map[string]any)
	assert.Equal(t, "S77-RichQuote", sc["scenario_name"])
	assert.Nil(t, sc["skipped"])
	assert.Equal(t, true, sc["success"])
}

// M5: /run api104 from the group makes the widget calls, each refused, and the report is successful.
func TestB65F_BotRunGroup(t *testing.T) {
	b := newB65FBot(t)
	report := b65fRunBot(t, b, b65fGroup)
	n := 0
	for _, m := range b.methods {
		switch m {
		case "sendWidget", "sendSharedWidget", "answerWidgetQuery", "editMessageWidgetData", "editSharedWidgetData":
			n++
		}
	}
	assert.Equal(t, 7, n, "seven widget calls")
	assert.Equal(t, true, report["success"])
	scs, _ := report["scenarios"].([]any)
	require.Len(t, scs, 2)
	for i, name := range []string{"S76-Widgets", "S77-RichQuote"} {
		sc := scs[i].(map[string]any)
		assert.Equal(t, name, sc["scenario_name"])
		assert.Nil(t, sc["skipped"], name)
		assert.Equal(t, true, sc["success"], name)
	}
	require.Len(t, b.bodies, len(b.methods))
	count := map[string]int{}
	for _, m := range b.methods {
		count[m]++
	}
	assert.Equal(t, 1, count["sendRichMessage"], "S77's rich message")
	assert.Equal(t, 5, count["deleteMessage"], "S76's target and S77's four messages")
	k := b65gCheckS77(t, b, b65fGroup)
	var s76 []map[string]any
	for i, m := range b.methods[:k] {
		if m == "deleteMessage" {
			s76 = append(s76, b.bodies[i])
		}
	}
	assert.Equal(t, []map[string]any{{"chat_id": float64(b65fGroup), "message_id": float64(760)}}, s76, "S76 deletes its own target")
}

// b65gCheckS77 checks S77's requests from the sendRichMessage call on (B-65 65g, O-T2-05, O-T3-02): four quote replies
// whose whole reply_parameters object is the planned one (as JSON values: the mock decoded the body into a map; the
// bytes are pinned by sender TestB65G_SendMessageRetry): target 770, positions 7, 7, 18 and none on the refusal; then
// S77's own cleanup: 770..773 in chat.
// It returns the index of the sendRichMessage call.
func b65gCheckS77(t *testing.T, b *b65fBot, chat int64) int {
	t.Helper()
	k := slices.Index(b.methods, "sendRichMessage")
	require.GreaterOrEqual(t, k, 0, "S77 sent its rich message")
	require.GreaterOrEqual(t, len(b.methods), k+9, "S77's four replies and four deletes follow")
	require.GreaterOrEqual(t, len(b.bodies), k+9)
	assert.Equal(t, float64(chat), b.bodies[k]["chat_id"])
	// The whole reply_parameters object of each reply (O-T3-02): no extra key, and none on the refusal's position.
	want := []string{
		`{"message_id":770,"quote_rich_texts":["quote"],"quote_position":7}`,
		`{"message_id":770,"quote_rich_texts":[["quote ",{"type":"bold","text":"bold"}]],"quote_position":7}`,
		`{"message_id":770,"quote_rich_texts":["end","second"],"quote_position":18}`,
		`{"message_id":770,"quote_rich_texts":["absent text"]}`,
	}
	for i, w := range want {
		require.Equal(t, "sendMessage", b.methods[k+1+i], "reply %d", i)
		body := b.bodies[k+1+i]
		assert.Equal(t, float64(chat), body["chat_id"], "reply %d", i)
		rp, ok := body["reply_parameters"].(map[string]any)
		require.True(t, ok, "reply %d has reply_parameters", i)
		got, err := json.Marshal(rp)
		require.NoError(t, err)
		assert.JSONEq(t, w, string(got), "reply %d", i)
	}
	for i, id := range []float64{770, 771, 772, 773} {
		require.Equal(t, "deleteMessage", b.methods[k+5+i], "delete %d", i)
		assert.Equal(t, map[string]any{"chat_id": float64(chat), "message_id": id}, b.bodies[k+5+i], "S77's cleanup")
	}
	return k
}
