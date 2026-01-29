package sender

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prilive-com/galigo/tg"
)

// ================== Chat Information Requests ==================

// GetChatRequest represents a getChat request.
type GetChatRequest struct {
	ChatID tg.ChatID `json:"chat_id"`
}

// GetChatMemberRequest represents a getChatMember request.
type GetChatMemberRequest struct {
	ChatID tg.ChatID `json:"chat_id"`
	UserID int64     `json:"user_id"`
}

// ================== Chat Information Methods ==================

// GetChat returns full information about a chat.
func (c *Client) GetChat(ctx context.Context, chatID tg.ChatID) (*tg.ChatFullInfo, error) {
	if err := validateChatID(chatID); err != nil {
		return nil, err
	}

	var result tg.ChatFullInfo
	if err := c.callJSON(ctx, "getChat", GetChatRequest{ChatID: chatID}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetChatAdministrators returns a list of administrators in a chat.
func (c *Client) GetChatAdministrators(ctx context.Context, chatID tg.ChatID) ([]tg.ChatMember, error) {
	if err := validateChatID(chatID); err != nil {
		return nil, err
	}

	resp, err := c.executeRequest(ctx, "getChatAdministrators", GetChatRequest{ChatID: chatID})
	if err != nil {
		return nil, err
	}

	// Custom unmarshaling for ChatMember union type
	var rawMembers []json.RawMessage
	if err := json.Unmarshal(resp.Result, &rawMembers); err != nil {
		return nil, fmt.Errorf("galigo: getChatAdministrators: failed to parse response: %w", err)
	}

	members := make([]tg.ChatMember, 0, len(rawMembers))
	for _, raw := range rawMembers {
		member, err := tg.UnmarshalChatMember(raw)
		if err != nil {
			return nil, fmt.Errorf("galigo: getChatAdministrators: %w", err)
		}
		members = append(members, member)
	}

	return members, nil
}

// GetChatMemberCount returns the number of members in a chat.
func (c *Client) GetChatMemberCount(ctx context.Context, chatID tg.ChatID) (int, error) {
	if err := validateChatID(chatID); err != nil {
		return 0, err
	}

	var result int
	if err := c.callJSON(ctx, "getChatMemberCount", GetChatRequest{ChatID: chatID}, &result); err != nil {
		return 0, err
	}
	return result, nil
}

// GetChatMember returns information about a member of a chat.
func (c *Client) GetChatMember(ctx context.Context, chatID tg.ChatID, userID int64) (tg.ChatMember, error) {
	if err := validateChatID(chatID); err != nil {
		return nil, err
	}
	if err := validateUserID(userID); err != nil {
		return nil, err
	}

	resp, err := c.executeRequest(ctx, "getChatMember", GetChatMemberRequest{
		ChatID: chatID,
		UserID: userID,
	})
	if err != nil {
		return nil, err
	}

	return tg.UnmarshalChatMember(resp.Result)
}
