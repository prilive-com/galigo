package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/prilive-com/galigo/tg"
)

// ==================== Bot API 9.5 Steps ====================

// SetChatMemberTagStep sets a tag on a member.
type SetChatMemberTagStep struct {
	Tag string
}

func (s *SetChatMemberTagStep) Name() string { return "setChatMemberTag" }

func (s *SetChatMemberTagStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	userID := rt.TestUserID
	if userID == 0 {
		userID = rt.AdminUserID
	}
	if userID == 0 {
		return nil, Skip("no TestUserID or AdminUserID for setChatMemberTag")
	}

	// Gracefully skip if bot lacks can_manage_tags
	if rt.ChatCtx != nil && !rt.ChatCtx.CanManageTags {
		return nil, Skip("bot lacks can_manage_tags right")
	}

	err := rt.Sender.SetChatMemberTag(ctx, rt.ChatID, userID, s.Tag)
	if err != nil {
		// CHAT_CREATOR_REQUIRED: bot must be the chat creator (owner), not just admin
		var apiErr *tg.APIError
		if errors.As(err, &apiErr) && apiErr.Code == 400 &&
			strings.Contains(apiErr.Description, "CHAT_CREATOR_REQUIRED") {
			return nil, Skip("setChatMemberTag requires chat creator (owner) rights")
		}
		return nil, err
	}

	return &StepResult{
		Method:   "setChatMemberTag",
		Evidence: map[string]any{"user_id": userID, "tag": s.Tag},
	}, nil
}

// VerifyChatMemberTagStep reads back a member and asserts their tag.
type VerifyChatMemberTagStep struct {
	ExpectedTag string
}

func (s *VerifyChatMemberTagStep) Name() string { return "getChatMember (verify tag)" }

func (s *VerifyChatMemberTagStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	userID := rt.TestUserID
	if userID == 0 {
		userID = rt.AdminUserID
	}
	if userID == 0 {
		return nil, Skip("no TestUserID or AdminUserID for tag readback")
	}

	member, err := rt.Sender.GetChatMember(ctx, rt.ChatID, userID)
	if err != nil {
		return nil, fmt.Errorf("getChatMember for tag readback: %w", err)
	}

	var actualTag string
	switch m := member.(type) {
	case tg.ChatMemberMember:
		actualTag = m.Tag
	case tg.ChatMemberRestricted:
		actualTag = m.Tag
	default:
		return nil, Skip(fmt.Sprintf("member type %T does not support tags (only regular/restricted members)", member))
	}

	if actualTag != s.ExpectedTag {
		return nil, fmt.Errorf("tag mismatch: expected %q, got %q", s.ExpectedTag, actualTag)
	}

	return &StepResult{
		Method: "getChatMember",
		Evidence: map[string]any{
			"user_id": userID, "expected_tag": s.ExpectedTag,
			"actual_tag": actualTag, "tags_match": true,
		},
	}, nil
}

// SendDateTimeMessageStep sends an HTML message with a <tg-time> entity.
type SendDateTimeMessageStep struct{}

func (s *SendDateTimeMessageStep) Name() string { return "sendMessage (date_time entity)" }

func (s *SendDateTimeMessageStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	unix := time.Now().Add(24 * time.Hour).Unix()
	html := tg.TimeHTML(unix, "wDT", "tomorrow at this time")
	text := "[galigo 9.5] Date/time: " + html

	msg, err := rt.Sender.SendMessage(ctx, rt.ChatID, text, WithParseMode("HTML"))
	if err != nil {
		return nil, err
	}
	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	hasDateTime := false
	for _, e := range msg.Entities {
		if e.Type == "date_time" {
			hasDateTime = true
		}
	}

	return &StepResult{
		Method:     "sendMessage",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"has_date_time_entity": hasDateTime,
			"entity_count":         len(msg.Entities),
			"unix_time":            unix,
		},
	}, nil
}

// SendMessageDraftStep sends a streaming draft.
type SendMessageDraftStep struct {
	DraftID int
	Text    string
}

func (s *SendMessageDraftStep) Name() string { return "sendMessageDraft" }

func (s *SendMessageDraftStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	err := rt.Sender.SendMessageDraft(ctx, rt.ChatID, s.DraftID, s.Text)
	if err != nil {
		// TEXTDRAFT_PEER_INVALID: sendMessageDraft only works in private (user-to-bot) chats
		var apiErr *tg.APIError
		if errors.As(err, &apiErr) && apiErr.Code == 400 &&
			strings.Contains(apiErr.Description, "PEER_INVALID") {
			return nil, Skip("sendMessageDraft requires a private chat (not group)")
		}
		return nil, err
	}
	return &StepResult{
		Method:   "sendMessageDraft",
		Evidence: map[string]any{"draft_id": s.DraftID, "text_len": len(s.Text)},
	}, nil
}
