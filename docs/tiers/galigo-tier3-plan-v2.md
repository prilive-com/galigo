# galigo Tier 3 Implementation Plan v2.0

## Consolidated Technical Specification

**Version:** 2.0 (Enhanced)  
**Target:** galigo v2.2.0 / v3.0.0  
**Telegram Bot API:** 9.3 (Dec 31, 2025)  
**Go Version:** 1.25  
**Estimated Effort:** 8-10 weeks (16 PRs)  
**Prerequisites:** Tier 1 + Tier 2 complete

---

## Executive Summary

This consolidated plan combines multiple independent analyses to deliver advanced features for production bots: payments, stickers, business accounts, gifts, stories, drafts, verification, and the newest Bot API 9.x additions.

### Key Improvements in v2.0

| Area | v1.0 Plan | v2.0 Enhanced Plan |
|------|-----------|-------------------|
| Draft Streaming | Basic method | **SSE/chunked streaming with callbacks** |
| Checklists | Missing | **`sendChecklist`, `editMessageChecklist`** |
| Suggested Posts | Missing | **`approveSuggestedPost`, `declineSuggestedPost`** |
| Verification | Missing | **4 verification methods** |
| Business Messages | Missing | **`readBusinessMessage`, `deleteBusinessMessages`** |
| Stars Transfer | Partial | **`transferBusinessAccountStars`** |
| Stories | Basic | **Added `repostStory`** |
| Gift Browsing | Combined | **Separate `getUserGifts`, `getChatGifts`, `getBusinessAccountGifts`** |
| Safety | Basic | **Value operation safeguards, no-retry defaults** |

---

## Tier 3 Complete Method List

### Payments & Stars (12 methods)
- `sendInvoice`, `createInvoiceLink`
- `answerShippingQuery`, `answerPreCheckoutQuery`
- `getStarTransactions`, `refundStarPayment`
- `editUserStarSubscription`, `setUserEmojiStatus`
- `sendPaidMedia`
- `getBusinessAccountStarBalance`, `transferBusinessAccountStars` ✨NEW

### Stickers (16 methods)
- `sendSticker`, `getStickerSet`, `getCustomEmojiStickers`
- `uploadStickerFile`, `createNewStickerSet`, `addStickerToSet`
- `setStickerPositionInSet`, `deleteStickerFromSet`, `replaceStickerInSet`
- `setStickerEmojiList`, `setStickerKeywords`, `setStickerMaskPosition`
- `setStickerSetTitle`, `setStickerSetThumbnail`
- `setCustomEmojiStickerSetThumbnail`, `deleteStickerSet`

### Games (3 methods)
- `sendGame`, `setGameScore`, `getGameHighScores`

### Gifts (10 methods) ✨EXPANDED
- `getAvailableGifts`, `sendGift`, `giftPremiumSubscription`
- `getUserGifts`, `getChatGifts` ✨NEW
- `getBusinessAccountGifts` ✨NEW
- `saveGift`, `convertGiftToStars`, `upgradeGift`, `transferGift`

### Business Account (14 methods) ✨EXPANDED
- `getBusinessConnection`
- `readBusinessMessage`, `deleteBusinessMessages` ✨NEW
- `setBusinessAccountName`, `setBusinessAccountUsername`, `setBusinessAccountBio`
- `setBusinessAccountProfilePhoto`, `removeBusinessAccountProfilePhoto`
- `setBusinessAccountGiftSettings`
- `getBusinessAccountStarBalance`, `transferBusinessAccountStars` ✨NEW
- `postStory`, `repostStory` ✨NEW, `editStory`, `deleteStory`

### Verification (4 methods) ✨NEW
- `verifyUser`, `verifyChat`
- `removeUserVerification`, `removeChatVerification`

### Suggested Posts (2 methods) ✨NEW
- `approveSuggestedPost`, `declineSuggestedPost`

### Checklists (2 methods) ✨NEW
- `sendChecklist`, `editMessageChecklist`

### Draft Streaming (1 method) ✨ENHANCED
- `sendMessageDraft` (streaming API with chunked response)

### Webhooks (3 methods)
- `setWebhook`, `deleteWebhook`, `getWebhookInfo`

### Passport (1 method)
- `setPassportDataErrors`

### Chat Boosts (2 methods)
- `getUserChatBoosts`, `getAvailableBoosts`

### Web Apps (2 methods)
- `answerWebAppQuery`, `getStory`

**TOTAL: ~72 methods** (up from 61 in v1.0)

---

## PR Dependency Graph

```
Tier 2 Complete
      │
      ▼
PR10 (Tier 3 Types + Safety) ◄──────────────────────────────────┐
      │                                                          │
      ├────────┬────────┬────────┬────────┬────────┐            │
      ▼        ▼        ▼        ▼        ▼        ▼            │
   PR11     PR12     PR13     PR14     PR15     PR16            │
(Payments)(Stickers)(Games)(Gifts)  (Verify)(Business)          │
      │        │        │        │        │        │            │
      └────────┴────────┴────────┴────────┴────────┘            │
                              │                                  │
              ┌───────────────┼───────────────┐                  │
              ▼               ▼               ▼                  │
           PR17            PR18            PR19                  │
        (Stories)    (Suggested Posts) (Checklists)              │
              │               │               │                  │
              └───────────────┴───────────────┘                  │
                              │                                  │
                              ▼                                  │
                    PR20 (Draft Streaming) ◄─────────────────────┤
                              │                                  │
                              ▼                                  │
                    PR21 (Webhooks)                              │
                              │                                  │
                              ▼                                  │
                    PR22 (Passport + Boosts)                     │
                              │                                  │
                              ▼                                  │
                    PR23 (Integration Tests)                     │
                              │                                  │
                              ▼                                  │
                    PR24 (Docs + Coverage Matrix)                │
                              │                                  │
                              ▼                                  │
                    PR25 (Release v2.2.0/v3.0.0)                 │
```

---

## PR10: Tier 3 Types + Safety Foundations

**Goal:** Define all types and safety rules for value-moving operations.  
**Estimated Time:** 10-12 hours  
**Breaking Changes:** None (additive)

### 10.1 Critical Safety Rules

**IMPORTANT:** Methods that spend Stars, send gifts, or transfer value must have special handling:

```go
// ValueOperation marks methods that move monetary value
type ValueOperation interface {
    isValueOperation()
}

// DefaultRetryPolicy for value operations: NO AUTOMATIC RETRY
// Users must explicitly opt-in with WithRetryValueOperations()
```

**Implementation Requirements:**

1. **No automatic retry** for value-moving operations by default
2. **Explicit opt-in** required: `WithAllowRetry()` option
3. **Structured logging redaction** - never log gift IDs + business_connection_id together
4. **Idempotency keys** where supported by the API

```go
// Example: Safe value operation call
err := client.SendGift(ctx, userID, giftID,
    sender.WithNoRetry(), // Explicit, but this should be default
)

// Example: Explicit retry opt-in (advanced users only)
err := client.SendGift(ctx, userID, giftID,
    sender.WithAllowRetry(3), // User explicitly accepts retry risk
)
```

### 10.2 New Types: Drafts

**File:** `tg/drafts.go` (new)

```go
package tg

// MessageDraft represents a draft message being streamed
type MessageDraft struct {
    Text     string          `json:"text,omitempty"`
    Entities []MessageEntity `json:"entities,omitempty"`
}

// DraftChunk represents a chunk in the draft streaming response
type DraftChunk struct {
    // Type indicates the chunk type: "text", "done", "error"
    Type string `json:"type"`
    
    // Text contains the incremental text for "text" chunks
    Text string `json:"text,omitempty"`
    
    // Message contains the final message for "done" chunks
    Message *Message `json:"message,omitempty"`
    
    // Error contains error info for "error" chunks
    Error string `json:"error,omitempty"`
}

// SuggestedPostParameters contains parameters for suggested posts
type SuggestedPostParameters struct {
    ChatID    ChatID `json:"chat_id"`
    SendDate  *int64 `json:"send_date,omitempty"` // Unix time, max 30 days in future
    Comment   string `json:"comment,omitempty"`
}
```

### 10.3 New Types: Checklists

**File:** `tg/checklists.go` (new)

```go
package tg

// InputChecklist represents a checklist to be sent
type InputChecklist struct {
    Title string               `json:"title"`           // 1-256 chars
    Items []InputChecklistItem `json:"items"`           // 1-100 items
}

// InputChecklistItem represents an item in a checklist
type InputChecklistItem struct {
    Text      string          `json:"text"`             // 1-1024 chars
    Completed bool            `json:"completed,omitempty"`
    Collapsed bool            `json:"collapsed,omitempty"`
    Items     []InputChecklistItem `json:"items,omitempty"` // Nested items
}

// Checklist represents a checklist in a message
type Checklist struct {
    Title string          `json:"title"`
    Items []ChecklistItem `json:"items"`
}

// ChecklistItem represents an item in a received checklist
type ChecklistItem struct {
    ID        string          `json:"id"`
    Text      string          `json:"text"`
    Completed bool            `json:"completed"`
    Collapsed bool            `json:"collapsed,omitempty"`
    Items     []ChecklistItem `json:"items,omitempty"`
}
```

### 10.4 New Types: Verification

**File:** `tg/verification.go` (new)

```go
package tg

// VerificationStatus represents a user/chat verification status
type VerificationStatus struct {
    IsVerified        bool   `json:"is_verified"`
    CustomDescription string `json:"custom_description,omitempty"` // 0-70 chars
}
```

### 10.5 Extended Gift Types

**File:** `tg/gifts.go` (update)

```go
// OwnedGifts represents a paginated list of owned gifts
type OwnedGifts struct {
    TotalCount int        `json:"total_count"`
    Gifts      []OwnedGift `json:"gifts"`
    NextOffset string     `json:"next_offset,omitempty"`
}

// GiftFilter contains filter parameters for gift queries
type GiftFilter struct {
    ExcludeUnsaved   bool `json:"exclude_unsaved,omitempty"`
    ExcludeSaved     bool `json:"exclude_saved,omitempty"`
    ExcludeUnlimited bool `json:"exclude_unlimited,omitempty"`
    ExcludeLimited   bool `json:"exclude_limited,omitempty"`
    ExcludeUnique    bool `json:"exclude_unique,omitempty"`
    SortByPrice      bool `json:"sort_by_price,omitempty"`
}

// AcceptedGiftTypes describes which gift types a business accepts
type AcceptedGiftTypes struct {
    UnlimitedGifts bool `json:"unlimited_gifts"`
    LimitedGifts   bool `json:"limited_gifts"`
    UniqueGifts    bool `json:"unique_gifts"`
    PremiumGifts   bool `json:"premium_gifts"`
}
```

### 10.6 Extended Story Types

**File:** `tg/stories.go` (update)

```go
// InputStoryContentPhoto represents a photo story
type InputStoryContentPhoto struct {
    Type  string    `json:"type"` // "photo"
    Photo InputFile `json:"photo"`
}

func (InputStoryContentPhoto) inputStoryContent() {}

// InputStoryContentVideo represents a video story
type InputStoryContentVideo struct {
    Type                string    `json:"type"` // "video"
    Video               InputFile `json:"video"`
    Duration            *float64  `json:"duration,omitempty"`
    CoverFrameTimestamp *float64  `json:"cover_frame_timestamp,omitempty"`
    IsAnimation         *bool     `json:"is_animation,omitempty"`
}

func (InputStoryContentVideo) inputStoryContent() {}

// StoryActivePeriods - allowed values for story active_period
const (
    StoryPeriod6Hours  = 21600  // 6 hours
    StoryPeriod12Hours = 43200  // 12 hours
    StoryPeriod24Hours = 86400  // 24 hours
    StoryPeriod48Hours = 172800 // 48 hours
)

// ValidStoryPeriods returns all valid story active periods
func ValidStoryPeriods() []int {
    return []int{StoryPeriod6Hours, StoryPeriod12Hours, StoryPeriod24Hours, StoryPeriod48Hours}
}
```

### 10.7 Star Amount Type

```go
// StarAmount represents a Telegram Stars amount
type StarAmount struct {
    Amount         int  `json:"amount"`
    NanostarAmount *int `json:"nanostar_amount,omitempty"` // Bot API 8.2
}

// TotalNanostars returns the total amount in nanostars
func (s StarAmount) TotalNanostars() int64 {
    total := int64(s.Amount) * 1_000_000_000
    if s.NanostarAmount != nil {
        total += int64(*s.NanostarAmount)
    }
    return total
}
```

### Definition of Done (PR10)

- [ ] All Tier 3 types defined
- [ ] Safety rules documented and implemented
- [ ] No automatic retry for value operations
- [ ] Idempotency strategy documented
- [ ] JSON round-trip tests for new types
- [ ] No `map[string]any` in exported signatures
- [ ] int64 for all IDs that can exceed 32-bit

---

## PR11: Payments & Telegram Stars

**Goal:** Full payment processing with enhanced Star operations.  
**Estimated Time:** 6-8 hours  
**Breaking Changes:** None (additive)

### Methods

| Method | Value Op? | Description |
|--------|-----------|-------------|
| `sendInvoice` | Yes | Send an invoice |
| `createInvoiceLink` | No | Create invoice link |
| `answerShippingQuery` | No | Answer shipping query |
| `answerPreCheckoutQuery` | Yes | Answer pre-checkout (commits payment) |
| `getStarTransactions` | No | Get transaction history |
| `refundStarPayment` | Yes | Refund a payment |
| `editUserStarSubscription` | Yes | Cancel/re-enable subscription |
| `setUserEmojiStatus` | No | Set user emoji status |
| `sendPaidMedia` | Yes | Send paid media |

### Implementation with Safety

```go
// RefundStarPayment refunds a successful payment in Telegram Stars.
// WARNING: This is a value operation. No automatic retry.
func (c *Client) RefundStarPayment(ctx context.Context, userID int64, telegramPaymentChargeID string, opts ...ValueOption) error {
    if userID == 0 {
        return tg.NewValidationError("user_id", "is required")
    }
    if telegramPaymentChargeID == "" {
        return tg.NewValidationError("telegram_payment_charge_id", "is required")
    }
    
    params := map[string]any{
        "user_id":                    userID,
        "telegram_payment_charge_id": telegramPaymentChargeID,
    }
    
    // Use no-retry executor for value operations
    return c.executor.CallNoRetry(ctx, "refundStarPayment", params, nil, opts...)
}

// ValueOption configures value operation behavior
type ValueOption func(*valueConfig)

type valueConfig struct {
    allowRetry bool
    maxRetries int
}

// WithAllowRetry explicitly opts into retry for value operations
// USE WITH CAUTION: May cause double-spends on network issues
func WithAllowRetry(maxRetries int) ValueOption {
    return func(c *valueConfig) {
        c.allowRetry = true
        c.maxRetries = maxRetries
    }
}
```

### Definition of Done (PR11)

- [ ] All 9 payment methods
- [ ] Value operation safety for refunds
- [ ] Subscription support (Bot API 7.9)
- [ ] Stars currency handling
- [ ] Tests without actual payments

---

## PR12: Stickers

**Goal:** Full sticker and sticker set management.  
**Estimated Time:** 6-8 hours  
**Breaking Changes:** None (additive)

(Same as v1.0 - 16 methods, no changes needed)

---

## PR13: Games

**Goal:** Game sending and score management.  
**Estimated Time:** 3-4 hours  
**Breaking Changes:** None (additive)

(Same as v1.0 - 3 methods)

---

## PR14: Gifts (Expanded)

**Goal:** Complete gift catalog, sending, and browsing.  
**Estimated Time:** 6-8 hours  
**Breaking Changes:** None (additive)

### Methods

| Method | Value Op? | Description |
|--------|-----------|-------------|
| `getAvailableGifts` | No | Get gift catalog |
| `sendGift` | Yes | Send a gift |
| `giftPremiumSubscription` | Yes | Gift premium subscription |
| `getUserGifts` | No | Get user's owned gifts ✨NEW |
| `getChatGifts` | No | Get chat's owned gifts ✨NEW |
| `getBusinessAccountGifts` | No | Get business account gifts ✨NEW |
| `saveGift` | No | Save/unsave gift |
| `convertGiftToStars` | Yes | Convert gift to Stars |
| `upgradeGift` | Yes | Upgrade gift |
| `transferGift` | Yes | Transfer unique gift |

### Implementation

**File:** `sender/methods_gifts.go` (new)

```go
package sender

import (
    "context"
    
    "github.com/example/galigo/tg"
)

// GetAvailableGifts returns the list of gifts that can be sent.
func (c *Client) GetAvailableGifts(ctx context.Context) (*tg.Gifts, error) {
    var gifts tg.Gifts
    if err := c.executor.Call(ctx, "getAvailableGifts", nil, &gifts); err != nil {
        return nil, err
    }
    return &gifts, nil
}

// SendGiftRequest contains parameters for sendGift
type SendGiftRequest struct {
    // Exactly one of UserID or ChatID must be set
    UserID        *int64          `json:"user_id,omitempty"`
    ChatID        *tg.ChatID      `json:"chat_id,omitempty"`
    GiftID        string          `json:"gift_id"`
    PayForUpgrade *bool           `json:"pay_for_upgrade,omitempty"`
    Text          string          `json:"text,omitempty"` // 0-255 chars
    TextParseMode tg.ParseMode    `json:"text_parse_mode,omitempty"`
    TextEntities  []tg.MessageEntity `json:"text_entities,omitempty"`
}

// Validate checks request validity
func (r SendGiftRequest) Validate() error {
    // Mutual exclusivity check
    hasUser := r.UserID != nil && *r.UserID != 0
    hasChat := r.ChatID != nil && !r.ChatID.IsZero()
    
    if !hasUser && !hasChat {
        return tg.NewValidationError("user_id/chat_id", "exactly one is required")
    }
    if hasUser && hasChat {
        return tg.NewValidationError("user_id/chat_id", "mutually exclusive, provide only one")
    }
    if r.GiftID == "" {
        return tg.NewValidationError("gift_id", "is required")
    }
    if len(r.Text) > 255 {
        return tg.NewValidationError("text", "must be at most 255 characters")
    }
    return nil
}

// SendGift sends a gift to a user or chat.
// WARNING: This is a value operation. No automatic retry.
func (c *Client) SendGift(ctx context.Context, req SendGiftRequest, opts ...ValueOption) error {
    if err := req.Validate(); err != nil {
        return err
    }
    
    return c.executor.CallNoRetry(ctx, "sendGift", req, nil, opts...)
}

// GiftPremiumSubscription gifts a Telegram Premium subscription.
// WARNING: This is a value operation. No automatic retry.
func (c *Client) GiftPremiumSubscription(ctx context.Context, userID int64, monthCount, starCount int, opts ...GiftOption) error {
    if userID == 0 {
        return tg.NewValidationError("user_id", "is required")
    }
    
    // Validate allowed month/star combinations
    if !isValidPremiumGiftCombo(monthCount, starCount) {
        return tg.NewValidationError("month_count/star_count", "invalid combination")
    }
    
    req := struct {
        UserID     int64  `json:"user_id"`
        MonthCount int    `json:"month_count"`
        StarCount  int    `json:"star_count"`
        Text       string `json:"text,omitempty"`
        TextParseMode tg.ParseMode `json:"text_parse_mode,omitempty"`
        TextEntities []tg.MessageEntity `json:"text_entities,omitempty"`
    }{
        UserID:     userID,
        MonthCount: monthCount,
        StarCount:  starCount,
    }
    
    for _, opt := range opts {
        opt(&req)
    }
    
    return c.executor.CallNoRetry(ctx, "giftPremiumSubscription", req, nil)
}

// isValidPremiumGiftCombo validates month/star combinations
func isValidPremiumGiftCombo(months, stars int) bool {
    // Known valid combinations as of Bot API 9.3
    validCombos := map[int][]int{
        1:  {500},
        3:  {1000},
        6:  {1500},
        12: {2500},
    }
    
    allowed, ok := validCombos[months]
    if !ok {
        return false
    }
    for _, s := range allowed {
        if stars == s {
            return true
        }
    }
    return false
}

// GetUserGiftsRequest contains parameters for getUserGifts
type GetUserGiftsRequest struct {
    UserID int64         `json:"user_id"`
    Filter *tg.GiftFilter `json:"filter,omitempty"`
    Offset string        `json:"offset,omitempty"`
    Limit  int           `json:"limit,omitempty"` // 1-100, default 100
}

// GetUserGifts returns gifts owned by a user.
func (c *Client) GetUserGifts(ctx context.Context, req GetUserGiftsRequest) (*tg.OwnedGifts, error) {
    if req.UserID == 0 {
        return nil, tg.NewValidationError("user_id", "is required")
    }
    if req.Limit < 0 || req.Limit > 100 {
        return nil, tg.NewValidationError("limit", "must be 1-100")
    }
    
    var gifts tg.OwnedGifts
    if err := c.executor.Call(ctx, "getUserGifts", req, &gifts); err != nil {
        return nil, err
    }
    return &gifts, nil
}

// GetChatGiftsRequest contains parameters for getChatGifts
type GetChatGiftsRequest struct {
    ChatID tg.ChatID     `json:"chat_id"`
    Filter *tg.GiftFilter `json:"filter,omitempty"`
    Offset string        `json:"offset,omitempty"`
    Limit  int           `json:"limit,omitempty"`
}

// GetChatGifts returns gifts owned by a chat.
func (c *Client) GetChatGifts(ctx context.Context, req GetChatGiftsRequest) (*tg.OwnedGifts, error) {
    if req.ChatID.IsZero() {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    
    var gifts tg.OwnedGifts
    if err := c.executor.Call(ctx, "getChatGifts", req, &gifts); err != nil {
        return nil, err
    }
    return &gifts, nil
}

// ConvertGiftToStars converts a gift to Telegram Stars.
// WARNING: This is a value operation. No automatic retry.
func (c *Client) ConvertGiftToStars(ctx context.Context, businessConnectionID, ownedGiftID string, opts ...ValueOption) error {
    if businessConnectionID == "" {
        return tg.NewValidationError("business_connection_id", "is required")
    }
    if ownedGiftID == "" {
        return tg.NewValidationError("owned_gift_id", "is required")
    }
    
    params := map[string]any{
        "business_connection_id": businessConnectionID,
        "owned_gift_id":          ownedGiftID,
    }
    
    return c.executor.CallNoRetry(ctx, "convertGiftToStars", params, nil, opts...)
}

// UpgradeGift upgrades a gift to a unique one.
// WARNING: This is a value operation. No automatic retry.
func (c *Client) UpgradeGift(ctx context.Context, businessConnectionID, ownedGiftID string, keepOriginalDetails *bool, starCount *int, opts ...ValueOption) error {
    if businessConnectionID == "" {
        return tg.NewValidationError("business_connection_id", "is required")
    }
    if ownedGiftID == "" {
        return tg.NewValidationError("owned_gift_id", "is required")
    }
    
    params := map[string]any{
        "business_connection_id": businessConnectionID,
        "owned_gift_id":          ownedGiftID,
    }
    if keepOriginalDetails != nil {
        params["keep_original_details"] = *keepOriginalDetails
    }
    if starCount != nil {
        params["star_count"] = *starCount
    }
    
    return c.executor.CallNoRetry(ctx, "upgradeGift", params, nil, opts...)
}

// TransferGift transfers a unique gift to another user.
// WARNING: This is a value operation. No automatic retry.
func (c *Client) TransferGift(ctx context.Context, businessConnectionID, ownedGiftID string, newOwnerChatID tg.ChatID, starCount *int, opts ...ValueOption) error {
    if businessConnectionID == "" {
        return tg.NewValidationError("business_connection_id", "is required")
    }
    if ownedGiftID == "" {
        return tg.NewValidationError("owned_gift_id", "is required")
    }
    if newOwnerChatID.IsZero() {
        return tg.NewValidationError("new_owner_chat_id", "is required")
    }
    
    params := map[string]any{
        "business_connection_id": businessConnectionID,
        "owned_gift_id":          ownedGiftID,
        "new_owner_chat_id":      newOwnerChatID,
    }
    if starCount != nil {
        params["star_count"] = *starCount
    }
    
    return c.executor.CallNoRetry(ctx, "transferGift", params, nil, opts...)
}
```

### Definition of Done (PR14)

- [ ] All 10 gift methods
- [ ] `sendGift` mutual exclusivity validation
- [ ] `giftPremiumSubscription` combo validation
- [ ] Gift browsing with filters and pagination
- [ ] Value operation safety for spending methods
- [ ] Tests for validation logic

---

## PR15: Verification APIs ✨NEW

**Goal:** User and chat verification management.  
**Estimated Time:** 2-3 hours  
**Breaking Changes:** None (additive)

### Methods

| Method | Description |
|--------|-------------|
| `verifyUser` | Verify a user |
| `verifyChat` | Verify a chat |
| `removeUserVerification` | Remove user verification |
| `removeChatVerification` | Remove chat verification |

### Implementation

**File:** `sender/methods_verification.go` (new)

```go
package sender

import (
    "context"
    
    "github.com/example/galigo/tg"
)

// VerifyUser verifies a user on behalf of the organization.
// Requires appropriate bot permissions.
func (c *Client) VerifyUser(ctx context.Context, userID int64, customDescription string) error {
    if userID == 0 {
        return tg.NewValidationError("user_id", "is required")
    }
    if len(customDescription) > 70 {
        return tg.NewValidationError("custom_description", "must be at most 70 characters")
    }
    
    params := map[string]any{"user_id": userID}
    if customDescription != "" {
        params["custom_description"] = customDescription
    }
    
    return c.executor.Call(ctx, "verifyUser", params, nil)
}

// VerifyChat verifies a chat on behalf of the organization.
// Requires appropriate bot permissions.
func (c *Client) VerifyChat(ctx context.Context, chatID tg.ChatID, customDescription string) error {
    if chatID.IsZero() {
        return tg.NewValidationError("chat_id", "is required")
    }
    if len(customDescription) > 70 {
        return tg.NewValidationError("custom_description", "must be at most 70 characters")
    }
    
    params := map[string]any{"chat_id": chatID}
    if customDescription != "" {
        params["custom_description"] = customDescription
    }
    
    return c.executor.Call(ctx, "verifyChat", params, nil)
}

// RemoveUserVerification removes verification from a user.
func (c *Client) RemoveUserVerification(ctx context.Context, userID int64) error {
    if userID == 0 {
        return tg.NewValidationError("user_id", "is required")
    }
    
    params := map[string]any{"user_id": userID}
    return c.executor.Call(ctx, "removeUserVerification", params, nil)
}

// RemoveChatVerification removes verification from a chat.
func (c *Client) RemoveChatVerification(ctx context.Context, chatID tg.ChatID) error {
    if chatID.IsZero() {
        return tg.NewValidationError("chat_id", "is required")
    }
    
    params := map[string]any{"chat_id": chatID}
    return c.executor.Call(ctx, "removeChatVerification", params, nil)
}
```

### Definition of Done (PR15)

- [ ] All 4 verification methods
- [ ] Custom description length validation (0-70 chars)
- [ ] Documentation about required permissions
- [ ] Tests

---

## PR16: Business Account (Expanded)

**Goal:** Complete business account management.  
**Estimated Time:** 8-10 hours  
**Breaking Changes:** None (additive)

### Methods

| Method | Value Op? | Description |
|--------|-----------|-------------|
| `getBusinessConnection` | No | Get connection info |
| `readBusinessMessage` | No | Mark message as read ✨NEW |
| `deleteBusinessMessages` | No | Delete messages ✨NEW |
| `setBusinessAccountName` | No | Set name |
| `setBusinessAccountUsername` | No | Set username |
| `setBusinessAccountBio` | No | Set bio |
| `setBusinessAccountProfilePhoto` | No | Set photo |
| `removeBusinessAccountProfilePhoto` | No | Remove photo |
| `setBusinessAccountGiftSettings` | No | Set gift settings |
| `getBusinessAccountStarBalance` | No | Get Stars balance |
| `transferBusinessAccountStars` | Yes | Transfer Stars ✨NEW |
| `getBusinessAccountGifts` | No | Get gifts ✨NEW |

### Implementation

**File:** `sender/methods_business.go` (new)

```go
package sender

import (
    "context"
    
    "github.com/example/galigo/tg"
)

// GetBusinessConnection returns information about a business connection.
func (c *Client) GetBusinessConnection(ctx context.Context, businessConnectionID string) (*tg.BusinessConnection, error) {
    if businessConnectionID == "" {
        return nil, tg.NewValidationError("business_connection_id", "is required")
    }
    
    params := map[string]any{"business_connection_id": businessConnectionID}
    
    var conn tg.BusinessConnection
    if err := c.executor.Call(ctx, "getBusinessConnection", params, &conn); err != nil {
        return nil, err
    }
    return &conn, nil
}

// ReadBusinessMessage marks a message as read in a business chat.
// Note: chat_id must have been active in last 24 hours (server enforced).
func (c *Client) ReadBusinessMessage(ctx context.Context, businessConnectionID string, chatID tg.ChatID, messageID int) error {
    if businessConnectionID == "" {
        return tg.NewValidationError("business_connection_id", "is required")
    }
    if chatID.IsZero() {
        return tg.NewValidationError("chat_id", "is required")
    }
    if messageID == 0 {
        return tg.NewValidationError("message_id", "is required")
    }
    
    params := map[string]any{
        "business_connection_id": businessConnectionID,
        "chat_id":                chatID,
        "message_id":             messageID,
    }
    
    return c.executor.Call(ctx, "readBusinessMessage", params, nil)
}

// DeleteBusinessMessages deletes messages in a business chat.
// message_ids must contain 1-100 message IDs.
func (c *Client) DeleteBusinessMessages(ctx context.Context, businessConnectionID string, messageIDs []int) error {
    if businessConnectionID == "" {
        return tg.NewValidationError("business_connection_id", "is required")
    }
    if len(messageIDs) < 1 || len(messageIDs) > 100 {
        return tg.NewValidationError("message_ids", "must have 1-100 message IDs")
    }
    
    params := map[string]any{
        "business_connection_id": businessConnectionID,
        "message_ids":            messageIDs,
    }
    
    return c.executor.Call(ctx, "deleteBusinessMessages", params, nil)
}

// SetBusinessAccountName sets the name of a business account.
// first_name: 1-64 chars, last_name: 0-64 chars
func (c *Client) SetBusinessAccountName(ctx context.Context, businessConnectionID, firstName, lastName string) error {
    if businessConnectionID == "" {
        return tg.NewValidationError("business_connection_id", "is required")
    }
    if firstName == "" || len(firstName) > 64 {
        return tg.NewValidationError("first_name", "must be 1-64 characters")
    }
    if len(lastName) > 64 {
        return tg.NewValidationError("last_name", "must be at most 64 characters")
    }
    
    params := map[string]any{
        "business_connection_id": businessConnectionID,
        "first_name":             firstName,
    }
    if lastName != "" {
        params["last_name"] = lastName
    }
    
    return c.executor.Call(ctx, "setBusinessAccountName", params, nil)
}

// SetBusinessAccountUsername sets the username of a business account.
// username: 0-32 chars (empty to remove)
func (c *Client) SetBusinessAccountUsername(ctx context.Context, businessConnectionID, username string) error {
    if businessConnectionID == "" {
        return tg.NewValidationError("business_connection_id", "is required")
    }
    if len(username) > 32 {
        return tg.NewValidationError("username", "must be at most 32 characters")
    }
    
    params := map[string]any{
        "business_connection_id": businessConnectionID,
    }
    if username != "" {
        params["username"] = username
    }
    
    return c.executor.Call(ctx, "setBusinessAccountUsername", params, nil)
}

// SetBusinessAccountBio sets the bio of a business account.
// bio: 0-140 chars (empty to remove)
func (c *Client) SetBusinessAccountBio(ctx context.Context, businessConnectionID, bio string) error {
    if businessConnectionID == "" {
        return tg.NewValidationError("business_connection_id", "is required")
    }
    if len(bio) > 140 {
        return tg.NewValidationError("bio", "must be at most 140 characters")
    }
    
    params := map[string]any{
        "business_connection_id": businessConnectionID,
    }
    if bio != "" {
        params["bio"] = bio
    }
    
    return c.executor.Call(ctx, "setBusinessAccountBio", params, nil)
}

// SetBusinessAccountProfilePhoto sets the profile photo.
func (c *Client) SetBusinessAccountProfilePhoto(ctx context.Context, businessConnectionID string, photo tg.InputFile, isPublic *bool) error {
    if businessConnectionID == "" {
        return tg.NewValidationError("business_connection_id", "is required")
    }
    
    params := map[string]any{
        "business_connection_id": businessConnectionID,
    }
    if isPublic != nil {
        params["is_public"] = *isPublic
    }
    
    if photo.IsUpload() {
        files := []FilePart{{
            FieldName: "photo",
            FileName:  photo.GetUpload().Name,
            Reader:    photo.GetUpload().Reader,
        }}
        return c.executor.CallMultipart(ctx, "setBusinessAccountProfilePhoto", params, files, nil)
    }
    
    params["photo"] = photo.GetValue()
    return c.executor.Call(ctx, "setBusinessAccountProfilePhoto", params, nil)
}

// RemoveBusinessAccountProfilePhoto removes the profile photo.
func (c *Client) RemoveBusinessAccountProfilePhoto(ctx context.Context, businessConnectionID string, isPublic *bool) error {
    if businessConnectionID == "" {
        return tg.NewValidationError("business_connection_id", "is required")
    }
    
    params := map[string]any{
        "business_connection_id": businessConnectionID,
    }
    if isPublic != nil {
        params["is_public"] = *isPublic
    }
    
    return c.executor.Call(ctx, "removeBusinessAccountProfilePhoto", params, nil)
}

// SetBusinessAccountGiftSettings sets gift settings.
func (c *Client) SetBusinessAccountGiftSettings(ctx context.Context, businessConnectionID string, showGiftButton bool, acceptedGiftTypes *tg.AcceptedGiftTypes) error {
    if businessConnectionID == "" {
        return tg.NewValidationError("business_connection_id", "is required")
    }
    
    params := map[string]any{
        "business_connection_id": businessConnectionID,
        "show_gift_button":       showGiftButton,
    }
    if acceptedGiftTypes != nil {
        params["accepted_gift_types"] = acceptedGiftTypes
    }
    
    return c.executor.Call(ctx, "setBusinessAccountGiftSettings", params, nil)
}

// GetBusinessAccountStarBalance returns the Stars balance.
func (c *Client) GetBusinessAccountStarBalance(ctx context.Context, businessConnectionID string) (*tg.StarAmount, error) {
    if businessConnectionID == "" {
        return nil, tg.NewValidationError("business_connection_id", "is required")
    }
    
    params := map[string]any{
        "business_connection_id": businessConnectionID,
    }
    
    var balance tg.StarAmount
    if err := c.executor.Call(ctx, "getBusinessAccountStarBalance", params, &balance); err != nil {
        return nil, err
    }
    return &balance, nil
}

// TransferBusinessAccountStars transfers Stars from business account to bot.
// WARNING: This is a value operation. No automatic retry.
func (c *Client) TransferBusinessAccountStars(ctx context.Context, businessConnectionID string, starCount int, opts ...ValueOption) error {
    if businessConnectionID == "" {
        return tg.NewValidationError("business_connection_id", "is required")
    }
    if starCount < 1 || starCount > 10000 {
        return tg.NewValidationError("star_count", "must be 1-10000")
    }
    
    params := map[string]any{
        "business_connection_id": businessConnectionID,
        "star_count":             starCount,
    }
    
    return c.executor.CallNoRetry(ctx, "transferBusinessAccountStars", params, nil, opts...)
}

// GetBusinessAccountGiftsRequest contains parameters for getBusinessAccountGifts
type GetBusinessAccountGiftsRequest struct {
    BusinessConnectionID string        `json:"business_connection_id"`
    Filter               *tg.GiftFilter `json:"filter,omitempty"`
    Offset               string        `json:"offset,omitempty"`
    Limit                int           `json:"limit,omitempty"`
}

// GetBusinessAccountGifts returns gifts owned by a business account.
func (c *Client) GetBusinessAccountGifts(ctx context.Context, req GetBusinessAccountGiftsRequest) (*tg.OwnedGifts, error) {
    if req.BusinessConnectionID == "" {
        return nil, tg.NewValidationError("business_connection_id", "is required")
    }
    
    var gifts tg.OwnedGifts
    if err := c.executor.Call(ctx, "getBusinessAccountGifts", req, &gifts); err != nil {
        return nil, err
    }
    return &gifts, nil
}
```

### Definition of Done (PR16)

- [ ] All 12 business methods
- [ ] `readBusinessMessage` implemented
- [ ] `deleteBusinessMessages` with 1-100 validation
- [ ] Profile field length validation
- [ ] `transferBusinessAccountStars` with value safety
- [ ] Integration test example documented

---

## PR17: Stories (Expanded)

**Goal:** Story posting and management.  
**Estimated Time:** 4-5 hours  
**Breaking Changes:** None (additive)

### Methods

| Method | Description |
|--------|-------------|
| `postStory` | Post a new story |
| `repostStory` | Repost existing story ✨NEW |
| `editStory` | Edit a story |
| `deleteStory` | Delete a story |
| `getStory` | Get a story |

### Implementation

```go
// PostStoryRequest contains parameters for postStory
type PostStoryRequest struct {
    BusinessConnectionID string               `json:"business_connection_id"`
    Content              tg.InputStoryContent `json:"content"`
    ActivePeriod         int                  `json:"active_period"` // Must be valid period
    Caption              string               `json:"caption,omitempty"`
    ParseMode            tg.ParseMode         `json:"parse_mode,omitempty"`
    CaptionEntities      []tg.MessageEntity   `json:"caption_entities,omitempty"`
    Areas                []tg.StoryArea       `json:"areas,omitempty"`
    PostToChatPage       *bool                `json:"post_to_chat_page,omitempty"`
    ProtectContent       *bool                `json:"protect_content,omitempty"`
}

// PostStory posts a story on behalf of a business account.
func (c *Client) PostStory(ctx context.Context, req PostStoryRequest) (*tg.Story, error) {
    if req.BusinessConnectionID == "" {
        return nil, tg.NewValidationError("business_connection_id", "is required")
    }
    if req.Content == nil {
        return nil, tg.NewValidationError("content", "is required")
    }
    if !isValidStoryPeriod(req.ActivePeriod) {
        return nil, tg.NewValidationError("active_period", "must be one of: 21600, 43200, 86400, 172800")
    }
    
    var story tg.Story
    if err := c.executeStoryRequest(ctx, "postStory", req, &story); err != nil {
        return nil, err
    }
    return &story, nil
}

func isValidStoryPeriod(period int) bool {
    for _, p := range tg.ValidStoryPeriods() {
        if period == p {
            return true
        }
    }
    return false
}

// RepostStoryRequest contains parameters for repostStory
type RepostStoryRequest struct {
    BusinessConnectionID string             `json:"business_connection_id"`
    FromChatID           tg.ChatID          `json:"from_chat_id"`
    FromStoryID          int                `json:"from_story_id"`
    ActivePeriod         int                `json:"active_period"`
    Caption              string             `json:"caption,omitempty"`
    ParseMode            tg.ParseMode       `json:"parse_mode,omitempty"`
    CaptionEntities      []tg.MessageEntity `json:"caption_entities,omitempty"`
    Areas                []tg.StoryArea     `json:"areas,omitempty"`
    PostToChatPage       *bool              `json:"post_to_chat_page,omitempty"`
    ProtectContent       *bool              `json:"protect_content,omitempty"`
}

// RepostStory reposts an existing story.
// Note: Requires specific business rights (server enforced).
func (c *Client) RepostStory(ctx context.Context, req RepostStoryRequest) (*tg.Story, error) {
    if req.BusinessConnectionID == "" {
        return nil, tg.NewValidationError("business_connection_id", "is required")
    }
    if req.FromChatID.IsZero() {
        return nil, tg.NewValidationError("from_chat_id", "is required")
    }
    if req.FromStoryID == 0 {
        return nil, tg.NewValidationError("from_story_id", "is required")
    }
    if !isValidStoryPeriod(req.ActivePeriod) {
        return nil, tg.NewValidationError("active_period", "must be one of: 21600, 43200, 86400, 172800")
    }
    
    var story tg.Story
    if err := c.executor.Call(ctx, "repostStory", req, &story); err != nil {
        return nil, err
    }
    return &story, nil
}

// EditStory, DeleteStory, GetStory... (same as v1.0)
```

### Definition of Done (PR17)

- [ ] All 5 story methods
- [ ] `repostStory` implemented
- [ ] `active_period` validation
- [ ] Media upload for story content
- [ ] Story areas support
- [ ] Tests

---

## PR18: Suggested Posts ✨NEW

**Goal:** Channel suggested post moderation.  
**Estimated Time:** 3-4 hours  
**Breaking Changes:** None (additive)

### Methods

| Method | Description |
|--------|-------------|
| `approveSuggestedPost` | Approve and schedule post |
| `declineSuggestedPost` | Decline a suggested post |

### Implementation

**File:** `sender/methods_suggested_posts.go` (new)

```go
package sender

import (
    "context"
    "time"
    
    "github.com/example/galigo/tg"
)

// ApproveSuggestedPost approves a suggested post and schedules it.
// send_date must not be more than 30 days in the future.
func (c *Client) ApproveSuggestedPost(ctx context.Context, chatID tg.ChatID, messageID int, sendDate *time.Time) error {
    if chatID.IsZero() {
        return tg.NewValidationError("chat_id", "is required")
    }
    if messageID == 0 {
        return tg.NewValidationError("message_id", "is required")
    }
    
    params := map[string]any{
        "chat_id":    chatID,
        "message_id": messageID,
    }
    
    if sendDate != nil {
        // Validate: not more than 30 days in future
        maxDate := time.Now().AddDate(0, 0, 30)
        if sendDate.After(maxDate) {
            return tg.NewValidationError("send_date", "must not be more than 30 days in the future")
        }
        params["send_date"] = sendDate.Unix()
    }
    
    return c.executor.Call(ctx, "approveSuggestedPost", params, nil)
}

// DeclineSuggestedPost declines a suggested post.
func (c *Client) DeclineSuggestedPost(ctx context.Context, chatID tg.ChatID, messageID int, comment string) error {
    if chatID.IsZero() {
        return tg.NewValidationError("chat_id", "is required")
    }
    if messageID == 0 {
        return tg.NewValidationError("message_id", "is required")
    }
    
    params := map[string]any{
        "chat_id":    chatID,
        "message_id": messageID,
    }
    if comment != "" {
        params["comment"] = comment
    }
    
    return c.executor.Call(ctx, "declineSuggestedPost", params, nil)
}
```

### Definition of Done (PR18)

- [ ] `approveSuggestedPost` with 30-day validation
- [ ] `declineSuggestedPost` with optional comment
- [ ] Documentation about admin requirements
- [ ] Tests

---

## PR19: Checklists ✨NEW

**Goal:** Checklist messages for business accounts.  
**Estimated Time:** 4-5 hours  
**Breaking Changes:** None (additive)

### Methods

| Method | Description |
|--------|-------------|
| `sendChecklist` | Send a checklist message |
| `editMessageChecklist` | Edit a checklist message |

### Implementation

**File:** `sender/methods_checklists.go` (new)

```go
package sender

import (
    "context"
    
    "github.com/example/galigo/tg"
)

// SendChecklistRequest contains parameters for sendChecklist
type SendChecklistRequest struct {
    BusinessConnectionID string              `json:"business_connection_id"`
    ChatID               tg.ChatID           `json:"chat_id"`
    Checklist            tg.InputChecklist   `json:"checklist"`
    MessageThreadID      *int                `json:"message_thread_id,omitempty"`
    DisableNotification  *bool               `json:"disable_notification,omitempty"`
    ProtectContent       *bool               `json:"protect_content,omitempty"`
    MessageEffectID      string              `json:"message_effect_id,omitempty"`
    ReplyParameters      *tg.ReplyParameters `json:"reply_parameters,omitempty"`
    ReplyMarkup          any                 `json:"reply_markup,omitempty"`
}

// SendChecklist sends a checklist message.
func (c *Client) SendChecklist(ctx context.Context, req SendChecklistRequest) (*tg.Message, error) {
    if req.BusinessConnectionID == "" {
        return nil, tg.NewValidationError("business_connection_id", "is required")
    }
    if req.ChatID.IsZero() {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    
    // Validate checklist
    if err := validateChecklist(req.Checklist); err != nil {
        return nil, err
    }
    
    var msg tg.Message
    if err := c.executor.Call(ctx, "sendChecklist", req, &msg); err != nil {
        return nil, err
    }
    return &msg, nil
}

func validateChecklist(cl tg.InputChecklist) error {
    if cl.Title == "" || len(cl.Title) > 256 {
        return tg.NewValidationError("checklist.title", "must be 1-256 characters")
    }
    if len(cl.Items) < 1 || len(cl.Items) > 100 {
        return tg.NewValidationError("checklist.items", "must have 1-100 items")
    }
    
    for i, item := range cl.Items {
        if item.Text == "" || len(item.Text) > 1024 {
            return tg.NewValidationError("checklist.items", "item %d text must be 1-1024 characters", i)
        }
        // Recursively validate nested items
        for j, nested := range item.Items {
            if nested.Text == "" || len(nested.Text) > 1024 {
                return tg.NewValidationError("checklist.items", "item %d.%d text must be 1-1024 characters", i, j)
            }
        }
    }
    
    return nil
}

// EditMessageChecklistRequest contains parameters for editMessageChecklist
type EditMessageChecklistRequest struct {
    BusinessConnectionID string            `json:"business_connection_id"`
    ChatID               tg.ChatID         `json:"chat_id"`
    MessageID            int               `json:"message_id"`
    Checklist            tg.InputChecklist `json:"checklist"`
    ReplyMarkup          any               `json:"reply_markup,omitempty"`
}

// EditMessageChecklist edits a checklist message.
func (c *Client) EditMessageChecklist(ctx context.Context, req EditMessageChecklistRequest) (*tg.Message, error) {
    if req.BusinessConnectionID == "" {
        return nil, tg.NewValidationError("business_connection_id", "is required")
    }
    if req.ChatID.IsZero() {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    if req.MessageID == 0 {
        return nil, tg.NewValidationError("message_id", "is required")
    }
    
    if err := validateChecklist(req.Checklist); err != nil {
        return nil, err
    }
    
    var msg tg.Message
    if err := c.executor.Call(ctx, "editMessageChecklist", req, &msg); err != nil {
        return nil, err
    }
    return &msg, nil
}
```

### Definition of Done (PR19)

- [ ] `sendChecklist` with validation
- [ ] `editMessageChecklist`
- [ ] Nested item support
- [ ] Tests

---

## PR20: Draft Streaming (sendMessageDraft) ✨CRITICAL

**Goal:** Streaming draft message API for AI-powered bots.  
**Estimated Time:** 8-10 hours  
**Breaking Changes:** None (additive)  
**Complexity:** HIGH - requires streaming response handling

### The Challenge

`sendMessageDraft` is **not** a simple request/response API. It returns a **streaming response** (SSE/chunked) with partial text chunks as the draft is being generated. This is the "hardest" Tier 3 piece.

### API Design

```go
// High-level API with callback
func (c *Client) SendMessageDraft(
    ctx context.Context,
    req SendMessageDraftRequest,
    onChunk func(tg.DraftChunk) error,
) (*tg.Message, error)

// Low-level API for advanced users
func (c *Client) SendMessageDraftStream(
    ctx context.Context,
    req SendMessageDraftRequest,
) (io.ReadCloser, error)
```

### Implementation

**File:** `sender/methods_draft.go` (new)

```go
package sender

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
    
    "github.com/example/galigo/tg"
)

// SendMessageDraftRequest contains parameters for sendMessageDraft
type SendMessageDraftRequest struct {
    BusinessConnectionID    string                       `json:"business_connection_id,omitempty"`
    ChatID                  tg.ChatID                    `json:"chat_id"`
    Text                    string                       `json:"text"`
    ParseMode               tg.ParseMode                 `json:"parse_mode,omitempty"`
    Entities                []tg.MessageEntity           `json:"entities,omitempty"`
    LinkPreviewOptions      *tg.LinkPreviewOptions       `json:"link_preview_options,omitempty"`
    MessageThreadID         *int                         `json:"message_thread_id,omitempty"`
    DirectMessagesTopicID   *int                         `json:"direct_messages_topic_id,omitempty"`
    DisableNotification     *bool                        `json:"disable_notification,omitempty"`
    ProtectContent          *bool                        `json:"protect_content,omitempty"`
    MessageEffectID         string                       `json:"message_effect_id,omitempty"`
    ReplyParameters         *tg.ReplyParameters          `json:"reply_parameters,omitempty"`
    ReplyMarkup             any                          `json:"reply_markup,omitempty"`
    SuggestedPostParameters *tg.SuggestedPostParameters  `json:"suggested_post_parameters,omitempty"`
}

// DraftChunkCallback is called for each chunk in the draft stream
type DraftChunkCallback func(chunk tg.DraftChunk) error

// SendMessageDraft sends a message draft with streaming response.
// The onChunk callback is called for each chunk received.
// Returns the final message when the draft is complete.
//
// Example:
//
//	msg, err := client.SendMessageDraft(ctx, req, func(chunk tg.DraftChunk) error {
//	    if chunk.Type == "text" {
//	        fmt.Print(chunk.Text) // Print incremental text
//	    }
//	    return nil
//	})
func (c *Client) SendMessageDraft(
    ctx context.Context,
    req SendMessageDraftRequest,
    onChunk DraftChunkCallback,
) (*tg.Message, error) {
    if req.ChatID.IsZero() {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    if req.Text == "" {
        return nil, tg.NewValidationError("text", "is required")
    }
    
    // Get the streaming response
    stream, err := c.SendMessageDraftStream(ctx, req)
    if err != nil {
        return nil, err
    }
    defer stream.Close()
    
    // Process the stream
    return c.processDraftStream(ctx, stream, onChunk)
}

// SendMessageDraftStream returns a raw stream for advanced usage.
// The caller is responsible for reading and closing the stream.
func (c *Client) SendMessageDraftStream(
    ctx context.Context,
    req SendMessageDraftRequest,
) (io.ReadCloser, error) {
    if req.ChatID.IsZero() {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    if req.Text == "" {
        return nil, tg.NewValidationError("text", "is required")
    }
    
    // Build the request
    body, err := json.Marshal(req)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal request: %w", err)
    }
    
    url := c.buildURL("sendMessageDraft")
    httpReq, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(body)))
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }
    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Accept", "text/event-stream") // SSE
    
    // Send the request (don't use standard executor - we need the raw response)
    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("failed to send request: %w", err)
    }
    
    if resp.StatusCode != http.StatusOK {
        defer resp.Body.Close()
        // Try to parse error response
        var apiErr tg.APIError
        if err := json.NewDecoder(resp.Body).Decode(&apiErr); err == nil {
            return nil, &apiErr
        }
        return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }
    
    return resp.Body, nil
}

// processDraftStream reads SSE events and calls the callback
func (c *Client) processDraftStream(
    ctx context.Context,
    stream io.Reader,
    onChunk DraftChunkCallback,
) (*tg.Message, error) {
    scanner := bufio.NewScanner(stream)
    scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 1MB max line
    
    var finalMessage *tg.Message
    var dataBuffer strings.Builder
    
    for scanner.Scan() {
        // Check for context cancellation
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        default:
        }
        
        line := scanner.Text()
        
        // SSE format: "data: {...}\n\n"
        if strings.HasPrefix(line, "data: ") {
            data := strings.TrimPrefix(line, "data: ")
            dataBuffer.WriteString(data)
        } else if line == "" && dataBuffer.Len() > 0 {
            // End of event - parse the accumulated data
            var chunk tg.DraftChunk
            if err := json.Unmarshal([]byte(dataBuffer.String()), &chunk); err != nil {
                dataBuffer.Reset()
                continue // Skip malformed chunks
            }
            dataBuffer.Reset()
            
            // Call the callback
            if onChunk != nil {
                if err := onChunk(chunk); err != nil {
                    return nil, fmt.Errorf("callback error: %w", err)
                }
            }
            
            // Check for completion
            if chunk.Type == "done" {
                finalMessage = chunk.Message
            } else if chunk.Type == "error" {
                return nil, fmt.Errorf("draft error: %s", chunk.Error)
            }
        }
    }
    
    if err := scanner.Err(); err != nil {
        return nil, fmt.Errorf("stream read error: %w", err)
    }
    
    if finalMessage == nil {
        return nil, fmt.Errorf("stream ended without final message")
    }
    
    return finalMessage, nil
}
```

### Usage Example

```go
// Example: AI chatbot with streaming response
func handleUserMessage(ctx context.Context, bot *galigo.Bot, chatID int64, userText string) error {
    // Generate AI response text (your AI model)
    aiResponse := generateAIResponse(userText)
    
    // Send as streaming draft
    msg, err := bot.SendMessageDraft(ctx, sender.SendMessageDraftRequest{
        ChatID: tg.ChatIDFromInt64(chatID),
        Text:   aiResponse,
    }, func(chunk tg.DraftChunk) error {
        // Optional: log progress
        if chunk.Type == "text" {
            log.Printf("Chunk: %s", chunk.Text)
        }
        return nil
    })
    
    if err != nil {
        return fmt.Errorf("failed to send draft: %w", err)
    }
    
    log.Printf("Final message ID: %d", msg.MessageID)
    return nil
}
```

### Tests

```go
func TestSendMessageDraft_Streaming(t *testing.T) {
    // Create a fake streaming server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/event-stream")
        w.WriteHeader(http.StatusOK)
        
        flusher, ok := w.(http.Flusher)
        require.True(t, ok)
        
        // Send chunks
        chunks := []string{
            `{"type":"text","text":"Hello "}`,
            `{"type":"text","text":"world!"}`,
            `{"type":"done","message":{"message_id":123,"text":"Hello world!"}}`,
        }
        
        for _, chunk := range chunks {
            fmt.Fprintf(w, "data: %s\n\n", chunk)
            flusher.Flush()
            time.Sleep(10 * time.Millisecond)
        }
    }))
    defer server.Close()
    
    client := newTestClient(t, server.URL)
    
    var receivedChunks []tg.DraftChunk
    msg, err := client.SendMessageDraft(ctx, SendMessageDraftRequest{
        ChatID: tg.ChatIDFromInt64(123),
        Text:   "test",
    }, func(chunk tg.DraftChunk) error {
        receivedChunks = append(receivedChunks, chunk)
        return nil
    })
    
    require.NoError(t, err)
    assert.Equal(t, 123, msg.MessageID)
    assert.Len(t, receivedChunks, 3)
    assert.Equal(t, "text", receivedChunks[0].Type)
    assert.Equal(t, "Hello ", receivedChunks[0].Text)
}

func TestSendMessageDraft_ContextCancellation(t *testing.T) {
    // Test that context cancellation doesn't leak goroutines
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/event-stream")
        w.WriteHeader(http.StatusOK)
        
        // Slow stream
        for i := 0; i < 100; i++ {
            fmt.Fprintf(w, "data: {\"type\":\"text\",\"text\":\"chunk %d\"}\n\n", i)
            w.(http.Flusher).Flush()
            time.Sleep(100 * time.Millisecond)
        }
    }))
    defer server.Close()
    
    client := newTestClient(t, server.URL)
    
    ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
    defer cancel()
    
    _, err := client.SendMessageDraft(ctx, SendMessageDraftRequest{
        ChatID: tg.ChatIDFromInt64(123),
        Text:   "test",
    }, nil)
    
    assert.ErrorIs(t, err, context.DeadlineExceeded)
}
```

### Definition of Done (PR20)

- [ ] High-level `SendMessageDraft()` with callback
- [ ] Low-level `SendMessageDraftStream()` for advanced users
- [ ] SSE parsing implemented correctly
- [ ] Context cancellation works without goroutine leaks
- [ ] Tests with fake streaming server
- [ ] Documentation with AI bot example

---

## PR21-PR25: Remaining PRs

(Same as v1.0 with minor updates)

| PR | Title | Hours |
|----|-------|-------|
| PR21 | Webhooks | 4-6 |
| PR22 | Passport + Boosts | 3-4 |
| PR23 | Integration Tests | 6-8 |
| PR24 | Docs + Coverage Matrix | 6-8 |
| PR25 | Release v2.2.0/v3.0.0 | 2-3 |

---

## Summary

| PR | Title | Methods | Hours | New in v2.0 |
|----|-------|---------|-------|-------------|
| PR10 | Types + Safety | - | 10-12 | Safety rules |
| PR11 | Payments | 9 | 6-8 | |
| PR12 | Stickers | 16 | 6-8 | |
| PR13 | Games | 3 | 3-4 | |
| PR14 | Gifts | 10 | 6-8 | +4 methods |
| PR15 | Verification | 4 | 2-3 | ✨NEW |
| PR16 | Business | 12 | 8-10 | +4 methods |
| PR17 | Stories | 5 | 4-5 | +repostStory |
| PR18 | Suggested Posts | 2 | 3-4 | ✨NEW |
| PR19 | Checklists | 2 | 4-5 | ✨NEW |
| PR20 | Draft Streaming | 1 | 8-10 | ✨STREAMING |
| PR21 | Webhooks | 3 | 4-6 | |
| PR22 | Passport + Boosts | 3 | 3-4 | |
| PR23 | Integration Tests | - | 6-8 | |
| PR24 | Docs + Coverage | - | 6-8 | |
| PR25 | Release | - | 2-3 | |
| **TOTAL** | | **~72** | **84-106** | |

**Timeline:** 8-10 weeks

---

## Complete API Coverage After Tier 3

| Tier | Methods | Cumulative |
|------|---------|------------|
| Tier 1 | 28 | 28 |
| Tier 2 | 52 | 80 |
| Tier 3 | 72 | **152** |

**Coverage:** ~97% of Telegram Bot API 9.3

---

## Bot API Coverage Matrix (PR24)

Include a coverage matrix in the repository:

```markdown
| Category | Implemented | Total | Coverage |
|----------|-------------|-------|----------|
| Updates | 4 | 4 | 100% |
| Messages | 25 | 26 | 96% |
| Editing | 7 | 7 | 100% |
| Chat Info | 5 | 5 | 100% |
| Moderation | 10 | 10 | 100% |
| Invite Links | 6 | 6 | 100% |
| Chat Settings | 9 | 9 | 100% |
| Bot Profile | 14 | 14 | 100% |
| Forum Topics | 13 | 13 | 100% |
| Inline Mode | 3 | 3 | 100% |
| Payments | 9 | 9 | 100% |
| Stickers | 16 | 16 | 100% |
| Games | 3 | 3 | 100% |
| Gifts | 10 | 10 | 100% |
| Business | 14 | 14 | 100% |
| Verification | 4 | 4 | 100% |
| Suggested Posts | 2 | 2 | 100% |
| Checklists | 2 | 2 | 100% |
| Drafts | 1 | 1 | 100% |
| Stories | 5 | 5 | 100% |
| Webhooks | 3 | 3 | 100% |
| Passport | 1 | 1 | 100% |
| Chat Boosts | 2 | 2 | 100% |
| **TOTAL** | **152** | **155** | **98%** |
```

---

## References

- [Telegram Bot API 9.3](https://core.telegram.org/bots/api)
- [Bot API Changelog](https://core.telegram.org/bots/api-changelog)
- [Telegram Payments](https://core.telegram.org/bots/payments)
- [Telegram Passport](https://core.telegram.org/passport)

---

*Consolidated Tier 3 Plan v2.0 - January 2026*
*Prerequisites: Tier 1 + Tier 2 must be complete*
*Combines insights from multiple independent analyses*