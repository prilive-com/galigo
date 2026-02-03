package engine

import (
	"context"
	"fmt"
)

// SeedMessagesStep sends N messages and stores their IDs for bulk operations.
type SeedMessagesStep struct {
	Count int // Default: 3
}

func (s *SeedMessagesStep) Name() string { return "seedMessages" }

func (s *SeedMessagesStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	count := s.Count
	if count == 0 {
		count = 3
	}

	rt.BulkMessageIDs = make([]int, 0, count)

	for i := 1; i <= count; i++ {
		msg, err := rt.Sender.SendMessage(ctx, rt.ChatID,
			fmt.Sprintf("Bulk test message %d/%d", i, count))
		if err != nil {
			return nil, fmt.Errorf("seedMessages %d/%d: %w", i, count, err)
		}
		rt.BulkMessageIDs = append(rt.BulkMessageIDs, msg.MessageID)
		rt.TrackMessage(rt.ChatID, msg.MessageID)
	}

	return &StepResult{
		Method:     "sendMessage",
		MessageIDs: rt.BulkMessageIDs,
		Evidence: map[string]any{
			"seeded_count": count,
			"message_ids":  rt.BulkMessageIDs,
		},
	}, nil
}

// ForwardMessagesStep forwards messages from BulkMessageIDs.
type ForwardMessagesStep struct{}

func (s *ForwardMessagesStep) Name() string { return "forwardMessages" }

func (s *ForwardMessagesStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if len(rt.BulkMessageIDs) == 0 {
		return nil, fmt.Errorf("no bulk message IDs — run SeedMessagesStep first")
	}

	result, err := rt.Sender.ForwardMessages(ctx, rt.ChatID, rt.ChatID, rt.BulkMessageIDs)
	if err != nil {
		return nil, err
	}

	var newIDs []int
	for _, msgID := range result {
		newIDs = append(newIDs, msgID.MessageID)
		rt.TrackMessage(rt.ChatID, msgID.MessageID)
	}

	return &StepResult{
		Method:     "forwardMessages",
		MessageIDs: newIDs,
		Evidence: map[string]any{
			"original_count":  len(rt.BulkMessageIDs),
			"forwarded_count": len(result),
		},
	}, nil
}

// CopyMessagesStep copies messages from BulkMessageIDs.
type CopyMessagesStep struct{}

func (s *CopyMessagesStep) Name() string { return "copyMessages" }

func (s *CopyMessagesStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if len(rt.BulkMessageIDs) == 0 {
		return nil, fmt.Errorf("no bulk message IDs — run SeedMessagesStep first")
	}

	result, err := rt.Sender.CopyMessages(ctx, rt.ChatID, rt.ChatID, rt.BulkMessageIDs)
	if err != nil {
		return nil, err
	}

	var newIDs []int
	for _, msgID := range result {
		newIDs = append(newIDs, msgID.MessageID)
		rt.TrackMessage(rt.ChatID, msgID.MessageID)
	}

	return &StepResult{
		Method:     "copyMessages",
		MessageIDs: newIDs,
		Evidence: map[string]any{
			"original_count": len(rt.BulkMessageIDs),
			"copied_count":   len(result),
		},
	}, nil
}

// DeleteMessagesStep deletes all tracked messages for this chat at once.
type DeleteMessagesStep struct{}

func (s *DeleteMessagesStep) Name() string { return "deleteMessages" }

func (s *DeleteMessagesStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	var msgIDs []int
	for _, cm := range rt.CreatedMessages {
		if cm.ChatID == rt.ChatID {
			msgIDs = append(msgIDs, cm.MessageID)
		}
	}

	if len(msgIDs) == 0 {
		return nil, fmt.Errorf("no messages to delete")
	}

	err := rt.Sender.DeleteMessages(ctx, rt.ChatID, msgIDs)
	if err != nil {
		return nil, err
	}

	// Clear tracked messages for this chat
	remaining := make([]CreatedMessage, 0)
	for _, cm := range rt.CreatedMessages {
		if cm.ChatID != rt.ChatID {
			remaining = append(remaining, cm)
		}
	}
	rt.CreatedMessages = remaining
	rt.BulkMessageIDs = nil

	return &StepResult{
		Method: "deleteMessages",
		Evidence: map[string]any{
			"deleted_count": len(msgIDs),
		},
	}, nil
}
