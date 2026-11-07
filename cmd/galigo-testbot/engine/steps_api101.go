package engine

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/prilive-com/galigo/tg"
)

// ==================== Bot API 10.1 steps (rich messages) ====================

// richBlockTypes returns the "type" of each block of a rich message, in order (every RichBlock kind, and
// RichBlockUnknown, has a Type field).
func richBlockTypes(m *tg.RichMessage) []string {
	out := make([]string, 0, len(m.Blocks))
	for _, b := range m.Blocks {
		out = append(out, reflect.ValueOf(b).FieldByName("Type").String())
	}
	return out
}

// SendRichMessageStep sends a rich message to rt.ChatID and checks the reply: a rich message, no text, and the
// block types WantBlocks.
type SendRichMessageStep struct {
	RichMessage tg.InputRichMessage
	WantBlocks  []string
}

func (s *SendRichMessageStep) Name() string { return "sendRichMessage" }

func (s *SendRichMessageStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: "sendRichMessage"}
	msg, err := rt.Sender.SendRichMessage(ctx, rt.ChatID, s.RichMessage)
	if err != nil {
		return res, err
	}
	if msg == nil {
		return res, errors.New("sendRichMessage: no message in the reply")
	}
	rt.TrackMessage(rt.ChatID, msg.MessageID)
	rt.LastMessage = msg
	res.MessageIDs = []int{msg.MessageID}
	if msg.RichMessage == nil {
		return res, errors.New("sendRichMessage: the reply has no rich_message")
	}
	if msg.Text != "" {
		return res, fmt.Errorf("sendRichMessage: the reply has text %q", msg.Text)
	}
	if got := richBlockTypes(msg.RichMessage); !slices.Equal(got, s.WantBlocks) {
		return res, fmt.Errorf("sendRichMessage: blocks %v, want %v", got, s.WantBlocks)
	}
	res.Evidence = map[string]any{"message_id": msg.MessageID, "blocks": s.WantBlocks}
	return res, nil
}

// EditMessageRichStep edits rt.LastMessage (its chat and message id) with RichMessage only and checks the reply: a
// rich message with the block types WantBlocks.
type EditMessageRichStep struct {
	RichMessage tg.InputRichMessage
	WantBlocks  []string
}

func (s *EditMessageRichStep) Name() string { return "editMessageText (rich_message)" }

func (s *EditMessageRichStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: "editMessageText"}
	if rt.LastMessage == nil || rt.LastMessage.Chat == nil {
		return res, errors.New("editMessageText: no message to edit")
	}
	msg, err := rt.Sender.EditMessageRich(ctx, rt.LastMessage.Chat.ID, rt.LastMessage.MessageID, s.RichMessage)
	if err != nil {
		return res, err
	}
	if msg == nil || msg.RichMessage == nil {
		return res, errors.New("editMessageText: the reply has no rich_message")
	}
	if got := richBlockTypes(msg.RichMessage); !slices.Equal(got, s.WantBlocks) {
		return res, fmt.Errorf("editMessageText: blocks %v, want %v", got, s.WantBlocks)
	}
	rt.LastMessage = msg
	res.MessageIDs = []int{msg.MessageID}
	res.Evidence = map[string]any{"message_id": msg.MessageID, "blocks": s.WantBlocks}
	return res, nil
}

// SendRichMessageDraftStep streams a rich draft to rt.ChatID. The method works in private chats only; in a group the
// server answers 400 TEXTDRAFT_PEER_INVALID (probed 2026-11-07), and the step is skipped.
type SendRichMessageDraftStep struct {
	DraftID     int
	RichMessage tg.InputRichMessage
}

func (s *SendRichMessageDraftStep) Name() string { return "sendRichMessageDraft" }

func (s *SendRichMessageDraftStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	err := rt.Sender.SendRichMessageDraft(ctx, rt.ChatID, s.DraftID, s.RichMessage)
	if err != nil {
		var apiErr *tg.APIError
		if errors.As(err, &apiErr) && apiErr.Code == 400 && strings.Contains(apiErr.Description, "PEER_INVALID") {
			return nil, Skip("sendRichMessageDraft requires a private chat (not group)")
		}
		return nil, err
	}
	return &StepResult{StepName: s.Name(), Method: "sendRichMessageDraft", Evidence: map[string]any{"draft_id": s.DraftID}}, nil
}
