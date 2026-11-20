package engine

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.3 reply markup step, S75 (B-65 65d, TEST-PLAN §5).

const (
	b65dGroup = int64(-1009876543210) // the fake test group (more than 32 bits)
	b65dAdmin = int64(777000111)      // the fake admin id: not the group, so a send to it fails the call check
	b65dMsgID = 42
	b65dText  = "galigo 10.3 reply markup"
	b65dP     = "sendMessage (reply markup): the reply markup differs: "
)

// fakeMarkup is a SenderClient for SendReplyMarkupEchoStep: it embeds the interface (calling any other method
// panics) and records every SendMessage call. The reply carries the markup that reply builds, as a fresh value.
type fakeMarkup struct {
	SenderClient

	calls []fakeMarkupCall
	reply func() *tg.InlineKeyboardMarkup
	msgID int
	err   error
}

type fakeMarkupCall struct {
	chatID int64
	text   string
	opts   SendOptions
}

func (f *fakeMarkup) SendMessage(_ context.Context, chatID int64, text string, opts ...SendOption) (*tg.Message, error) {
	var o SendOptions
	for _, opt := range opts {
		opt(&o)
	}
	f.calls = append(f.calls, fakeMarkupCall{chatID: chatID, text: text, opts: o})
	id := f.msgID
	if id == 0 {
		id = b65dMsgID
	}
	msg := &tg.Message{MessageID: id, Chat: &tg.Chat{ID: chatID}}
	if f.reply != nil {
		msg.ReplyMarkup = f.reply()
	}
	return msg, f.err
}

// b65dMarkup returns, as a fresh value, the step fixture M: two rows; row 0 button 0 has callback "x" (1 byte),
// row 0 button 1 is the disabled "off" (3 bytes), row 1 holds one button with two kinds.
func b65dMarkup() tg.InlineKeyboardMarkup {
	return tg.InlineKeyboardMarkup{
		InlineKeyboard: [][]tg.InlineKeyboardButton{
			{{Text: "cb", CallbackData: "x"}, {Text: "off", Disabled: &tg.DisabledButton{}}},
			{{Text: "both", CallbackData: "y", Disabled: &tg.DisabledButton{}}},
		},
		ForceReply: true,
	}
}

// b65dReply returns a reply builder: a fresh copy of M, changed by change.
func b65dReply(change func(*tg.InlineKeyboardMarkup)) func() *tg.InlineKeyboardMarkup {
	return func() *tg.InlineKeyboardMarkup {
		m := b65dMarkup()
		if change != nil {
			change(&m)
		}
		return &m
	}
}

func b65dStep() *SendReplyMarkupEchoStep {
	return &SendReplyMarkupEchoStep{Text: b65dText, Markup: b65dMarkup()}
}

// b65dShared: exactly one send to the group with the text and only the reply markup, the message tracked once and
// reported once (X-T2-04).
func b65dShared(t *testing.T, f *fakeMarkup, rt *Runtime, res *StepResult, sent tg.InlineKeyboardMarkup) {
	t.Helper()
	require.Len(t, f.calls, 1)
	assert.Equal(t, fakeMarkupCall{chatID: b65dGroup, text: b65dText, opts: SendOptions{ReplyMarkup: &sent}}, f.calls[0])
	assert.Equal(t, []CreatedMessage{{ChatID: b65dGroup, MessageID: b65dMsgID}}, rt.CreatedMessages)
	require.NotNil(t, res)
	assert.Equal(t, "sendMessage (reply markup)", res.StepName)
	assert.Equal(t, "sendMessage", res.Method)
	assert.Equal(t, []int{b65dMsgID}, res.MessageIDs)
	assert.Equal(t, map[string]any{"message_id": b65dMsgID, "rows": 2, "force_reply": true}, res.Evidence)
}

// T-step-ok
func TestB65D_StepOK(t *testing.T) {
	f := &fakeMarkup{reply: b65dReply(nil)}
	rt := NewRuntime(f, b65dGroup, b65dAdmin)
	res, err := b65dStep().Execute(context.Background(), rt)
	require.NoError(t, err)
	b65dShared(t, f, rt, res, b65dMarkup())
}

// T-step-ok, the equal leaf (X-T3-02): equal WebApp values at different addresses are equal.
func TestB65D_StepEqualPointers(t *testing.T) {
	webApp := func(m *tg.InlineKeyboardMarkup) {
		m.InlineKeyboard[0][0].WebApp = &tg.WebAppInfo{URL: "https://w.example/"}
	}
	step := b65dStep()
	webApp(&step.Markup)
	f := &fakeMarkup{reply: b65dReply(webApp)}
	rt := NewRuntime(f, b65dGroup, b65dAdmin)
	res, err := step.Execute(context.Background(), rt)
	require.NoError(t, err)
	sent := b65dMarkup()
	webApp(&sent)
	b65dShared(t, f, rt, res, sent)
	assert.NotSame(t, step.Markup.InlineKeyboard[0][0].WebApp, rt.LastMessage.ReplyMarkup.InlineKeyboard[0][0].WebApp)
}

// b65dMismatch is one T-step-mismatch / T-step-noleak leaf: a reply with one difference and the whole error.
type b65dMismatch struct {
	name   string
	change func(*tg.InlineKeyboardMarkup)
	nilRM  bool
	want   string
}

func b65dMismatches() []b65dMismatch {
	return []b65dMismatch{
		{name: "a disabled dropped", change: func(m *tg.InlineKeyboardMarkup) { m.InlineKeyboard[0][1].Disabled = nil },
			want: b65dP + "row 0 button 1: Disabled differs (sent set=true, received set=false)"},
		{name: "b force_reply false", change: func(m *tg.InlineKeyboardMarkup) { m.ForceReply = false },
			want: b65dP + "force_reply sent true, received false"},
		{name: "c extra button", change: func(m *tg.InlineKeyboardMarkup) {
			m.InlineKeyboard[0] = append(m.InlineKeyboard[0], tg.InlineKeyboardButton{Text: "z"})
		}, want: b65dP + "row 0: 2 buttons sent, 3 received"},
		{name: "d second row missing", change: func(m *tg.InlineKeyboardMarkup) { m.InlineKeyboard = m.InlineKeyboard[:1] },
			want: b65dP + "2 rows sent, 1 received"},
		{name: "e row 1 callback", change: func(m *tg.InlineKeyboardMarkup) { m.InlineKeyboard[1][0].CallbackData = "z" },
			want: b65dP + "row 1 button 0: CallbackData differs (sent 1 bytes, received 1 bytes)"},
		{name: "f style", change: func(m *tg.InlineKeyboardMarkup) { m.InlineKeyboard[0][0].Style = tg.ButtonStylePrimary },
			want: b65dP + "row 0 button 0: Style differs (sent 0 bytes, received 7 bytes)"},
		{name: "g nil markup", nilRM: true, want: "sendMessage (reply markup): the reply has no reply markup"},
		{name: "h pay", change: func(m *tg.InlineKeyboardMarkup) { m.InlineKeyboard[0][0].Pay = true },
			want: b65dP + "row 0 button 0: Pay differs"},
		{name: "i button missing", change: func(m *tg.InlineKeyboardMarkup) { m.InlineKeyboard[0] = m.InlineKeyboard[0][:1] },
			want: b65dP + "row 0: 2 buttons sent, 1 received"},
		{name: "j extra row", change: func(m *tg.InlineKeyboardMarkup) {
			m.InlineKeyboard = append(m.InlineKeyboard, []tg.InlineKeyboardButton{{Text: "z"}})
		}, want: b65dP + "2 rows sent, 3 received"},
		{name: "k row 1 url", change: func(m *tg.InlineKeyboardMarkup) { m.InlineKeyboard[1][0].URL = "https://u.example/" },
			want: b65dP + "row 1 button 0: URL differs (sent 0 bytes, received 18 bytes)"},
		// C3-01: four differences; the error names the first in row, button and field order.
		{name: "l several differences", change: func(m *tg.InlineKeyboardMarkup) {
			m.InlineKeyboard[0][0].Style = tg.ButtonStylePrimary
			m.InlineKeyboard[0][0].CallbackData = "z"
			m.InlineKeyboard[0][1].Text = "z"
			m.InlineKeyboard[1][0].URL = "https://u.example/"
		}, want: b65dP + "row 0 button 0: Style differs (sent 0 bytes, received 7 bytes)"},
	}
}

// T-step-mismatch: each difference fails the step with its whole error; the message is still sent, tracked and
// reported (X-P3-04).
func TestB65D_StepMismatch(t *testing.T) {
	for _, tc := range b65dMismatches() {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeMarkup{reply: b65dReply(tc.change)}
			if tc.nilRM {
				f.reply = nil
			}
			rt := NewRuntime(f, b65dGroup, b65dAdmin)
			res, err := b65dStep().Execute(context.Background(), rt)
			require.Error(t, err)
			assert.Equal(t, tc.want, err.Error())
			b65dShared(t, f, rt, res, b65dMarkup())
		})
	}
}

// b65dSecrets are the T-step-noleak leaves: one difference each, holding text the server could send.
func b65dSecrets() []b65dMismatch {
	return []b65dMismatch{
		{name: "1 text", change: func(m *tg.InlineKeyboardMarkup) { m.InlineKeyboard[0][1].Text = "B65D-SECRET-TEXT" },
			want: b65dP + "row 0 button 1: Text differs (sent 3 bytes, received 16 bytes)"},
		{name: "2 callback", change: func(m *tg.InlineKeyboardMarkup) { m.InlineKeyboard[0][0].CallbackData = "B65D-SECRET-DATA" },
			want: b65dP + "row 0 button 0: CallbackData differs (sent 1 bytes, received 16 bytes)"},
		{name: "3 web app", change: func(m *tg.InlineKeyboardMarkup) {
			m.InlineKeyboard[0][0].WebApp = &tg.WebAppInfo{URL: "https://B65D-SECRET-URL.example/"}
		}, want: b65dP + "row 0 button 0: WebApp differs (sent set=false, received set=true)"},
	}
}

// T-step-noleak: the error names positions, fields, presence and lengths, never the received value (X-P3-01).
func TestB65D_StepNoLeak(t *testing.T) {
	for _, tc := range b65dSecrets() {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeMarkup{reply: b65dReply(tc.change)}
			rt := NewRuntime(f, b65dGroup, b65dAdmin)
			res, err := b65dStep().Execute(context.Background(), rt)
			require.Error(t, err)
			assert.Equal(t, tc.want, err.Error())
			assert.NotContains(t, err.Error(), "B65D-SECRET")
			assert.NotContains(t, fmt.Sprintf("%v", res.Evidence), "B65D-SECRET")
			b65dShared(t, f, rt, res, b65dMarkup())
		})
	}
}

// T-step-noleak with the pointer set on both sides (C2-01): the received pointer's contents are not printed.
func TestB65D_StepNoLeakBothSet(t *testing.T) {
	markup := func(url string) tg.InlineKeyboardMarkup {
		return tg.InlineKeyboardMarkup{InlineKeyboard: [][]tg.InlineKeyboardButton{{{Text: "w", WebApp: &tg.WebAppInfo{URL: url}}}}}
	}
	f := &fakeMarkup{reply: func() *tg.InlineKeyboardMarkup { m := markup("https://B65D-SECRET-URL.example/"); return &m }}
	rt := NewRuntime(f, b65dGroup, b65dAdmin)
	step := &SendReplyMarkupEchoStep{Text: b65dText, Markup: markup("https://w.example/")}
	res, err := step.Execute(context.Background(), rt)
	require.Error(t, err)
	assert.Equal(t, b65dP+"row 0 button 0: WebApp differs (sent set=true, received set=true)", err.Error())
	assert.NotContains(t, err.Error(), "B65D-SECRET")
	assert.NotContains(t, fmt.Sprintf("%v", res.Evidence), "B65D-SECRET")
	assert.Equal(t, []CreatedMessage{{ChatID: b65dGroup, MessageID: b65dMsgID}}, rt.CreatedMessages)
	assert.Equal(t, map[string]any{"message_id": b65dMsgID, "rows": 1, "force_reply": false}, res.Evidence)
}

// T-step-senderr (guard): a send error is returned; nothing is tracked or reported.
func TestB65D_StepSendError(t *testing.T) {
	sendErr := errors.New("send failed")
	f := &fakeMarkup{reply: b65dReply(nil), err: sendErr}
	rt := NewRuntime(f, b65dGroup, b65dAdmin)
	res, err := b65dStep().Execute(context.Background(), rt)
	require.ErrorIs(t, err, sendErr)
	require.Len(t, f.calls, 1)
	assert.Empty(t, rt.CreatedMessages)
	require.NotNil(t, res)
	assert.Empty(t, res.MessageIDs)
	assert.Nil(t, res.Evidence)
}

// T-step-fallback (C1-01/C1-02): a nil row against an empty row is a difference reflect.DeepEqual sees and no compared
// field shows; the error is the whole fallback text and holds no text of the equal sibling row (a canary).
func TestB65D_StepFallback(t *testing.T) {
	const canary = "B65D-SECRET-FALLBACK"
	markup := func(last []tg.InlineKeyboardButton) tg.InlineKeyboardMarkup {
		return tg.InlineKeyboardMarkup{InlineKeyboard: [][]tg.InlineKeyboardButton{{{Text: canary, CallbackData: canary}}, last}}
	}
	for _, tc := range []struct {
		name       string
		sent, recv []tg.InlineKeyboardButton
	}{
		{"empty sent, nil received", []tg.InlineKeyboardButton{}, nil},
		{"nil sent, empty received", nil, []tg.InlineKeyboardButton{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeMarkup{reply: func() *tg.InlineKeyboardMarkup { m := markup(tc.recv); return &m }}
			rt := NewRuntime(f, b65dGroup, b65dAdmin)
			step := &SendReplyMarkupEchoStep{Text: b65dText, Markup: markup(tc.sent)}
			res, err := step.Execute(context.Background(), rt)
			require.Error(t, err)
			assert.Equal(t, b65dP+"a difference outside the compared fields", err.Error())
			assert.NotContains(t, err.Error(), canary)
			assert.NotContains(t, fmt.Sprintf("%v", res.Evidence), canary)
			assert.Equal(t, []CreatedMessage{{ChatID: b65dGroup, MessageID: b65dMsgID}}, rt.CreatedMessages)
			assert.Equal(t, map[string]any{"message_id": b65dMsgID, "rows": 2, "force_reply": false}, res.Evidence)
		})
	}
}
