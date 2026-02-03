package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// ================== Game Request Types ==================

// SendGameRequest represents a sendGame request.
type SendGameRequest struct {
	ChatID              int64                    `json:"chat_id"`
	GameShortName       string                   `json:"game_short_name"`
	MessageThreadID     int                      `json:"message_thread_id,omitempty"`
	DisableNotification bool                     `json:"disable_notification,omitempty"`
	ProtectContent      bool                     `json:"protect_content,omitempty"`
	ReplyParameters     *tg.ReplyParameters      `json:"reply_parameters,omitempty"`
	ReplyMarkup         *tg.InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

// SetGameScoreRequest represents a setGameScore request.
type SetGameScoreRequest struct {
	UserID             int64  `json:"user_id"`
	Score              int    `json:"score"`
	Force              bool   `json:"force,omitempty"`
	DisableEditMessage bool   `json:"disable_edit_message,omitempty"`
	ChatID             int64  `json:"chat_id,omitempty"`
	MessageID          int    `json:"message_id,omitempty"`
	InlineMessageID    string `json:"inline_message_id,omitempty"`
}

// GetGameHighScoresRequest represents a getGameHighScores request.
type GetGameHighScoresRequest struct {
	UserID          int64  `json:"user_id"`
	ChatID          int64  `json:"chat_id,omitempty"`
	MessageID       int    `json:"message_id,omitempty"`
	InlineMessageID string `json:"inline_message_id,omitempty"`
}

// ================== Game Methods ==================

// SendGame sends a game.
func (c *Client) SendGame(ctx context.Context, req SendGameRequest) (*tg.Message, error) {
	if req.ChatID == 0 {
		return nil, tg.NewValidationError("chat_id", "required")
	}
	if req.GameShortName == "" {
		return nil, tg.NewValidationError("game_short_name", "required")
	}

	var result tg.Message
	if err := c.callJSON(ctx, "sendGame", req, &result, extractChatID(tg.ChatID(req.ChatID))); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetGameScore sets the score of the specified user in a game message.
// Returns the edited message when chat_id + message_id are used.
// Returns nil when inline_message_id is used (Telegram returns true).
func (c *Client) SetGameScore(ctx context.Context, req SetGameScoreRequest) (*tg.Message, error) {
	if req.UserID <= 0 {
		return nil, tg.NewValidationError("user_id", "must be positive")
	}
	if req.Score < 0 {
		return nil, tg.NewValidationError("score", "must be non-negative")
	}
	if req.ChatID == 0 && req.InlineMessageID == "" {
		return nil, tg.NewValidationError("chat_id", "either chat_id+message_id or inline_message_id required")
	}

	if req.InlineMessageID != "" {
		// Inline mode: Telegram returns true, not a message
		return nil, c.callJSON(ctx, "setGameScore", req, nil)
	}

	var result tg.Message
	if err := c.callJSON(ctx, "setGameScore", req, &result, extractChatID(tg.ChatID(req.ChatID))); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetGameHighScores returns data for high score tables.
func (c *Client) GetGameHighScores(ctx context.Context, req GetGameHighScoresRequest) ([]tg.GameHighScore, error) {
	if req.UserID <= 0 {
		return nil, tg.NewValidationError("user_id", "must be positive")
	}
	if req.ChatID == 0 && req.InlineMessageID == "" {
		return nil, tg.NewValidationError("chat_id", "either chat_id+message_id or inline_message_id required")
	}

	var result []tg.GameHighScore
	if err := c.callJSON(ctx, "getGameHighScores", req, &result); err != nil {
		return nil, err
	}
	return result, nil
}
