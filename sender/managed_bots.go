package sender

import (
	"context"
	"fmt"

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
// An empty token in the reply (a null result included) is an error.
//
// The token is a tg.SecretToken. The fmt verbs %v, %+v, %#v and %s, slog, and JSON and text marshalling write
// [REDACTED] in its place. fmt prints the real value for a verb that does not fit a string (such as %d) and for
// an unexported struct field that holds it; slog's text handler prints such a field the same way. Call Value()
// to use or store the token.
func (c *Client) GetManagedBotToken(ctx context.Context, userID int64) (tg.SecretToken, error) {
	return c.managedBotToken(ctx, "getManagedBotToken", userID)
}

// ReplaceManagedBotToken revokes the current token of a managed bot and returns the new one (Bot API 9.6).
//
// It is not retried: a repeat after a lost reply would revoke the token the first call returned. Telegram refused
// the call only when the error is a *tg.APIError with a 4xx code or a *tg.ValidationError. After any other error
// (an empty token in the reply, a 5xx *tg.APIError, a parse error, a timeout or cancel, a read error) the token may
// already be replaced: call GetManagedBotToken to read the current one, and do not repeat the replace. The returned
// token is redacted in output as GetManagedBotToken describes; call Value() to use or store it.
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
	if token.IsEmpty() {
		return "", fmt.Errorf("galigo: %s: empty token in response", method)
	}
	return token, nil
}

// SavePreparedKeyboardButton stores a keyboard button that a Mini App user can send (Bot API 9.6). Telegram
// checks the button type. An empty id in the reply (a null result included) is an error.
func (c *Client) SavePreparedKeyboardButton(ctx context.Context, req SavePreparedKeyboardButtonRequest) (*tg.PreparedKeyboardButton, error) {
	if req.UserID <= 0 {
		return nil, tg.NewValidationError("user_id", "must be positive")
	}
	var result tg.PreparedKeyboardButton
	if err := c.callJSON(ctx, "savePreparedKeyboardButton", req, &result); err != nil {
		return nil, err
	}
	if result.ID == "" {
		return nil, fmt.Errorf("galigo: savePreparedKeyboardButton: empty id in response")
	}
	return &result, nil
}

// SetManagedBotAccessSettingsRequest represents a setManagedBotAccessSettings request (Bot API 10.0). UserID is
// the managed bot's id.
type SetManagedBotAccessSettingsRequest struct {
	UserID             int64 `json:"user_id"`
	IsAccessRestricted bool  `json:"is_access_restricted"`
	// AddedUserIDs are the users who can access the bot when access is restricted. A nil list is not sent; a
	// non-nil empty list sends []. Telegram does not specify how it handles an absent or empty list. The spec
	// allows up to 10 ids; the client does not check that.
	AddedUserIDs []int64 `json:"added_user_ids,omitzero"`
}

// GetManagedBotAccessSettings returns the access settings of a bot that this bot manages (Bot API 10.0). userID is
// the managed bot's id. A null result is an error. A missing or null is_access_restricted reads as not restricted.
func (c *Client) GetManagedBotAccessSettings(ctx context.Context, userID int64) (*tg.BotAccessSettings, error) {
	if userID <= 0 {
		return nil, tg.NewValidationError("user_id", "must be positive")
	}
	var result *tg.BotAccessSettings
	if err := c.callJSON(ctx, "getManagedBotAccessSettings", managedBotUserRequest{UserID: userID}, &result); err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("galigo: getManagedBotAccessSettings: null result in response")
	}
	return result, nil
}

// SetManagedBotAccessSettings changes the access settings of a bot that this bot manages (Bot API 10.0). It is
// not retried.
func (c *Client) SetManagedBotAccessSettings(ctx context.Context, req SetManagedBotAccessSettingsRequest) error {
	if req.UserID <= 0 {
		return tg.NewValidationError("user_id", "must be positive")
	}
	return c.callJSON(ctx, "setManagedBotAccessSettings", req, nil)
}
