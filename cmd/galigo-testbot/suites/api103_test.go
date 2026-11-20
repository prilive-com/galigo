package suites

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/evidence"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.3: S74 rich messages (B-65 65c, TEST-PLAN §4, rows T-sc and T-run), S75 reply markup (B-65 65d).

// T-sc: S74's name, covered method, timeout and exact steps, every sent value written out here (not taken from
// s74Blocks), so a changed value fails this row and not only the live run.
func TestB65C_S74Steps(t *testing.T) {
	got := AllAPI103Scenarios()
	require.Len(t, got, 2) // B-65 65d: S74, S75
	assert.Equal(t, "S75-ReplyMarkupAPI103", got[1].Name())
	s := got[0]
	assert.Equal(t, "S74-RichAPI103", s.Name())
	assert.Equal(t, []string{"sendRichMessage"}, s.Covers())
	assert.Equal(t, 90*time.Second, s.Timeout())
	text := func(s string) tg.RichText { return tg.RichText{Plain: s} }
	a, b := text("a"), text("b")
	want := []engine.Step{
		&engine.SendRichMessageStep{RichMessage: tg.InputRichMessage{Blocks: []tg.InputRichBlock{
			tg.InputRichBlockButtons{Align: "center", Buttons: []tg.RichMessageButton{
				{Text: text("callback"), Style: "primary", CallbackData: "s74"},
				{Text: text("url"), URL: "https://t.me/"},
				{Text: text("copy"), CopyText: &tg.CopyTextButton{Text: "galigo"}},
				{Text: text("off"), Disabled: &tg.DisabledButton{}},
			}},
			tg.InputRichBlockParagraph{Text: tg.RichText{Parts: []tg.RichText{text("galigo 10.3 "),
				{Node: tg.RichTextButton{Type: "button", Button: tg.RichMessageButton{Text: text("node"), CallbackData: "s74-node"}}}}}},
			tg.InputRichBlockTable{Cells: [][]tg.RichBlockTableCell{{
				{Text: &a, Align: "left", VAlign: "top"}, {Text: &b, Align: "left", VAlign: "top"}}}, IsCompact: true},
			tg.InputRichBlockExpandableBlockQuotation{Text: text("galigo 10.3 expandable"), Credit: &tg.RichText{Plain: "galigo"}},
		}}, WantBlocks: []string{"buttons", "paragraph", "table", "expandable_blockquote"}},
		&engine.VerifyRichAPI103Step{},
		&engine.SendRichMessageStep{RichMessage: tg.InputRichMessage{
			Markdown: "galigo 10.3 document\n\n![](tg://document?id=d)",
			Media: []tg.InputRichMessageMedia{{ID: "d",
				Media: sender.NewInputMediaDocument(sender.FromBytes(fixtures.DocumentBytes(), "document.txt"))}}},
			WantBlocks: []string{"paragraph", "document"}},
		&engine.SendRichDocumentBlockStep{WantBlocks: []string{"document"}},
		&engine.CleanupStep{},
	}
	assert.Equal(t, view102eSteps(want), view102eSteps(s.Steps()))

	// The link names the media item, and the upload can be read again on a retry (two reads, the same bytes).
	step, ok := s.Steps()[2].(*engine.SendRichMessageStep)
	require.True(t, ok)
	links := regexp.MustCompile(`tg://(\w+)\?id=(\w+)`).FindAllStringSubmatch(step.RichMessage.Markdown, -1)
	require.Len(t, links, 1)
	require.Len(t, step.RichMessage.Media, 1)
	m := step.RichMessage.Media[0]
	assert.Equal(t, m.ID, links[0][2])
	assert.Equal(t, "document", links[0][1])
	assert.Equal(t, "document", m.Media.InputRichMediaType())
	im, ok := m.Media.(sender.InputMedia)
	require.True(t, ok)
	require.NotNil(t, im.Media.Source)
	for range 2 {
		data, err := io.ReadAll(im.Media.Source())
		require.NoError(t, err)
		assert.Equal(t, fixtures.DocumentBytes(), data)
	}
}

// fake103 runs S74 on the real runner: the first sendRichMessage returns what S74 sent (so the verify step checks
// S74's own values), the second a paragraph and a document, the third a document; deleteMessage is recorded.
type fake103 struct {
	engine.SenderClient
	sent      []tg.InputRichMessage
	chats     []int64
	deleted   [][2]int64
	breakNode bool // the first reply loses its button node
}

func (f *fake103) SendRichMessage(_ context.Context, chatID int64, rm tg.InputRichMessage) (*tg.Message, error) {
	f.sent = append(f.sent, rm)
	f.chats = append(f.chats, chatID)
	var js []byte
	switch len(f.sent) {
	case 1:
		out, err := json.Marshal(rm)
		if err != nil {
			return nil, err
		}
		js = out
	case 2:
		js = []byte(`{"blocks":[{"type":"paragraph","text":"galigo 10.3 document"},` +
			`{"type":"document","document":{"file_id":"BQAC-fake-doc","file_unique_id":"AgAD-fake","file_name":"document.txt"}}]}`)
	default:
		js = []byte(`{"blocks":[{"type":"document","document":{"file_id":"BQAC-fake-doc","file_unique_id":"AgAD-fake"},"caption":{"text":"document block"}}]}`)
	}
	var rich tg.RichMessage
	if err := json.Unmarshal(js, &rich); err != nil {
		return nil, err
	}
	if f.breakNode && len(rich.Blocks) > 1 {
		rich.Blocks[1] = tg.RichBlockParagraph{Type: "paragraph", Text: tg.RichText{Plain: "no node"}}
	}
	return &tg.Message{MessageID: 740 + len(f.sent), Chat: &tg.Chat{ID: chatID}, RichMessage: &rich}, nil
}

func (f *fake103) DeleteMessage(_ context.Context, chatID int64, messageID int) error {
	f.deleted = append(f.deleted, [2]int64{chatID, int64(messageID)})
	return nil
}

// T-run: S74 on the real runner: the verify step checks the blocks S74 sent, the document block goes by the file id
// of the second reply, and the three messages are deleted.
func TestB65C_S74Run(t *testing.T) {
	f := &fake103{}
	rt := engine.NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
	runner := engine.NewRunner(rt, engine.RunnerConfig{MaxMessages: 100}, slog.New(slog.DiscardHandler))
	res := runner.Run(context.Background(), S74_RichAPI103())
	require.True(t, res.Success, "%+v", res)
	require.Len(t, res.Steps, 5)
	assert.Equal(t, map[string]any{"message_id": 741,
		"checked": []string{"buttons", "text_button", "is_compact", "expandable_blockquote"}}, res.Steps[1].Evidence)
	require.Len(t, f.sent, 3)
	assert.Equal(t, []int64{testutil.TestChatID, testutil.TestChatID, testutil.TestChatID}, f.chats)
	out, err := json.Marshal(f.sent[2])
	require.NoError(t, err)
	assert.JSONEq(t, `{"blocks":[{"type":"document","document":{"media":"BQAC-fake-doc","type":"document"},"caption":{"text":"document block"}}]}`,
		string(out))
	assert.ElementsMatch(t, [][2]int64{{testutil.TestChatID, 741}, {testutil.TestChatID, 742}, {testutil.TestChatID, 743}}, f.deleted)
	assert.Empty(t, rt.CreatedMessages)
}

// T-run, failure: a reply without the button node fails the verify step, and the cleanup after the stop deletes the
// one sent message.
func TestB65C_S74RunVerifyFails(t *testing.T) {
	f := &fake103{breakNode: true}
	rt := engine.NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
	runner := engine.NewRunner(rt, engine.RunnerConfig{MaxMessages: 100}, slog.New(slog.DiscardHandler))
	res := runner.Run(context.Background(), S74_RichAPI103())
	require.False(t, res.Success)
	assert.Contains(t, res.Error, "block 1 has no button node with callback data")
	assert.Len(t, f.sent, 1)
	assert.Equal(t, [][2]int64{{testutil.TestChatID, 741}}, f.deleted)
}

// Bot API 10.3 reply markup, S75 (B-65 65d, TEST-PLAN §5, rows T-sc, T-run and T-run-noleak).

const (
	b65dGroup = int64(-1009876543210) // the fake test group
	b65dAdmin = int64(777000111)      // the fake admin id: not the group
	b65dP     = "sendMessage (reply markup): the reply markup differs: "
)

// b65dS75Markup is S75's markup, written out here (not taken from the suite), so a changed value fails T-sc.
func b65dS75Markup() tg.InlineKeyboardMarkup {
	return tg.InlineKeyboardMarkup{
		InlineKeyboard: [][]tg.InlineKeyboardButton{{
			{Text: "callback", CallbackData: "s75"},
			{Text: "off", Disabled: &tg.DisabledButton{}},
		}},
		ForceReply: true,
	}
}

// T-sc
func TestB65D_S75Steps(t *testing.T) {
	s := S75_ReplyMarkupAPI103()
	assert.Equal(t, "S75-ReplyMarkupAPI103", s.Name())
	assert.Equal(t, []string{"sendMessage"}, s.Covers())
	assert.Equal(t, 30*time.Second, s.Timeout())
	assert.Equal(t, []engine.Step{
		&engine.SendReplyMarkupEchoStep{Text: "galigo 10.3 reply markup", Markup: b65dS75Markup()},
		&engine.CleanupStep{},
	}, s.Steps())
}

// fake75 runs S75 on the real runner: SendMessage replies with a fresh copy of the sent markup, changed by change;
// every send and delete is recorded.
type fake75 struct {
	engine.SenderClient
	change  func(*tg.InlineKeyboardMarkup)
	msgID   int
	chats   []int64
	texts   []string
	deleted [][2]int64
}

func (f *fake75) SendMessage(_ context.Context, chatID int64, text string, opts ...engine.SendOption) (*tg.Message, error) {
	f.chats = append(f.chats, chatID)
	f.texts = append(f.texts, text)
	var o engine.SendOptions
	for _, opt := range opts {
		opt(&o)
	}
	msg := &tg.Message{MessageID: f.msgID, Chat: &tg.Chat{ID: chatID}}
	if o.ReplyMarkup != nil {
		raw, err := json.Marshal(o.ReplyMarkup)
		if err != nil {
			return nil, err
		}
		var m tg.InlineKeyboardMarkup
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		if f.change != nil {
			f.change(&m)
		}
		msg.ReplyMarkup = &m
	}
	return msg, nil
}

func (f *fake75) DeleteMessage(_ context.Context, chatID int64, messageID int) error {
	f.deleted = append(f.deleted, [2]int64{chatID, int64(messageID)})
	return nil
}

// b65dRun runs S75 on the real runner with a budget of 10 and returns the result, the runner and the fake.
func b65dRun(t *testing.T, f *fake75, logger *slog.Logger) (*engine.ScenarioResult, *engine.Runner) {
	t.Helper()
	rt := engine.NewRuntime(f, b65dGroup, b65dAdmin)
	runner := engine.NewRunner(rt, engine.RunnerConfig{MaxMessages: 10}, logger)
	res := runner.Run(context.Background(), S75_ReplyMarkupAPI103())
	return res, runner
}

// T-run (i) and (iii): the echo is ok; the planned cleanup deletes the reply's message.
func TestB65D_S75Run(t *testing.T) {
	for _, id := range []int{42, 81} {
		f := &fake75{msgID: id}
		res, runner := b65dRun(t, f, slog.New(slog.DiscardHandler))
		require.True(t, res.Success, "%+v", res)
		require.Len(t, res.Steps, 2)
		echo, clean := res.Steps[0], res.Steps[1]
		assert.Equal(t, "sendMessage (reply markup)", echo.StepName)
		assert.Equal(t, "sendMessage", echo.Method)
		assert.True(t, echo.Success)
		assert.Empty(t, echo.Error)
		assert.Equal(t, []int{id}, echo.MessageIDs)
		assert.Equal(t, map[string]any{"message_id": id, "rows": 1, "force_reply": true}, echo.Evidence)
		assert.Empty(t, clean.StepName)
		assert.Equal(t, "deleteMessage", clean.Method)
		assert.True(t, clean.Success)
		assert.Empty(t, clean.Error)
		assert.Empty(t, clean.MessageIDs)
		assert.Equal(t, map[string]any{"deleted_count": 1}, clean.Evidence)
		assert.Equal(t, []int64{b65dGroup}, f.chats)
		assert.Equal(t, []string{"galigo 10.3 reply markup"}, f.texts)
		assert.Equal(t, [][2]int64{{b65dGroup, int64(id)}}, f.deleted)
		assert.Equal(t, 1, runner.MessageCount())
	}
}

// T-run (ii): a mismatch fails the echo step; the cleanup after the stop deletes the message.
func TestB65D_S75RunMismatch(t *testing.T) {
	f := &fake75{msgID: 42, change: func(m *tg.InlineKeyboardMarkup) { m.InlineKeyboard[0][1].Disabled = nil }}
	res, runner := b65dRun(t, f, slog.New(slog.DiscardHandler))
	require.False(t, res.Success)
	require.Len(t, res.Steps, 2)
	echo, clean := res.Steps[0], res.Steps[1]
	assert.Equal(t, "sendMessage (reply markup)", echo.StepName)
	assert.False(t, echo.Success)
	assert.Equal(t, b65dP+"row 0 button 1: Disabled differs (sent set=true, received set=false)", echo.Error)
	assert.Equal(t, []int{42}, echo.MessageIDs)
	assert.Equal(t, "cleanup after stop", clean.StepName)
	assert.Equal(t, "deleteMessage", clean.Method)
	assert.True(t, clean.Success)
	assert.Equal(t, map[string]any{"deleted_count": 1}, clean.Evidence)
	assert.Equal(t, [][2]int64{{b65dGroup, 42}}, f.deleted)
	assert.Equal(t, 1, runner.MessageCount())
}

// T-run-noleak: a reply with server text in one field fails the step with an error that names only positions and
// lengths; the text is in neither the log nor the saved report (X-P3-01). The strings are for S75's markup.
func TestB65D_S75RunNoLeak(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*tg.InlineKeyboardMarkup)
		want   string
	}{
		{"1 text", func(m *tg.InlineKeyboardMarkup) { m.InlineKeyboard[0][1].Text = "B65D-SECRET-TEXT" },
			b65dP + "row 0 button 1: Text differs (sent 3 bytes, received 16 bytes)"},
		{"2 callback", func(m *tg.InlineKeyboardMarkup) { m.InlineKeyboard[0][0].CallbackData = "B65D-SECRET-DATA" },
			b65dP + "row 0 button 0: CallbackData differs (sent 3 bytes, received 16 bytes)"},
		{"3 web app", func(m *tg.InlineKeyboardMarkup) {
			m.InlineKeyboard[0][0].WebApp = &tg.WebAppInfo{URL: "https://B65D-SECRET-URL.example/"}
		}, b65dP + "row 0 button 0: WebApp differs (sent set=false, received set=true)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			f := &fake75{msgID: 42, change: tc.change}
			res, _ := b65dRun(t, f, logger)
			report := evidence.NewReport()
			report.AddScenario(res)
			report.Finalize()
			path, err := report.Save(t.TempDir())
			require.NoError(t, err)
			saved, err := os.ReadFile(path)
			require.NoError(t, err)

			// the positive control: the mismatch happened and was reported
			require.False(t, res.Success)
			require.Len(t, res.Steps, 2)
			assert.Equal(t, tc.want, res.Steps[0].Error)
			assert.Equal(t, "cleanup after stop", res.Steps[1].StepName)
			assert.Equal(t, map[string]any{"deleted_count": 1}, res.Steps[1].Evidence)
			assert.Contains(t, logs.String(), tc.want)
			wantJSON, err := json.Marshal(tc.want)
			require.NoError(t, err)
			assert.Contains(t, string(saved), string(wantJSON[1:len(wantJSON)-1]))

			assert.NotContains(t, logs.String(), "B65D-SECRET")
			assert.NotContains(t, string(saved), "B65D-SECRET")
		})
	}
}
