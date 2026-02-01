# Tier 3 Implementation Plan — Final Version 7.0

**Version:** 7.0 (All Reviews Incorporated — Production Ready)  
**Date:** January 2026  
**Status:** Ready for Implementation  
**Based on:** Developer critique + Consultant 1 + Consultant 2 + Final developer review

---

## Executive Summary

This plan covers **~61 methods** organized into 8 implementation epics, preceded by a **blocking Types PR**. All previous review issues have been addressed, including the final 6 minor issues from the last review.

### Final Corrections Applied (v7.0)

| Issue | Resolution |
|-------|------------|
| `unmarshalTransactionPartner` swallows errors | ✅ Return `Unknown{Raw: data}` on unmarshal failure |
| `RevenueWithdrawalState` no Unknown | ✅ Added `RevenueWithdrawalStateUnknown` + unmarshal function |
| `*RevenueWithdrawalState` interface pointer | ✅ Fixed to `RevenueWithdrawalState` (interface already pointer) |
| `*int` usage inconsistent | ✅ Audited: `Offset` uses plain `int` with `omitempty` (default 0) |
| Rate limiter on time-critical ops | ✅ Skip `waitForRateLimiter` for `AnswerPreCheckoutQuery` |
| Import path mismatch | ✅ Fixed to `github.com/prilive-com/galigo/tg` |
| `PassportElementError` send-only | ✅ Documented: no unmarshal needed |

---

## Guiding Principles (Non-Negotiable)

1. **Use actual codebase patterns**: `callJSON`, `executeRequest`, `withRetry`
2. **Typed request structs**: No `map[string]any` in exported APIs
3. **Use existing error types**: `tg.NewValidationError(field, message)`
4. **Use `nil` for void methods**: Not `var result bool`
5. **No retry for value operations**: Payments, gifts, refunds use `callJSON` directly
6. **Upload types in `sender/`**: Avoid import cycles with `tg/`
7. **Extend existing types**: Don't duplicate (e.g., Sticker in `tg/types.go`)
8. **`*int` for truly optional fields**: Plain `int` with `omitempty` when default 0 is meaningful
9. **Unknown fallbacks**: All polymorphic types have `Unknown` variant
10. **Error handling in unmarshal**: Return `Unknown{Raw: data}` on parse failure
11. **Skip rate limiter for time-critical ops**: `AnswerPreCheckoutQuery` (10s deadline)

---

## PR0: Missing Types (BLOCKING)

**This PR must be merged before any epic can begin.**

### File Structure

```
tg/
├── types.go          # Extend: Sticker, add ReplyParameters, LinkPreviewOptions
├── payments.go       # NEW: LabeledPrice, StarTransaction, TransactionPartner union
├── checklists.go     # NEW: InputChecklist, ChecklistTask
├── inline.go         # NEW: InlineQueryResult partial union
├── games.go          # NEW: Game, GameHighScore, CallbackGame
├── gifts.go          # NEW: Gift, OwnedGift (full fields)
├── business.go       # NEW: BusinessConnection, Story, OwnedGifts
├── boosts.go         # NEW: ChatBoost, ChatBoostSource union
├── passport.go       # NEW: PassportElementError (send-only, no unmarshal)
├── stickers.go       # NEW: StickerSet, MaskPosition (response types only)

sender/
├── stickers_input.go # NEW: InputSticker (contains InputFile)
├── business_input.go # NEW: InputStoryContent, InputProfilePhoto
├── rate_limiter.go   # NEW: Optional proactive rate limiting
```

---

### tg/types.go — Extensions

```go
// tg/types.go — ADD these types (verify InlineKeyboardMarkup, MessageEntity exist)

// ReplyParameters describes reply behavior
// Added in Bot API 7.0, extended in 9.2 for checklists
type ReplyParameters struct {
    MessageID                int             `json:"message_id"`
    ChatID                   any             `json:"chat_id,omitempty"`
    AllowSendingWithoutReply bool            `json:"allow_sending_without_reply,omitempty"`
    Quote                    string          `json:"quote,omitempty"`
    QuoteParseMode           string          `json:"quote_parse_mode,omitempty"`
    QuoteEntities            []MessageEntity `json:"quote_entities,omitempty"`
    QuotePosition            int             `json:"quote_position,omitempty"` // 0 is valid, omitempty OK
}

// LinkPreviewOptions describes link preview generation options
type LinkPreviewOptions struct {
    IsDisabled       bool   `json:"is_disabled,omitempty"`
    URL              string `json:"url,omitempty"`
    PreferSmallMedia bool   `json:"prefer_small_media,omitempty"`
    PreferLargeMedia bool   `json:"prefer_large_media,omitempty"`
    ShowAboveText    bool   `json:"show_above_text,omitempty"`
}

// WebAppInfo describes a Web App
type WebAppInfo struct {
    URL string `json:"url"`
}

// LoginUrl represents a parameter of the inline keyboard button
type LoginUrl struct {
    URL                string `json:"url"`
    ForwardText        string `json:"forward_text,omitempty"`
    BotUsername        string `json:"bot_username,omitempty"`
    RequestWriteAccess bool   `json:"request_write_access,omitempty"`
}

// SwitchInlineQueryChosenChat allows switching to inline mode in a chosen chat
type SwitchInlineQueryChosenChat struct {
    Query             string `json:"query,omitempty"`
    AllowUserChats    bool   `json:"allow_user_chats,omitempty"`
    AllowBotChats     bool   `json:"allow_bot_chats,omitempty"`
    AllowGroupChats   bool   `json:"allow_group_chats,omitempty"`
    AllowChannelChats bool   `json:"allow_channel_chats,omitempty"`
}
```

---

### tg/payments.go — NEW FILE (CORRECTED)

```go
// tg/payments.go — Payment and Stars types

package tg

import "encoding/json"

// LabeledPrice represents a portion of the price
type LabeledPrice struct {
    Label  string `json:"label"`
    Amount int    `json:"amount"` // Smallest currency unit (cents, etc.)
}

// ShippingOption represents one shipping option
type ShippingOption struct {
    ID     string         `json:"id"`
    Title  string         `json:"title"`
    Prices []LabeledPrice `json:"prices"`
}

// ShippingAddress represents a shipping address
type ShippingAddress struct {
    CountryCode string `json:"country_code"`
    State       string `json:"state"`
    City        string `json:"city"`
    StreetLine1 string `json:"street_line1"`
    StreetLine2 string `json:"street_line2"`
    PostCode    string `json:"post_code"`
}

// OrderInfo represents information about an order
type OrderInfo struct {
    Name            string           `json:"name,omitempty"`
    PhoneNumber     string           `json:"phone_number,omitempty"`
    Email           string           `json:"email,omitempty"`
    ShippingAddress *ShippingAddress `json:"shipping_address,omitempty"`
}

// SuccessfulPayment contains information about a successful payment
type SuccessfulPayment struct {
    Currency                   string     `json:"currency"`
    TotalAmount                int        `json:"total_amount"`
    InvoicePayload             string     `json:"invoice_payload"`
    ShippingOptionID           string     `json:"shipping_option_id,omitempty"`
    OrderInfo                  *OrderInfo `json:"order_info,omitempty"`
    TelegramPaymentChargeID    string     `json:"telegram_payment_charge_id"`
    ProviderPaymentChargeID    string     `json:"provider_payment_charge_id"`
    SubscriptionExpirationDate int64      `json:"subscription_expiration_date,omitempty"`
    IsRecurring                bool       `json:"is_recurring,omitempty"`
    IsFirstRecurring           bool       `json:"is_first_recurring,omitempty"`
}

// StarAmount represents an amount of Telegram Stars
type StarAmount struct {
    Amount         int `json:"amount"`
    NanostarAmount int `json:"nanostar_amount,omitempty"`
}

// StarTransactions contains a list of Star transactions
type StarTransactions struct {
    Transactions []StarTransaction `json:"transactions"`
}

// StarTransaction describes a Telegram Star transaction
type StarTransaction struct {
    ID             string             `json:"id"`
    Amount         int                `json:"amount"`
    NanostarAmount int                `json:"nanostar_amount,omitempty"`
    Date           int64              `json:"date"`
    Source         TransactionPartner `json:"source,omitempty"`
    Receiver       TransactionPartner `json:"receiver,omitempty"`
}

// Custom UnmarshalJSON for StarTransaction to handle polymorphic Source/Receiver
func (s *StarTransaction) UnmarshalJSON(data []byte) error {
    type Alias StarTransaction
    aux := &struct {
        Source   json.RawMessage `json:"source,omitempty"`
        Receiver json.RawMessage `json:"receiver,omitempty"`
        *Alias
    }{Alias: (*Alias)(s)}

    if err := json.Unmarshal(data, aux); err != nil {
        return err
    }

    if len(aux.Source) > 0 && string(aux.Source) != "null" {
        s.Source = unmarshalTransactionPartner(aux.Source)
    }
    if len(aux.Receiver) > 0 && string(aux.Receiver) != "null" {
        s.Receiver = unmarshalTransactionPartner(aux.Receiver)
    }
    return nil
}

// --- TransactionPartner Union ---

// TransactionPartner describes the source or receiver of a Star transaction
type TransactionPartner interface {
    transactionPartnerTag()
}

// TransactionPartnerUser represents a transaction with a user
type TransactionPartnerUser struct {
    Type               string `json:"type"` // Always "user"
    User               User   `json:"user"`
    InvoicePayload     string `json:"invoice_payload,omitempty"`
    PaidMediaPayload   string `json:"paid_media_payload,omitempty"`
    SubscriptionPeriod int    `json:"subscription_period,omitempty"`
    Gift               *Gift  `json:"gift,omitempty"`
}

func (TransactionPartnerUser) transactionPartnerTag() {}

// TransactionPartnerFragment represents a withdrawal to Fragment
type TransactionPartnerFragment struct {
    Type            string                 `json:"type"` // Always "fragment"
    WithdrawalState RevenueWithdrawalState `json:"withdrawal_state,omitempty"` // Interface, not *interface
}

func (TransactionPartnerFragment) transactionPartnerTag() {}

// TransactionPartnerTelegramAds represents a transfer to Telegram Ads
type TransactionPartnerTelegramAds struct {
    Type string `json:"type"` // Always "telegram_ads"
}

func (TransactionPartnerTelegramAds) transactionPartnerTag() {}

// TransactionPartnerTelegramApi represents payment from Telegram API usage
type TransactionPartnerTelegramApi struct {
    Type         string `json:"type"` // Always "telegram_api"
    RequestCount int    `json:"request_count"`
}

func (TransactionPartnerTelegramApi) transactionPartnerTag() {}

// TransactionPartnerOther represents an unknown transaction partner type
type TransactionPartnerOther struct {
    Type string `json:"type"` // Always "other"
}

func (TransactionPartnerOther) transactionPartnerTag() {}

// TransactionPartnerUnknown is a fallback for future/unknown partner types
// Keeps the library forward-compatible when Telegram adds new types
type TransactionPartnerUnknown struct {
    Type string          `json:"type"`
    Raw  json.RawMessage `json:"-"` // Preserved for debugging
}

func (TransactionPartnerUnknown) transactionPartnerTag() {}

// unmarshalTransactionPartner decodes a TransactionPartner from JSON.
// Returns TransactionPartnerUnknown on any error (including malformed known types).
func unmarshalTransactionPartner(data json.RawMessage) TransactionPartner {
    var probe struct {
        Type string `json:"type"`
    }
    if err := json.Unmarshal(data, &probe); err != nil {
        return TransactionPartnerUnknown{Raw: data}
    }

    switch probe.Type {
    case "user":
        var p TransactionPartnerUser
        if err := json.Unmarshal(data, &p); err != nil {
            // Malformed "user" — return Unknown with raw data for debugging
            return TransactionPartnerUnknown{Type: probe.Type, Raw: data}
        }
        return p
    case "fragment":
        var p TransactionPartnerFragment
        if err := json.Unmarshal(data, &p); err != nil {
            return TransactionPartnerUnknown{Type: probe.Type, Raw: data}
        }
        return p
    case "telegram_ads":
        var p TransactionPartnerTelegramAds
        if err := json.Unmarshal(data, &p); err != nil {
            return TransactionPartnerUnknown{Type: probe.Type, Raw: data}
        }
        return p
    case "telegram_api":
        var p TransactionPartnerTelegramApi
        if err := json.Unmarshal(data, &p); err != nil {
            return TransactionPartnerUnknown{Type: probe.Type, Raw: data}
        }
        return p
    case "other":
        var p TransactionPartnerOther
        if err := json.Unmarshal(data, &p); err != nil {
            return TransactionPartnerUnknown{Type: probe.Type, Raw: data}
        }
        return p
    default:
        return TransactionPartnerUnknown{Type: probe.Type, Raw: data}
    }
}

// --- RevenueWithdrawalState Union (CORRECTED: includes Unknown) ---

// RevenueWithdrawalState describes the state of a revenue withdrawal
type RevenueWithdrawalState interface {
    revenueWithdrawalStateTag()
}

type RevenueWithdrawalStatePending struct {
    Type string `json:"type"` // Always "pending"
}

func (RevenueWithdrawalStatePending) revenueWithdrawalStateTag() {}

type RevenueWithdrawalStateSucceeded struct {
    Type string `json:"type"` // Always "succeeded"
    Date int64  `json:"date"`
    URL  string `json:"url"`
}

func (RevenueWithdrawalStateSucceeded) revenueWithdrawalStateTag() {}

type RevenueWithdrawalStateFailed struct {
    Type string `json:"type"` // Always "failed"
}

func (RevenueWithdrawalStateFailed) revenueWithdrawalStateTag() {}

// RevenueWithdrawalStateUnknown is a fallback for future withdrawal states
type RevenueWithdrawalStateUnknown struct {
    Type string          `json:"type"`
    Raw  json.RawMessage `json:"-"`
}

func (RevenueWithdrawalStateUnknown) revenueWithdrawalStateTag() {}

// unmarshalRevenueWithdrawalState decodes a RevenueWithdrawalState from JSON.
// Returns RevenueWithdrawalStateUnknown on any error.
func unmarshalRevenueWithdrawalState(data json.RawMessage) RevenueWithdrawalState {
    var probe struct {
        Type string `json:"type"`
    }
    if err := json.Unmarshal(data, &probe); err != nil {
        return RevenueWithdrawalStateUnknown{Raw: data}
    }

    switch probe.Type {
    case "pending":
        var s RevenueWithdrawalStatePending
        if err := json.Unmarshal(data, &s); err != nil {
            return RevenueWithdrawalStateUnknown{Type: probe.Type, Raw: data}
        }
        return s
    case "succeeded":
        var s RevenueWithdrawalStateSucceeded
        if err := json.Unmarshal(data, &s); err != nil {
            return RevenueWithdrawalStateUnknown{Type: probe.Type, Raw: data}
        }
        return s
    case "failed":
        var s RevenueWithdrawalStateFailed
        if err := json.Unmarshal(data, &s); err != nil {
            return RevenueWithdrawalStateUnknown{Type: probe.Type, Raw: data}
        }
        return s
    default:
        return RevenueWithdrawalStateUnknown{Type: probe.Type, Raw: data}
    }
}

// Custom UnmarshalJSON for TransactionPartnerFragment to handle nested polymorphic type
func (p *TransactionPartnerFragment) UnmarshalJSON(data []byte) error {
    type Alias TransactionPartnerFragment
    aux := &struct {
        WithdrawalState json.RawMessage `json:"withdrawal_state,omitempty"`
        *Alias
    }{Alias: (*Alias)(p)}

    if err := json.Unmarshal(data, aux); err != nil {
        return err
    }

    if len(aux.WithdrawalState) > 0 && string(aux.WithdrawalState) != "null" {
        p.WithdrawalState = unmarshalRevenueWithdrawalState(aux.WithdrawalState)
    }
    return nil
}
```

---

### tg/checklists.go — NEW FILE

```go
// tg/checklists.go — Checklist types (Bot API 9.1)

package tg

// InputChecklist represents a checklist to be sent
// No InputFile, so this can live in tg/
type InputChecklist struct {
    Title                    string               `json:"title"`
    ParseMode                string               `json:"parse_mode,omitempty"`
    TitleEntities            []MessageEntity      `json:"title_entities,omitempty"`
    Tasks                    []InputChecklistTask `json:"tasks"`
    OthersCanAddTasks        bool                 `json:"others_can_add_tasks,omitempty"`
    OthersCanMarkTasksAsDone bool                 `json:"others_can_mark_tasks_as_done,omitempty"`
}

// InputChecklistTask represents a task in a checklist to be sent
type InputChecklistTask struct {
    ID           int             `json:"id"`
    Text         string          `json:"text"`
    ParseMode    string          `json:"parse_mode,omitempty"`
    TextEntities []MessageEntity `json:"text_entities,omitempty"`
}

// Checklist represents a checklist in a message
type Checklist struct {
    Title                    string          `json:"title"`
    TitleEntities            []MessageEntity `json:"title_entities,omitempty"`
    Tasks                    []ChecklistTask `json:"tasks"`
    OthersCanAddTasks        bool            `json:"others_can_add_tasks"`
    OthersCanMarkTasksAsDone bool            `json:"others_can_mark_tasks_as_done"`
}

// ChecklistTask represents a task in a received checklist
type ChecklistTask struct {
    ID            int             `json:"id"`
    Text          string          `json:"text"`
    TextEntities  []MessageEntity `json:"text_entities,omitempty"`
    IsDone        bool            `json:"is_done"`
    CompletedByID int64           `json:"completed_by_id,omitempty"`
}
```

---

### tg/inline.go — NEW FILE

```go
// tg/inline.go — Inline query types

package tg

import "encoding/json"

// --- InlineQueryResult Union (Partial Implementation) ---
//
// Telegram has 20+ InlineQueryResult types. We implement commonly-used ones
// and provide InlineQueryResultUnknown as a forward-compatible fallback.
// Add more concrete types as needed.

// InlineQueryResult represents one result of an inline query
type InlineQueryResult interface {
    inlineQueryResultTag()
    // GetType returns the result type for serialization
    GetType() string
}

// InlineQueryResultArticle represents a link to an article or web page
type InlineQueryResultArticle struct {
    Type                string                `json:"type"` // Always "article"
    ID                  string                `json:"id"`
    Title               string                `json:"title"`
    InputMessageContent InputMessageContent   `json:"input_message_content"`
    ReplyMarkup         *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
    URL                 string                `json:"url,omitempty"`
    HideURL             bool                  `json:"hide_url,omitempty"`
    Description         string                `json:"description,omitempty"`
    ThumbnailURL        string                `json:"thumbnail_url,omitempty"`
    ThumbnailWidth      int                   `json:"thumbnail_width,omitempty"`
    ThumbnailHeight     int                   `json:"thumbnail_height,omitempty"`
}

func (InlineQueryResultArticle) inlineQueryResultTag() {}
func (r InlineQueryResultArticle) GetType() string     { return "article" }

// InlineQueryResultPhoto represents a link to a photo
type InlineQueryResultPhoto struct {
    Type                  string                `json:"type"` // Always "photo"
    ID                    string                `json:"id"`
    PhotoURL              string                `json:"photo_url"`
    ThumbnailURL          string                `json:"thumbnail_url"`
    PhotoWidth            int                   `json:"photo_width,omitempty"`
    PhotoHeight           int                   `json:"photo_height,omitempty"`
    Title                 string                `json:"title,omitempty"`
    Description           string                `json:"description,omitempty"`
    Caption               string                `json:"caption,omitempty"`
    ParseMode             string                `json:"parse_mode,omitempty"`
    CaptionEntities       []MessageEntity       `json:"caption_entities,omitempty"`
    ShowCaptionAboveMedia bool                  `json:"show_caption_above_media,omitempty"`
    ReplyMarkup           *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
    InputMessageContent   InputMessageContent   `json:"input_message_content,omitempty"`
}

func (InlineQueryResultPhoto) inlineQueryResultTag() {}
func (r InlineQueryResultPhoto) GetType() string     { return "photo" }

// InlineQueryResultDocument represents a link to a file
type InlineQueryResultDocument struct {
    Type                string                `json:"type"` // Always "document"
    ID                  string                `json:"id"`
    Title               string                `json:"title"`
    DocumentURL         string                `json:"document_url"`
    MimeType            string                `json:"mime_type"`
    Caption             string                `json:"caption,omitempty"`
    ParseMode           string                `json:"parse_mode,omitempty"`
    CaptionEntities     []MessageEntity       `json:"caption_entities,omitempty"`
    Description         string                `json:"description,omitempty"`
    ReplyMarkup         *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
    InputMessageContent InputMessageContent   `json:"input_message_content,omitempty"`
    ThumbnailURL        string                `json:"thumbnail_url,omitempty"`
    ThumbnailWidth      int                   `json:"thumbnail_width,omitempty"`
    ThumbnailHeight     int                   `json:"thumbnail_height,omitempty"`
}

func (InlineQueryResultDocument) inlineQueryResultTag() {}
func (r InlineQueryResultDocument) GetType() string     { return "document" }

// InlineQueryResultUnknown is a fallback for unknown/future result types
// Keeps the library forward-compatible
type InlineQueryResultUnknown struct {
    Type string          `json:"type"`
    Raw  json.RawMessage `json:"-"`
}

func (InlineQueryResultUnknown) inlineQueryResultTag() {}
func (r InlineQueryResultUnknown) GetType() string     { return r.Type }

// --- InputMessageContent ---

// InputMessageContent represents the content of a message to be sent
type InputMessageContent interface {
    inputMessageContentTag()
}

// InputTextMessageContent represents text content
type InputTextMessageContent struct {
    MessageText        string              `json:"message_text"`
    ParseMode          string              `json:"parse_mode,omitempty"`
    Entities           []MessageEntity     `json:"entities,omitempty"`
    LinkPreviewOptions *LinkPreviewOptions `json:"link_preview_options,omitempty"`
}

func (InputTextMessageContent) inputMessageContentTag() {}

// InputLocationMessageContent represents location content
type InputLocationMessageContent struct {
    Latitude             float64 `json:"latitude"`
    Longitude            float64 `json:"longitude"`
    HorizontalAccuracy   float64 `json:"horizontal_accuracy,omitempty"`
    LivePeriod           int     `json:"live_period,omitempty"`
    Heading              int     `json:"heading,omitempty"`
    ProximityAlertRadius int     `json:"proximity_alert_radius,omitempty"`
}

func (InputLocationMessageContent) inputMessageContentTag() {}

// InputVenueMessageContent represents venue content
type InputVenueMessageContent struct {
    Latitude        float64 `json:"latitude"`
    Longitude       float64 `json:"longitude"`
    Title           string  `json:"title"`
    Address         string  `json:"address"`
    FoursquareID    string  `json:"foursquare_id,omitempty"`
    FoursquareType  string  `json:"foursquare_type,omitempty"`
    GooglePlaceID   string  `json:"google_place_id,omitempty"`
    GooglePlaceType string  `json:"google_place_type,omitempty"`
}

func (InputVenueMessageContent) inputMessageContentTag() {}

// InputContactMessageContent represents contact content
type InputContactMessageContent struct {
    PhoneNumber string `json:"phone_number"`
    FirstName   string `json:"first_name"`
    LastName    string `json:"last_name,omitempty"`
    VCard       string `json:"vcard,omitempty"`
}

func (InputContactMessageContent) inputMessageContentTag() {}

// InputInvoiceMessageContent represents invoice content
type InputInvoiceMessageContent struct {
    Title                     string         `json:"title"`
    Description               string         `json:"description"`
    Payload                   string         `json:"payload"`
    ProviderToken             string         `json:"provider_token,omitempty"`
    Currency                  string         `json:"currency"`
    Prices                    []LabeledPrice `json:"prices"`
    MaxTipAmount              int            `json:"max_tip_amount,omitempty"`
    SuggestedTipAmounts       []int          `json:"suggested_tip_amounts,omitempty"`
    ProviderData              string         `json:"provider_data,omitempty"`
    PhotoURL                  string         `json:"photo_url,omitempty"`
    PhotoSize                 int            `json:"photo_size,omitempty"`
    PhotoWidth                int            `json:"photo_width,omitempty"`
    PhotoHeight               int            `json:"photo_height,omitempty"`
    NeedName                  bool           `json:"need_name,omitempty"`
    NeedPhoneNumber           bool           `json:"need_phone_number,omitempty"`
    NeedEmail                 bool           `json:"need_email,omitempty"`
    NeedShippingAddress       bool           `json:"need_shipping_address,omitempty"`
    SendPhoneNumberToProvider bool           `json:"send_phone_number_to_provider,omitempty"`
    SendEmailToProvider       bool           `json:"send_email_to_provider,omitempty"`
    IsFlexible                bool           `json:"is_flexible,omitempty"`
}

func (InputInvoiceMessageContent) inputMessageContentTag() {}

// --- Other Inline Types ---

// InlineQueryResultsButton represents a button above inline query results
type InlineQueryResultsButton struct {
    Text           string      `json:"text"`
    WebApp         *WebAppInfo `json:"web_app,omitempty"`
    StartParameter string      `json:"start_parameter,omitempty"`
}

// SentWebAppMessage describes an inline message sent by a Web App
type SentWebAppMessage struct {
    InlineMessageID string `json:"inline_message_id,omitempty"`
}

// InlineQuery represents an incoming inline query
type InlineQuery struct {
    ID       string    `json:"id"`
    From     User      `json:"from"`
    Query    string    `json:"query"`
    Offset   string    `json:"offset"`
    ChatType string    `json:"chat_type,omitempty"`
    Location *Location `json:"location,omitempty"`
}

// Location represents a point on the map
type Location struct {
    Latitude             float64 `json:"latitude"`
    Longitude            float64 `json:"longitude"`
    HorizontalAccuracy   float64 `json:"horizontal_accuracy,omitempty"`
    LivePeriod           int     `json:"live_period,omitempty"`
    Heading              int     `json:"heading,omitempty"`
    ProximityAlertRadius int     `json:"proximity_alert_radius,omitempty"`
}
```

---

### tg/games.go — NEW FILE

```go
// tg/games.go — Game types

package tg

// Game represents a game
type Game struct {
    Title        string          `json:"title"`
    Description  string          `json:"description"`
    Photo        []PhotoSize     `json:"photo"`
    Text         string          `json:"text,omitempty"`
    TextEntities []MessageEntity `json:"text_entities,omitempty"`
    Animation    *Animation      `json:"animation,omitempty"`
}

// GameHighScore represents one row of the high scores table
type GameHighScore struct {
    Position int  `json:"position"`
    User     User `json:"user"`
    Score    int  `json:"score"`
}

// CallbackGame is a placeholder for the "callback_game" button
// When pressed, Telegram opens the game
type CallbackGame struct{}
```

---

### tg/gifts.go — NEW FILE

```go
// tg/gifts.go — Gift types (Bot API 8.0+)

package tg

// Gifts represents a list of available gifts
type Gifts struct {
    Gifts []Gift `json:"gifts"`
}

// Gift represents a gift that can be sent
type Gift struct {
    ID               string  `json:"id"`
    Sticker          Sticker `json:"sticker"`
    StarCount        int     `json:"star_count"`
    UpgradeStarCount int     `json:"upgrade_star_count,omitempty"`
    TotalCount       int     `json:"total_count,omitempty"`
    RemainingCount   int     `json:"remaining_count,omitempty"`
}

// OwnedGifts represents a list of gifts owned by a user
type OwnedGifts struct {
    TotalCount int         `json:"total_count"`
    Gifts      []OwnedGift `json:"gifts"`
    NextOffset string      `json:"next_offset,omitempty"`
}

// OwnedGift represents a gift owned by a user (FULL FIELDS)
type OwnedGift struct {
    // Common fields
    Type        string `json:"type"` // "regular" or "unique"
    Gift        *Gift  `json:"gift,omitempty"`
    OwnedGiftID string `json:"owned_gift_id,omitempty"`

    // Sender info
    SenderUser *User `json:"sender_user,omitempty"`
    SendDate   int64 `json:"send_date"`

    // Message
    Text         string          `json:"text,omitempty"`
    TextEntities []MessageEntity `json:"text_entities,omitempty"`

    // State flags
    IsSaved       bool `json:"is_saved,omitempty"`
    CanBeUpgraded bool `json:"can_be_upgraded,omitempty"`
    WasRefunded   bool `json:"was_refunded,omitempty"`

    // Star values
    ConvertStarCount        int `json:"convert_star_count,omitempty"`
    PrepaidUpgradeStarCount int `json:"prepaid_upgrade_star_count,omitempty"`

    // For unique gifts
    TransferStarCount int `json:"transfer_star_count,omitempty"`
}

// AcceptedGiftTypes describes which gift types are accepted
type AcceptedGiftTypes struct {
    UnlimitedGifts      bool `json:"unlimited_gifts,omitempty"`
    LimitedGifts        bool `json:"limited_gifts,omitempty"`
    UniqueGifts         bool `json:"unique_gifts,omitempty"`
    PremiumSubscription bool `json:"premium_subscription,omitempty"`
}
```

---

### tg/business.go — NEW FILE

```go
// tg/business.go — Business account types

package tg

// BusinessConnection represents a connection with a business account
type BusinessConnection struct {
    ID         string            `json:"id"`
    User       User              `json:"user"`
    UserChatID int64             `json:"user_chat_id"`
    Date       int64             `json:"date"`
    Rights     BusinessBotRights `json:"rights"`
    IsEnabled  bool              `json:"is_enabled"`
}

// BusinessBotRights describes the rights of a bot in a business account
type BusinessBotRights struct {
    CanReply                   bool `json:"can_reply"`
    CanReadMessages            bool `json:"can_read_messages"`
    CanDeleteAllMessages       bool `json:"can_delete_all_messages"`
    CanEditName                bool `json:"can_edit_name"`
    CanEditBio                 bool `json:"can_edit_bio"`
    CanEditProfilePhoto        bool `json:"can_edit_profile_photo"`
    CanEditUsername            bool `json:"can_edit_username"`
    CanChangeGiftSettings      bool `json:"can_change_gift_settings"`
    CanViewGiftsAndStars       bool `json:"can_view_gifts_and_stars"`
    CanConvertGiftsToStars     bool `json:"can_convert_gifts_to_stars"`
    CanTransferAndUpgradeGifts bool `json:"can_transfer_and_upgrade_gifts"`
    CanTransferStars           bool `json:"can_transfer_stars"`
    CanManageStories           bool `json:"can_manage_stories"`
}

// Story represents a story posted to a chat
type Story struct {
    ID         int         `json:"id"`
    Chat       Chat        `json:"chat"`
    Date       int64       `json:"date"`
    ExpireDate int64       `json:"expire_date"`
    Areas      []StoryArea `json:"areas,omitempty"`
}

// StoryArea represents a clickable area on a story
type StoryArea struct {
    Position StoryAreaPosition `json:"position"`
    // Type is polymorphic - simplified here
}

// StoryAreaPosition describes the position of a story area
type StoryAreaPosition struct {
    XPercentage      float64 `json:"x_percentage"`
    YPercentage      float64 `json:"y_percentage"`
    WidthPercentage  float64 `json:"width_percentage"`
    HeightPercentage float64 `json:"height_percentage"`
    RotationAngle    float64 `json:"rotation_angle"`
}
```

---

### tg/boosts.go — NEW FILE

```go
// tg/boosts.go — Chat boost types

package tg

import "encoding/json"

// UserChatBoosts represents a list of boosts added to a chat by a user
type UserChatBoosts struct {
    Boosts []ChatBoost `json:"boosts"`
}

// ChatBoost represents a boost added to a chat
type ChatBoost struct {
    BoostID        string          `json:"boost_id"`
    AddDate        int64           `json:"add_date"`
    ExpirationDate int64           `json:"expiration_date"`
    Source         ChatBoostSource `json:"source"`
}

// Custom UnmarshalJSON for ChatBoost
func (b *ChatBoost) UnmarshalJSON(data []byte) error {
    type Alias ChatBoost
    aux := &struct {
        Source json.RawMessage `json:"source"`
        *Alias
    }{Alias: (*Alias)(b)}

    if err := json.Unmarshal(data, aux); err != nil {
        return err
    }

    b.Source = unmarshalChatBoostSource(aux.Source)
    return nil
}

// --- ChatBoostSource Union ---

// ChatBoostSource describes the source of a chat boost
type ChatBoostSource interface {
    chatBoostSourceTag()
    GetSource() string
}

// ChatBoostSourcePremium represents a boost from a Premium subscriber
type ChatBoostSourcePremium struct {
    Source string `json:"source"` // Always "premium"
    User   User   `json:"user"`
}

func (ChatBoostSourcePremium) chatBoostSourceTag() {}
func (s ChatBoostSourcePremium) GetSource() string { return "premium" }

// ChatBoostSourceGiftCode represents a boost from a gift code
type ChatBoostSourceGiftCode struct {
    Source string `json:"source"` // Always "gift_code"
    User   User   `json:"user"`
}

func (ChatBoostSourceGiftCode) chatBoostSourceTag() {}
func (s ChatBoostSourceGiftCode) GetSource() string { return "gift_code" }

// ChatBoostSourceGiveaway represents a boost from a giveaway
type ChatBoostSourceGiveaway struct {
    Source            string `json:"source"` // Always "giveaway"
    GiveawayMessageID int    `json:"giveaway_message_id"`
    User              *User  `json:"user,omitempty"`
    PrizeStarCount    int    `json:"prize_star_count,omitempty"`
    IsUnclaimed       bool   `json:"is_unclaimed,omitempty"`
}

func (ChatBoostSourceGiveaway) chatBoostSourceTag() {}
func (s ChatBoostSourceGiveaway) GetSource() string { return "giveaway" }

// ChatBoostSourceUnknown is a fallback for future boost source types
type ChatBoostSourceUnknown struct {
    Source string          `json:"source"`
    Raw    json.RawMessage `json:"-"`
}

func (ChatBoostSourceUnknown) chatBoostSourceTag() {}
func (s ChatBoostSourceUnknown) GetSource() string { return s.Source }

// unmarshalChatBoostSource decodes a ChatBoostSource from JSON.
// Returns ChatBoostSourceUnknown on any error.
func unmarshalChatBoostSource(data json.RawMessage) ChatBoostSource {
    var probe struct {
        Source string `json:"source"`
    }
    if err := json.Unmarshal(data, &probe); err != nil {
        return ChatBoostSourceUnknown{Raw: data}
    }

    switch probe.Source {
    case "premium":
        var s ChatBoostSourcePremium
        if err := json.Unmarshal(data, &s); err != nil {
            return ChatBoostSourceUnknown{Source: probe.Source, Raw: data}
        }
        return s
    case "gift_code":
        var s ChatBoostSourceGiftCode
        if err := json.Unmarshal(data, &s); err != nil {
            return ChatBoostSourceUnknown{Source: probe.Source, Raw: data}
        }
        return s
    case "giveaway":
        var s ChatBoostSourceGiveaway
        if err := json.Unmarshal(data, &s); err != nil {
            return ChatBoostSourceUnknown{Source: probe.Source, Raw: data}
        }
        return s
    default:
        return ChatBoostSourceUnknown{Source: probe.Source, Raw: data}
    }
}
```

---

### tg/passport.go — NEW FILE

```go
// tg/passport.go — Telegram Passport types
//
// NOTE: PassportElementError types are SEND-ONLY (sent to Telegram, not received).
// No unmarshal function is needed. The interface + tag pattern is used only for
// type-safe slice typing: []PassportElementError

package tg

// --- PassportElementError Union (SEND-ONLY) ---

// PassportElementError describes an error in Telegram Passport data
// This is a SEND-ONLY type — no UnmarshalJSON needed
type PassportElementError interface {
    passportElementErrorTag()
    GetSource() string
}

// PassportElementErrorDataField represents an error in a data field
type PassportElementErrorDataField struct {
    Source    string `json:"source"` // Always "data"
    Type      string `json:"type"`
    FieldName string `json:"field_name"`
    DataHash  string `json:"data_hash"`
    Message   string `json:"message"`
}

func (PassportElementErrorDataField) passportElementErrorTag() {}
func (e PassportElementErrorDataField) GetSource() string      { return "data" }

// PassportElementErrorFrontSide represents an error with the front side
type PassportElementErrorFrontSide struct {
    Source   string `json:"source"` // Always "front_side"
    Type     string `json:"type"`
    FileHash string `json:"file_hash"`
    Message  string `json:"message"`
}

func (PassportElementErrorFrontSide) passportElementErrorTag() {}
func (e PassportElementErrorFrontSide) GetSource() string      { return "front_side" }

// PassportElementErrorReverseSide represents an error with the reverse side
type PassportElementErrorReverseSide struct {
    Source   string `json:"source"` // Always "reverse_side"
    Type     string `json:"type"`
    FileHash string `json:"file_hash"`
    Message  string `json:"message"`
}

func (PassportElementErrorReverseSide) passportElementErrorTag() {}
func (e PassportElementErrorReverseSide) GetSource() string      { return "reverse_side" }

// PassportElementErrorSelfie represents an error with the selfie
type PassportElementErrorSelfie struct {
    Source   string `json:"source"` // Always "selfie"
    Type     string `json:"type"`
    FileHash string `json:"file_hash"`
    Message  string `json:"message"`
}

func (PassportElementErrorSelfie) passportElementErrorTag() {}
func (e PassportElementErrorSelfie) GetSource() string      { return "selfie" }

// PassportElementErrorFile represents an error with a document scan
type PassportElementErrorFile struct {
    Source   string `json:"source"` // Always "file"
    Type     string `json:"type"`
    FileHash string `json:"file_hash"`
    Message  string `json:"message"`
}

func (PassportElementErrorFile) passportElementErrorTag() {}
func (e PassportElementErrorFile) GetSource() string      { return "file" }

// PassportElementErrorFiles represents an error with multiple document scans
type PassportElementErrorFiles struct {
    Source     string   `json:"source"` // Always "files"
    Type       string   `json:"type"`
    FileHashes []string `json:"file_hashes"`
    Message    string   `json:"message"`
}

func (PassportElementErrorFiles) passportElementErrorTag() {}
func (e PassportElementErrorFiles) GetSource() string      { return "files" }

// PassportElementErrorTranslationFile represents an error with one translation
type PassportElementErrorTranslationFile struct {
    Source   string `json:"source"` // Always "translation_file"
    Type     string `json:"type"`
    FileHash string `json:"file_hash"`
    Message  string `json:"message"`
}

func (PassportElementErrorTranslationFile) passportElementErrorTag() {}
func (e PassportElementErrorTranslationFile) GetSource() string      { return "translation_file" }

// PassportElementErrorTranslationFiles represents an error with translations
type PassportElementErrorTranslationFiles struct {
    Source     string   `json:"source"` // Always "translation_files"
    Type       string   `json:"type"`
    FileHashes []string `json:"file_hashes"`
    Message    string   `json:"message"`
}

func (PassportElementErrorTranslationFiles) passportElementErrorTag() {}
func (e PassportElementErrorTranslationFiles) GetSource() string      { return "translation_files" }

// PassportElementErrorUnspecified represents an unspecified error
type PassportElementErrorUnspecified struct {
    Source      string `json:"source"` // Always "unspecified"
    Type        string `json:"type"`
    ElementHash string `json:"element_hash"`
    Message     string `json:"message"`
}

func (PassportElementErrorUnspecified) passportElementErrorTag() {}
func (e PassportElementErrorUnspecified) GetSource() string      { return "unspecified" }
```

---

### tg/stickers.go — NEW FILE (Response Types Only)

```go
// tg/stickers.go — Sticker response types
// NOTE: InputSticker lives in sender/ due to InputFile dependency

package tg

// StickerSet represents a sticker set
type StickerSet struct {
    Name        string     `json:"name"`
    Title       string     `json:"title"`
    StickerType string     `json:"sticker_type"` // "regular", "mask", "custom_emoji"
    Stickers    []Sticker  `json:"stickers"`
    Thumbnail   *PhotoSize `json:"thumbnail,omitempty"`
}

// MaskPosition describes the position for mask stickers
type MaskPosition struct {
    Point  string  `json:"point"` // "forehead", "eyes", "mouth", "chin"
    XShift float64 `json:"x_shift"`
    YShift float64 `json:"y_shift"`
    Scale  float64 `json:"scale"`
}

// NOTE: Sticker type should be extended in tg/types.go (existing)
// Add these fields if missing:
//   Type             string        // "regular", "mask", "custom_emoji"
//   IsAnimated       bool
//   IsVideo          bool
//   MaskPosition     *MaskPosition
//   CustomEmojiID    string
//   NeedsRepainting  bool
```

---

### sender/stickers_input.go — NEW FILE (CORRECTED IMPORT PATH)

```go
// sender/stickers_input.go — Sticker input types
// Lives in sender/ because it contains InputFile (avoids tg/ → sender/ import cycle)

package sender

import "github.com/prilive-com/galigo/tg"

// InputSticker represents a sticker to be uploaded
type InputSticker struct {
    Sticker      InputFile        `json:"-"` // Handled by multipart encoder
    Format       string           `json:"format"` // "static", "animated", "video"
    EmojiList    []string         `json:"emoji_list"`
    MaskPosition *tg.MaskPosition `json:"mask_position,omitempty"`
    Keywords     []string         `json:"keywords,omitempty"`
}
```

---

### sender/business_input.go — NEW FILE

```go
// sender/business_input.go — Business input types
// Lives in sender/ because they may contain InputFile

package sender

// --- InputStoryContent ---

// InputStoryContent represents content for a story
type InputStoryContent interface {
    inputStoryContentTag()
}

// InputStoryContentPhoto represents a photo for a story
type InputStoryContentPhoto struct {
    Photo InputFile `json:"-"` // Handled by multipart encoder
}

func (InputStoryContentPhoto) inputStoryContentTag() {}

// InputStoryContentVideo represents a video for a story
type InputStoryContentVideo struct {
    Video          InputFile `json:"-"` // Handled by multipart encoder
    Duration       float64   `json:"duration,omitempty"`
    CoverFrameTime float64   `json:"cover_frame_time,omitempty"`
    IsAnimation    bool      `json:"is_animation,omitempty"`
}

func (InputStoryContentVideo) inputStoryContentTag() {}

// --- InputProfilePhoto ---

// InputProfilePhoto represents a profile photo to set
type InputProfilePhoto interface {
    inputProfilePhotoTag()
}

// InputProfilePhotoStatic represents a static profile photo
type InputProfilePhotoStatic struct {
    Photo InputFile `json:"-"` // Handled by multipart encoder
}

func (InputProfilePhotoStatic) inputProfilePhotoTag() {}

// InputProfilePhotoAnimated represents an animated profile photo
type InputProfilePhotoAnimated struct {
    Animation     InputFile `json:"-"` // Handled by multipart encoder
    MainFrameTime float64   `json:"main_frame_time,omitempty"`
}

func (InputProfilePhotoAnimated) inputProfilePhotoTag() {}
```

---

### sender/rate_limiter.go — NEW FILE

```go
// sender/rate_limiter.go — Optional proactive rate limiting

package sender

import (
    "context"

    "golang.org/x/time/rate"
)

// WithRateLimiter configures optional proactive rate limiting.
// The limiter is applied BEFORE each request (proactive pacing).
//
// This is separate from reactive 429 handling, which always respects retry_after.
//
// NOTE: Time-critical operations like AnswerPreCheckoutQuery skip the rate limiter
// to avoid missing Telegram's 10-second response deadline.
//
// Example:
//
//	limiter := rate.NewLimiter(rate.Limit(30), 5) // 30 req/s, burst 5
//	client := sender.New(token, sender.WithRateLimiter(limiter))
func WithRateLimiter(limiter *rate.Limiter) Option {
    return func(c *Client) {
        c.rateLimiter = limiter
    }
}

// waitForRateLimiter waits for the rate limiter if configured.
// Returns nil immediately if no limiter is set.
func (c *Client) waitForRateLimiter(ctx context.Context) error {
    if c.rateLimiter == nil {
        return nil
    }
    return c.rateLimiter.Wait(ctx)
}
```

---

## Epic A: Payments & Stars (7 methods)

**Blocked by:** PR0 (types)

### Request Types (CORRECTED: *int vs int audit)

```go
// sender/payments.go

type SendInvoiceRequest struct {
    ChatID                    any               `json:"chat_id"`
    MessageThreadID           int               `json:"message_thread_id,omitempty"` // Optional but 0 is invalid
    Title                     string            `json:"title"`
    Description               string            `json:"description"`
    Payload                   string            `json:"payload"`
    ProviderToken             string            `json:"provider_token"`
    Currency                  string            `json:"currency"`
    Prices                    []tg.LabeledPrice `json:"prices"`
    MaxTipAmount              int               `json:"max_tip_amount,omitempty"`  // 0 = no tips
    SuggestedTipAmounts       []int             `json:"suggested_tip_amounts,omitempty"`
    StartParameter            string            `json:"start_parameter,omitempty"`
    ProviderData              string            `json:"provider_data,omitempty"`
    PhotoURL                  string            `json:"photo_url,omitempty"`
    PhotoSize                 int               `json:"photo_size,omitempty"`
    PhotoWidth                int               `json:"photo_width,omitempty"`
    PhotoHeight               int               `json:"photo_height,omitempty"`
    NeedName                  bool              `json:"need_name,omitempty"`
    NeedPhoneNumber           bool              `json:"need_phone_number,omitempty"`
    NeedEmail                 bool              `json:"need_email,omitempty"`
    NeedShippingAddress       bool              `json:"need_shipping_address,omitempty"`
    SendPhoneNumberToProvider bool              `json:"send_phone_number_to_provider,omitempty"`
    SendEmailToProvider       bool              `json:"send_email_to_provider,omitempty"`
    IsFlexible                bool              `json:"is_flexible,omitempty"`
    DisableNotification       bool              `json:"disable_notification,omitempty"`
    ProtectContent            bool              `json:"protect_content,omitempty"`
    ReplyParameters           *tg.ReplyParameters      `json:"reply_parameters,omitempty"`
    ReplyMarkup               *tg.InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

type CreateInvoiceLinkRequest struct {
    Title               string            `json:"title"`
    Description         string            `json:"description"`
    Payload             string            `json:"payload"`
    ProviderToken       string            `json:"provider_token"`
    Currency            string            `json:"currency"`
    Prices              []tg.LabeledPrice `json:"prices"`
    MaxTipAmount        int               `json:"max_tip_amount,omitempty"`
    SuggestedTipAmounts []int             `json:"suggested_tip_amounts,omitempty"`
    ProviderData        string            `json:"provider_data,omitempty"`
    PhotoURL            string            `json:"photo_url,omitempty"`
    PhotoSize           int               `json:"photo_size,omitempty"`
    PhotoWidth          int               `json:"photo_width,omitempty"`
    PhotoHeight         int               `json:"photo_height,omitempty"`
    NeedName            bool              `json:"need_name,omitempty"`
    NeedPhoneNumber     bool              `json:"need_phone_number,omitempty"`
    NeedEmail           bool              `json:"need_email,omitempty"`
    NeedShippingAddress bool              `json:"need_shipping_address,omitempty"`
    IsFlexible          bool              `json:"is_flexible,omitempty"`
    SubscriptionPeriod  int               `json:"subscription_period,omitempty"` // 0 = not subscription
}

type AnswerShippingQueryRequest struct {
    ShippingQueryID string              `json:"shipping_query_id"`
    OK              bool                `json:"ok"`
    ShippingOptions []tg.ShippingOption `json:"shipping_options,omitempty"`
    ErrorMessage    string              `json:"error_message,omitempty"`
}

type AnswerPreCheckoutQueryRequest struct {
    PreCheckoutQueryID string `json:"pre_checkout_query_id"`
    OK                 bool   `json:"ok"`
    ErrorMessage       string `json:"error_message,omitempty"`
}

type RefundStarPaymentRequest struct {
    UserID                  int64  `json:"user_id"`
    TelegramPaymentChargeID string `json:"telegram_payment_charge_id"`
}

// GetStarTransactionsRequest — Offset uses plain int (default 0 is meaningful)
type GetStarTransactionsRequest struct {
    Offset int `json:"offset,omitempty"` // Default 0, plain int is fine
    Limit  int `json:"limit,omitempty"`  // Default 100, range 1-100
}
```

### Method Implementations (CORRECTED: rate limiter handling)

```go
// sender/payments.go

func (c *Client) SendInvoice(ctx context.Context, req SendInvoiceRequest) (*tg.Message, error) {
    if err := c.waitForRateLimiter(ctx); err != nil {
        return nil, err
    }

    if err := validateChatID(req.ChatID); err != nil {
        return nil, err
    }
    if req.Title == "" {
        return nil, tg.NewValidationError("title", "required")
    }
    if len(req.Title) > 32 {
        return nil, tg.NewValidationError("title", "must be 1-32 characters")
    }
    if req.Description == "" {
        return nil, tg.NewValidationError("description", "required")
    }
    if len(req.Description) > 255 {
        return nil, tg.NewValidationError("description", "must be 1-255 characters")
    }
    if req.Payload == "" {
        return nil, tg.NewValidationError("payload", "required")
    }
    if len(req.Payload) > 128 {
        return nil, tg.NewValidationError("payload", "must be 1-128 bytes")
    }
    if req.Currency == "" {
        return nil, tg.NewValidationError("currency", "required")
    }
    if len(req.Prices) == 0 {
        return nil, tg.NewValidationError("prices", "at least one price required")
    }

    var result tg.Message
    if err := c.callJSON(ctx, "sendInvoice", req, &result); err != nil {
        return nil, err
    }
    return &result, nil
}

func (c *Client) CreateInvoiceLink(ctx context.Context, req CreateInvoiceLinkRequest) (string, error) {
    if err := c.waitForRateLimiter(ctx); err != nil {
        return "", err
    }

    if req.Title == "" {
        return "", tg.NewValidationError("title", "required")
    }
    if req.Description == "" {
        return "", tg.NewValidationError("description", "required")
    }
    if req.Payload == "" {
        return "", tg.NewValidationError("payload", "required")
    }
    if req.Currency == "" {
        return "", tg.NewValidationError("currency", "required")
    }
    if len(req.Prices) == 0 {
        return "", tg.NewValidationError("prices", "at least one price required")
    }

    var result string
    if err := c.callJSON(ctx, "createInvoiceLink", req, &result); err != nil {
        return "", err
    }
    return result, nil
}

func (c *Client) AnswerShippingQuery(ctx context.Context, req AnswerShippingQueryRequest) error {
    if err := c.waitForRateLimiter(ctx); err != nil {
        return err
    }

    if req.ShippingQueryID == "" {
        return tg.NewValidationError("shipping_query_id", "required")
    }
    if req.OK && len(req.ShippingOptions) == 0 {
        return tg.NewValidationError("shipping_options", "required when ok is true")
    }
    if !req.OK && req.ErrorMessage == "" {
        return tg.NewValidationError("error_message", "required when ok is false")
    }

    return c.callJSON(ctx, "answerShippingQuery", req, nil)
}

// AnswerPreCheckoutQuery must be called within 10 seconds.
// VALUE OPERATION — no retry to prevent double-charging.
// NOTE: Skips rate limiter to avoid missing the 10s deadline.
func (c *Client) AnswerPreCheckoutQuery(ctx context.Context, req AnswerPreCheckoutQueryRequest) error {
    // SKIP rate limiter — time-critical operation (10s deadline)

    if req.PreCheckoutQueryID == "" {
        return tg.NewValidationError("pre_checkout_query_id", "required")
    }
    if !req.OK && req.ErrorMessage == "" {
        return tg.NewValidationError("error_message", "required when ok is false")
    }

    // NO RETRY — value operation
    return c.callJSON(ctx, "answerPreCheckoutQuery", req, nil)
}

// RefundStarPayment refunds a Telegram Stars payment.
// VALUE OPERATION — no retry to prevent double-refund.
func (c *Client) RefundStarPayment(ctx context.Context, req RefundStarPaymentRequest) error {
    if err := c.waitForRateLimiter(ctx); err != nil {
        return err
    }

    if req.UserID <= 0 {
        return tg.NewValidationError("user_id", "must be positive")
    }
    if req.TelegramPaymentChargeID == "" {
        return tg.NewValidationError("telegram_payment_charge_id", "required")
    }

    // NO RETRY — value operation
    return c.callJSON(ctx, "refundStarPayment", req, nil)
}

func (c *Client) GetStarTransactions(ctx context.Context, req GetStarTransactionsRequest) (*tg.StarTransactions, error) {
    if err := c.waitForRateLimiter(ctx); err != nil {
        return nil, err
    }

    // Limit validation: 0 means default (100), explicit values must be 1-100
    if req.Limit != 0 && (req.Limit < 1 || req.Limit > 100) {
        return nil, tg.NewValidationError("limit", "must be 1-100")
    }

    var result tg.StarTransactions
    if err := c.callJSON(ctx, "getStarTransactions", req, &result); err != nil {
        return nil, err
    }
    return &result, nil
}

func (c *Client) GetMyStarBalance(ctx context.Context) (*tg.StarAmount, error) {
    if err := c.waitForRateLimiter(ctx); err != nil {
        return nil, err
    }

    var result tg.StarAmount
    if err := c.callJSON(ctx, "getMyStarBalance", nil, &result); err != nil {
        return nil, err
    }
    return &result, nil
}
```

---

## Epic B-H: Remaining Epics

*(Same structure as Epic A, with appropriate corrections applied)*

**Key corrections applied to all epics:**
1. Plain `int` with `omitempty` for fields where 0 is a valid default
2. `*int` only for fields where 0 vs absent is meaningfully different
3. Rate limiter skipped for time-critical operations
4. Import path corrected to `github.com/prilive-com/galigo/tg`

---

## Implementation Summary

### Total Methods: ~61

| Epic | Methods | Value Ops | Multipart | Time-Critical |
|------|---------|-----------|-----------|---------------|
| **PR0: Types** | — | — | — | — |
| A: Payments & Stars | 7 | 2 | 0 | 1 |
| B: Stickers | 15 | 0 | 5 | 0 |
| C: Games | 5 | 0 | 0 | 0 |
| D: Business Account | 15 | 2 | 2 | 0 |
| E: Verification | 4 | 0 | 0 | 0 |
| F: Gifts | 5 | 3 | 0 | 0 |
| G: Bot API 9.1-9.3 | 6 | 0 | 0 | 0 |
| H: Inline & Other | 4 | 0 | 0 | 0 |
| **Total** | **~61** | **7** | **7** | **1** |

### Value Operations (NO RETRY)

1. `AnswerPreCheckoutQuery` — **Also skips rate limiter** (10s deadline)
2. `RefundStarPayment`
3. `TransferBusinessAccountStars`
4. `TransferGift`
5. `SendGift`
6. `UpgradeGift`
7. `ConvertGiftToStars`

### Polymorphic Types with Unknown Fallback

| Type | Unknown Variant | Unmarshal Function |
|------|-----------------|-------------------|
| `TransactionPartner` | `TransactionPartnerUnknown` | `unmarshalTransactionPartner` |
| `RevenueWithdrawalState` | `RevenueWithdrawalStateUnknown` | `unmarshalRevenueWithdrawalState` |
| `ChatBoostSource` | `ChatBoostSourceUnknown` | `unmarshalChatBoostSource` |
| `InlineQueryResult` | `InlineQueryResultUnknown` | *(not needed for send-only)* |
| `PassportElementError` | *(send-only, no unmarshal)* | *(not needed)* |

### `*int` vs `int` Guidelines

| Use `int` with `omitempty` | Use `*int` |
|---------------------------|------------|
| Default 0 is meaningful (e.g., `Offset`) | 0 vs absent has different semantics |
| 0 is a valid value (e.g., `Position`) | Required field with no default |
| Telegram's default = 0 | Rare cases where explicit 0 matters |

---

## Testing Strategy

### UnmarshalJSON Error Handling Tests

```go
func TestUnmarshalTransactionPartner_MalformedKnownType(t *testing.T) {
    // "user" type with invalid field types should return Unknown
    data := `{"type": "user", "user": "not_an_object"}`

    result := unmarshalTransactionPartner(json.RawMessage(data))

    unknown, ok := result.(tg.TransactionPartnerUnknown)
    require.True(t, ok, "malformed known type should decode to Unknown")
    assert.Equal(t, "user", unknown.Type)
    assert.NotEmpty(t, unknown.Raw)
}

func TestUnmarshalTransactionPartner_FutureType(t *testing.T) {
    data := `{"type": "future_partner", "new_field": "value"}`

    result := unmarshalTransactionPartner(json.RawMessage(data))

    unknown, ok := result.(tg.TransactionPartnerUnknown)
    require.True(t, ok, "unknown type should decode to Unknown")
    assert.Equal(t, "future_partner", unknown.Type)
}

func TestUnmarshalRevenueWithdrawalState_Unknown(t *testing.T) {
    data := `{"type": "new_state", "extra": 123}`

    result := unmarshalRevenueWithdrawalState(json.RawMessage(data))

    unknown, ok := result.(tg.RevenueWithdrawalStateUnknown)
    require.True(t, ok)
    assert.Equal(t, "new_state", unknown.Type)
}
```

### Time-Critical Operation Test

```go
func TestAnswerPreCheckoutQuery_SkipsRateLimiter(t *testing.T) {
    // Configure a very slow rate limiter
    slowLimiter := rate.NewLimiter(rate.Every(time.Hour), 1)
    slowLimiter.Allow() // Consume the one allowed request

    client := sender.New(token, sender.WithRateLimiter(slowLimiter))

    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()

    // This should NOT block on the rate limiter
    err := client.AnswerPreCheckoutQuery(ctx, sender.AnswerPreCheckoutQueryRequest{
        PreCheckoutQueryID: "test",
        OK:                 true,
    })

    // Should fail fast (connection error or validation), not timeout
    assert.NotEqual(t, context.DeadlineExceeded, err)
}
```

---

## Definition of Done

### PR0: Types
- [ ] All new types compile
- [ ] No import cycles (`tg/` ← `sender/`)
- [ ] UnmarshalJSON tests pass (including malformed input)
- [ ] Unknown fallback tests pass for all polymorphic types
- [ ] `RevenueWithdrawalState` has Unknown variant + unmarshal function
- [ ] `PassportElementError` documented as send-only (no unmarshal)

### Per Epic
- [ ] All methods use `callJSON` or `executeRequest`
- [ ] All request types use typed structs
- [ ] All validation uses `tg.NewValidationError()`
- [ ] All void methods use `nil`
- [ ] Value operations marked `// NO RETRY`
- [ ] Time-critical operations skip rate limiter with comment
- [ ] `int` vs `*int` follows guidelines (default 0 meaningful → `int`)
- [ ] Coverage ≥ 80%

---

## Acceptance Criteria

- [ ] `go test -race ./...` passes
- [ ] No import cycles
- [ ] All polymorphic types have Unknown fallback
- [ ] All unmarshal functions return Unknown on error (not partial struct)
- [ ] `AnswerPreCheckoutQuery` skips rate limiter
- [ ] Import path is `github.com/prilive-com/galigo/tg`
- [ ] Value operations do not retry
- [ ] Multipart methods are retry-safe