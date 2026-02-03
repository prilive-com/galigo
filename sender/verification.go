package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// ================== Verification Request Types ==================

// SetPassportDataErrorsRequest represents a setPassportDataErrors request.
type SetPassportDataErrorsRequest struct {
	UserID int64                     `json:"user_id"`
	Errors []tg.PassportElementError `json:"errors"`
}

// VerifyUserRequest represents a verifyUser request.
type VerifyUserRequest struct {
	UserID            int64  `json:"user_id"`
	CustomDescription string `json:"custom_description,omitempty"`
}

// VerifyChatRequest represents a verifyChat request.
type VerifyChatRequest struct {
	ChatID            tg.ChatID `json:"chat_id"`
	CustomDescription string    `json:"custom_description,omitempty"`
}

// RemoveUserVerificationRequest represents a removeUserVerification request.
type RemoveUserVerificationRequest struct {
	UserID int64 `json:"user_id"`
}

// RemoveChatVerificationRequest represents a removeChatVerification request.
type RemoveChatVerificationRequest struct {
	ChatID tg.ChatID `json:"chat_id"`
}

// ================== Verification Methods ==================

// SetPassportDataErrors informs a user that some of the Telegram Passport elements they provided contain errors.
func (c *Client) SetPassportDataErrors(ctx context.Context, req SetPassportDataErrorsRequest) error {
	if req.UserID <= 0 {
		return tg.NewValidationError("user_id", "must be positive")
	}
	if len(req.Errors) == 0 {
		return tg.NewValidationError("errors", "at least one error required")
	}

	return c.callJSON(ctx, "setPassportDataErrors", req, nil)
}

// VerifyUser verifies a user on behalf of the organization.
func (c *Client) VerifyUser(ctx context.Context, req VerifyUserRequest) error {
	if req.UserID <= 0 {
		return tg.NewValidationError("user_id", "must be positive")
	}

	return c.callJSON(ctx, "verifyUser", req, nil)
}

// VerifyChat verifies a chat on behalf of the organization.
func (c *Client) VerifyChat(ctx context.Context, req VerifyChatRequest) error {
	if err := validateChatID(req.ChatID); err != nil {
		return err
	}

	return c.callJSON(ctx, "verifyChat", req, nil, extractChatID(req.ChatID))
}

// RemoveUserVerification removes verification from a previously verified user.
func (c *Client) RemoveUserVerification(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return tg.NewValidationError("user_id", "must be positive")
	}

	return c.callJSON(ctx, "removeUserVerification", RemoveUserVerificationRequest{UserID: userID}, nil)
}

// RemoveChatVerification removes verification from a previously verified chat.
func (c *Client) RemoveChatVerification(ctx context.Context, chatID tg.ChatID) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}

	return c.callJSON(ctx, "removeChatVerification", RemoveChatVerificationRequest{ChatID: chatID}, nil, extractChatID(chatID))
}
