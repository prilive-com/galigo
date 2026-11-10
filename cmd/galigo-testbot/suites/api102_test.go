package suites

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/tg"
)

// s68Kinds are the block kinds S68 sends and wants back, in order: the kinds the server returned in the live trial
// (E16/live/trial-api102.txt).
var s68Kinds = []string{"heading", "paragraph", "pre", "divider", "mathematical_expression", "anchor", "list",
	"blockquote", "pullquote", "table", "details", "map", "footer"}

// T-tb-1: the 10.2 suite is S68, with its covered methods and these exact steps (whole values through viewSteps).
func TestAPI102Scenarios(t *testing.T) {
	got := AllAPI102Scenarios()
	require.Len(t, got, 1)
	assert.Equal(t, "S68-RichBlocks", got[0].Name())
	assert.Equal(t, []string{"sendRichMessage", "sendRichMessageDraft"}, got[0].Covers())
	text := func(s string) tg.RichText { return tg.RichText{Plain: s} }
	cellA, cellB := text("a"), text("b")
	zoom := 12
	assert.Equal(t, viewSteps([]engine.Step{
		&engine.SendRichMessageStep{RichMessage: tg.InputRichMessage{Blocks: []tg.InputRichBlock{
			tg.InputRichBlockSectionHeading{Text: text("galigo 10.2 blocks"), Size: 2},
			tg.InputRichBlockParagraph{Text: text("paragraph")},
			tg.InputRichBlockPreformatted{Text: text("x := 1"), Language: "go"},
			tg.InputRichBlockDivider{},
			tg.InputRichBlockMathematicalExpression{Expression: "a^2+b^2=c^2"},
			tg.InputRichBlockAnchor{Name: "galigo"},
			tg.InputRichBlockList{Items: []tg.InputRichBlockListItem{
				{Blocks: []tg.InputRichBlock{tg.InputRichBlockParagraph{Text: text("item")}}}}},
			tg.InputRichBlockBlockQuotation{Blocks: []tg.InputRichBlock{tg.InputRichBlockParagraph{Text: text("quote")}}},
			tg.InputRichBlockPullQuotation{Text: text("pull")},
			tg.InputRichBlockTable{Cells: [][]tg.RichBlockTableCell{{
				{Text: &cellA, Align: "left", VAlign: "top"}, {Text: &cellB, Align: "left", VAlign: "top"}}}, IsBordered: true},
			tg.InputRichBlockDetails{Summary: text("details"),
				Blocks: []tg.InputRichBlock{tg.InputRichBlockParagraph{Text: text("inside")}}},
			tg.InputRichBlockMap{Location: tg.Location{Latitude: 52.37, Longitude: 4.89}, Zoom: &zoom},
			tg.InputRichBlockFooter{Text: text("footer")},
		}}, WantBlocks: s68Kinds},
		&engine.CleanupStep{}, // before the draft: a Skip ends the scenario (as S67)
		&engine.SendRichMessageDraftStep{DraftID: 1, RichMessage: tg.InputRichMessage{
			Blocks: []tg.InputRichBlock{tg.InputRichBlockThinking{Text: text("Thinking")}}}},
	}), viewSteps(got[0].Steps()))

	// T-tb-2: the wanted kinds are the kinds S68 sends (the top-level "type" of each block, in order), and the sent
	// message encodes.
	step, ok := got[0].Steps()[0].(*engine.SendRichMessageStep)
	require.True(t, ok)
	out, err := json.Marshal(step.RichMessage)
	require.NoError(t, err)
	var sent struct {
		Blocks []struct {
			Type string `json:"type"`
		} `json:"blocks"`
	}
	require.NoError(t, json.Unmarshal(out, &sent))
	var kinds []string
	for _, b := range sent.Blocks {
		kinds = append(kinds, b.Type)
	}
	assert.Equal(t, s68Kinds, kinds)
	assert.Equal(t, s68Kinds, step.WantBlocks)
}

// fake102dGroup runs S68 on the real runner as in a group: send returns a rich message with the S68 kinds, the draft is
// refused with the group error, and deleteMessage is recorded. Any other SenderClient method panics.
type fake102dGroup struct {
	engine.SenderClient
	sent    []tg.InputRichMessage
	drafts  int
	deleted [][2]int64
}

func (f *fake102dGroup) SendRichMessage(_ context.Context, _ int64, rm tg.InputRichMessage) (*tg.Message, error) {
	f.sent = append(f.sent, rm)
	var b strings.Builder
	b.WriteString(`{"blocks":[`)
	for i, k := range s68Kinds {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"type":"` + k + `"}`)
	}
	b.WriteString(`]}`)
	var rich tg.RichMessage
	if err := json.Unmarshal([]byte(b.String()), &rich); err != nil {
		return nil, err
	}
	return &tg.Message{MessageID: 502, Chat: &tg.Chat{ID: testutil.TestChatID}, RichMessage: &rich}, nil
}

func (f *fake102dGroup) SendRichMessageDraft(context.Context, int64, int, tg.InputRichMessage) error {
	f.drafts++
	return &tg.APIError{Code: 400, Description: "Bad Request: TEXTDRAFT_PEER_INVALID"}
}

func (f *fake102dGroup) DeleteMessage(_ context.Context, chatID int64, messageID int) error {
	f.deleted = append(f.deleted, [2]int64{chatID, int64(messageID)})
	return nil
}

// T-tb-3: in a group the draft is skipped, and the sent message is still deleted.
func TestAPI102D_S68GroupCleanup(t *testing.T) {
	f := &fake102dGroup{}
	rt := engine.NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
	runner := engine.NewRunner(rt, engine.RunnerConfig{MaxMessages: 100}, slog.New(slog.DiscardHandler))

	res := runner.Run(context.Background(), S68_RichBlocks())
	assert.True(t, res.Success)
	assert.True(t, res.Skipped)
	require.Len(t, f.sent, 1)
	assert.Len(t, f.sent[0].Blocks, 13)
	assert.Equal(t, 1, f.drafts)
	assert.Equal(t, [][2]int64{{testutil.TestChatID, 502}}, f.deleted)
	assert.Empty(t, rt.CreatedMessages)
}
