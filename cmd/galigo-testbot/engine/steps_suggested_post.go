package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/tg"
)

// Server texts for suggested_post_parameters outside a channel direct messages chat (probed 2026-11-12, B-48c1).
const (
	SuggestedPostWrongChat   = "suggested posts can be sent only to channel direct messages"
	SuggestedPostWrongAmount = "invalid amount of Telegram Stars specified"
)

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
