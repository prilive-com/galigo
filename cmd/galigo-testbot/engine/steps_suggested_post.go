package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Server texts for suggested_post_parameters outside a channel direct messages chat (probed 2026-11-12, B-48c1).
const (
	SuggestedPostWrongChat   = "suggested posts can be sent only to channel direct messages"
	SuggestedPostWrongAmount = "invalid amount of Telegram Stars specified"
)

// Server texts for approveSuggestedPost and declineSuggestedPost outside a channel direct messages chat (probed
// 2026-11-12, B-48c2): an existing message that is not a suggested post, and a message id with no message.
const (
	SuggestedPostNotUsable = "message can't be used in the method"
	SuggestedPostNotFound  = "suggested post not found"
)

// DecideSuggestedPostStep calls approveSuggestedPost or declineSuggestedPost (Bot API 9.2) in rt.ChatID, which is
// not a channel direct messages chat, so the server must refuse it with 400 and WantError. The target is
// rt.LastMessage (a plain message: SuggestedPostNotUsable), or, with Missing, a message id after it that holds no
// message (SuggestedPostNotFound). The two texts differ (MEASURED: no message_id also gives SuggestedPostNotFound), so
// the pair shows that message_id reached the server and the request was parsed. It does not show chat_id's value (the
// admin's private chat gives the same refusal); the mock tests prove the exact body. SendDate (approve) and Comment
// (decline) are sent too; the server's refusal does not show them.
type DecideSuggestedPostStep struct {
	Method    string // "approveSuggestedPost" or "declineSuggestedPost"
	Missing   bool
	SendDate  int64
	Comment   string
	WantError string
}

func (s *DecideSuggestedPostStep) Name() string {
	if s.Missing {
		return s.Method + " (9.2), missing message"
	}
	return s.Method + " (9.2)"
}

func (s *DecideSuggestedPostStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: s.Method}
	if rt.LastMessage == nil {
		return res, fmt.Errorf("%s: no target message (run a send step first)", s.Method)
	}
	id := rt.LastMessage.MessageID
	if s.Missing {
		id += 1000000
	}
	var err error
	switch s.Method {
	case "approveSuggestedPost":
		err = rt.Sender.ApproveSuggestedPost(ctx, sender.ApproveSuggestedPostRequest{ChatID: rt.ChatID, MessageID: id, SendDate: s.SendDate})
	case "declineSuggestedPost":
		err = rt.Sender.DeclineSuggestedPost(ctx, sender.DeclineSuggestedPostRequest{ChatID: rt.ChatID, MessageID: id, Comment: s.Comment})
	default:
		return res, fmt.Errorf("unknown method %q", s.Method)
	}
	if err == nil {
		return res, fmt.Errorf("%s: the server took message %d, which is not a suggested post", s.Method, id)
	}
	var apiErr *tg.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 400 || !strings.Contains(apiErr.Description, s.WantError) {
		return res, fmt.Errorf("%s: want a 400 error containing %q, got: %w", s.Method, s.WantError, err)
	}
	res.Evidence = map[string]any{"message_id": id, "error_code": apiErr.Code, "description": apiErr.Description}
	return res, nil
}

// SendSuggestedPostStep sends a message with suggested_post_parameters (Bot API 9.2) to rt.ChatID, which is not a
// channel direct messages chat, so the server must refuse it with 400 and WantError: that proves the parameter
// reached the server and was parsed. Method is "sendMessage" (JSON body) or "sendPhoto" (an upload: form body). If
// the server takes the message, galigo dropped the parameter: the message is tracked for cleanup and the step fails.
type SendSuggestedPostStep struct {
	Method    string
	Params    *tg.SuggestedPostParameters
	WantError string
}

func (s *SendSuggestedPostStep) Name() string { return s.Method + " suggested_post_parameters (9.2)" }

func (s *SendSuggestedPostStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: s.Method}
	opt := WithSuggestedPostParameters(s.Params)
	var msg *tg.Message
	var err error
	switch s.Method {
	case "sendMessage":
		msg, err = rt.Sender.SendMessage(ctx, rt.ChatID, "galigo suggested post", opt)
	case "sendPhoto":
		msg, err = rt.Sender.SendPhoto(ctx, rt.ChatID, MediaFromBytes(fixtures.PhotoBytes(), "galigo-suggested.jpg", "photo"), opt)
	default:
		return res, fmt.Errorf("unknown method %q", s.Method)
	}
	if err == nil {
		if msg != nil {
			rt.TrackMessage(rt.ChatID, msg.MessageID)
			res.MessageIDs = []int{msg.MessageID}
		}
		return res, fmt.Errorf("%s: the server took the message; galigo did not send suggested_post_parameters", s.Method)
	}
	var apiErr *tg.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 400 || !strings.Contains(apiErr.Description, s.WantError) {
		return res, fmt.Errorf("%s: want a 400 error containing %q, got: %w", s.Method, s.WantError, err)
	}
	res.Evidence = map[string]any{"error_code": apiErr.Code, "description": apiErr.Description}
	return res, nil
}
