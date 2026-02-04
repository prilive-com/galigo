package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Scenario is a named sequence of steps that declares method coverage.
type Scenario interface {
	Name() string
	Description() string
	Covers() []string // Methods this scenario exercises
	Steps() []Step
	Timeout() time.Duration
}

// BaseScenario provides common implementation.
type BaseScenario struct {
	ScenarioName        string
	ScenarioDescription string
	CoveredMethods      []string
	ScenarioSteps       []Step
	ScenarioTimeout     time.Duration
}

func (s *BaseScenario) Name() string           { return s.ScenarioName }
func (s *BaseScenario) Description() string    { return s.ScenarioDescription }
func (s *BaseScenario) Covers() []string       { return s.CoveredMethods }
func (s *BaseScenario) Steps() []Step          { return s.ScenarioSteps }
func (s *BaseScenario) Timeout() time.Duration { return s.ScenarioTimeout }

// Step represents a single test step.
type Step interface {
	Name() string
	Execute(ctx context.Context, rt *Runtime) (*StepResult, error)
}

// StepResult captures evidence from step execution.
type StepResult struct {
	StepName   string        `json:"step_name"`
	Method     string        `json:"method,omitempty"`
	Duration   time.Duration `json:"duration"`
	Success    bool          `json:"success"`
	Error      string        `json:"error,omitempty"`
	MessageIDs []int         `json:"message_ids,omitempty"`
	FileIDs    []string      `json:"file_ids,omitempty"`
	Evidence   any           `json:"evidence,omitempty"`
}

// ScenarioResult captures the result of running a scenario.
type ScenarioResult struct {
	ScenarioName string        `json:"scenario_name"`
	Covers       []string      `json:"covers"`
	StartTime    time.Time     `json:"start_time"`
	EndTime      time.Time     `json:"end_time"`
	Duration     time.Duration `json:"duration"`
	Success      bool          `json:"success"`
	Skipped      bool          `json:"skipped,omitempty"`
	SkipReason   string        `json:"skip_reason,omitempty"`
	Error        string        `json:"error,omitempty"`
	Steps        []StepResult  `json:"steps"`
}

// CreatedMessage tracks messages for cleanup.
type CreatedMessage struct {
	ChatID    int64
	MessageID int
}

// ChatContext holds probed chat capabilities.
type ChatContext struct {
	ChatID   int64
	ChatType string // "private", "group", "supergroup", "channel"
	IsForum  bool

	// Bot capabilities (probed via getChatMember)
	BotIsAdmin         bool
	CanChangeInfo      bool
	CanDeleteMessages  bool
	CanRestrictMembers bool
	CanPinMessages     bool
	CanManageTopics    bool
	CanInviteUsers     bool
}

// ChatPhotoSnapshot stores state for restore.
type ChatPhotoSnapshot struct {
	HadPhoto bool
	FileID   string // BigFileID for restore via FromFileID
}

// PermissionsSnapshot stores permissions for restore.
type PermissionsSnapshot struct {
	Permissions *tg.ChatPermissions
}

// Runtime provides context for step execution.
type Runtime struct {
	Sender SenderClient
	ChatID int64

	// AdminUserID is a human user ID for operations that require a real user (e.g. createNewStickerSet).
	AdminUserID int64

	// State shared between steps
	CreatedMessages    []CreatedMessage
	LastMessage        *tg.Message
	LastMessageID      *tg.MessageID
	BulkMessageIDs     []int             // For bulk operations
	CapturedFileIDs    map[string]string // name -> file_id for reuse
	CreatedStickerSets []string          // sticker set names to clean up

	// Probed capabilities
	ChatCtx *ChatContext

	// Snapshots for save/restore
	OriginalChatPhoto   *ChatPhotoSnapshot
	OriginalPermissions *PermissionsSnapshot

	// Optional chat IDs from config
	ForumChatID int64
	TestUserID  int64

	// CallbackChan receives callback queries from polling (interactive scenarios only).
	CallbackChan chan *tg.CallbackQuery
}

// NewRuntime creates a new runtime for scenario execution.
func NewRuntime(sender SenderClient, chatID int64, adminUserID int64) *Runtime {
	return &Runtime{
		Sender:             sender,
		ChatID:             chatID,
		AdminUserID:        adminUserID,
		CreatedMessages:    make([]CreatedMessage, 0),
		CapturedFileIDs:    make(map[string]string),
		CreatedStickerSets: make([]string, 0),
	}
}

// ProbeChat discovers chat capabilities by calling getChat and getChatMember.
func (rt *Runtime) ProbeChat(ctx context.Context) error {
	chat, err := rt.Sender.GetChat(ctx, rt.ChatID)
	if err != nil {
		return fmt.Errorf("probeChat: %w", err)
	}

	rt.ChatCtx = &ChatContext{
		ChatID:   chat.ID,
		ChatType: chat.Type,
		IsForum:  chat.IsForum,
	}

	me, err := rt.Sender.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("probeChat getMe: %w", err)
	}

	member, err := rt.Sender.GetChatMember(ctx, rt.ChatID, me.ID)
	if err != nil {
		// Not a member or can't query — that's OK, just not admin
		return nil
	}

	status := member.Status()
	if status == "creator" {
		rt.ChatCtx.BotIsAdmin = true
		rt.ChatCtx.CanChangeInfo = true
		rt.ChatCtx.CanDeleteMessages = true
		rt.ChatCtx.CanRestrictMembers = true
		rt.ChatCtx.CanPinMessages = true
		rt.ChatCtx.CanManageTopics = true
		rt.ChatCtx.CanInviteUsers = true
	} else if status == "administrator" {
		rt.ChatCtx.BotIsAdmin = true
		if admin, ok := member.(*tg.ChatMemberAdministrator); ok {
			rt.ChatCtx.CanChangeInfo = admin.CanChangeInfo
			rt.ChatCtx.CanDeleteMessages = admin.CanDeleteMessages
			rt.ChatCtx.CanRestrictMembers = admin.CanRestrictMembers
			rt.ChatCtx.CanInviteUsers = admin.CanInviteUsers
			// Pointer fields — dereference safely
			if admin.CanPinMessages != nil {
				rt.ChatCtx.CanPinMessages = *admin.CanPinMessages
			}
			if admin.CanManageTopics != nil {
				rt.ChatCtx.CanManageTopics = *admin.CanManageTopics
			}
		}
	}

	return nil
}

// TrackMessage adds a message to the cleanup list.
func (rt *Runtime) TrackMessage(chatID int64, messageID int) {
	rt.CreatedMessages = append(rt.CreatedMessages, CreatedMessage{
		ChatID:    chatID,
		MessageID: messageID,
	})
}

// TrackStickerSet adds a sticker set name to the cleanup list.
func (rt *Runtime) TrackStickerSet(name string) {
	rt.CreatedStickerSets = append(rt.CreatedStickerSets, name)
}

// SenderClient is the interface for sending messages (allows mocking).
type SenderClient interface {
	GetMe(ctx context.Context) (*tg.User, error)
	SendMessage(ctx context.Context, chatID int64, text string, opts ...SendOption) (*tg.Message, error)
	EditMessageText(ctx context.Context, chatID int64, messageID int, text string) (*tg.Message, error)
	DeleteMessage(ctx context.Context, chatID int64, messageID int) error
	ForwardMessage(ctx context.Context, chatID, fromChatID int64, messageID int) (*tg.Message, error)
	CopyMessage(ctx context.Context, chatID, fromChatID int64, messageID int) (*tg.MessageID, error)
	SendChatAction(ctx context.Context, chatID int64, action string) error

	// Media methods (Phase B)
	SendPhoto(ctx context.Context, chatID int64, photo MediaInput, caption string) (*tg.Message, error)
	SendDocument(ctx context.Context, chatID int64, document MediaInput, caption string) (*tg.Message, error)
	SendAnimation(ctx context.Context, chatID int64, animation MediaInput, caption string) (*tg.Message, error)
	SendVideo(ctx context.Context, chatID int64, video MediaInput, caption string) (*tg.Message, error)
	SendAudio(ctx context.Context, chatID int64, audio MediaInput, caption string) (*tg.Message, error)
	SendVoice(ctx context.Context, chatID int64, voice MediaInput, caption string) (*tg.Message, error)
	SendSticker(ctx context.Context, chatID int64, sticker MediaInput) (*tg.Message, error)
	SendVideoNote(ctx context.Context, chatID int64, videoNote MediaInput) (*tg.Message, error)
	SendMediaGroup(ctx context.Context, chatID int64, media []MediaInput) ([]*tg.Message, error)
	GetFile(ctx context.Context, fileID string) (*tg.File, error)
	EditMessageCaption(ctx context.Context, chatID int64, messageID int, caption string) (*tg.Message, error)
	EditMessageReplyMarkup(ctx context.Context, chatID int64, messageID int, markup *tg.InlineKeyboardMarkup) (*tg.Message, error)
	EditMessageMedia(ctx context.Context, chatID int64, messageID int, media sender.InputMedia) (*tg.Message, error)

	// Callback query methods (interactive scenarios)
	AnswerCallbackQuery(ctx context.Context, callbackQueryID string, text string, showAlert bool) error

	// Tier 2: Chat info
	GetChat(ctx context.Context, chatID int64) (*tg.ChatFullInfo, error)
	GetChatAdministrators(ctx context.Context, chatID int64) ([]tg.ChatMember, error)
	GetChatMemberCount(ctx context.Context, chatID int64) (int, error)
	GetChatMember(ctx context.Context, chatID int64, userID int64) (tg.ChatMember, error)

	// Tier 2: Chat settings
	SetChatTitle(ctx context.Context, chatID int64, title string) error
	SetChatDescription(ctx context.Context, chatID int64, description string) error

	// Tier 2: Pin messages
	PinChatMessage(ctx context.Context, chatID int64, messageID int, silent bool) error
	UnpinChatMessage(ctx context.Context, chatID int64, messageID int) error
	UnpinAllChatMessages(ctx context.Context, chatID int64) error

	// Tier 2: Polls
	SendPollSimple(ctx context.Context, chatID int64, question string, options []string) (*tg.Message, error)
	SendQuiz(ctx context.Context, chatID int64, question string, options []string, correctOptionID int) (*tg.Message, error)
	StopPoll(ctx context.Context, chatID int64, messageID int) (*tg.Poll, error)

	// Tier 2: Forum
	GetForumTopicIconStickers(ctx context.Context) ([]*tg.Sticker, error)

	// Extended: Stickers
	GetStickerSet(ctx context.Context, name string) (*tg.StickerSet, error)
	UploadStickerFile(ctx context.Context, userID int64, sticker MediaInput, stickerFormat string) (*tg.File, error)
	CreateNewStickerSet(ctx context.Context, userID int64, name, title string, stickers []StickerInput) error
	AddStickerToSet(ctx context.Context, userID int64, name string, sticker StickerInput) error
	SetStickerPositionInSet(ctx context.Context, sticker string, position int) error
	DeleteStickerFromSet(ctx context.Context, sticker string) error
	SetStickerSetTitle(ctx context.Context, name, title string) error
	DeleteStickerSet(ctx context.Context, name string) error
	SetStickerEmojiList(ctx context.Context, sticker string, emojiList []string) error
	ReplaceStickerInSet(ctx context.Context, userID int64, name, oldSticker string, sticker StickerInput) error

	// Extended: Stars & Payments
	GetMyStarBalance(ctx context.Context) (*tg.StarAmount, error)
	GetStarTransactions(ctx context.Context, limit int) (*tg.StarTransactions, error)
	SendInvoice(ctx context.Context, chatID int64, title, description, payload, currency string, prices []tg.LabeledPrice) (*tg.Message, error)

	// Extended: Gifts
	GetAvailableGifts(ctx context.Context) (*tg.Gifts, error)

	// Extended: Checklists
	SendChecklist(ctx context.Context, chatID int64, title string, tasks []string) (*tg.Message, error)
	EditMessageChecklist(ctx context.Context, chatID int64, messageID int, title string, tasks []ChecklistTaskInput) (*tg.Message, error)

	// Geo & Contact
	SendLocation(ctx context.Context, chatID int64, lat, lon float64) (*tg.Message, error)
	SendVenue(ctx context.Context, chatID int64, lat, lon float64, title, address string) (*tg.Message, error)
	SendContact(ctx context.Context, chatID int64, phone, firstName, lastName string) (*tg.Message, error)
	SendDice(ctx context.Context, chatID int64, emoji string) (*tg.Message, error)

	// Reactions & User info
	SetMessageReaction(ctx context.Context, chatID int64, messageID int, emoji string, isBig bool) error
	GetUserProfilePhotos(ctx context.Context, userID int64) (*tg.UserProfilePhotos, error)
	GetUserChatBoosts(ctx context.Context, chatID, userID int64) (*tg.UserChatBoosts, error)

	// Bulk operations
	ForwardMessages(ctx context.Context, chatID, fromChatID int64, messageIDs []int) ([]tg.MessageID, error)
	CopyMessages(ctx context.Context, chatID, fromChatID int64, messageIDs []int) ([]tg.MessageID, error)
	DeleteMessages(ctx context.Context, chatID int64, messageIDs []int) error

	// Chat settings
	SetChatPhoto(ctx context.Context, chatID int64, photo sender.InputFile) error
	DeleteChatPhoto(ctx context.Context, chatID int64) error
	SetChatPermissions(ctx context.Context, chatID int64, perms tg.ChatPermissions) error

	// Bot Identity
	SetMyCommands(ctx context.Context, commands []tg.BotCommand) error
	GetMyCommands(ctx context.Context) ([]tg.BotCommand, error)
	DeleteMyCommands(ctx context.Context) error
	SetMyName(ctx context.Context, name string) error
	GetMyName(ctx context.Context) (*tg.BotName, error)
	SetMyDescription(ctx context.Context, description string) error
	GetMyDescription(ctx context.Context) (*tg.BotDescription, error)
	SetMyShortDescription(ctx context.Context, shortDescription string) error
	GetMyShortDescription(ctx context.Context) (*tg.BotShortDescription, error)
	SetMyDefaultAdministratorRights(ctx context.Context, rights *tg.ChatAdministratorRights, forChannels bool) error
	GetMyDefaultAdministratorRights(ctx context.Context, forChannels bool) (*tg.ChatAdministratorRights, error)

	// Webhook management methods
	SetWebhook(ctx context.Context, url string) error
	DeleteWebhook(ctx context.Context) error
	GetWebhookInfo(ctx context.Context) (*WebhookInfo, error)

	// Polling
	GetUpdates(ctx context.Context, offset int64, limit int, timeout int) ([]tg.Update, error)
}

// WebhookInfo contains information about the current webhook configuration.
type WebhookInfo struct {
	URL                string
	PendingUpdateCount int
	HasCustomCert      bool
}

// MediaInput represents a file input for media uploads.
// Use one of: FromReader, FromFileID, FromURL.
type MediaInput struct {
	Reader   func() []byte // Factory to get fresh bytes (can be called multiple times)
	FileName string
	FileID   string
	URL      string
	Type     string // "photo", "video", "document", etc.
}

// SendOption configures message sending.
type SendOption func(*SendOptions)

// SendOptions holds optional parameters for sending.
type SendOptions struct {
	ReplyMarkup *tg.InlineKeyboardMarkup
	ParseMode   string
}

// WithReplyMarkup sets the reply markup.
func WithReplyMarkup(markup *tg.InlineKeyboardMarkup) SendOption {
	return func(o *SendOptions) {
		o.ReplyMarkup = markup
	}
}

// WithParseMode sets the parse mode.
func WithParseMode(mode string) SendOption {
	return func(o *SendOptions) {
		o.ParseMode = mode
	}
}

// MediaFromBytes creates a MediaInput from bytes.
func MediaFromBytes(data []byte, filename, mediaType string) MediaInput {
	return MediaInput{
		Reader: func() []byte {
			cp := make([]byte, len(data))
			copy(cp, data)
			return cp
		},
		FileName: filename,
		Type:     mediaType,
	}
}

// MediaFromFileID creates a MediaInput from a file ID.
func MediaFromFileID(fileID string) MediaInput {
	return MediaInput{FileID: fileID}
}

// MediaFromURL creates a MediaInput from a URL.
func MediaFromURL(url string) MediaInput {
	return MediaInput{URL: url}
}

// StickerInput represents a sticker for creating/adding to sticker sets.
type StickerInput struct {
	Sticker   MediaInput
	Format    string   // "static", "animated", "video"
	EmojiList []string // e.g., ["😀"]
}

// ChecklistTaskInput represents a task in a checklist edit.
type ChecklistTaskInput struct {
	ID   int
	Text string
	Done bool
}
