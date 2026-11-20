package suites

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.3 rich messages, S74 (B-65 65c, TEST-PLAN §4, rows T-sc and T-run).

// T-sc: S74's name, covered method, timeout and exact steps, every sent value written out here (not taken from
// s74Blocks), so a changed value fails this row and not only the live run.
func TestB65C_S74Steps(t *testing.T) {
	got := AllAPI103Scenarios()
	require.Len(t, got, 1)
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
