package engine

import (
	"context"
	"time"

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
	Error        string        `json:"error,omitempty"`
	Steps        []StepResult  `json:"steps"`
}

// CreatedMessage tracks messages for cleanup.
type CreatedMessage struct {
	ChatID    int64
	MessageID int
}

// Runtime provides context for step execution.
type Runtime struct {
	Sender SenderClient
	ChatID int64

	// State shared between steps
	CreatedMessages []CreatedMessage
	LastMessage     *tg.Message
	LastMessageID   *tg.MessageID
	CapturedFileIDs map[string]string // name -> file_id for reuse
}

// NewRuntime creates a new runtime for scenario execution.
func NewRuntime(sender SenderClient, chatID int64) *Runtime {
	return &Runtime{
		Sender:          sender,
		ChatID:          chatID,
		CreatedMessages: make([]CreatedMessage, 0),
		CapturedFileIDs: make(map[string]string),
	}
}

// TrackMessage adds a message to the cleanup list.
func (rt *Runtime) TrackMessage(chatID int64, messageID int) {
	rt.CreatedMessages = append(rt.CreatedMessages, CreatedMessage{
		ChatID:    chatID,
		MessageID: messageID,
	})
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
	SendMediaGroup(ctx context.Context, chatID int64, media []MediaInput) ([]*tg.Message, error)
	GetFile(ctx context.Context, fileID string) (*tg.File, error)
	EditMessageCaption(ctx context.Context, chatID int64, messageID int, caption string) (*tg.Message, error)
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
