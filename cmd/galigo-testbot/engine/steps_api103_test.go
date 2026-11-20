package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.3 rich message steps (B-65 65c, TEST-PLAN §4).

// T-types-unknown: an unknown block is named with its type, so a missing decode case shows in the step's error.
func TestB65C_RichBlockTypes(t *testing.T) {
	got := richBlockTypes(api101dRich(tg.RichBlockParagraph{Type: "paragraph"}, tg.RichBlockUnknown{Type: "buttons"},
		tg.RichBlockUnknown{}))
	assert.Equal(t, []string{"paragraph", "unknown:buttons", "unknown:"}, got)
}

// b65cGood returns, as fresh values, the blocks S74's first message decodes into.
func b65cGood() []tg.RichBlock {
	return []tg.RichBlock{
		tg.RichBlockButtons{Type: "buttons", Align: "center", Buttons: []tg.RichMessageButton{
			{Text: tg.RichText{Plain: "cb"}, Style: tg.ButtonStylePrimary, CallbackData: "s74"},
			{Text: tg.RichText{Plain: "url"}, URL: "https://t.me/"},
			{Text: tg.RichText{Plain: "copy"}, CopyText: &tg.CopyTextButton{Text: "galigo"}},
			{Text: tg.RichText{Plain: "off"}, Disabled: &tg.DisabledButton{}},
		}},
		tg.RichBlockParagraph{Type: "paragraph", Text: tg.RichText{Parts: []tg.RichText{{Plain: "a"},
			{Node: tg.RichTextButton{Type: "button", Button: tg.RichMessageButton{Text: tg.RichText{Plain: "n"}, CallbackData: "s74-node"}}}}}},
		tg.RichBlockTable{Type: "table", IsCompact: true},
		tg.RichBlockExpandableBlockQuotation{Type: "expandable_blockquote", Text: tg.RichText{Plain: "galigo 10.3 expandable"}},
	}
}

// b65cButtons changes button i of block 0 in blocks.
func b65cButtons(blocks []tg.RichBlock, i int, change func(*tg.RichMessageButton)) {
	row := blocks[0].(tg.RichBlockButtons)
	change(&row.Buttons[i])
	blocks[0] = row
}

// T-verify-ok
func TestB65C_VerifyRichAPI103Step(t *testing.T) {
	step := &VerifyRichAPI103Step{}
	f := &fake101d{}
	rt := NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
	rt.LastMessage = api101dReply(api101dRich(b65cGood()...), "")
	res, err := step.Execute(context.Background(), rt)
	require.NoError(t, err)
	assert.Equal(t, "verify rich message (10.3)", res.StepName)
	assert.Equal(t, "sendRichMessage", res.Method)
	assert.Equal(t, map[string]any{"message_id": api101dMsgID,
		"checked": []string{"buttons", "text_button", "is_compact", "expandable_blockquote"}}, res.Evidence)
	assert.Empty(t, res.MessageIDs)
	assert.Empty(t, f.calls, "no API call")
	assert.Empty(t, rt.CreatedMessages)
}

// T-verify-each: one row per condition, each the good value with one thing broken; the error names it exactly.
func TestB65C_VerifyRichAPI103StepEach(t *testing.T) {
	const pre = "verify rich message: "
	divider := tg.RichBlockDivider{Type: "divider"}
	rows := []struct {
		name   string
		change func(rt *Runtime, b []tg.RichBlock) []tg.RichBlock
		want   string
	}{
		{"no last message", func(rt *Runtime, b []tg.RichBlock) []tg.RichBlock { rt.LastMessage = nil; return b },
			pre + "no rich message in the last message"},
		{"no rich message", func(rt *Runtime, b []tg.RichBlock) []tg.RichBlock { rt.LastMessage.RichMessage = nil; return b },
			pre + "no rich message in the last message"},
		{"3 blocks", func(_ *Runtime, b []tg.RichBlock) []tg.RichBlock { return b[:3] }, pre + "3 blocks, want 4"},
		{"block 0 unknown", func(_ *Runtime, b []tg.RichBlock) []tg.RichBlock {
			b[0] = tg.RichBlockUnknown{Type: "buttons"}
			return b
		},
			pre + "block 0 is tg.RichBlockUnknown, want tg.RichBlockButtons"},
		{"align", func(_ *Runtime, b []tg.RichBlock) []tg.RichBlock {
			row := b[0].(tg.RichBlockButtons)
			row.Align = "left"
			b[0] = row
			return b
		}, pre + `buttons align "left", want center`},
		{"3 buttons", func(_ *Runtime, b []tg.RichBlock) []tg.RichBlock {
			row := b[0].(tg.RichBlockButtons)
			row.Buttons = row.Buttons[:3]
			b[0] = row
			return b
		}, pre + "3 buttons, want 4"},
		{"button 0 no callback", func(_ *Runtime, b []tg.RichBlock) []tg.RichBlock {
			b65cButtons(b, 0, func(x *tg.RichMessageButton) { x.CallbackData = "" })
			return b
		}, pre + "button 0 is not a primary callback button"},
		{"button 0 style", func(_ *Runtime, b []tg.RichBlock) []tg.RichBlock {
			b65cButtons(b, 0, func(x *tg.RichMessageButton) { x.Style = tg.ButtonStyleDanger })
			return b
		}, pre + "button 0 is not a primary callback button"},
		{"button 1 no URL", func(_ *Runtime, b []tg.RichBlock) []tg.RichBlock {
			b65cButtons(b, 1, func(x *tg.RichMessageButton) { x.URL = "" })
			return b
		}, pre + "button 1 has no URL"},
		{"button 2 no copy", func(_ *Runtime, b []tg.RichBlock) []tg.RichBlock {
			b65cButtons(b, 2, func(x *tg.RichMessageButton) { x.CopyText = nil })
			return b
		}, pre + `button 2 does not copy "galigo"`},
		{"button 2 copy text", func(_ *Runtime, b []tg.RichBlock) []tg.RichBlock {
			b65cButtons(b, 2, func(x *tg.RichMessageButton) { x.CopyText = &tg.CopyTextButton{Text: "x"} })
			return b
		}, pre + `button 2 does not copy "galigo"`},
		{"button 3 not disabled", func(_ *Runtime, b []tg.RichBlock) []tg.RichBlock {
			b65cButtons(b, 3, func(x *tg.RichMessageButton) { x.Disabled = nil })
			return b
		}, pre + "button 3 is not disabled"},
		{"block 1 heading", func(_ *Runtime, b []tg.RichBlock) []tg.RichBlock {
			b[1] = tg.RichBlockSectionHeading{Type: "heading"}
			return b
		}, pre + "block 1 is tg.RichBlockSectionHeading, want tg.RichBlockParagraph"},
		{"block 1 no button node", func(_ *Runtime, b []tg.RichBlock) []tg.RichBlock {
			b[1] = tg.RichBlockParagraph{Type: "paragraph", Text: tg.RichText{Parts: []tg.RichText{{Plain: "a"},
				{Node: tg.RichTextBold{Type: "bold", Text: tg.RichText{Plain: "b"}}}}}}
			return b
		}, pre + "block 1 has no button node with callback data"},
		{"block 1 node without callback", func(_ *Runtime, b []tg.RichBlock) []tg.RichBlock {
			b[1] = tg.RichBlockParagraph{Type: "paragraph", Text: tg.RichText{Parts: []tg.RichText{{Plain: "a"},
				{Node: tg.RichTextButton{Type: "button", Button: tg.RichMessageButton{URL: "https://t.me/"}}}}}}
			return b
		}, pre + "block 1 has no button node with callback data"},
		{"block 2 not a table", func(_ *Runtime, b []tg.RichBlock) []tg.RichBlock { b[2] = divider; return b },
			pre + "block 2 is tg.RichBlockDivider, want tg.RichBlockTable"},
		{"table not compact", func(_ *Runtime, b []tg.RichBlock) []tg.RichBlock {
			b[2] = tg.RichBlockTable{Type: "table", IsStriped: true}
			return b
		}, pre + "the table is not compact"},
		{"block 3 not expandable", func(_ *Runtime, b []tg.RichBlock) []tg.RichBlock { b[3] = divider; return b },
			pre + "block 3 is tg.RichBlockDivider, want tg.RichBlockExpandableBlockQuotation"},
		{"quotation text", func(_ *Runtime, b []tg.RichBlock) []tg.RichBlock {
			b[3] = tg.RichBlockExpandableBlockQuotation{Type: "expandable_blockquote", Text: tg.RichText{Plain: "x"}}
			return b
		}, pre + `quotation text "x", want "galigo 10.3 expandable"`},
	}
	require.Len(t, rows, 19)
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			f := &fake101d{}
			rt := NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
			rt.LastMessage = api101dReply(api101dRich(), "")
			blocks := r.change(rt, b65cGood())
			if rt.LastMessage != nil && rt.LastMessage.RichMessage != nil {
				rt.LastMessage.RichMessage.Blocks = blocks
			}
			res, err := (&VerifyRichAPI103Step{}).Execute(context.Background(), rt)
			assert.EqualError(t, err, r.want)
			assert.False(t, IsSkip(err))
			require.NotNil(t, res)
			assert.Nil(t, res.Evidence)
			assert.Empty(t, f.calls)
		})
	}
}

// b65cDoc is a decoded document block with the given file id.
func b65cDoc(id string) tg.RichBlock {
	return tg.RichBlockDocument{Type: "document", Document: tg.Document{FileID: id, FileUniqueID: "AgAD-fake"}}
}

// b65cDocSent is what SendRichDocumentBlockStep must send for the file id.
func b65cDocSent(id string) tg.InputRichMessage {
	return tg.InputRichMessage{Blocks: []tg.InputRichBlock{tg.InputRichBlockDocument{
		Document: sender.NewInputMediaDocument(sender.FromFileID(id)),
		Caption:  &tg.RichBlockCaption{Text: tg.RichText{Plain: "document block"}},
	}}}
}

// T-docblock: the step sends the first document of the last message by file_id, to rt.ChatID, and tracks the reply.
func TestB65C_SendRichDocumentBlockStep(t *testing.T) {
	step := &SendRichDocumentBlockStep{WantBlocks: []string{"document"}}
	reply := api101dReply(api101dRich(b65cDoc("BQAC-fake-doc")), "")
	f := &fake101d{msg: reply}
	rt := NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
	rt.LastMessage = api101dReply(api101dRich(api101dPara(), b65cDoc("BQAC-fake-doc"), b65cDoc("BQAC-fake-doc2")), "")
	rt.LastMessage.Chat = &tg.Chat{ID: testutil.TestChatID - 1} // the step sends to rt.ChatID, not to the last message's chat (C1-01)
	res, err := step.Execute(context.Background(), rt)
	require.NoError(t, err)
	assert.Equal(t, "sendRichMessage (document block)", res.StepName)
	assert.Equal(t, "sendRichMessage", res.Method)
	assert.Equal(t, []fake101dCall{{method: "sendRichMessage", chatID: testutil.TestChatID, rich: b65cDocSent("BQAC-fake-doc")}}, f.calls)
	assert.Equal(t, []int{api101dMsgID}, res.MessageIDs)
	assert.Equal(t, []CreatedMessage{{ChatID: testutil.TestChatID, MessageID: api101dMsgID}}, rt.CreatedMessages)
	assert.Same(t, reply, rt.LastMessage)
	assert.Equal(t, map[string]any{"message_id": api101dMsgID, "blocks": []string{"document"}}, res.Evidence)
}

// T-docblock-err
func TestB65C_SendRichDocumentBlockStepErrors(t *testing.T) {
	step := &SendRichDocumentBlockStep{WantBlocks: []string{"document"}}
	withDoc := func() *tg.Message { return api101dReply(api101dRich(api101dPara(), b65cDoc("BQAC-fake-doc")), "") }
	noCall := []struct {
		name string
		last *tg.Message
		want string
	}{
		{"no last message", nil, "sendRichMessage: no rich message to take the document from"},
		{"no rich message", api101dReply(nil, "t"), "sendRichMessage: no rich message to take the document from"},
		{"no document block", api101dReply(api101dRich(api101dPara()), ""),
			"sendRichMessage: no document file_id in the rich message of the last message"},
		{"empty file id", api101dReply(api101dRich(b65cDoc("")), ""),
			"sendRichMessage: no document file_id in the rich message of the last message"},
	}
	for _, r := range noCall {
		t.Run(r.name, func(t *testing.T) {
			f := &fake101d{msg: api101dReply(api101dRich(b65cDoc("BQAC-fake-doc")), "")}
			rt := NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
			rt.LastMessage = r.last
			_, err := step.Execute(context.Background(), rt)
			assert.EqualError(t, err, r.want)
			assert.Empty(t, f.calls)
			assert.Empty(t, rt.CreatedMessages)
		})
	}

	t.Run("send error", func(t *testing.T) {
		boom := errors.New("boom")
		f := &fake101d{err: boom}
		rt := NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
		last := withDoc()
		rt.LastMessage = last
		res, err := step.Execute(context.Background(), rt)
		assert.ErrorIs(t, err, boom)
		assert.Equal(t, "sendRichMessage (document block)", res.StepName) // C2-02: the name also on a failed send
		assert.Len(t, f.calls, 1)
		assert.Empty(t, rt.CreatedMessages)
		assert.Same(t, last, rt.LastMessage)
	})

	t.Run("no message in the reply", func(t *testing.T) {
		f := &fake101d{}
		rt := NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
		rt.LastMessage = withDoc()
		res, err := step.Execute(context.Background(), rt)
		assert.EqualError(t, err, "sendRichMessage: no message in the reply")
		assert.Equal(t, "sendRichMessage (document block)", res.StepName)
		assert.Empty(t, rt.CreatedMessages)
	})

	tracked := []struct {
		name  string
		reply *tg.Message
		want  string
	}{
		{"reply without rich message", api101dReply(nil, "t"), "sendRichMessage: the reply has no rich_message"},
		{"other blocks", api101dReply(api101dRich(api101dPara()), ""), "sendRichMessage: blocks [paragraph], want [document]"},
		{"reply with text", api101dReply(api101dRich(b65cDoc("BQAC-fake-doc")), "t"), `sendRichMessage: the reply has text "t"`},
	}
	for _, r := range tracked {
		t.Run(r.name, func(t *testing.T) {
			f := &fake101d{msg: r.reply}
			rt := NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
			rt.LastMessage = withDoc()
			res, err := step.Execute(context.Background(), rt)
			assert.EqualError(t, err, r.want)
			assert.Equal(t, "sendRichMessage (document block)", res.StepName)
			assert.Equal(t, []int{api101dMsgID}, res.MessageIDs)
			assert.Equal(t, []CreatedMessage{{ChatID: testutil.TestChatID, MessageID: api101dMsgID}}, rt.CreatedMessages,
				"the sent message is tracked, so the cleanup deletes it")
		})
	}
}
