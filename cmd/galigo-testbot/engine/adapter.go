package engine

import (
	"context"

	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// SenderAdapter adapts sender.Client to SenderClient interface.
type SenderAdapter struct {
	client *sender.Client
}

// NewSenderAdapter creates a new adapter wrapping a sender.Client.
func NewSenderAdapter(client *sender.Client) *SenderAdapter {
	return &SenderAdapter{client: client}
}

// GetMe returns basic information about the bot.
func (a *SenderAdapter) GetMe(ctx context.Context) (*tg.User, error) {
	return a.client.GetMe(ctx)
}

// SendMessage sends a text message.
func (a *SenderAdapter) SendMessage(ctx context.Context, chatID int64, text string, opts ...SendOption) (*tg.Message, error) {
	options := &SendOptions{}
	for _, opt := range opts {
		opt(options)
	}

	req := sender.SendMessageRequest{
		ChatID:    chatID,
		Text:      text,
		ParseMode: tg.ParseMode(options.ParseMode),
	}

	if options.ReplyMarkup != nil {
		req.ReplyMarkup = options.ReplyMarkup
	}

	return a.client.SendMessage(ctx, req)
}

// EditMessageText edits a message's text.
func (a *SenderAdapter) EditMessageText(ctx context.Context, chatID int64, messageID int, text string) (*tg.Message, error) {
	return a.client.EditMessageText(ctx, sender.EditMessageTextRequest{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
	})
}

// DeleteMessage deletes a message.
func (a *SenderAdapter) DeleteMessage(ctx context.Context, chatID int64, messageID int) error {
	return a.client.DeleteMessage(ctx, sender.DeleteMessageRequest{
		ChatID:    chatID,
		MessageID: messageID,
	})
}

// ForwardMessage forwards a message.
func (a *SenderAdapter) ForwardMessage(ctx context.Context, chatID, fromChatID int64, messageID int) (*tg.Message, error) {
	return a.client.ForwardMessage(ctx, sender.ForwardMessageRequest{
		ChatID:     chatID,
		FromChatID: fromChatID,
		MessageID:  messageID,
	})
}

// CopyMessage copies a message.
func (a *SenderAdapter) CopyMessage(ctx context.Context, chatID, fromChatID int64, messageID int) (*tg.MessageID, error) {
	return a.client.CopyMessage(ctx, sender.CopyMessageRequest{
		ChatID:     chatID,
		FromChatID: fromChatID,
		MessageID:  messageID,
	})
}

// SendChatAction sends a chat action.
func (a *SenderAdapter) SendChatAction(ctx context.Context, chatID int64, action string) error {
	return a.client.SendChatAction(ctx, chatID, action)
}

// Ensure SenderAdapter implements SenderClient.
var _ SenderClient = (*SenderAdapter)(nil)
