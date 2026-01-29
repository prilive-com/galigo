package engine

import (
	"context"
	"fmt"
)

// ================= Tier 2: Chat Info Steps =================

// GetChatStep gets full chat info.
type GetChatStep struct{}

func (s *GetChatStep) Name() string { return "getChat" }

func (s *GetChatStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	chat, err := rt.Sender.GetChat(ctx, rt.ChatID)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "getChat",
		Evidence: map[string]any{
			"chat_id": chat.ID,
			"type":    chat.Type,
			"title":   chat.Title,
		},
	}, nil
}

// GetChatAdministratorsStep gets chat admins.
type GetChatAdministratorsStep struct{}

func (s *GetChatAdministratorsStep) Name() string { return "getChatAdministrators" }

func (s *GetChatAdministratorsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	admins, err := rt.Sender.GetChatAdministrators(ctx, rt.ChatID)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "getChatAdministrators",
		Evidence: map[string]any{
			"admin_count": len(admins),
		},
	}, nil
}

// GetChatMemberCountStep gets member count.
type GetChatMemberCountStep struct{}

func (s *GetChatMemberCountStep) Name() string { return "getChatMemberCount" }

func (s *GetChatMemberCountStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	count, err := rt.Sender.GetChatMemberCount(ctx, rt.ChatID)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "getChatMemberCount",
		Evidence: map[string]any{
			"member_count": count,
		},
	}, nil
}

// GetChatMemberStep gets info about a specific member (the bot itself).
type GetChatMemberStep struct {
	UserID int64 // If 0, uses bot's own ID from GetMe
}

func (s *GetChatMemberStep) Name() string { return "getChatMember" }

func (s *GetChatMemberStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	userID := s.UserID
	if userID == 0 {
		// Get bot's own ID
		me, err := rt.Sender.GetMe(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get bot ID: %w", err)
		}
		userID = me.ID
	}

	member, err := rt.Sender.GetChatMember(ctx, rt.ChatID, userID)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "getChatMember",
		Evidence: map[string]any{
			"user_id": userID,
			"status":  member.Status(),
		},
	}, nil
}

// ================= Tier 2: Chat Settings Steps =================

// SetChatTitleStep sets the chat title.
type SetChatTitleStep struct {
	Title string
}

func (s *SetChatTitleStep) Name() string { return "setChatTitle" }

func (s *SetChatTitleStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	err := rt.Sender.SetChatTitle(ctx, rt.ChatID, s.Title)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "setChatTitle",
		Evidence: map[string]any{
			"title": s.Title,
		},
	}, nil
}

// SetChatDescriptionStep sets the chat description.
type SetChatDescriptionStep struct {
	Description string
}

func (s *SetChatDescriptionStep) Name() string { return "setChatDescription" }

func (s *SetChatDescriptionStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	err := rt.Sender.SetChatDescription(ctx, rt.ChatID, s.Description)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "setChatDescription",
		Evidence: map[string]any{
			"description": s.Description,
		},
	}, nil
}

// ================= Tier 2: Pin Message Steps =================

// PinChatMessageStep pins the last sent message.
type PinChatMessageStep struct {
	Silent bool
}

func (s *PinChatMessageStep) Name() string { return "pinChatMessage" }

func (s *PinChatMessageStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.LastMessage == nil {
		return nil, fmt.Errorf("no message to pin")
	}

	err := rt.Sender.PinChatMessage(ctx, rt.ChatID, rt.LastMessage.MessageID, s.Silent)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "pinChatMessage",
		Evidence: map[string]any{
			"message_id": rt.LastMessage.MessageID,
			"silent":     s.Silent,
		},
	}, nil
}

// UnpinChatMessageStep unpins the last sent message.
type UnpinChatMessageStep struct{}

func (s *UnpinChatMessageStep) Name() string { return "unpinChatMessage" }

func (s *UnpinChatMessageStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.LastMessage == nil {
		return nil, fmt.Errorf("no message to unpin")
	}

	err := rt.Sender.UnpinChatMessage(ctx, rt.ChatID, rt.LastMessage.MessageID)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "unpinChatMessage",
		Evidence: map[string]any{
			"message_id": rt.LastMessage.MessageID,
		},
	}, nil
}

// UnpinAllChatMessagesStep unpins all messages.
type UnpinAllChatMessagesStep struct{}

func (s *UnpinAllChatMessagesStep) Name() string { return "unpinAllChatMessages" }

func (s *UnpinAllChatMessagesStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	err := rt.Sender.UnpinAllChatMessages(ctx, rt.ChatID)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "unpinAllChatMessages",
	}, nil
}

// ================= Tier 2: Poll Steps =================

// SendPollSimpleStep sends a simple poll.
type SendPollSimpleStep struct {
	Question string
	Options  []string
}

func (s *SendPollSimpleStep) Name() string { return "sendPoll (simple)" }

func (s *SendPollSimpleStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	msg, err := rt.Sender.SendPollSimple(ctx, rt.ChatID, s.Question, s.Options)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	return &StepResult{
		Method:     "sendPoll",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"question":   s.Question,
			"type":       "regular",
		},
	}, nil
}

// SendQuizStep sends a quiz poll.
type SendQuizStep struct {
	Question        string
	Options         []string
	CorrectOptionID int
}

func (s *SendQuizStep) Name() string { return "sendPoll (quiz)" }

func (s *SendQuizStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	msg, err := rt.Sender.SendQuiz(ctx, rt.ChatID, s.Question, s.Options, s.CorrectOptionID)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	return &StepResult{
		Method:     "sendPoll",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"message_id":        msg.MessageID,
			"question":          s.Question,
			"type":              "quiz",
			"correct_option_id": s.CorrectOptionID,
		},
	}, nil
}

// StopPollStep stops the last poll message.
type StopPollStep struct{}

func (s *StopPollStep) Name() string { return "stopPoll" }

func (s *StopPollStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.LastMessage == nil {
		return nil, fmt.Errorf("no poll message to stop")
	}

	poll, err := rt.Sender.StopPoll(ctx, rt.ChatID, rt.LastMessage.MessageID)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		Method: "stopPoll",
		Evidence: map[string]any{
			"is_closed": poll.IsClosed,
		},
	}, nil
}

// ================= Tier 2: Forum Steps =================

// GetForumTopicIconStickersStep gets forum topic icon stickers.
type GetForumTopicIconStickersStep struct{}

func (s *GetForumTopicIconStickersStep) Name() string { return "getForumTopicIconStickers" }

func (s *GetForumTopicIconStickersStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	stickers, err := rt.Sender.GetForumTopicIconStickers(ctx)
	if err != nil {
		return nil, err
	}

	var fileIDs []string
	for _, st := range stickers {
		if st != nil {
			fileIDs = append(fileIDs, st.FileID)
		}
	}

	return &StepResult{
		Method:  "getForumTopicIconStickers",
		FileIDs: fileIDs,
		Evidence: map[string]any{
			"sticker_count": len(stickers),
		},
	}, nil
}

// ================= Helper: Save/Restore Title =================

// SaveChatTitleStep saves the current chat title in CapturedFileIDs for later restore.
type SaveChatTitleStep struct{}

func (s *SaveChatTitleStep) Name() string { return "getChat (save title)" }

func (s *SaveChatTitleStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	chat, err := rt.Sender.GetChat(ctx, rt.ChatID)
	if err != nil {
		return nil, err
	}

	rt.CapturedFileIDs["original_title"] = chat.Title
	rt.CapturedFileIDs["original_description"] = chat.Description

	return &StepResult{
		Method: "getChat",
		Evidence: map[string]any{
			"saved_title":       chat.Title,
			"saved_description": chat.Description,
		},
	}, nil
}

// RestoreChatTitleStep restores the saved chat title and description.
type RestoreChatTitleStep struct{}

func (s *RestoreChatTitleStep) Name() string { return "restore chat settings" }

func (s *RestoreChatTitleStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	title := rt.CapturedFileIDs["original_title"]
	if title != "" {
		if err := rt.Sender.SetChatTitle(ctx, rt.ChatID, title); err != nil {
			return nil, fmt.Errorf("restore title: %w", err)
		}
	}

	desc := rt.CapturedFileIDs["original_description"]
	if err := rt.Sender.SetChatDescription(ctx, rt.ChatID, desc); err != nil {
		return nil, fmt.Errorf("restore description: %w", err)
	}

	return &StepResult{
		Method: "setChatTitle",
		Evidence: map[string]any{
			"restored_title":       title,
			"restored_description": desc,
		},
	}, nil
}
