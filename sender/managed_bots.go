package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// ================== Bot API 9.6: Managed Bots ==================

// SavePreparedKeyboardButtonRequest represents a savePreparedKeyboardButton request.
type SavePreparedKeyboardButtonRequest struct {
	UserID int64             `json:"user_id"`
	Button tg.KeyboardButton `json:"button"`
}

// managedBotUserRequest is the body of getManagedBotToken and replaceManagedBotToken.
type managedBotUserRequest struct {
	UserID int64 `json:"user_id"`
}

// GetManagedBotToken returns the token of a bot that this bot manages (Bot API 9.6). userID is the managed bot's id.
//
// The token is a tg.SecretToken, so fmt and slog print it redacted. Call Value() to use or store it: JSON and
// text marshalling also write [REDACTED].
func (c *Client) GetManagedBotToken(ctx context.Context, userID int64) (tg.SecretToken, error) {
	return c.managedBotToken(ctx, "getManagedBotToken", userID)
}

// ReplaceManagedBotToken revokes the current token of a managed bot and returns the new one (Bot API 9.6).
//
// It is not retried: a repeat after a lost reply would revoke the token the first call returned. If the outcome is
// unknown (timeout, cancel), call GetManagedBotToken to read the current token. Call Value() to use or store the
// token: fmt, slog, JSON and text marshalling print it redacted.
func (c *Client) ReplaceManagedBotToken(ctx context.Context, userID int64) (tg.SecretToken, error) {
	return c.managedBotToken(ctx, "replaceManagedBotToken", userID)
}

func (c *Client) managedBotToken(ctx context.Context, method string, userID int64) (tg.SecretToken, error) {
	if userID <= 0 {
		return "", tg.NewValidationError("user_id", "must be positive")
	}
	var token tg.SecretToken
	if err := c.callJSON(ctx, method, managedBotUserRequest{UserID: userID}, &token); err != nil {
		return "", err
	}
	return token, nil
}

// SavePreparedKeyboardButton stores a keyboard button that a Mini App user can send (Bot API 9.6). Telegram
// checks the button type.
func (c *Client) SavePreparedKeyboardButton(ctx context.Context, req SavePreparedKeyboardButtonRequest) (*tg.PreparedKeyboardButton, error) {
	if req.UserID <= 0 {
		return nil, tg.NewValidationError("user_id", "must be positive")
	}
	var result tg.PreparedKeyboardButton
	if err := c.callJSON(ctx, "savePreparedKeyboardButton", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
