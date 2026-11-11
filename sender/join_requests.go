package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// Results of a join request query (Bot API 10.1), for AnswerChatJoinRequestQueryRequest.Result.
const (
	JoinRequestApprove = "approve" // allow the user to join the chat
	JoinRequestDecline = "decline" // do not allow the user to join the chat
	JoinRequestQueue   = "queue"   // leave the decision to other administrators
)

// AnswerChatJoinRequestQueryRequest represents an answerChatJoinRequestQuery request (Bot API 10.1).
type AnswerChatJoinRequestQueryRequest struct {
	ChatJoinRequestQueryID string `json:"chat_join_request_query_id"`
	Result                 string `json:"result"` // JoinRequestApprove, JoinRequestDecline or JoinRequestQueue
}

// SendChatJoinRequestWebAppRequest represents a sendChatJoinRequestWebApp request (Bot API 10.1).
type SendChatJoinRequestWebAppRequest struct {
	ChatJoinRequestQueryID string `json:"chat_join_request_query_id"`
	WebAppURL              string `json:"web_app_url"` // HTTPS URL of the Web App
}

// AnswerChatJoinRequestQuery processes a received chat join request query (Bot API 10.1); see
// tg.ChatJoinRequest.QueryID. The result is sent as given; the server checks its value.
// It is not retried.
func (c *Client) AnswerChatJoinRequestQuery(ctx context.Context, req AnswerChatJoinRequestQueryRequest) error {
	if req.ChatJoinRequestQueryID == "" {
		return tg.NewValidationError("chat_join_request_query_id", "required")
	}
	if req.Result == "" {
		return tg.NewValidationError("result", "required")
	}
	return c.callJSON(ctx, "answerChatJoinRequestQuery", req, nil)
}

// SendChatJoinRequestWebApp processes a received chat join request query by showing a Mini App to the user
// before the decision (Bot API 10.1); see tg.ChatJoinRequest.QueryID. Then call AnswerChatJoinRequestQuery
// with the result of the user's interaction with the Mini App. The URL is sent as given; the server checks it.
// It is not retried.
func (c *Client) SendChatJoinRequestWebApp(ctx context.Context, req SendChatJoinRequestWebAppRequest) error {
	if req.ChatJoinRequestQueryID == "" {
		return tg.NewValidationError("chat_join_request_query_id", "required")
	}
	if req.WebAppURL == "" {
		return tg.NewValidationError("web_app_url", "required")
	}
	return c.callJSON(ctx, "sendChatJoinRequestWebApp", req, nil)
}

// chatJoinRequestDecision is the body of approveChatJoinRequest and declineChatJoinRequest.
type chatJoinRequestDecision struct {
	ChatID tg.ChatID `json:"chat_id"`
	UserID int64     `json:"user_id"`
}

// ApproveChatJoinRequest approves a chat join request (Bot API 5.4); see tg.ChatJoinRequest. The bot must be an
// administrator in the chat with the can_invite_users right. It is not retried.
func (c *Client) ApproveChatJoinRequest(ctx context.Context, chatID tg.ChatID, userID int64) error {
	return c.decideChatJoinRequest(ctx, "approveChatJoinRequest", chatID, userID)
}

// DeclineChatJoinRequest declines a chat join request (Bot API 5.4); see tg.ChatJoinRequest. The bot must be an
// administrator in the chat with the can_invite_users right. It is not retried.
func (c *Client) DeclineChatJoinRequest(ctx context.Context, chatID tg.ChatID, userID int64) error {
	return c.decideChatJoinRequest(ctx, "declineChatJoinRequest", chatID, userID)
}

// decideChatJoinRequest validates the ids and sends approveChatJoinRequest or declineChatJoinRequest.
func (c *Client) decideChatJoinRequest(ctx context.Context, method string, chatID tg.ChatID, userID int64) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}
	if err := validateUserID(userID); err != nil {
		return err
	}
	return c.callJSON(ctx, method, chatJoinRequestDecision{ChatID: chatID, UserID: userID}, nil, extractChatID(chatID))
}
