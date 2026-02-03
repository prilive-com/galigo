package testutil

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// TelegramEnvelope is the standard Telegram API response format.
type TelegramEnvelope struct {
	OK          bool        `json:"ok"`
	Result      any         `json:"result,omitempty"`
	ErrorCode   int         `json:"error_code,omitempty"`
	Description string      `json:"description,omitempty"`
	Parameters  *Parameters `json:"parameters,omitempty"`
}

// Parameters contains optional error parameters (e.g., retry_after).
type Parameters struct {
	RetryAfter      int   `json:"retry_after,omitempty"`
	MigrateToChatID int64 `json:"migrate_to_chat_id,omitempty"`
}

// ReplyOK writes a successful Telegram API response.
func ReplyOK(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(TelegramEnvelope{
		OK:     true,
		Result: result,
	})
}

// ReplyError writes a Telegram API error response.
func ReplyError(w http.ResponseWriter, code int, description string, params *Parameters) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(TelegramEnvelope{
		OK:          false,
		ErrorCode:   code,
		Description: description,
		Parameters:  params,
	})
}

// ReplyRateLimit writes a 429 rate limit response with retry_after in both JSON and HTTP header.
func ReplyRateLimit(w http.ResponseWriter, retryAfter int) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	ReplyError(w, 429, "Too Many Requests: retry after "+strconv.Itoa(retryAfter), &Parameters{
		RetryAfter: retryAfter,
	})
}

// ReplyRateLimitHeaderOnly writes a 429 rate limit response with retry_after ONLY in HTTP header.
// Useful for testing HTTP header fallback parsing.
func ReplyRateLimitHeaderOnly(w http.ResponseWriter, retryAfter int) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	ReplyError(w, 429, "Too Many Requests: retry after "+strconv.Itoa(retryAfter), nil)
}

// ReplyServerError writes a 5xx server error response.
func ReplyServerError(w http.ResponseWriter, code int, description string) {
	ReplyError(w, code, description, nil)
}

// ReplyBadRequest writes a 400 bad request error.
func ReplyBadRequest(w http.ResponseWriter, description string) {
	ReplyError(w, 400, "Bad Request: "+description, nil)
}

// ReplyForbidden writes a 403 forbidden error (e.g., bot blocked).
func ReplyForbidden(w http.ResponseWriter, description string) {
	ReplyError(w, 403, "Forbidden: "+description, nil)
}

// ReplyNotFound writes a 404 not found error.
func ReplyNotFound(w http.ResponseWriter, description string) {
	ReplyError(w, 404, "Not Found: "+description, nil)
}

// ReplyMessage writes a successful message response.
func ReplyMessage(w http.ResponseWriter, messageID int) {
	ReplyOK(w, map[string]any{
		"message_id": messageID,
		"date":       1234567890,
		"chat": map[string]any{
			"id":   TestChatID,
			"type": "private",
		},
		"text": "Test message",
	})
}

// ReplyMessageWithChat writes a successful message response for a specific chat.
func ReplyMessageWithChat(w http.ResponseWriter, messageID int, chatID int64) {
	ReplyOK(w, map[string]any{
		"message_id": messageID,
		"date":       1234567890,
		"chat": map[string]any{
			"id":   chatID,
			"type": "private",
		},
		"text": "Test message",
	})
}

// ReplyBool writes a successful boolean response (for deleteMessage, etc.).
func ReplyBool(w http.ResponseWriter, result bool) {
	ReplyOK(w, result)
}

// ReplyMessageID writes a successful MessageId response (for copyMessage).
func ReplyMessageID(w http.ResponseWriter, messageID int) {
	ReplyOK(w, map[string]any{
		"message_id": messageID,
	})
}

// ReplyUpdates writes a successful getUpdates response.
func ReplyUpdates(w http.ResponseWriter, updates []map[string]any) {
	ReplyOK(w, updates)
}

// ReplyEmptyUpdates writes an empty getUpdates response.
func ReplyEmptyUpdates(w http.ResponseWriter) {
	ReplyOK(w, []map[string]any{})
}

// ReplyUser writes a successful getMe response.
func ReplyUser(w http.ResponseWriter) {
	ReplyOK(w, map[string]any{
		"id":         TestBotID,
		"is_bot":     true,
		"first_name": "Test Bot",
		"username":   "testbot",
	})
}

// ReplyWebhookInfo writes a successful getWebhookInfo response.
func ReplyWebhookInfo(w http.ResponseWriter, url string, pendingCount int) {
	ReplyOK(w, map[string]any{
		"url":                    url,
		"has_custom_certificate": false,
		"pending_update_count":   pendingCount,
	})
}
