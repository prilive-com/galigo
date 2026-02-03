package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// ================== Poll Requests ==================

// SendPollRequest represents a sendPoll request (enhanced version).
type SendPollRequest struct {
	ChatID                tg.ChatID         `json:"chat_id"`
	Question              string            `json:"question"`
	Options               []InputPollOption `json:"options"`
	IsAnonymous           *bool             `json:"is_anonymous,omitempty"`
	Type                  string            `json:"type,omitempty"` // "regular" or "quiz"
	AllowsMultipleAnswers bool              `json:"allows_multiple_answers,omitempty"`
	CorrectOptionID       *int              `json:"correct_option_id,omitempty"` // For quiz
	Explanation           string            `json:"explanation,omitempty"`
	ExplanationParseMode  tg.ParseMode      `json:"explanation_parse_mode,omitempty"`
	OpenPeriod            int               `json:"open_period,omitempty"` // 5-600 seconds
	CloseDate             int64             `json:"close_date,omitempty"`
	IsClosed              bool              `json:"is_closed,omitempty"`
	DisableNotification   bool              `json:"disable_notification,omitempty"`
	ProtectContent        bool              `json:"protect_content,omitempty"`
	ReplyToMessageID      int               `json:"reply_to_message_id,omitempty"`
	ReplyMarkup           any               `json:"reply_markup,omitempty"`
}

// InputPollOption represents a poll option.
type InputPollOption struct {
	Text          string             `json:"text"`
	TextParseMode tg.ParseMode       `json:"text_parse_mode,omitempty"`
	TextEntities  []tg.MessageEntity `json:"text_entities,omitempty"`
}

// StopPollRequest represents a stopPoll request.
type StopPollRequest struct {
	ChatID      tg.ChatID `json:"chat_id"`
	MessageID   int       `json:"message_id"`
	ReplyMarkup any       `json:"reply_markup,omitempty"`
}

// ================== Poll Methods ==================

// SendPollSimple sends a simple regular poll.
func (c *Client) SendPollSimple(ctx context.Context, chatID tg.ChatID, question string, options []string, opts ...PollOption) (*tg.Message, error) {
	if err := validateChatID(chatID); err != nil {
		return nil, err
	}
	if question == "" {
		return nil, tg.NewValidationError("question", "cannot be empty")
	}
	if len(options) < 2 {
		return nil, tg.NewValidationError("options", "must have at least 2 options")
	}
	if len(options) > 10 {
		return nil, tg.NewValidationError("options", "cannot exceed 10 options")
	}

	inputOptions := make([]InputPollOption, len(options))
	for i, opt := range options {
		inputOptions[i] = InputPollOption{Text: opt}
	}

	req := SendPollRequest{
		ChatID:   chatID,
		Question: question,
		Options:  inputOptions,
	}
	for _, opt := range opts {
		opt(&req)
	}

	var msg tg.Message
	if err := c.callJSON(ctx, "sendPoll", req, &msg, extractChatID(chatID)); err != nil {
		return nil, err
	}
	return &msg, nil
}

// SendQuiz sends a quiz poll with a correct answer.
func (c *Client) SendQuiz(ctx context.Context, chatID tg.ChatID, question string, options []string, correctOptionIndex int, opts ...PollOption) (*tg.Message, error) {
	if err := validateChatID(chatID); err != nil {
		return nil, err
	}
	if correctOptionIndex < 0 || correctOptionIndex >= len(options) {
		return nil, tg.NewValidationError("correct_option_id", "must be valid index within options")
	}

	inputOptions := make([]InputPollOption, len(options))
	for i, opt := range options {
		inputOptions[i] = InputPollOption{Text: opt}
	}

	req := SendPollRequest{
		ChatID:          chatID,
		Question:        question,
		Options:         inputOptions,
		Type:            "quiz",
		CorrectOptionID: &correctOptionIndex,
	}
	for _, opt := range opts {
		opt(&req)
	}

	var msg tg.Message
	if err := c.callJSON(ctx, "sendPoll", req, &msg, extractChatID(chatID)); err != nil {
		return nil, err
	}
	return &msg, nil
}

// StopPoll stops a poll and returns the final results.
func (c *Client) StopPoll(ctx context.Context, chatID tg.ChatID, messageID int, opts ...StopPollOption) (*tg.Poll, error) {
	if err := validateChatID(chatID); err != nil {
		return nil, err
	}
	if err := validateMessageID(messageID); err != nil {
		return nil, err
	}

	req := StopPollRequest{
		ChatID:    chatID,
		MessageID: messageID,
	}
	for _, opt := range opts {
		opt(&req)
	}

	var poll tg.Poll
	if err := c.callJSON(ctx, "stopPoll", req, &poll, extractChatID(chatID)); err != nil {
		return nil, err
	}
	return &poll, nil
}

// ================== Options ==================

// PollOption configures SendPoll.
type PollOption func(*SendPollRequest)

// WithPollAnonymous sets whether the poll is anonymous.
func WithPollAnonymous(anonymous bool) PollOption {
	return func(r *SendPollRequest) {
		r.IsAnonymous = &anonymous
	}
}

// WithMultipleAnswers allows multiple answers in regular polls.
func WithMultipleAnswers() PollOption {
	return func(r *SendPollRequest) {
		r.AllowsMultipleAnswers = true
	}
}

// WithQuizExplanation sets explanation shown after answering quiz.
func WithQuizExplanation(explanation string, parseMode tg.ParseMode) PollOption {
	return func(r *SendPollRequest) {
		r.Explanation = explanation
		r.ExplanationParseMode = parseMode
	}
}

// WithPollOpenPeriod sets how long the poll is active (5-600 seconds).
func WithPollOpenPeriod(seconds int) PollOption {
	return func(r *SendPollRequest) {
		r.OpenPeriod = seconds
	}
}

// StopPollOption configures StopPoll.
type StopPollOption func(*StopPollRequest)

// WithStopPollReplyMarkup sets inline keyboard for stopped poll.
func WithStopPollReplyMarkup(markup any) StopPollOption {
	return func(r *StopPollRequest) {
		r.ReplyMarkup = markup
	}
}
