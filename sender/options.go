package sender

import "github.com/prilive-com/galigo/tg"

// EditOption configures edit requests.
type EditOption func(*EditMessageTextRequest)

// WithEditParseMode sets the parse mode for editing.
func WithEditParseMode(mode tg.ParseMode) EditOption {
	return func(r *EditMessageTextRequest) {
		r.ParseMode = mode
	}
}

// WithEditKeyboard sets the reply markup for editing.
func WithEditKeyboard(kb *tg.InlineKeyboardMarkup) EditOption {
	return func(r *EditMessageTextRequest) {
		r.ReplyMarkup = kb
	}
}

// WithDisableWebPreview disables web page preview.
func WithDisableWebPreview(disable bool) EditOption {
	return func(r *EditMessageTextRequest) {
		r.DisableWebPagePreview = disable
	}
}

// ForwardOption configures forward requests.
type ForwardOption func(*ForwardMessageRequest)

// Silent disables notification for forwarding.
func Silent() ForwardOption {
	return func(r *ForwardMessageRequest) {
		r.DisableNotification = true
	}
}

// Protected protects content from forwarding.
func Protected() ForwardOption {
	return func(r *ForwardMessageRequest) {
		r.ProtectContent = true
	}
}

// CopyOption configures copy requests.
type CopyOption func(*CopyMessageRequest)

// WithCopyCaption sets a new caption when copying.
func WithCopyCaption(caption string) CopyOption {
	return func(r *CopyMessageRequest) {
		r.Caption = caption
	}
}

// WithCopyParseMode sets parse mode for copied caption.
func WithCopyParseMode(mode tg.ParseMode) CopyOption {
	return func(r *CopyMessageRequest) {
		r.ParseMode = mode
	}
}

// CopySilent disables notification when copying.
func CopySilent() CopyOption {
	return func(r *CopyMessageRequest) {
		r.DisableNotification = true
	}
}

// CopyProtected protects copied content.
func CopyProtected() CopyOption {
	return func(r *CopyMessageRequest) {
		r.ProtectContent = true
	}
}

// WithCopyReply sets reply-to message ID when copying.
func WithCopyReply(messageID int) CopyOption {
	return func(r *CopyMessageRequest) {
		r.ReplyToMessageID = messageID
	}
}

// WithCopyKeyboard sets keyboard when copying.
func WithCopyKeyboard(kb *tg.InlineKeyboardMarkup) CopyOption {
	return func(r *CopyMessageRequest) {
		r.ReplyMarkup = kb
	}
}

// AnswerOption configures callback answer requests.
type AnswerOption func(*AnswerCallbackQueryRequest)

// AnswerText sets the text for callback answer.
func AnswerText(text string) AnswerOption {
	return func(r *AnswerCallbackQueryRequest) {
		r.Text = text
	}
}

// Alert shows the answer as an alert.
func Alert() AnswerOption {
	return func(r *AnswerCallbackQueryRequest) {
		r.ShowAlert = true
	}
}

// WithAnswerURL sets URL to open.
func WithAnswerURL(url string) AnswerOption {
	return func(r *AnswerCallbackQueryRequest) {
		r.URL = url
	}
}

// WithAnswerCacheTime sets how long to cache the answer.
func WithAnswerCacheTime(seconds int) AnswerOption {
	return func(r *AnswerCallbackQueryRequest) {
		r.CacheTime = seconds
	}
}
