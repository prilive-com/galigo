package tg

import "strconv"

// ChatID represents a Telegram chat identifier.
// Valid types: int64 (numeric ID) or string (channel username like "@channelusername")
type ChatID = any

// Editable represents anything that can be edited (message, callback, stored reference).
// Implement this interface to edit messages stored in your database.
type Editable interface {
	// MessageSig returns message identifier and chat ID.
	// For inline messages: return (inline_message_id, 0)
	// For regular messages: return (message_id as string, chat_id)
	MessageSig() (messageID string, chatID int64)
}

// Message represents a Telegram message.
type Message struct {
	MessageID             int                   `json:"message_id"`
	MessageThreadID       int                   `json:"message_thread_id,omitempty"`
	From                  *User                 `json:"from,omitempty"`
	SenderChat            *Chat                 `json:"sender_chat,omitempty"`
	Date                  int64                 `json:"date"`
	Chat                  *Chat                 `json:"chat"`
	ForwardFrom           *User                 `json:"forward_from,omitempty"`
	ForwardFromChat       *Chat                 `json:"forward_from_chat,omitempty"`
	ForwardDate           int64                 `json:"forward_date,omitempty"`
	IsTopicMessage        bool                  `json:"is_topic_message,omitempty"`
	IsAutomaticForward    bool                  `json:"is_automatic_forward,omitempty"`
	ReplyToMessage        *Message              `json:"reply_to_message,omitempty"`
	ViaBot                *User                 `json:"via_bot,omitempty"`
	EditDate              int64                 `json:"edit_date,omitempty"`
	HasProtectedContent   bool                  `json:"has_protected_content,omitempty"`
	MediaGroupID          string                `json:"media_group_id,omitempty"`
	AuthorSignature       string                `json:"author_signature,omitempty"`
	Text                  string                `json:"text,omitempty"`
	Entities              []MessageEntity       `json:"entities,omitempty"`
	Caption               string                `json:"caption,omitempty"`
	CaptionEntities       []MessageEntity       `json:"caption_entities,omitempty"`
	Photo                 []PhotoSize           `json:"photo,omitempty"`
	Document              *Document             `json:"document,omitempty"`
	Video                 *Video                `json:"video,omitempty"`
	Audio                 *Audio                `json:"audio,omitempty"`
	Voice                 *Voice                `json:"voice,omitempty"`
	VideoNote             *VideoNote            `json:"video_note,omitempty"`
	Contact               *Contact              `json:"contact,omitempty"`
	Location              *Location             `json:"location,omitempty"`
	Venue                 *Venue                `json:"venue,omitempty"`
	Poll                  *Poll                 `json:"poll,omitempty"`
	NewChatMembers        []User                `json:"new_chat_members,omitempty"`
	LeftChatMember        *User                 `json:"left_chat_member,omitempty"`
	NewChatTitle          string                `json:"new_chat_title,omitempty"`
	NewChatPhoto          []PhotoSize           `json:"new_chat_photo,omitempty"`
	DeleteChatPhoto       bool                  `json:"delete_chat_photo,omitempty"`
	GroupChatCreated      bool                  `json:"group_chat_created,omitempty"`
	SupergroupChatCreated bool                  `json:"supergroup_chat_created,omitempty"`
	ChannelChatCreated    bool                  `json:"channel_chat_created,omitempty"`
	ReplyMarkup           *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

// MessageSig implements Editable.
func (m *Message) MessageSig() (string, int64) {
	if m == nil {
		return "", 0
	}
	var chatID int64
	if m.Chat != nil {
		chatID = m.Chat.ID
	}
	return strconv.Itoa(m.MessageID), chatID
}

var _ Editable = (*Message)(nil)

// User represents a Telegram user or bot.
type User struct {
	ID                      int64  `json:"id"`
	IsBot                   bool   `json:"is_bot"`
	FirstName               string `json:"first_name"`
	LastName                string `json:"last_name,omitempty"`
	Username                string `json:"username,omitempty"`
	LanguageCode            string `json:"language_code,omitempty"`
	IsPremium               bool   `json:"is_premium,omitempty"`
	AddedToAttachmentMenu   bool   `json:"added_to_attachment_menu,omitempty"`
	CanJoinGroups           bool   `json:"can_join_groups,omitempty"`
	CanReadAllGroupMessages bool   `json:"can_read_all_group_messages,omitempty"`
	SupportsInlineQueries   bool   `json:"supports_inline_queries,omitempty"`
}

// Chat represents a Telegram chat.
type Chat struct {
	ID                                 int64      `json:"id"`
	Type                               string     `json:"type"`
	Title                              string     `json:"title,omitempty"`
	Username                           string     `json:"username,omitempty"`
	FirstName                          string     `json:"first_name,omitempty"`
	LastName                           string     `json:"last_name,omitempty"`
	IsForum                            bool       `json:"is_forum,omitempty"`
	Photo                              *ChatPhoto `json:"photo,omitempty"`
	ActiveUsernames                    []string   `json:"active_usernames,omitempty"`
	Bio                                string     `json:"bio,omitempty"`
	HasPrivateForwards                 bool       `json:"has_private_forwards,omitempty"`
	HasRestrictedVoiceAndVideoMessages bool       `json:"has_restricted_voice_and_video_messages,omitempty"`
	JoinToSendMessages                 bool       `json:"join_to_send_messages,omitempty"`
	JoinByRequest                      bool       `json:"join_by_request,omitempty"`
	Description                        string     `json:"description,omitempty"`
	InviteLink                         string     `json:"invite_link,omitempty"`
	PinnedMessage                      *Message   `json:"pinned_message,omitempty"`
	SlowModeDelay                      int        `json:"slow_mode_delay,omitempty"`
	MessageAutoDeleteTime              int        `json:"message_auto_delete_time,omitempty"`
	HasProtectedContent                bool       `json:"has_protected_content,omitempty"`
	StickerSetName                     string     `json:"sticker_set_name,omitempty"`
	CanSetStickerSet                   bool       `json:"can_set_sticker_set,omitempty"`
	LinkedChatID                       int64      `json:"linked_chat_id,omitempty"`
}

// ChatPhoto represents a chat photo.
type ChatPhoto struct {
	SmallFileID       string `json:"small_file_id"`
	SmallFileUniqueID string `json:"small_file_unique_id"`
	BigFileID         string `json:"big_file_id"`
	BigFileUniqueID   string `json:"big_file_unique_id"`
}

// MessageEntity represents a special entity in a text message.
type MessageEntity struct {
	Type          string `json:"type"`
	Offset        int    `json:"offset"`
	Length        int    `json:"length"`
	URL           string `json:"url,omitempty"`
	User          *User  `json:"user,omitempty"`
	Language      string `json:"language,omitempty"`
	CustomEmojiID string `json:"custom_emoji_id,omitempty"`
}

// PhotoSize represents one size of a photo or thumbnail.
type PhotoSize struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int64  `json:"file_size,omitempty"`
}

// Document represents a general file.
type Document struct {
	FileID       string     `json:"file_id"`
	FileUniqueID string     `json:"file_unique_id"`
	Thumbnail    *PhotoSize `json:"thumbnail,omitempty"`
	FileName     string     `json:"file_name,omitempty"`
	MimeType     string     `json:"mime_type,omitempty"`
	FileSize     int64      `json:"file_size,omitempty"`
}

// Video represents a video file.
type Video struct {
	FileID       string     `json:"file_id"`
	FileUniqueID string     `json:"file_unique_id"`
	Width        int        `json:"width"`
	Height       int        `json:"height"`
	Duration     int        `json:"duration"`
	Thumbnail    *PhotoSize `json:"thumbnail,omitempty"`
	FileName     string     `json:"file_name,omitempty"`
	MimeType     string     `json:"mime_type,omitempty"`
	FileSize     int64      `json:"file_size,omitempty"`
}

// Audio represents an audio file.
type Audio struct {
	FileID       string     `json:"file_id"`
	FileUniqueID string     `json:"file_unique_id"`
	Duration     int        `json:"duration"`
	Performer    string     `json:"performer,omitempty"`
	Title        string     `json:"title,omitempty"`
	FileName     string     `json:"file_name,omitempty"`
	MimeType     string     `json:"mime_type,omitempty"`
	FileSize     int64      `json:"file_size,omitempty"`
	Thumbnail    *PhotoSize `json:"thumbnail,omitempty"`
}

// Voice represents a voice note.
type Voice struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
}

// VideoNote represents a video message.
type VideoNote struct {
	FileID       string     `json:"file_id"`
	FileUniqueID string     `json:"file_unique_id"`
	Length       int        `json:"length"`
	Duration     int        `json:"duration"`
	Thumbnail    *PhotoSize `json:"thumbnail,omitempty"`
	FileSize     int64      `json:"file_size,omitempty"`
}

// Contact represents a phone contact.
type Contact struct {
	PhoneNumber string `json:"phone_number"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name,omitempty"`
	UserID      int64  `json:"user_id,omitempty"`
	Vcard       string `json:"vcard,omitempty"`
}

// Location represents a point on the map.
type Location struct {
	Longitude            float64 `json:"longitude"`
	Latitude             float64 `json:"latitude"`
	HorizontalAccuracy   float64 `json:"horizontal_accuracy,omitempty"`
	LivePeriod           int     `json:"live_period,omitempty"`
	Heading              int     `json:"heading,omitempty"`
	ProximityAlertRadius int     `json:"proximity_alert_radius,omitempty"`
}

// Venue represents a venue.
type Venue struct {
	Location        Location `json:"location"`
	Title           string   `json:"title"`
	Address         string   `json:"address"`
	FoursquareID    string   `json:"foursquare_id,omitempty"`
	FoursquareType  string   `json:"foursquare_type,omitempty"`
	GooglePlaceID   string   `json:"google_place_id,omitempty"`
	GooglePlaceType string   `json:"google_place_type,omitempty"`
}

// Poll represents a poll.
type Poll struct {
	ID                    string          `json:"id"`
	Question              string          `json:"question"`
	Options               []PollOption    `json:"options"`
	TotalVoterCount       int             `json:"total_voter_count"`
	IsClosed              bool            `json:"is_closed"`
	IsAnonymous           bool            `json:"is_anonymous"`
	Type                  string          `json:"type"`
	AllowsMultipleAnswers bool            `json:"allows_multiple_answers"`
	CorrectOptionID       int             `json:"correct_option_id,omitempty"`
	Explanation           string          `json:"explanation,omitempty"`
	ExplanationEntities   []MessageEntity `json:"explanation_entities,omitempty"`
	OpenPeriod            int             `json:"open_period,omitempty"`
	CloseDate             int64           `json:"close_date,omitempty"`
}

// PollOption contains information about one answer option in a poll.
type PollOption struct {
	Text       string `json:"text"`
	VoterCount int    `json:"voter_count"`
}

// MessageID represents a message identifier (returned by copyMessage).
type MessageID struct {
	MessageID int `json:"message_id"`
}

// StoredMessage is a helper for implementing Editable with database-stored messages.
type StoredMessage struct {
	MsgID  int   `json:"message_id"`
	ChatID int64 `json:"chat_id"`
}

// MessageSig implements Editable.
func (m StoredMessage) MessageSig() (string, int64) {
	return strconv.Itoa(m.MsgID), m.ChatID
}

var _ Editable = StoredMessage{}

// InlineMessage represents an inline message reference.
type InlineMessage struct {
	InlineMessageID string `json:"inline_message_id"`
}

// MessageSig implements Editable for inline messages.
func (m InlineMessage) MessageSig() (string, int64) {
	return m.InlineMessageID, 0
}

var _ Editable = InlineMessage{}
