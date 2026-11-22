package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/prilive-com/galigo/tg"
)

// ReplyRichQuoteStep (Bot API 10.4) replies to rt.LastMessage, which must be a rich message, with
// ReplyParameters.QuoteRichTexts = Texts and QuotePosition = Position. It never sets rt.LastMessage, so every row
// quotes the same source. A returned message is tracked before any check.
//
// Without WantError the reply must point to the source and its Quote must hold the same RichTexts as Go values (so a
// kind that decodes as RichTextUnknown fails), the sent position and IsManual true. With WantError the call must return
// no message and a 400 *tg.APIError whose description contains WantError.
type ReplyRichQuoteStep struct {
	Label     string
	Texts     []tg.RichText
	Position  int
	WantError string
}

func (s *ReplyRichQuoteStep) Name() string { return "sendMessage (rich quote " + s.Label + ")" }

func (s *ReplyRichQuoteStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	src := rt.LastMessage
	if src == nil || src.RichMessage == nil {
		return nil, errors.New("sendMessage (rich quote): the last message is not a rich message")
	}
	msg, err := rt.Sender.SendMessage(ctx, rt.ChatID, "galigo 10.4 rich quote "+s.Label,
		WithReplyParameters(&tg.ReplyParameters{MessageID: src.MessageID, QuoteRichTexts: s.Texts, QuotePosition: s.Position}))
	res := &StepResult{StepName: s.Name(), Method: "sendMessage"}
	if msg != nil {
		rt.TrackMessage(rt.ChatID, msg.MessageID)
		res.MessageIDs = []int{msg.MessageID}
	}
	if s.WantError != "" {
		if err == nil {
			return res, fmt.Errorf("sendMessage: the server took the quote; want no message and a 400 error containing %q", s.WantError)
		}
		if msg != nil {
			return res, fmt.Errorf("sendMessage: a message came with the error; want no message and a 400 error containing %q: %w", s.WantError, err)
		}
		var apiErr *tg.APIError
		if !errors.As(err, &apiErr) || apiErr.Code != 400 || !strings.Contains(apiErr.Description, s.WantError) {
			return res, fmt.Errorf("sendMessage: want a 400 error containing %q, got: %w", s.WantError, err)
		}
		res.Evidence = map[string]any{"label": s.Label, "error_code": apiErr.Code, "description": apiErr.Description}
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("sendMessage: %w", err)
	}
	if msg == nil {
		return res, errors.New("sendMessage: no message in the reply")
	}
	if msg.ReplyToMessage == nil || msg.ReplyToMessage.MessageID != src.MessageID {
		return res, fmt.Errorf("sendMessage: the reply does not point to message %d", src.MessageID)
	}
	q := msg.Quote
	if q == nil {
		return res, errors.New("sendMessage: the reply has no quote")
	}
	quote, _ := json.Marshal(q) // for reading only; the checks compare Go values
	res.Evidence = map[string]any{"label": s.Label, "quote": string(quote), "position": q.Position, "is_manual": q.IsManual}
	if !reflect.DeepEqual(q.RichTexts, s.Texts) {
		return res, fmt.Errorf("sendMessage: quote rich_texts %s, want the sent value", quote)
	}
	if q.Position != s.Position {
		return res, fmt.Errorf("sendMessage: quote position %d, want %d", q.Position, s.Position)
	}
	if !q.IsManual {
		return res, errors.New("sendMessage: quote is_manual is false")
	}
	return res, nil
}
