package engine

import (
	"context"
	"fmt"
)

// ==================== Bot API 9.4 Steps ====================

// GetUserProfileAudiosStep gets profile audios of a user (9.4).
type GetUserProfileAudiosStep struct{}

func (s *GetUserProfileAudiosStep) Name() string { return "getUserProfileAudios" }

func (s *GetUserProfileAudiosStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	userID := rt.AdminUserID
	if userID == 0 {
		return nil, Skip("no AdminUserID available for getUserProfileAudios")
	}

	audios, err := rt.Sender.GetUserProfileAudios(ctx, userID)
	if err != nil {
		return nil, err
	}

	evidence := map[string]any{
		"user_id":     userID,
		"total_count": audios.TotalCount,
		"audio_count": len(audios.Audios),
	}

	// Log first audio details if present
	if len(audios.Audios) > 0 {
		evidence["first_audio_duration"] = audios.Audios[0].Duration
		if audios.Audios[0].Title != "" {
			evidence["first_audio_title"] = audios.Audios[0].Title
		}
	}

	return &StepResult{
		Method:   "getUserProfileAudios",
		Evidence: evidence,
	}, nil
}

// VerifyChatInfo94Step verifies 9.4 ChatFullInfo fields are deserializable.
type VerifyChatInfo94Step struct{}

func (s *VerifyChatInfo94Step) Name() string { return "getChat (9.4 fields)" }

func (s *VerifyChatInfo94Step) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	chat, err := rt.Sender.GetChat(ctx, rt.ChatID)
	if err != nil {
		return nil, err
	}

	evidence := map[string]any{
		"chat_id":   chat.ID,
		"chat_type": chat.Type,
	}

	// Log 9.4 fields if present (all optional, may be nil)
	if chat.FirstProfileAudio != nil {
		evidence["first_profile_audio_duration"] = chat.FirstProfileAudio.Duration
		evidence["has_first_profile_audio"] = true
	} else {
		evidence["has_first_profile_audio"] = false
	}

	if chat.UniqueGiftColors != nil {
		evidence["has_unique_gift_colors"] = true
		evidence["unique_gift_colors_light_main"] = chat.UniqueGiftColors.LightThemeMainColor
		evidence["unique_gift_colors_dark_main"] = chat.UniqueGiftColors.DarkThemeMainColor
	} else {
		evidence["has_unique_gift_colors"] = false
	}

	// paid_message_star_count (0 means not set or free)
	evidence["paid_message_star_count"] = chat.PaidMessageStarCount

	return &StepResult{
		Method:   "getChat",
		Evidence: evidence,
	}, nil
}

// SendMessageWithStyledButtonsStep sends a message with 9.4 styled buttons.
type SendMessageWithStyledButtonsStep struct {
	Text    string
	Buttons []ButtonDef // Use Style and IconCustomEmojiID fields
}

func (s *SendMessageWithStyledButtonsStep) Name() string { return "sendMessage (styled buttons)" }

func (s *SendMessageWithStyledButtonsStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	markup := buildKeyboard(s.Buttons)
	msg, err := rt.Sender.SendMessage(ctx, rt.ChatID, s.Text, WithReplyMarkup(markup))
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	// Collect evidence about the buttons
	buttonEvidence := make([]map[string]any, len(s.Buttons))
	for i, b := range s.Buttons {
		buttonEvidence[i] = map[string]any{
			"text":  b.Text,
			"style": b.Style,
		}
		if b.IconCustomEmojiID != "" {
			buttonEvidence[i]["icon_custom_emoji_id"] = b.IconCustomEmojiID
		}
	}

	return &StepResult{
		Method:     "sendMessage",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"message_id":   msg.MessageID,
			"text":         s.Text,
			"has_keyboard": msg.ReplyMarkup != nil,
			"buttons":      buttonEvidence,
		},
	}, nil
}

// VerifyVideoQualitiesStep verifies the 9.4 Video.Qualities field deserializes correctly.
type VerifyVideoQualitiesStep struct{}

func (s *VerifyVideoQualitiesStep) Name() string { return "verify video qualities (9.4)" }

func (s *VerifyVideoQualitiesStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.LastMessage == nil {
		return nil, fmt.Errorf("no last message")
	}
	if rt.LastMessage.Video == nil {
		return nil, fmt.Errorf("last message has no video")
	}

	v := rt.LastMessage.Video
	evidence := map[string]any{
		"file_id":  v.FileID,
		"width":    v.Width,
		"height":   v.Height,
		"duration": v.Duration,
	}

	// Log qualities if present (9.4 field)
	if len(v.Qualities) > 0 {
		evidence["qualities_count"] = len(v.Qualities)
		for i, q := range v.Qualities {
			evidence[fmt.Sprintf("quality_%d", i)] = map[string]any{
				"width":     q.Width,
				"height":    q.Height,
				"file_id":   q.FileID,
				"file_size": q.FileSize,
			}
		}
	} else {
		evidence["qualities_count"] = 0
		evidence["note"] = "Telegram did not include qualities (normal for short/small videos)"
	}

	return &StepResult{
		Method:   "sendVideo",
		Evidence: evidence,
	}, nil
}
