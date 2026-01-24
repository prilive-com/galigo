package galigo

import (
	"context"
	"log/slog"

	"github.com/prilive-com/galigo/receiver"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Bot is the unified Telegram bot client combining receiver and sender.
type Bot struct {
	token    tg.SecretToken
	logger   *slog.Logger
	receiver *receiver.PollingClient
	webhook  *receiver.WebhookHandler
	sender   *sender.Client
	updates  chan tg.Update
	config   botConfig
}

type botConfig struct {
	// Mode
	mode receiver.Mode

	// Polling settings
	pollingTimeout   int
	pollingLimit     int
	pollingMaxErrors int
	deleteWebhook    bool
	allowedUpdates   []string

	// Webhook settings
	webhookPort   int
	webhookSecret string
	allowedDomain string

	// Sender settings
	senderConfig sender.Config

	// Receiver settings
	receiverConfig receiver.Config

	// Buffer
	updateBufferSize int

	// Logger
	logger *slog.Logger
}

// Option configures the Bot.
type Option func(*botConfig)

// WithPolling configures long polling mode.
func WithPolling(timeout, limit int) Option {
	return func(c *botConfig) {
		c.mode = receiver.ModeLongPolling
		c.pollingTimeout = timeout
		c.pollingLimit = limit
	}
}

// WithWebhook configures webhook mode.
func WithWebhook(port int, secret string) Option {
	return func(c *botConfig) {
		c.mode = receiver.ModeWebhook
		c.webhookPort = port
		c.webhookSecret = secret
	}
}

// WithLogger sets a custom logger.
func WithLogger(logger *slog.Logger) Option {
	return func(c *botConfig) {
		c.logger = logger
	}
}

// WithRetries sets max retry attempts.
func WithRetries(max int) Option {
	return func(c *botConfig) {
		c.senderConfig.MaxRetries = max
	}
}

// WithRateLimit sets rate limiting.
func WithRateLimit(globalRPS float64, burst int) Option {
	return func(c *botConfig) {
		c.senderConfig.GlobalRPS = globalRPS
		c.senderConfig.GlobalBurst = burst
	}
}

// WithPollingMaxErrors sets max consecutive errors.
func WithPollingMaxErrors(max int) Option {
	return func(c *botConfig) {
		c.pollingMaxErrors = max
	}
}

// WithAllowedUpdates filters update types.
func WithAllowedUpdates(types ...string) Option {
	return func(c *botConfig) {
		c.allowedUpdates = types
	}
}

// WithDeleteWebhook deletes existing webhook before polling.
func WithDeleteWebhook(delete bool) Option {
	return func(c *botConfig) {
		c.deleteWebhook = delete
	}
}

// WithUpdateBufferSize sets the updates channel buffer size.
func WithUpdateBufferSize(size int) Option {
	return func(c *botConfig) {
		c.updateBufferSize = size
	}
}

// New creates a new unified Bot.
func New(token string, opts ...Option) (*Bot, error) {
	if token == "" {
		return nil, tg.ErrInvalidToken
	}

	cfg := botConfig{
		mode:             receiver.ModeLongPolling,
		pollingTimeout:   30,
		pollingLimit:     100,
		pollingMaxErrors: 10,
		updateBufferSize: 100,
		senderConfig:     sender.DefaultConfig(),
		receiverConfig:   receiver.DefaultConfig(),
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	secretToken := tg.SecretToken(token)
	cfg.senderConfig.Token = secretToken
	cfg.receiverConfig.Token = secretToken
	cfg.receiverConfig.Mode = cfg.mode
	cfg.receiverConfig.PollingTimeout = cfg.pollingTimeout
	cfg.receiverConfig.PollingLimit = cfg.pollingLimit
	cfg.receiverConfig.PollingMaxErrors = cfg.pollingMaxErrors
	cfg.receiverConfig.DeleteWebhookFirst = cfg.deleteWebhook
	cfg.receiverConfig.AllowedUpdates = cfg.allowedUpdates
	cfg.receiverConfig.WebhookPort = cfg.webhookPort
	cfg.receiverConfig.WebhookSecret = cfg.webhookSecret

	// Use configured logger or default
	logger := cfg.logger
	if logger == nil {
		logger = slog.Default()
	}

	// Create sender
	senderClient, err := sender.NewFromConfig(cfg.senderConfig, sender.WithLogger(logger))
	if err != nil {
		return nil, err
	}

	// Create updates channel
	updates := make(chan tg.Update, cfg.updateBufferSize)

	bot := &Bot{
		token:   secretToken,
		logger:  logger,
		sender:  senderClient,
		updates: updates,
		config:  cfg,
	}

	// Create receiver based on mode
	if cfg.mode == receiver.ModeLongPolling {
		bot.receiver = receiver.NewPollingClient(
			secretToken,
			updates,
			logger,
			cfg.receiverConfig,
			receiver.WithPollingMaxErrors(cfg.pollingMaxErrors),
			receiver.WithPollingAllowedUpdates(cfg.allowedUpdates),
			receiver.WithPollingDeleteWebhook(cfg.deleteWebhook),
		)
	} else {
		bot.webhook = receiver.NewWebhookHandler(logger, updates, cfg.receiverConfig)
	}

	return bot, nil
}

// Start begins receiving updates.
func (b *Bot) Start(ctx context.Context) error {
	if b.receiver != nil {
		return b.receiver.Start(ctx)
	}
	// Webhook mode: handler is used via WebhookHandler()
	return nil
}

// Stop gracefully stops the bot.
func (b *Bot) Stop() {
	if b.receiver != nil {
		b.receiver.Stop()
	}
}

// Close releases all resources.
func (b *Bot) Close() error {
	b.Stop()
	return b.sender.Close()
}

// Updates returns the updates channel.
func (b *Bot) Updates() <-chan tg.Update {
	return b.updates
}

// WebhookHandler returns the HTTP handler for webhook mode.
func (b *Bot) WebhookHandler() *receiver.WebhookHandler {
	return b.webhook
}

// IsHealthy returns health status for K8s probes.
func (b *Bot) IsHealthy() bool {
	if b.receiver != nil {
		return b.receiver.IsHealthy()
	}
	return true
}

// SendMessage sends a text message.
func (b *Bot) SendMessage(ctx context.Context, chatID tg.ChatID, text string, opts ...SendOption) (*tg.Message, error) {
	req := sender.SendMessageRequest{
		ChatID: chatID,
		Text:   text,
	}
	for _, opt := range opts {
		opt(&req)
	}
	return b.sender.SendMessage(ctx, req)
}

// SendPhoto sends a photo.
func (b *Bot) SendPhoto(ctx context.Context, chatID tg.ChatID, photo string, opts ...PhotoOption) (*tg.Message, error) {
	req := sender.SendPhotoRequest{
		ChatID: chatID,
		Photo:  photo,
	}
	for _, opt := range opts {
		opt(&req)
	}
	return b.sender.SendPhoto(ctx, req)
}

// Edit edits a message text.
func (b *Bot) Edit(ctx context.Context, e tg.Editable, text string, opts ...sender.EditOption) (*tg.Message, error) {
	return b.sender.Edit(ctx, e, text, opts...)
}

// Delete deletes a message.
func (b *Bot) Delete(ctx context.Context, e tg.Editable) error {
	return b.sender.Delete(ctx, e)
}

// Forward forwards a message.
func (b *Bot) Forward(ctx context.Context, e tg.Editable, toChatID tg.ChatID, opts ...sender.ForwardOption) (*tg.Message, error) {
	return b.sender.Forward(ctx, e, toChatID, opts...)
}

// Copy copies a message.
func (b *Bot) Copy(ctx context.Context, e tg.Editable, toChatID tg.ChatID, opts ...sender.CopyOption) (*tg.MessageID, error) {
	return b.sender.Copy(ctx, e, toChatID, opts...)
}

// Answer answers a callback query.
func (b *Bot) Answer(ctx context.Context, cb *tg.CallbackQuery, opts ...sender.AnswerOption) error {
	return b.sender.Answer(ctx, cb, opts...)
}

// Acknowledge silently acknowledges a callback query.
func (b *Bot) Acknowledge(ctx context.Context, cb *tg.CallbackQuery) error {
	return b.sender.Acknowledge(ctx, cb)
}

// Sender returns the underlying sender client for advanced usage.
func (b *Bot) Sender() *sender.Client {
	return b.sender
}

// SendOption configures send message requests.
type SendOption func(*sender.SendMessageRequest)

// WithParseMode sets the parse mode.
func WithParseMode(mode tg.ParseMode) SendOption {
	return func(r *sender.SendMessageRequest) {
		r.ParseMode = mode
	}
}

// WithKeyboard sets the reply keyboard.
func WithKeyboard(kb *tg.InlineKeyboardMarkup) SendOption {
	return func(r *sender.SendMessageRequest) {
		r.ReplyMarkup = kb
	}
}

// WithReplyTo sets the reply-to message ID.
func WithReplyTo(messageID int) SendOption {
	return func(r *sender.SendMessageRequest) {
		r.ReplyToMessageID = messageID
	}
}

// Silent disables notification.
func Silent() SendOption {
	return func(r *sender.SendMessageRequest) {
		r.DisableNotification = true
	}
}

// PhotoOption configures send photo requests.
type PhotoOption func(*sender.SendPhotoRequest)

// WithPhotoCaption sets the photo caption.
func WithPhotoCaption(caption string) PhotoOption {
	return func(r *sender.SendPhotoRequest) {
		r.Caption = caption
	}
}

// WithPhotoParseMode sets parse mode for caption.
func WithPhotoParseMode(mode tg.ParseMode) PhotoOption {
	return func(r *sender.SendPhotoRequest) {
		r.ParseMode = mode
	}
}

// WithPhotoKeyboard sets the reply keyboard.
func WithPhotoKeyboard(kb *tg.InlineKeyboardMarkup) PhotoOption {
	return func(r *sender.SendPhotoRequest) {
		r.ReplyMarkup = kb
	}
}

// PhotoSilent disables notification for photo.
func PhotoSilent() PhotoOption {
	return func(r *sender.SendPhotoRequest) {
		r.DisableNotification = true
	}
}
