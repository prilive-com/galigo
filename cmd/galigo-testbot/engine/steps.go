package engine

import (
	"context"
	"fmt"
)

// GetMeStep verifies bot identity.
type GetMeStep struct{}

func (s *GetMeStep) Name() string { return "getMe" }

func (s *GetMeStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	user, err := rt.Sender.GetMe(ctx)
	if err != nil {
		return nil, err
	}

	if !user.IsBot {
		return nil, fmt.Errorf("expected bot, got user")
	}

	return &StepResult{
		Method: "getMe",
		Evidence: map[string]any{
			"username":   user.Username,
			"id":         user.ID,
			"first_name": user.FirstName,
		},
	}, nil
}

// SendMessageStep sends a text message.
type SendMessageStep struct {
	Text string
}

func (s *SendMessageStep) Name() string { return "sendMessage" }

func (s *SendMessageStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	msg, err := rt.Sender.SendMessage(ctx, rt.ChatID, s.Text)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	return &StepResult{
		Method:     "sendMessage",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"text":       s.Text,
		},
	}, nil
}

// EditMessageTextStep edits the last message's text.
type EditMessageTextStep struct {
	Text string
}

func (s *EditMessageTextStep) Name() string { return "editMessageText" }

func (s *EditMessageTextStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.LastMessage == nil {
		return nil, fmt.Errorf("no message to edit")
	}

	msg, err := rt.Sender.EditMessageText(ctx, rt.ChatID, rt.LastMessage.MessageID, s.Text)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg

	return &StepResult{
		Method: "editMessageText",
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"new_text":   s.Text,
		},
	}, nil
}

// DeleteLastMessageStep deletes the last sent message.
type DeleteLastMessageStep struct{}

func (s *DeleteLastMessageStep) Name() string { return "deleteMessage" }

func (s *DeleteLastMessageStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.LastMessage == nil {
		return nil, fmt.Errorf("no message to delete")
	}

	err := rt.Sender.DeleteMessage(ctx, rt.ChatID, rt.LastMessage.MessageID)
	if err != nil {
		return nil, err
	}

	// Remove from tracked messages since we deleted it
	for i, cm := range rt.CreatedMessages {
		if cm.MessageID == rt.LastMessage.MessageID && cm.ChatID == rt.ChatID {
			rt.CreatedMessages = append(rt.CreatedMessages[:i], rt.CreatedMessages[i+1:]...)
			break
		}
	}

	msgID := rt.LastMessage.MessageID
	rt.LastMessage = nil

	return &StepResult{
		Method: "deleteMessage",
		Evidence: map[string]any{
			"deleted_message_id": msgID,
		},
	}, nil
}

// ForwardMessageStep forwards the last message.
type ForwardMessageStep struct {
	ToChatID int64 // If 0, uses rt.ChatID
}

func (s *ForwardMessageStep) Name() string { return "forwardMessage" }

func (s *ForwardMessageStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.LastMessage == nil {
		return nil, fmt.Errorf("no message to forward")
	}

	toChatID := s.ToChatID
	if toChatID == 0 {
		toChatID = rt.ChatID
	}

	msg, err := rt.Sender.ForwardMessage(ctx, toChatID, rt.ChatID, rt.LastMessage.MessageID)
	if err != nil {
		return nil, err
	}

	rt.TrackMessage(toChatID, msg.MessageID)

	return &StepResult{
		Method:     "forwardMessage",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"original_message_id": rt.LastMessage.MessageID,
			"forwarded_message_id": msg.MessageID,
			"to_chat_id":          toChatID,
		},
	}, nil
}

// CopyMessageStep copies the last message.
type CopyMessageStep struct {
	ToChatID int64 // If 0, uses rt.ChatID
}

func (s *CopyMessageStep) Name() string { return "copyMessage" }

func (s *CopyMessageStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.LastMessage == nil {
		return nil, fmt.Errorf("no message to copy")
	}

	toChatID := s.ToChatID
	if toChatID == 0 {
		toChatID = rt.ChatID
	}

	msgID, err := rt.Sender.CopyMessage(ctx, toChatID, rt.ChatID, rt.LastMessage.MessageID)
	if err != nil {
		return nil, err
	}

	rt.LastMessageID = msgID
	rt.TrackMessage(toChatID, msgID.MessageID)

	return &StepResult{
		Method:     "copyMessage",
		MessageIDs: []int{msgID.MessageID},
		Evidence: map[string]any{
			"original_message_id": rt.LastMessage.MessageID,
			"copied_message_id":   msgID.MessageID,
			"to_chat_id":          toChatID,
		},
	}, nil
}

// SendChatActionStep sends a chat action.
type SendChatActionStep struct {
	Action string // e.g., "typing"
}

func (s *SendChatActionStep) Name() string { return "sendChatAction" }

func (s *SendChatActionStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	action := s.Action
	if action == "" {
		action = "typing"
	}

	err := rt.Sender.SendChatAction(ctx, rt.ChatID, action)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "sendChatAction",
		Evidence: map[string]any{
			"action": action,
		},
	}, nil
}

// CleanupStep deletes all tracked messages.
type CleanupStep struct{}

func (s *CleanupStep) Name() string { return "cleanup" }

func (s *CleanupStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	deleted := 0
	var lastErr error

	for _, cm := range rt.CreatedMessages {
		if err := rt.Sender.DeleteMessage(ctx, cm.ChatID, cm.MessageID); err != nil {
			lastErr = err
			// Continue trying to delete other messages
		} else {
			deleted++
		}
	}

	// Clear the list
	rt.CreatedMessages = rt.CreatedMessages[:0]
	rt.LastMessage = nil

	result := &StepResult{
		Method: "deleteMessage",
		Evidence: map[string]any{
			"deleted_count": deleted,
		},
	}

	// Only fail if we couldn't delete any messages and there were messages to delete
	if lastErr != nil && deleted == 0 {
		return result, lastErr
	}

	return result, nil
}
