package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.1 rich message steps (B-25 10.1d, TEST-PLAN §4, rows T-tb-1 … T-tb-3).

const (
	api101dMsgID    = 4242
	api101dEditChat = int64(-100777) // the edited message's chat: ≠ rt.ChatID, so a step that uses rt.ChatID fails
	api101dEditMsg  = 31             // the edited message's id: ≠ api101dMsgID
)

// fake101d is a SenderClient for the 10.1d steps: it embeds the interface (calling any other method panics) and
// overrides the three rich message methods. It records each call's arguments and returns the scripted reply.
type fake101d struct {
	SenderClient

	calls []fake101dCall
	msg   *tg.Message
	err   error
}

type fake101dCall struct {
	method    string
	chatID    int64
	messageID int
	draftID   int
	rich      tg.InputRichMessage
}

func (f *fake101d) SendRichMessage(_ context.Context, chatID int64, rm tg.InputRichMessage) (*tg.Message, error) {
	f.calls = append(f.calls, fake101dCall{method: "sendRichMessage", chatID: chatID, rich: rm})
	return f.msg, f.err
}

func (f *fake101d) EditMessageRich(_ context.Context, chatID int64, messageID int, rm tg.InputRichMessage) (*tg.Message, error) {
	f.calls = append(f.calls, fake101dCall{method: "editMessageText", chatID: chatID, messageID: messageID, rich: rm})
	return f.msg, f.err
}

func (f *fake101d) SendRichMessageDraft(_ context.Context, chatID int64, draftID int, rm tg.InputRichMessage) error {
	f.calls = append(f.calls, fake101dCall{method: "sendRichMessageDraft", chatID: chatID, draftID: draftID, rich: rm})
	return f.err
}

func api101dRich(blocks ...tg.RichBlock) *tg.RichMessage {
	return &tg.RichMessage{Blocks: tg.RichBlocks(blocks)}
}

func api101dPara() tg.RichBlock {
	return tg.RichBlockParagraph{Type: "paragraph", Text: tg.RichText{Plain: "p"}}
}

func api101dReply(rich *tg.RichMessage, text string) *tg.Message {
	return &tg.Message{MessageID: api101dMsgID, Chat: &tg.Chat{ID: testutil.TestChatID}, RichMessage: rich, Text: text}
}

var api101dIn = tg.InputRichMessage{Markdown: "**m**", IsRTL: true}

// T-tb-1
func TestAPI101D_SendRichMessageStep(t *testing.T) {
	step := &SendRichMessageStep{RichMessage: api101dIn, WantBlocks: []string{"paragraph"}}
	wantCall := []fake101dCall{{method: "sendRichMessage", chatID: testutil.TestChatID, rich: api101dIn}}

	t.Run("ok", func(t *testing.T) {
		reply := api101dReply(api101dRich(api101dPara()), "")
		f := &fake101d{msg: reply}
		rt := NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
		res, err := step.Execute(context.Background(), rt)
		require.NoError(t, err)
		assert.Equal(t, "sendRichMessage", res.Method)
		assert.Equal(t, []int{api101dMsgID}, res.MessageIDs)
		assert.Same(t, reply, rt.LastMessage)
		assert.Equal(t, []CreatedMessage{{ChatID: testutil.TestChatID, MessageID: api101dMsgID}}, rt.CreatedMessages)
		assert.Equal(t, wantCall, f.calls)
	})

	bad := []struct {
		name  string
		reply *tg.Message
		err   error
		want  string
	}{
		{"no rich message", api101dReply(nil, ""), nil, "the reply has no rich_message"},
		{"text", api101dReply(api101dRich(api101dPara()), "t"), nil, `the reply has text "t"`},
		{"other blocks", api101dReply(api101dRich(api101dPara(), tg.RichBlockDivider{Type: "divider"}), ""), nil,
			"blocks [paragraph divider], want [paragraph]"},
		{"no message", nil, nil, "no message in the reply"},
	}
	for _, r := range bad {
		t.Run(r.name, func(t *testing.T) {
			f := &fake101d{msg: r.reply}
			_, err := step.Execute(context.Background(), NewRuntime(f, testutil.TestChatID, testutil.TestUserID))
			require.Error(t, err)
			assert.Contains(t, err.Error(), r.want)
			assert.False(t, IsSkip(err))
			assert.Equal(t, wantCall, f.calls)
		})
	}

	t.Run("adapter error", func(t *testing.T) {
		boom := errors.New("boom")
		f := &fake101d{err: boom}
		rt := NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
		_, err := step.Execute(context.Background(), rt)
		assert.ErrorIs(t, err, boom)
		assert.Nil(t, rt.LastMessage)
		assert.Empty(t, rt.CreatedMessages)
	})
}

// T-tb-2
func TestAPI101D_EditMessageRichStep(t *testing.T) {
	in := tg.InputRichMessage{HTML: "<p>e</p>"}
	step := &EditMessageRichStep{RichMessage: in, WantBlocks: []string{"paragraph"}}
	last := &tg.Message{MessageID: api101dEditMsg, Chat: &tg.Chat{ID: api101dEditChat}}

	t.Run("no last message", func(t *testing.T) {
		f := &fake101d{}
		_, err := step.Execute(context.Background(), NewRuntime(f, testutil.TestChatID, testutil.TestUserID))
		require.EqualError(t, err, "editMessageText: no message to edit")
		assert.Empty(t, f.calls)
	})

	t.Run("ok", func(t *testing.T) {
		reply := api101dReply(api101dRich(api101dPara()), "")
		f := &fake101d{msg: reply}
		rt := NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
		rt.LastMessage = last
		res, err := step.Execute(context.Background(), rt)
		require.NoError(t, err)
		assert.Equal(t, "editMessageText", res.Method)
		assert.Equal(t, []int{api101dMsgID}, res.MessageIDs)
		assert.Same(t, reply, rt.LastMessage)
		assert.Equal(t, []fake101dCall{{method: "editMessageText", chatID: api101dEditChat, messageID: api101dEditMsg,
			rich: tg.InputRichMessage{HTML: "<p>e</p>"}}}, f.calls)
	})

	for name, reply := range map[string]*tg.Message{
		"no rich message": api101dReply(nil, "x"),
		"other blocks":    api101dReply(api101dRich(tg.RichBlockDivider{Type: "divider"}), ""),
	} {
		t.Run(name, func(t *testing.T) {
			f := &fake101d{msg: reply}
			rt := NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
			rt.LastMessage = last
			_, err := step.Execute(context.Background(), rt)
			require.Error(t, err)
			assert.False(t, IsSkip(err))
			assert.Same(t, last, rt.LastMessage)
		})
	}
}

// T-tb-3
func TestAPI101D_SendRichMessageDraftStep(t *testing.T) {
	in := tg.InputRichMessage{Markdown: "d"}
	step := &SendRichMessageDraftStep{DraftID: 7, RichMessage: in}
	wantCall := []fake101dCall{{method: "sendRichMessageDraft", chatID: testutil.TestChatID, draftID: 7,
		rich: tg.InputRichMessage{Markdown: "d"}}}

	t.Run("ok", func(t *testing.T) {
		f := &fake101d{}
		res, err := step.Execute(context.Background(), NewRuntime(f, testutil.TestChatID, testutil.TestUserID))
		require.NoError(t, err)
		assert.Equal(t, "sendRichMessageDraft", res.Method)
		assert.Equal(t, wantCall, f.calls)
	})

	rows := []struct {
		name string
		err  error
		skip bool
	}{
		{"group refused", &tg.APIError{Code: 400, Description: "Bad Request: TEXTDRAFT_PEER_INVALID"}, true},
		{"other 400", &tg.APIError{Code: 400, Description: "Bad Request: RICH_MESSAGE_MARKDOWN_INVALID"}, false},
		{"403 peer invalid", &tg.APIError{Code: 403, Description: "Forbidden: TEXTDRAFT_PEER_INVALID"}, false},
		{"plain error", errors.New("Bad Request: TEXTDRAFT_PEER_INVALID"), false},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			f := &fake101d{err: r.err}
			_, err := step.Execute(context.Background(), NewRuntime(f, testutil.TestChatID, testutil.TestUserID))
			require.Error(t, err)
			assert.Equal(t, r.skip, IsSkip(err))
			if !r.skip {
				assert.ErrorIs(t, err, r.err)
			}
			assert.Equal(t, wantCall, f.calls)
		})
	}
}
