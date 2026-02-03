package tg

import (
	"encoding/json"
	"fmt"
)

// Update represents an incoming update from Telegram.
type Update struct {
	UpdateID           int                 `json:"update_id"`
	Message            *Message            `json:"message,omitempty"`
	EditedMessage      *Message            `json:"edited_message,omitempty"`
	ChannelPost        *Message            `json:"channel_post,omitempty"`
	EditedChannelPost  *Message            `json:"edited_channel_post,omitempty"`
	CallbackQuery      *CallbackQuery      `json:"callback_query,omitempty"`
	InlineQuery        *InlineQuery        `json:"inline_query,omitempty"`
	ChosenInlineResult *ChosenInlineResult `json:"chosen_inline_result,omitempty"`
	ShippingQuery      *ShippingQuery      `json:"shipping_query,omitempty"`
	PreCheckoutQuery   *PreCheckoutQuery   `json:"pre_checkout_query,omitempty"`
	Poll               *Poll               `json:"poll,omitempty"`
	PollAnswer         *PollAnswer         `json:"poll_answer,omitempty"`
	MyChatMember       *ChatMemberUpdated  `json:"my_chat_member,omitempty"`
	ChatMember         *ChatMemberUpdated  `json:"chat_member,omitempty"`
	ChatJoinRequest    *ChatJoinRequest    `json:"chat_join_request,omitempty"`
}

// CallbackQuery represents an incoming callback query from an inline keyboard.
type CallbackQuery struct {
	ID              string   `json:"id"`
	From            *User    `json:"from"`
	Message         *Message `json:"message,omitempty"`
	InlineMessageID string   `json:"inline_message_id,omitempty"`
	ChatInstance    string   `json:"chat_instance"`
	Data            string   `json:"data,omitempty"`
	GameShortName   string   `json:"game_short_name,omitempty"`
}

// MessageSig implements Editable.
func (c *CallbackQuery) MessageSig() (string, int64) {
	if c == nil {
		return "", 0
	}
	if c.InlineMessageID != "" {
		return c.InlineMessageID, 0
	}
	if c.Message != nil {
		return c.Message.MessageSig()
	}
	return "", 0
}

var _ Editable = (*CallbackQuery)(nil)

// InlineQuery represents an incoming inline query.
type InlineQuery struct {
	ID       string    `json:"id"`
	From     *User     `json:"from"`
	Query    string    `json:"query"`
	Offset   string    `json:"offset"`
	ChatType string    `json:"chat_type,omitempty"`
	Location *Location `json:"location,omitempty"`
}

// ChosenInlineResult represents a result chosen by a user.
type ChosenInlineResult struct {
	ResultID        string    `json:"result_id"`
	From            *User     `json:"from"`
	Location        *Location `json:"location,omitempty"`
	InlineMessageID string    `json:"inline_message_id,omitempty"`
	Query           string    `json:"query"`
}

// ShippingQuery represents an incoming shipping query.
type ShippingQuery struct {
	ID              string          `json:"id"`
	From            *User           `json:"from"`
	InvoicePayload  string          `json:"invoice_payload"`
	ShippingAddress ShippingAddress `json:"shipping_address"`
}

// ShippingAddress represents a shipping address.
type ShippingAddress struct {
	CountryCode string `json:"country_code"`
	State       string `json:"state"`
	City        string `json:"city"`
	StreetLine1 string `json:"street_line1"`
	StreetLine2 string `json:"street_line2"`
	PostCode    string `json:"post_code"`
}

// PreCheckoutQuery represents an incoming pre-checkout query.
type PreCheckoutQuery struct {
	ID               string     `json:"id"`
	From             *User      `json:"from"`
	Currency         string     `json:"currency"`
	TotalAmount      int        `json:"total_amount"`
	InvoicePayload   string     `json:"invoice_payload"`
	ShippingOptionID string     `json:"shipping_option_id,omitempty"`
	OrderInfo        *OrderInfo `json:"order_info,omitempty"`
}

// OrderInfo represents information about an order.
type OrderInfo struct {
	Name            string          `json:"name,omitempty"`
	PhoneNumber     string          `json:"phone_number,omitempty"`
	Email           string          `json:"email,omitempty"`
	ShippingAddress ShippingAddress `json:"shipping_address,omitempty"`
}

// PollAnswer represents an answer of a user in a non-anonymous poll.
type PollAnswer struct {
	PollID    string `json:"poll_id"`
	VoterChat *Chat  `json:"voter_chat,omitempty"`
	User      *User  `json:"user,omitempty"`
	OptionIDs []int  `json:"option_ids"`
}

// ChatMemberUpdated represents changes in the status of a chat member.
type ChatMemberUpdated struct {
	Chat                    *Chat           `json:"chat"`
	From                    *User           `json:"from"`
	Date                    int64           `json:"date"`
	OldChatMember           ChatMember      `json:"-"`
	NewChatMember           ChatMember      `json:"-"`
	InviteLink              *ChatInviteLink `json:"invite_link,omitempty"`
	ViaChatFolderInviteLink bool            `json:"via_chat_folder_invite_link,omitempty"`
}

// UnmarshalJSON implements custom unmarshaling for ChatMemberUpdated.
// The OldChatMember and NewChatMember fields are ChatMember interfaces
// that require discriminated union parsing.
func (u *ChatMemberUpdated) UnmarshalJSON(data []byte) error {
	// Alias to avoid infinite recursion
	type Alias ChatMemberUpdated
	var raw struct {
		Alias
		OldChatMember json.RawMessage `json:"old_chat_member"`
		NewChatMember json.RawMessage `json:"new_chat_member"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*u = ChatMemberUpdated(raw.Alias)

	if len(raw.OldChatMember) > 0 {
		m, err := UnmarshalChatMember(raw.OldChatMember)
		if err != nil {
			return fmt.Errorf("old_chat_member: %w", err)
		}
		u.OldChatMember = m
	}
	if len(raw.NewChatMember) > 0 {
		m, err := UnmarshalChatMember(raw.NewChatMember)
		if err != nil {
			return fmt.Errorf("new_chat_member: %w", err)
		}
		u.NewChatMember = m
	}
	return nil
}

// ChatInviteLink represents an invite link for a chat.
type ChatInviteLink struct {
	InviteLink              string `json:"invite_link"`
	Creator                 *User  `json:"creator"`
	CreatesJoinRequest      bool   `json:"creates_join_request"`
	IsPrimary               bool   `json:"is_primary"`
	IsRevoked               bool   `json:"is_revoked"`
	Name                    string `json:"name,omitempty"`
	ExpireDate              int64  `json:"expire_date,omitempty"`
	MemberLimit             int    `json:"member_limit,omitempty"`
	PendingJoinRequestCount int    `json:"pending_join_request_count,omitempty"`
}

// ChatJoinRequest represents a join request sent to a chat.
type ChatJoinRequest struct {
	Chat       *Chat           `json:"chat"`
	From       *User           `json:"from"`
	UserChatID int64           `json:"user_chat_id"`
	Date       int64           `json:"date"`
	Bio        string          `json:"bio,omitempty"`
	InviteLink *ChatInviteLink `json:"invite_link,omitempty"`
}
