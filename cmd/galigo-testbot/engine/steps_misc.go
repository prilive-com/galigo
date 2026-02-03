package engine

import (
	"context"
	"fmt"
)

// SendDiceStep sends an animated dice/emoji.
type SendDiceStep struct {
	Emoji string // 🎲 🎯 🏀 ⚽ 🎳 🎰
}

func (s *SendDiceStep) Name() string { return "sendDice" }

func (s *SendDiceStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	emoji := s.Emoji
	if emoji == "" {
		emoji = "🎲"
	}

	msg, err := rt.Sender.SendDice(ctx, rt.ChatID, emoji)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	return &StepResult{
		Method:     "sendDice",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"emoji":      emoji,
		},
	}, nil
}

// SetMessageReactionStep sets an emoji reaction on the last message.
type SetMessageReactionStep struct {
	Emoji string
	IsBig bool
}

func (s *SetMessageReactionStep) Name() string { return "setMessageReaction" }

func (s *SetMessageReactionStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.LastMessage == nil {
		return nil, fmt.Errorf("no message to react to")
	}

	emoji := s.Emoji
	if emoji == "" {
		emoji = "👍"
	}

	err := rt.Sender.SetMessageReaction(ctx, rt.ChatID, rt.LastMessage.MessageID, emoji, s.IsBig)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "setMessageReaction",
		Evidence: map[string]any{
			"message_id": rt.LastMessage.MessageID,
			"emoji":      emoji,
			"is_big":     s.IsBig,
		},
	}, nil
}

// GetUserProfilePhotosStep gets profile photos of a user.
type GetUserProfilePhotosStep struct{}

func (s *GetUserProfilePhotosStep) Name() string { return "getUserProfilePhotos" }

func (s *GetUserProfilePhotosStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	userID := rt.AdminUserID
	if userID == 0 {
		return nil, Skip("no AdminUserID available for getUserProfilePhotos")
	}

	photos, err := rt.Sender.GetUserProfilePhotos(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "getUserProfilePhotos",
		Evidence: map[string]any{
			"user_id":     userID,
			"total_count": photos.TotalCount,
			"photo_count": len(photos.Photos),
		},
	}, nil
}

// GetUserChatBoostsStep gets a user's boosts in the chat.
type GetUserChatBoostsStep struct{}

func (s *GetUserChatBoostsStep) Name() string { return "getUserChatBoosts" }

func (s *GetUserChatBoostsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.AdminUserID == 0 {
		return nil, Skip("AdminUserID not set")
	}

	boosts, err := rt.Sender.GetUserChatBoosts(ctx, rt.ChatID, rt.AdminUserID)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "getUserChatBoosts",
		Evidence: map[string]any{
			"boost_count": len(boosts.Boosts),
		},
	}, nil
}
