package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// ==================== Bot API 9.6 Steps ====================
//
// Each step makes exactly one Telegram call: the runner repeats a whole step after a 429 and keeps only the last
// attempt's result (runner.go runStepWithRetry). A send step tracks the message and keeps its id in the result
// before any check, and returns that result with every later error, so the report lists every sent id.

// Check errors of the 9.6 steps: one per check, so a test can tell which check failed.
var (
	errPoll96CorrectIDs     = errors.New("poll: correct_option_ids differ from the request")
	errPoll96AllowsRevoting = errors.New("poll: allows_revoting differs from the request")
	errPoll96Description    = errors.New("poll: description differs from the request")
	errPoll96PersistentID   = errors.New("poll: an option has no persistent_id")
	errPoll96NoPoll         = errors.New("poll: the reply has no poll")
	errPoll96NoSource       = errors.New("poll: no last message with a poll")
	errPoll96FewOptions     = errors.New("poll: the last poll has fewer than 2 options")
	errPoll96ReplyOption    = errors.New("reply: reply_to_poll_option_id differs from the option")
	errPreparedButtonNoID   = errors.New("savePreparedKeyboardButton: empty id")
)

// SendPoll96Step sends Req (chat id from rt.ChatID) and checks the 9.6 fields of the returned poll against Req:
// correct_option_ids, allows_revoting and description when Req sets them, and a persistent_id on every option.
type SendPoll96Step struct {
	Req sender.SendPollRequest
}

func (s *SendPoll96Step) Name() string { return "sendPoll (9.6)" }

func (s *SendPoll96Step) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	req := s.Req
	req.ChatID = rt.ChatID
	msg, err := rt.Sender.SendPoll(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("sendPoll: %w", err)
	}
	rt.TrackMessage(rt.ChatID, msg.MessageID)
	rt.LastMessage = msg
	res := &StepResult{StepName: s.Name(), Method: "sendPoll", MessageIDs: []int{msg.MessageID}}

	p := msg.Poll
	if p == nil {
		return res, errPoll96NoPoll
	}
	res.Evidence = map[string]any{
		"type": p.Type, "correct_option_ids": p.CorrectOptionIDs, "allows_revoting": p.AllowsRevoting,
		"description": p.Description, "options": len(p.Options),
	}
	if len(req.CorrectOptionIDs) > 0 && !slices.Equal(p.CorrectOptionIDs, req.CorrectOptionIDs) {
		return res, fmt.Errorf("%w: got %v, want %v", errPoll96CorrectIDs, p.CorrectOptionIDs, req.CorrectOptionIDs)
	}
	if req.AllowsRevoting != nil && p.AllowsRevoting != *req.AllowsRevoting {
		return res, fmt.Errorf("%w: got %v, want %v", errPoll96AllowsRevoting, p.AllowsRevoting, *req.AllowsRevoting)
	}
	if req.Description != "" && p.Description != req.Description {
		return res, fmt.Errorf("%w: got %q, want %q", errPoll96Description, p.Description, req.Description)
	}
	for i, o := range p.Options {
		if o.PersistentID == "" {
			return res, fmt.Errorf("%w: option %d", errPoll96PersistentID, i)
		}
	}
	return res, nil
}

// StopPoll96Step stops rt.LastMessage's poll and checks that the stopped poll's correct_option_ids equal the sent
// poll's.
type StopPoll96Step struct{}

func (s *StopPoll96Step) Name() string { return "stopPoll (9.6)" }

func (s *StopPoll96Step) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	last := rt.LastMessage
	if last == nil || last.Poll == nil {
		return nil, errPoll96NoSource
	}
	p, err := rt.Sender.StopPoll(ctx, rt.ChatID, last.MessageID)
	if err != nil {
		return nil, fmt.Errorf("stopPoll: %w", err)
	}
	res := &StepResult{StepName: s.Name(), Method: "stopPoll",
		Evidence: map[string]any{"is_closed": p.IsClosed, "correct_option_ids": p.CorrectOptionIDs}}
	if !slices.Equal(p.CorrectOptionIDs, last.Poll.CorrectOptionIDs) {
		return res, fmt.Errorf("%w: got %v, want %v", errPoll96CorrectIDs, p.CorrectOptionIDs, last.Poll.CorrectOptionIDs)
	}
	return res, nil
}

// SendQuizIDsStep sends a 3-option quiz through SendQuiz with correct option Index and checks that the returned
// poll's correct_option_ids is [Index], so the changed SendQuiz request reaches Telegram.
type SendQuizIDsStep struct {
	Index int
}

func (s *SendQuizIDsStep) Name() string { return "sendQuiz (correct_option_ids)" }

func (s *SendQuizIDsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	msg, err := rt.Sender.SendQuiz(ctx, rt.ChatID, "galigo 9.6 quiz ids", []string{"A", "B", "C"}, s.Index)
	if err != nil {
		return nil, fmt.Errorf("sendQuiz: %w", err)
	}
	rt.TrackMessage(rt.ChatID, msg.MessageID)
	rt.LastMessage = msg
	res := &StepResult{StepName: s.Name(), Method: "sendPoll", MessageIDs: []int{msg.MessageID}}

	if msg.Poll == nil {
		return res, errPoll96NoPoll
	}
	res.Evidence = map[string]any{"correct_option_ids": msg.Poll.CorrectOptionIDs}
	if want := []int{s.Index}; !slices.Equal(msg.Poll.CorrectOptionIDs, want) {
		return res, fmt.Errorf("%w: got %v, want %v", errPoll96CorrectIDs, msg.Poll.CorrectOptionIDs, want)
	}
	return res, nil
}

// ReplyToPollOptionStep replies to option 1 of rt.LastMessage's poll (by its persistent id) and checks the
// reply's reply_to_poll_option_id.
type ReplyToPollOptionStep struct{}

func (s *ReplyToPollOptionStep) Name() string { return "sendMessage (reply to poll option)" }

func (s *ReplyToPollOptionStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	last := rt.LastMessage
	if last == nil || last.Poll == nil {
		return nil, errPoll96NoSource
	}
	if len(last.Poll.Options) < 2 {
		return nil, fmt.Errorf("%w: %d", errPoll96FewOptions, len(last.Poll.Options))
	}
	option := last.Poll.Options[1].PersistentID
	msg, err := rt.Sender.SendMessage(ctx, rt.ChatID, "galigo 9.6 reply to option B",
		WithReplyParameters(&tg.ReplyParameters{MessageID: last.MessageID, PollOptionID: option}))
	if err != nil {
		return nil, fmt.Errorf("sendMessage: %w", err)
	}
	rt.TrackMessage(rt.ChatID, msg.MessageID)
	rt.LastMessage = msg
	res := &StepResult{StepName: s.Name(), Method: "sendMessage", MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{"reply_to_poll_option_id": msg.ReplyToPollOptionID}}
	if msg.ReplyToPollOptionID != option {
		return res, fmt.Errorf("%w: got %q, want %q", errPoll96ReplyOption, msg.ReplyToPollOptionID, option)
	}
	return res, nil
}

// SavePreparedKeyboardButtonStep saves Button for rt.AdminUserID (a real user; Skip without one) and checks that
// the returned id is not empty.
type SavePreparedKeyboardButtonStep struct {
	Button tg.KeyboardButton
}

func (s *SavePreparedKeyboardButtonStep) Name() string { return "savePreparedKeyboardButton" }

func (s *SavePreparedKeyboardButtonStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.AdminUserID == 0 {
		return nil, Skip("no AdminUserID for savePreparedKeyboardButton")
	}
	b, err := rt.Sender.SavePreparedKeyboardButton(ctx, rt.AdminUserID, s.Button)
	if err != nil {
		return nil, fmt.Errorf("savePreparedKeyboardButton: %w", err)
	}
	res := &StepResult{StepName: s.Name(), Method: "savePreparedKeyboardButton"}
	if b.ID == "" {
		return res, errPreparedButtonNoID
	}
	res.Evidence = map[string]any{"id_length": len(b.ID)}
	return res, nil
}
