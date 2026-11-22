package engine

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/tg"
)

// B-65 65g: ReplyRichQuoteStep (TEST-PLAN section 4, E-ok ... E-no-source). The fake is fake96 (steps_api96_test.go):
// it records each SendMessage call with its applied SendOptions and returns scripted replies in order.

func b65gTexts() []tg.RichText {
	return []tg.RichText{{Parts: []tg.RichText{{Plain: "quote "}, {Node: tg.RichTextBold{Type: "bold", Text: tg.RichText{Plain: "bold"}}}}}}
}

func b65gSource() *tg.Message {
	return &tg.Message{MessageID: 7, RichMessage: &tg.RichMessage{Blocks: tg.RichBlocks{tg.RichBlockParagraph{Type: "paragraph", Text: tg.RichText{Plain: "a"}}}}}
}

func b65gReply(id int) *tg.Message {
	return &tg.Message{MessageID: id, ReplyToMessage: &tg.Message{MessageID: 7}, Quote: &tg.TextQuote{RichTexts: b65gTexts(), Position: 7, IsManual: true}}
}

func b65gRuntime(f *fake96) *Runtime {
	rt := NewRuntime(f, testutil.TestChatID, 0)
	rt.LastMessage = b65gSource()
	return rt
}

func b65gTracked(rt *Runtime) []CreatedMessage { return rt.CreatedMessages }

// b65gCtxFake records the context of SendMessage (O-T3-03: the step must pass the runner's context on).
type b65gCtxFake struct {
	*fake96
	ctx context.Context
}

func (f *b65gCtxFake) SendMessage(ctx context.Context, chatID int64, text string, opts ...SendOption) (*tg.Message, error) {
	f.ctx = ctx
	return f.fake96.SendMessage(ctx, chatID, text, opts...)
}

type b65gCtxKey struct{}

func b65gIn(id int) []CreatedMessage {
	return []CreatedMessage{{ChatID: testutil.TestChatID, MessageID: id}}
}

// E-ok: the step sends exactly the planned request and accepts a reply whose quote matches.
func TestB65G_StepOK(t *testing.T) {
	f := &fake96{replies: []fake96Reply{{msg: b65gReply(42)}}}
	cf := &b65gCtxFake{fake96: f}
	rt := NewRuntime(cf, testutil.TestChatID, 0)
	rt.LastMessage = b65gSource()
	src := rt.LastMessage
	step := &ReplyRichQuoteStep{Label: "bold", Texts: b65gTexts(), Position: 7}
	ctx := context.WithValue(context.Background(), b65gCtxKey{}, "run")
	res, err := step.Execute(ctx, rt)
	require.NoError(t, err)
	require.NotNil(t, cf.ctx)
	assert.Equal(t, "run", cf.ctx.Value(b65gCtxKey{}), "the send gets the step's context")
	require.Len(t, f.messages, 1)
	assert.Equal(t, testutil.TestChatID, f.messages[0].chatID)
	assert.Equal(t, "galigo 10.4 rich quote bold", f.messages[0].text)
	assert.Equal(t, SendOptions{ReplyParameters: &tg.ReplyParameters{MessageID: 7, QuoteRichTexts: b65gTexts(), QuotePosition: 7}}, f.messages[0].opts)
	assert.Equal(t, b65gIn(42), b65gTracked(rt))
	assert.Equal(t, []int{42}, res.MessageIDs)
	assert.Same(t, src, rt.LastMessage, "the step never sets rt.LastMessage")
	assert.Equal(t, 1, f.calls)
	assert.Equal(t, "sendMessage", res.Method)
	assert.Equal(t, "sendMessage (rich quote bold)", res.StepName)
	q, _ := json.Marshal(b65gReply(42).Quote)
	assert.Equal(t, map[string]any{"label": "bold", "quote": string(q), "position": 7, "is_manual": true}, res.Evidence)
}

// E-bad: each wrong reply fails the step; the reply is still tracked and in MessageIDs.
func TestB65G_StepBadReply(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		edit       func(m *tg.Message)
	}{
		{"no quote", "no quote", func(m *tg.Message) { m.Quote = nil }},
		{"element changed", "rich_texts", func(m *tg.Message) { m.Quote.RichTexts = []tg.RichText{{Plain: "quote bold"}} }},
		{"unknown node", "rich_texts", func(m *tg.Message) {
			m.Quote.RichTexts = []tg.RichText{{Parts: []tg.RichText{{Plain: "quote "},
				{Node: tg.RichTextUnknown{Type: "bold", Raw: json.RawMessage(`{"type":"bold","text":"bold"}`)}}}}}
		}},
		{"position", "position 8", func(m *tg.Message) { m.Quote.Position = 8 }},
		{"not manual", "is_manual", func(m *tg.Message) { m.Quote.IsManual = false }},
		{"no reply target", "does not point", func(m *tg.Message) { m.ReplyToMessage = nil }},
		{"other reply target", "does not point", func(m *tg.Message) { m.ReplyToMessage.MessageID = 8 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reply := b65gReply(42)
			tc.edit(reply)
			f := &fake96{replies: []fake96Reply{{msg: reply}}}
			rt := b65gRuntime(f)
			src := rt.LastMessage
			res, err := (&ReplyRichQuoteStep{Label: "bold", Texts: b65gTexts(), Position: 7}).Execute(context.Background(), rt)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			require.NotNil(t, res)
			assert.Equal(t, []int{42}, res.MessageIDs)
			assert.Equal(t, b65gIn(42), b65gTracked(rt))
			assert.Same(t, src, rt.LastMessage, "rt.LastMessage unchanged after a failure")
			assert.Equal(t, 1, f.calls)
		})
	}
	// A good reply that comes with an error (A-T2-07), and no message with no error (the nil-message guard; O-T2-04).
	for _, tc := range []struct {
		name  string
		reply fake96Reply
		ids   []int
	}{
		{"good reply and an error", fake96Reply{msg: b65gReply(42), err: errors.New("x")}, []int{42}},
		{"no message, no error", fake96Reply{}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fake96{replies: []fake96Reply{tc.reply}}
			rt := b65gRuntime(f)
			src := rt.LastMessage
			var res *StepResult
			var err error
			require.NotPanics(t, func() { // a removed guard must fail here, not crash the binary (A-T3-02)
				res, err = (&ReplyRichQuoteStep{Label: "bold", Texts: b65gTexts(), Position: 7}).Execute(context.Background(), rt)
			})
			require.Error(t, err)
			require.NotNil(t, res)
			assert.Equal(t, tc.ids, res.MessageIDs)
			if tc.ids == nil {
				assert.Empty(t, b65gTracked(rt))
			} else {
				assert.Equal(t, b65gIn(42), b65gTracked(rt))
			}
			assert.Same(t, src, rt.LastMessage)
			assert.Equal(t, 1, f.calls)
		})
	}
}

// E-bad, two texts (O-T3-01): the reply keeps the first text and the length, and changes only the second.
func TestB65G_StepBadSecondText(t *testing.T) {
	texts := []tg.RichText{{Plain: "end"}, {Plain: "second"}}
	reply := &tg.Message{MessageID: 42, ReplyToMessage: &tg.Message{MessageID: 7},
		Quote: &tg.TextQuote{RichTexts: []tg.RichText{{Plain: "end"}, {Plain: "other"}}, Position: 18, IsManual: true}}
	f := &fake96{replies: []fake96Reply{{msg: reply}}}
	rt := b65gRuntime(f)
	src := rt.LastMessage
	res, err := (&ReplyRichQuoteStep{Label: "two texts", Texts: texts, Position: 18}).Execute(context.Background(), rt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rich_texts")
	require.NotNil(t, res)
	assert.Equal(t, []int{42}, res.MessageIDs)
	assert.Equal(t, b65gIn(42), b65gTracked(rt))
	assert.Same(t, src, rt.LastMessage)
	assert.Equal(t, 1, f.calls)
}

// E-refused: the measured refusal is the wanted result; nothing is tracked.
func TestB65G_StepRefused(t *testing.T) {
	f := &fake96{replies: []fake96Reply{{err: &tg.APIError{Code: 400, Description: "Bad Request: QUOTE_TEXT_INVALID"}}}}
	rt := b65gRuntime(f)
	src := rt.LastMessage
	res, err := (&ReplyRichQuoteStep{Label: "absent", Texts: []tg.RichText{{Plain: "absent text"}}, WantError: "QUOTE_TEXT_INVALID"}).Execute(context.Background(), rt)
	require.NoError(t, err)
	assert.Empty(t, b65gTracked(rt))
	assert.Empty(t, res.MessageIDs)
	assert.Same(t, src, rt.LastMessage, "rt.LastMessage unchanged after a refusal")
	assert.Equal(t, 1, f.calls)
	assert.Equal(t, map[string]any{"label": "absent", "error_code": 400, "description": "Bad Request: QUOTE_TEXT_INVALID"}, res.Evidence)
	require.Len(t, f.messages, 1)
	assert.Equal(t, &tg.ReplyParameters{MessageID: 7, QuoteRichTexts: []tg.RichText{{Plain: "absent text"}}}, f.messages[0].opts.ReplyParameters)
}

// E-refused-bad (O-T1-04): a refusal row needs no message AND a 400 with the text; each other case fails. A returned
// message is tracked (exact chat and id) and in MessageIDs; rt.LastMessage stays the source.
func TestB65G_StepRefusedBad(t *testing.T) {
	refusal := &tg.APIError{Code: 400, Description: "Bad Request: QUOTE_TEXT_INVALID"}
	for _, tc := range []struct {
		name  string
		reply fake96Reply
		msgID int
	}{
		{"nil, code 403", fake96Reply{err: &tg.APIError{Code: 403, Description: "Bad Request: QUOTE_TEXT_INVALID"}}, 0},
		{"nil, other text", fake96Reply{err: &tg.APIError{Code: 400, Description: "Bad Request: MESSAGE_ID_INVALID"}}, 0},
		{"nil, not an API error", fake96Reply{err: errors.New("QUOTE_TEXT_INVALID")}, 0},
		{"message, no error", fake96Reply{msg: &tg.Message{MessageID: 43}}, 43},
		{"message and the refusal", fake96Reply{msg: &tg.Message{MessageID: 43}, err: refusal}, 43},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fake96{replies: []fake96Reply{tc.reply}}
			rt := b65gRuntime(f)
			src := rt.LastMessage
			res, err := (&ReplyRichQuoteStep{Label: "absent", Texts: []tg.RichText{{Plain: "absent text"}}, WantError: "QUOTE_TEXT_INVALID"}).Execute(context.Background(), rt)
			require.Error(t, err)
			if tc.reply.err != nil {
				require.ErrorIs(t, err, tc.reply.err, "the step wraps the send error")
			}
			assert.NotContains(t, err.Error(), "%!", "no bad format verb in the error text")
			require.NotNil(t, res)
			if tc.msgID != 0 {
				assert.Equal(t, b65gIn(tc.msgID), b65gTracked(rt))
				assert.Equal(t, []int{tc.msgID}, res.MessageIDs)
			} else {
				assert.Empty(t, b65gTracked(rt))
				assert.Empty(t, res.MessageIDs)
			}
			assert.Same(t, src, rt.LastMessage, "rt.LastMessage unchanged")
			assert.Equal(t, 1, f.calls)
		})
	}
}

// E-malformed (PLAN section 4, O-P3-03): a nil message with a decode error fails the step and tracks nothing.
func TestB65G_StepMalformed(t *testing.T) {
	for _, want := range []string{"", "QUOTE_TEXT_INVALID"} {
		t.Run("WantError="+want, func(t *testing.T) {
			cause := errors.New("failed to parse response: json: cannot unmarshal object into Go struct field")
			f := &fake96{replies: []fake96Reply{{err: cause}}}
			rt := b65gRuntime(f)
			src := rt.LastMessage
			res, err := (&ReplyRichQuoteStep{Label: "bold", Texts: b65gTexts(), Position: 7, WantError: want}).Execute(context.Background(), rt)
			require.ErrorIs(t, err, cause, "the step returns the send error, not another check's error")
			require.NotNil(t, res)
			assert.Empty(t, b65gTracked(rt))
			assert.Empty(t, res.MessageIDs)
			assert.Same(t, src, rt.LastMessage)
			assert.Equal(t, 1, f.calls)
		})
	}
}

// E-no-source: without a rich source message the step fails and sends nothing.
func TestB65G_StepNoSource(t *testing.T) {
	for _, tc := range []struct {
		name string
		last *tg.Message
	}{{"nil", nil}, {"not rich", &tg.Message{MessageID: 7, Text: "t"}}} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fake96{replies: []fake96Reply{{msg: b65gReply(42)}}} // a send that must not happen returns normally
			rt := NewRuntime(f, testutil.TestChatID, 0)
			rt.LastMessage = tc.last
			var err error
			require.NotPanics(t, func() { // a removed source check must fail here, not crash the binary (A-T3-02)
				_, err = (&ReplyRichQuoteStep{Label: "bold", Texts: b65gTexts(), Position: 7}).Execute(context.Background(), rt)
			})
			require.Error(t, err)
			assert.Equal(t, 0, f.calls)
		})
	}
}
