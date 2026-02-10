# galigo Bot API 9.4 Implementation Plan (v2 — Verified)

**Target**: Full support for Telegram Bot API 9.4 (February 9, 2026)
**Approach**: Follows existing galigo patterns — no new abstractions, no architecture changes
**Version**: v2 — all gift types verified against pyTelegramBotAPI source + consultants K/L cross-check

---

## Codebase Audit: What Exists vs What's Needed

Before listing changes, here's a critical gap analysis. The galigo codebase is **behind on several Bot API 9.0–9.3 types** that 9.4 modifies:

| Type | Introduced In | Exists in galigo? | 9.4 Adds |
|------|--------------|-------------------|----------|
| `UniqueGiftModel` | 9.0 | ❌ **Missing** | `rarity` field |
| `UniqueGift` | 9.0 | ❌ **Missing** | `is_burned` field |
| `KeyboardButton` (reply) | Ancient | ❌ **Missing** (only `InlineKeyboardButton` exists) | `style`, `icon_custom_emoji_id` |
| `CopyTextButton` | 8.0 | ❌ **Missing** | — (but needed for complete `InlineKeyboardButton`) |
| `InlineKeyboardButton` | Ancient | ✅ Exists | `style`, `icon_custom_emoji_id` |
| `User` | Ancient | ✅ Exists | `allows_users_to_create_topics` |
| `Message` | Ancient | ✅ Exists | `chat_owner_left`, `chat_owner_changed` |
| `Video` | Ancient | ✅ Exists | `qualities` |
| `UniqueGiftColors` | 9.3 | ❌ **Missing** | — (needed for `UniqueGift.colors` and `ChatFullInfo.unique_gift_colors`) |
| `ChatFullInfo` | Ancient | ✅ Exists | `first_profile_audio` (9.4), `unique_gift_colors` (9.3), `paid_message_star_count` (9.3) |
| `InputProfilePhoto` | 9.0 | ✅ Exists (in `sender/business_input.go`) | — (reuse for `setMyProfilePhoto`) |

**Decision**: For 9.4 changes to `UniqueGiftModel` and `UniqueGift`, we must first create the base types (from 9.0). For `KeyboardButton`, we add it fresh with the 9.4 fields included. The `CopyTextButton` is a prerequisite for a complete `InlineKeyboardButton` but can be deferred — the 9.4 fields (`style`, `icon_custom_emoji_id`) are independent.

**Lightweight 9.3 additions included**: `UniqueGiftColors` type, `ChatFullInfo.unique_gift_colors`, and `ChatFullInfo.paid_message_star_count` are small additions from 9.3 that are prerequisite for a complete `UniqueGift` type and cost almost nothing to include.

---

## Implementation Phases

### Phase 1: Type Additions (tg package)

All changes in `tg/` — pure struct definitions, zero behavior changes. These are safe, backward-compatible additions.

#### 1A. `tg/keyboard.go` — Button styling fields

**Add to `InlineKeyboardButton`** (2 new fields):

```go
// InlineKeyboardButton represents a button in an inline keyboard.
type InlineKeyboardButton struct {
	Text                         string                       `json:"text"`
	IconCustomEmojiID            string                       `json:"icon_custom_emoji_id,omitempty"` // NEW 9.4
	Style                        string                       `json:"style,omitempty"`                // NEW 9.4: "danger"|"success"|"primary"
	URL                          string                       `json:"url,omitempty"`
	CallbackData                 string                       `json:"callback_data,omitempty"`
	WebApp                       *WebAppInfo                  `json:"web_app,omitempty"`
	LoginURL                     *LoginURL                    `json:"login_url,omitempty"`
	SwitchInlineQuery            string                       `json:"switch_inline_query,omitempty"`
	SwitchInlineQueryCurrentChat string                       `json:"switch_inline_query_current_chat,omitempty"`
	SwitchInlineQueryChosenChat  *SwitchInlineQueryChosenChat `json:"switch_inline_query_chosen_chat,omitempty"`
	Pay                          bool                         `json:"pay,omitempty"`
}
```

**Position matters**: Per the API docs, `icon_custom_emoji_id` and `style` are listed right after `text` — they're decorative modifiers, not action selectors. Placing them immediately after `Text` in the struct reflects this semantics.

**Add `ButtonStyle` constants:**

```go
// Button style constants for InlineKeyboardButton and KeyboardButton.
const (
	ButtonStyleDanger  = "danger"  // Red
	ButtonStyleSuccess = "success" // Green
	ButtonStylePrimary = "primary" // Blue
)
```

**Add `KeyboardButton` type** (does not exist in galigo yet):

```go
// KeyboardButton represents one button of a reply keyboard.
type KeyboardButton struct {
	Text              string                     `json:"text"`
	IconCustomEmojiID string                     `json:"icon_custom_emoji_id,omitempty"` // 9.4
	Style             string                     `json:"style,omitempty"`                // 9.4
	RequestUsers      *KeyboardButtonRequestUsers `json:"request_users,omitempty"`
	RequestChat       *KeyboardButtonRequestChat  `json:"request_chat,omitempty"`
	RequestContact    bool                        `json:"request_contact,omitempty"`
	RequestLocation   bool                        `json:"request_location,omitempty"`
	RequestPoll       *KeyboardButtonPollType     `json:"request_poll,omitempty"`
	WebApp            *WebAppInfo                 `json:"web_app,omitempty"`
}

// KeyboardButtonRequestUsers defines criteria for requesting users.
type KeyboardButtonRequestUsers struct {
	RequestID     int  `json:"request_id"`
	UserIsBot     *bool `json:"user_is_bot,omitempty"`
	UserIsPremium *bool `json:"user_is_premium,omitempty"`
	MaxQuantity   int   `json:"max_quantity,omitempty"`
}

// KeyboardButtonRequestChat defines criteria for requesting a chat.
type KeyboardButtonRequestChat struct {
	RequestID               int                      `json:"request_id"`
	ChatIsChannel           bool                     `json:"chat_is_channel"`
	ChatIsForum             *bool                    `json:"chat_is_forum,omitempty"`
	ChatHasUsername         *bool                    `json:"chat_has_username,omitempty"`
	ChatIsCreated           bool                     `json:"chat_is_created,omitempty"`
	UserAdministratorRights *ChatAdministratorRights `json:"user_administrator_rights,omitempty"`
	BotAdministratorRights  *ChatAdministratorRights `json:"bot_administrator_rights,omitempty"`
	BotIsMember             bool                     `json:"bot_is_member,omitempty"`
}

// KeyboardButtonPollType limits polls to a specific type.
type KeyboardButtonPollType struct {
	Type string `json:"type,omitempty"` // "quiz" or "regular"
}

// ReplyKeyboardMarkup represents a custom keyboard with reply options.
type ReplyKeyboardMarkup struct {
	Keyboard              [][]KeyboardButton `json:"keyboard"`
	IsPersistent          bool               `json:"is_persistent,omitempty"`
	ResizeKeyboard        bool               `json:"resize_keyboard,omitempty"`
	OneTimeKeyboard       bool               `json:"one_time_keyboard,omitempty"`
	InputFieldPlaceholder string             `json:"input_field_placeholder,omitempty"`
	Selective             bool               `json:"selective,omitempty"`
}

// ReplyKeyboardRemove requests removal of the custom keyboard.
type ReplyKeyboardRemove struct {
	RemoveKeyboard bool `json:"remove_keyboard"` // Always true
	Selective      bool `json:"selective,omitempty"`
}
```

**Note**: `KeyboardButton`, `ReplyKeyboardMarkup`, `KeyboardButtonRequestUsers`, `KeyboardButtonRequestChat`, `KeyboardButtonPollType`, and `ReplyKeyboardRemove` are **pre-9.4 types that were never implemented in galigo**. Adding them now with the 9.4 fields included is cleaner than adding them without and immediately patching.

**Why not just add the 9.4 fields?** Because `KeyboardButton` is the second target of `style` and `icon_custom_emoji_id`. If the type doesn't exist, we can't add the fields. The API docs explicitly list both `KeyboardButton` and `InlineKeyboardButton` as recipients.

#### 1B. `tg/types.go` — User, Message, Video

**User** — add 1 field after `SupportsInlineQueries`:

```go
SupportsInlineQueries   bool   `json:"supports_inline_queries,omitempty"`
AllowsUsersToCreateTopics bool `json:"allows_users_to_create_topics,omitempty"` // NEW 9.4
```

**Message** — add 2 fields. Place with other service messages, after `ChannelChatCreated`:

```go
ChannelChatCreated    bool              `json:"channel_chat_created,omitempty"`
ChatOwnerLeft         *ChatOwnerLeft    `json:"chat_owner_left,omitempty"`     // NEW 9.4
ChatOwnerChanged      *ChatOwnerChanged `json:"chat_owner_changed,omitempty"` // NEW 9.4
```

**Video** — add 1 field after `FileSize`:

```go
FileSize     int64          `json:"file_size,omitempty"`
Qualities    []VideoQuality `json:"qualities,omitempty"` // NEW 9.4
```

#### 1C. `tg/chat_full_info.go` — ChatFullInfo

Add 3 fields. Place `FirstProfileAudio` near `Photo`, others at end with other optional fields:

```go
Photo                              *ChatPhoto        `json:"photo,omitempty"`
FirstProfileAudio                  *Audio            `json:"first_profile_audio,omitempty"` // NEW 9.4

// ... (at end of struct, near other optional chat-level fields):
UniqueGiftColors                   *UniqueGiftColors `json:"unique_gift_colors,omitempty"`   // 9.3 — singular pointer, NOT slice
PaidMessageStarCount               int               `json:"paid_message_star_count,omitempty"` // 9.3
```

> **CRITICAL**: `UniqueGiftColors` is a **singular pointer** (`*UniqueGiftColors`), NOT a slice. The Bot API type table shows `UniqueGiftColors` (no `Array of` prefix). Consultant L incorrectly recommended `[]UniqueGiftColors` — this would cause JSON unmarshal failures.

#### 1D. `tg/types.go` — New types: VideoQuality, UserProfileAudios, ChatOwnerLeft, ChatOwnerChanged

Add after `UserProfilePhotos`:

```go
// VideoQuality represents an available quality version of a video.
type VideoQuality struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Codec        string `json:"codec"` // "h264", "h265", "av01"
	FileSize     int64  `json:"file_size,omitempty"`
}

// UserProfileAudios contains a list of profile audios for a user.
type UserProfileAudios struct {
	TotalCount int     `json:"total_count"`
	Audios     []Audio `json:"audios"`
}

// ChatOwnerLeft is a service message: the chat owner left the chat.
type ChatOwnerLeft struct {
	NewOwner *User `json:"new_owner,omitempty"`
}

// ChatOwnerChanged is a service message: chat ownership has transferred.
type ChatOwnerChanged struct {
	NewOwner *User `json:"new_owner"`
}
```

#### 1E. `tg/gifts.go` — UniqueGiftModel and UniqueGift (prerequisite types from 9.0 + 9.3 + 9.4 fields)

These types don't exist in galigo. They're needed as base types before 9.4 fields can be added.

> **VERIFICATION STATUS**: All types below verified against pyTelegramBotAPI v4.30.0 source code (lines 12071–13119), cross-checked with consultants K and L. Three corrections applied:
> 1. `UniqueGiftBackdropColors` fields are `int` (RGB), NOT `string` (original plan had `string`)
> 2. `UniqueGiftBackdrop` has `RarityPerMille` field (was missing in original plan)
> 3. `UniqueGiftColors` — complete 6-field definition (was TODO in original plan)

```go
// UniqueGiftModel describes the model of a unique gift.
// Added in Bot API 9.0, updated in 9.4.
type UniqueGiftModel struct {
	Name           string  `json:"name"`
	Sticker        Sticker `json:"sticker"`
	RarityPerMille int     `json:"rarity_per_mille"`         // 9.0 — required, 0 for crafted
	Rarity         string  `json:"rarity,omitempty"`         // NEW 9.4: "uncommon"|"rare"|"epic"|"legendary"
}

// UniqueGiftSymbol describes the symbol of a unique gift.
// Added in Bot API 9.0.
type UniqueGiftSymbol struct {
	Name           string  `json:"name"`
	Sticker        Sticker `json:"sticker"`
	RarityPerMille int     `json:"rarity_per_mille"` // required, 0 for crafted
}

// UniqueGiftBackdropColors describes the colors of the backdrop of a unique gift.
// All fields are RGB24 integers (0..16777215 / 0x000000..0xFFFFFF).
// Added in Bot API 9.0.
type UniqueGiftBackdropColors struct {
	CenterColor int `json:"center_color"`
	EdgeColor   int `json:"edge_color"`
	SymbolColor int `json:"symbol_color"` // NOTE: NOT "pattern_color"
	TextColor   int `json:"text_color"`
}

// UniqueGiftBackdrop describes the backdrop of a unique gift.
// Added in Bot API 9.0.
type UniqueGiftBackdrop struct {
	Name           string                   `json:"name"`
	Colors         UniqueGiftBackdropColors `json:"colors"`           // singular object, NOT array
	RarityPerMille int                      `json:"rarity_per_mille"` // required, 0 for crafted
}

// UniqueGiftColors describes the color scheme for a user's name,
// message replies and link previews based on a unique gift.
// Added in Bot API 9.3.
type UniqueGiftColors struct {
	ModelCustomEmojiID    string `json:"model_custom_emoji_id"`
	SymbolCustomEmojiID   string `json:"symbol_custom_emoji_id"`
	LightThemeMainColor   int    `json:"light_theme_main_color"`     // RGB24
	LightThemeOtherColors []int  `json:"light_theme_other_colors"`   // 1-3 RGB24 colors
	DarkThemeMainColor    int    `json:"dark_theme_main_color"`      // RGB24
	DarkThemeOtherColors  []int  `json:"dark_theme_other_colors"`    // 1-3 RGB24 colors
}

// UniqueGift represents a gift upgraded to a unique one.
// Added in Bot API 9.0, updated in 9.3 and 9.4.
type UniqueGift struct {
	BaseName string             `json:"base_name"`
	Name     string             `json:"name"`
	Number   int                `json:"number"`
	Model    UniqueGiftModel    `json:"model"`
	Symbol   UniqueGiftSymbol   `json:"symbol"`
	Backdrop UniqueGiftBackdrop `json:"backdrop"`
	Colors   *UniqueGiftColors  `json:"colors,omitempty"`    // 9.3 — optional, singular pointer NOT slice
	IsBurned bool               `json:"is_burned,omitempty"` // NEW 9.4
}

// UniqueGiftModel rarity constants (added in Bot API 9.4).
const (
	GiftRarityUncommon  = "uncommon"
	GiftRarityRare      = "rare"
	GiftRarityEpic      = "epic"
	GiftRarityLegendary = "legendary"
)
```

**Key implementation decisions (with rationale):**

| Decision | Choice | Reason |
|----------|--------|--------|
| Color field type | `int` | Bot API says "Integer (RGB)". All reference libraries use `int`. No hex parsing needed. |
| `UniqueGift.Colors` | `*UniqueGiftColors` (pointer) | Optional field → pointer + `omitempty`. **NOT** `[]UniqueGiftColors` (Consultant L error). |
| `UniqueGiftBackdrop.Colors` | `UniqueGiftBackdropColors` (value) | Required field → no pointer, no omitempty. **NOT** `[]` (Consultant L error). |
| `RarityPerMille` on all 3 | Required `int` | Present on Model, Symbol, AND Backdrop. Always required (0 for crafted). |
| `SymbolColor` not `PatternColor` | `symbol_color` | Verified in pyTelegramBotAPI source line 12095. |

---

### Phase 2: New Methods (sender package)

#### 2A. `sender/identity.go` — setMyProfilePhoto, removeMyProfilePhoto

These reuse the existing `InputProfilePhoto` interface from `sender/business_input.go` and follow the exact same pattern as `SetBusinessAccountProfilePhoto` / `RemoveBusinessAccountProfilePhoto` in `sender/business.go`.

**Request types** (add to identity.go request types section):

```go
// SetMyProfilePhotoRequest represents a setMyProfilePhoto request.
type SetMyProfilePhotoRequest struct {
	Photo InputProfilePhoto `json:"-"` // Handled by multipart
}

// RemoveMyProfilePhotoRequest is empty — the method takes no parameters.
// We use struct{}{} directly in the method call.
```

**Methods** (add to identity.go Bot Profile section):

```go
// SetMyProfilePhoto sets the bot's own profile photo.
func (c *Client) SetMyProfilePhoto(ctx context.Context, photo InputProfilePhoto) error {
	if photo == nil {
		return tg.NewValidationError("photo", "required")
	}

	payload, err := buildMyProfilePhotoPayload(photo)
	if err != nil {
		return err
	}

	return c.callJSON(ctx, "setMyProfilePhoto", payload, nil)
}

// RemoveMyProfilePhoto removes the bot's current profile photo.
func (c *Client) RemoveMyProfilePhoto(ctx context.Context) error {
	return c.callJSON(ctx, "removeMyProfilePhoto", struct{}{}, nil)
}
```

**Helper** (add to identity.go or a shared helper):

```go
// myProfilePhotoPayload is the multipart-ready payload for setMyProfilePhoto.
type myProfilePhotoPayload struct {
	Photo         string     `json:"photo"`
	AttachedFiles []FilePart `json:"_file_parts"`
}

// buildMyProfilePhotoPayload resolves InputProfilePhoto to a multipart-ready payload.
// Reuses the same logic as buildProfilePhotoPayload in business.go
// but without BusinessConnectionID.
func buildMyProfilePhotoPayload(photo InputProfilePhoto) (*myProfilePhotoPayload, error) {
	payload := &myProfilePhotoPayload{}

	switch p := photo.(type) {
	case *InputProfilePhotoStatic:
		ref, fp, err := resolveInputFile(p.Photo, "profile_photo")
		if err != nil {
			return nil, fmt.Errorf("photo: %w", err)
		}
		if fp != nil {
			payload.AttachedFiles = append(payload.AttachedFiles, *fp)
		}
		data, err := json.Marshal(map[string]any{"type": "static", "photo": ref})
		if err != nil {
			return nil, err
		}
		payload.Photo = string(data)

	case *InputProfilePhotoAnimated:
		ref, fp, err := resolveInputFile(p.Animation, "profile_animation")
		if err != nil {
			return nil, fmt.Errorf("animation: %w", err)
		}
		if fp != nil {
			payload.AttachedFiles = append(payload.AttachedFiles, *fp)
		}
		m := map[string]any{"type": "animated", "animation": ref}
		if p.MainFrameTime > 0 {
			m["main_frame_time"] = p.MainFrameTime
		}
		data, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		payload.Photo = string(data)

	default:
		return nil, fmt.Errorf("unsupported InputProfilePhoto type: %T", photo)
	}

	return payload, nil
}
```

**Design note**: The `buildMyProfilePhotoPayload` pattern is nearly identical to `buildProfilePhotoPayload` in `business.go`. Consider extracting a shared `buildPhotoPayload(photo InputProfilePhoto) (string, []FilePart, error)` helper to reduce duplication. However, this is a refactor decision — for the 9.4 implementation, duplicating is safer and follows the current pattern.

#### 2B. `sender/methods.go` — getUserProfileAudios

Follows the exact same pattern as `GetUserProfilePhotos`:

**Request type** (add to `sender/requests.go`):

```go
// GetUserProfileAudiosRequest represents a getUserProfileAudios request.
type GetUserProfileAudiosRequest struct {
	UserID int64 `json:"user_id"`
	Offset int   `json:"offset,omitempty"`
	Limit  int   `json:"limit,omitempty"`
}
```

**Method** (add to `sender/methods.go` after `GetUserProfilePhotos`):

```go
// GetUserProfileAudios returns profile audios for a user.
func (c *Client) GetUserProfileAudios(ctx context.Context, userID int64, opts ...GetUserProfileAudiosOption) (*tg.UserProfileAudios, error) {
	req := GetUserProfileAudiosRequest{UserID: userID}
	for _, opt := range opts {
		opt(&req)
	}
	resp, err := c.executeRequest(ctx, "getUserProfileAudios", req)
	if err != nil {
		return nil, err
	}
	var audios tg.UserProfileAudios
	if err := json.Unmarshal(resp.Result, &audios); err != nil {
		return nil, err
	}
	return &audios, nil
}
```

**Options** (add to `sender/methods.go` options section):

```go
// GetUserProfileAudiosOption configures GetUserProfileAudios.
type GetUserProfileAudiosOption func(*GetUserProfileAudiosRequest)

// WithAudiosOffset sets the offset for user profile audios.
func WithAudiosOffset(offset int) GetUserProfileAudiosOption {
	return func(r *GetUserProfileAudiosRequest) {
		r.Offset = offset
	}
}

// WithAudiosLimit sets the limit for user profile audios (1-100).
func WithAudiosLimit(limit int) GetUserProfileAudiosOption {
	return func(r *GetUserProfileAudiosRequest) {
		r.Limit = limit
	}
}
```

---

### Phase 3: Button Constructor Helpers (tg package)

The existing constructors (`Btn`, `BtnURL`, etc.) return bare structs. For styling, we add a **method chain pattern** that's compatible with the existing builder:

```go
// WithStyle returns a copy of the button with the given style.
// Valid values: ButtonStyleDanger ("danger"), ButtonStyleSuccess ("success"),
// ButtonStylePrimary ("primary").
func (b InlineKeyboardButton) WithStyle(style string) InlineKeyboardButton {
	b.Style = style
	return b
}

// WithIcon returns a copy of the button with a custom emoji icon.
// The bot must be eligible to use custom emoji in the message.
func (b InlineKeyboardButton) WithIcon(customEmojiID string) InlineKeyboardButton {
	b.IconCustomEmojiID = customEmojiID
	return b
}
```

**Usage example** (will go in examples and README):

```go
// Green "Read More" button with custom emoji
btn := tg.BtnURL("Read More", "https://example.com").
	WithStyle(tg.ButtonStyleSuccess).
	WithIcon("5368324170671202286")

// Red "Delete" callback button
btn := tg.Btn("Delete", "action:delete").
	WithStyle(tg.ButtonStyleDanger)

// Use in keyboard builder — fully compatible
kb := tg.NewKeyboard().
	Row(
		tg.BtnURL("🔬 Read More", articleURL).WithStyle(tg.ButtonStyleSuccess),
		tg.Btn("📌 Save", "save:"+id).WithStyle(tg.ButtonStylePrimary),
	).
	Build()
```

This approach is **zero-breaking-change**: existing code calling `Btn("text", "data")` continues to work. The `With*` methods return copies, not mutations.

---

### Phase 4: createForumTopic Scope Expansion

**No code changes needed.** The API now allows `createForumTopic` in private chats. The existing `sender/forum.go` `CreateForumTopic` method already accepts `tg.ChatID` (which is `any`), so private chat IDs work automatically.

**Only documentation update**: Update the godoc comment from:

```go
// CreateForumTopic creates a topic in a forum supergroup chat.
```

to:

```go
// CreateForumTopic creates a topic in a forum supergroup or private chat.
```

---

### Phase 5: Tests

Follow existing test patterns (testify assert/require, table-driven where appropriate).

#### 5A. `tg/keyboard_test.go` — Button styling

```go
func TestInlineKeyboardButton_WithStyle(t *testing.T) {
	btn := tg.Btn("Delete", "action:delete").WithStyle(tg.ButtonStyleDanger)
	assert.Equal(t, "Delete", btn.Text)
	assert.Equal(t, "action:delete", btn.CallbackData)
	assert.Equal(t, "danger", btn.Style)
}

func TestInlineKeyboardButton_WithIcon(t *testing.T) {
	btn := tg.BtnURL("Read", "https://example.com").WithIcon("5368324170671202286")
	assert.Equal(t, "5368324170671202286", btn.IconCustomEmojiID)
	assert.Equal(t, "https://example.com", btn.URL)
}

func TestInlineKeyboardButton_WithStyle_DoesNotMutateOriginal(t *testing.T) {
	original := tg.Btn("Click", "data")
	styled := original.WithStyle(tg.ButtonStyleSuccess)
	assert.Empty(t, original.Style)           // Original unchanged
	assert.Equal(t, "success", styled.Style)  // Copy has style
}

func TestInlineKeyboardButton_StyleJSON(t *testing.T) {
	btn := tg.Btn("OK", "ok").WithStyle(tg.ButtonStylePrimary).WithIcon("12345")
	data, err := json.Marshal(btn)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"style":"primary"`)
	assert.Contains(t, string(data), `"icon_custom_emoji_id":"12345"`)
}

func TestInlineKeyboardButton_StyleOmittedWhenEmpty(t *testing.T) {
	btn := tg.Btn("Click", "data")
	data, err := json.Marshal(btn)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "style")
	assert.NotContains(t, string(data), "icon_custom_emoji_id")
}
```

#### 5B. `tg/types_test.go` — New type deserialization

```go
func TestVideoQuality_Unmarshal(t *testing.T) {
	raw := `{"file_id":"abc","file_unique_id":"xyz","width":1920,"height":1080,"codec":"h265","file_size":12345678}`
	var vq tg.VideoQuality
	require.NoError(t, json.Unmarshal([]byte(raw), &vq))
	assert.Equal(t, "abc", vq.FileID)
	assert.Equal(t, 1920, vq.Width)
	assert.Equal(t, "h265", vq.Codec)
	assert.Equal(t, int64(12345678), vq.FileSize)
}

func TestVideo_WithQualities(t *testing.T) {
	raw := `{"file_id":"vid1","file_unique_id":"u1","width":1920,"height":1080,"duration":60,"qualities":[{"file_id":"q1","file_unique_id":"qu1","width":640,"height":360,"codec":"h264"}]}`
	var v tg.Video
	require.NoError(t, json.Unmarshal([]byte(raw), &v))
	require.Len(t, v.Qualities, 1)
	assert.Equal(t, 640, v.Qualities[0].Width)
	assert.Equal(t, "h264", v.Qualities[0].Codec)
}

func TestChatOwnerLeft_Unmarshal(t *testing.T) {
	raw := `{"new_owner":{"id":123,"is_bot":false,"first_name":"Alice"}}`
	var col tg.ChatOwnerLeft
	require.NoError(t, json.Unmarshal([]byte(raw), &col))
	require.NotNil(t, col.NewOwner)
	assert.Equal(t, int64(123), col.NewOwner.ID)
}

func TestChatOwnerLeft_WithoutNewOwner(t *testing.T) {
	raw := `{}`
	var col tg.ChatOwnerLeft
	require.NoError(t, json.Unmarshal([]byte(raw), &col))
	assert.Nil(t, col.NewOwner)
}

func TestChatOwnerChanged_Unmarshal(t *testing.T) {
	raw := `{"new_owner":{"id":456,"is_bot":false,"first_name":"Bob"}}`
	var coc tg.ChatOwnerChanged
	require.NoError(t, json.Unmarshal([]byte(raw), &coc))
	require.NotNil(t, coc.NewOwner)
	assert.Equal(t, int64(456), coc.NewOwner.ID)
}

func TestUserProfileAudios_Unmarshal(t *testing.T) {
	raw := `{"total_count":2,"audios":[{"file_id":"aud1","file_unique_id":"u1","duration":180}]}`
	var upa tg.UserProfileAudios
	require.NoError(t, json.Unmarshal([]byte(raw), &upa))
	assert.Equal(t, 2, upa.TotalCount)
	require.Len(t, upa.Audios, 1)
	assert.Equal(t, "aud1", upa.Audios[0].FileID)
}

func TestUser_AllowsUsersToCreateTopics(t *testing.T) {
	raw := `{"id":1,"is_bot":true,"first_name":"Bot","allows_users_to_create_topics":true}`
	var u tg.User
	require.NoError(t, json.Unmarshal([]byte(raw), &u))
	assert.True(t, u.AllowsUsersToCreateTopics)
}

func TestMessage_ChatOwnerServiceMessages(t *testing.T) {
	raw := `{"message_id":1,"date":1234,"chat":{"id":1,"type":"group"},"chat_owner_left":{"new_owner":{"id":99,"is_bot":false,"first_name":"X"}}}`
	var m tg.Message
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	require.NotNil(t, m.ChatOwnerLeft)
	assert.Equal(t, int64(99), m.ChatOwnerLeft.NewOwner.ID)
	assert.Nil(t, m.ChatOwnerChanged)
}
```

#### 5C. `tg/gifts_test.go` — UniqueGift fields

```go
func TestUniqueGiftModel_Rarity(t *testing.T) {
	raw := `{"name":"Star","sticker":{"file_id":"s1","file_unique_id":"su1","type":"custom_emoji","width":100,"height":100,"is_animated":false,"is_video":false},"rarity_per_mille":50,"rarity":"legendary"}`
	var m tg.UniqueGiftModel
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	assert.Equal(t, "legendary", m.Rarity)
	assert.Equal(t, 50, m.RarityPerMille)
}

func TestUniqueGift_IsBurned(t *testing.T) {
	raw := `{"base_name":"Gift","name":"Gift #1","number":1,"model":{"name":"M","sticker":{"file_id":"s","file_unique_id":"su","type":"regular","width":1,"height":1,"is_animated":false,"is_video":false},"rarity_per_mille":100},"symbol":{"name":"S","sticker":{"file_id":"s2","file_unique_id":"su2","type":"regular","width":1,"height":1,"is_animated":false,"is_video":false},"rarity_per_mille":200},"backdrop":{"name":"B","colors":{"center_color":16777215,"edge_color":0,"symbol_color":11184810,"text_color":1118481},"rarity_per_mille":300},"is_burned":true}`
	var g tg.UniqueGift
	require.NoError(t, json.Unmarshal([]byte(raw), &g))
	assert.True(t, g.IsBurned)
	// Verify color fields are int (RGB), not strings
	assert.Equal(t, 16777215, g.Backdrop.Colors.CenterColor)  // 0xFFFFFF
	assert.Equal(t, 0, g.Backdrop.Colors.EdgeColor)            // 0x000000
	assert.Equal(t, 11184810, g.Backdrop.Colors.SymbolColor)   // 0xAAAAAA
	// Verify rarity_per_mille on all sub-types
	assert.Equal(t, 100, g.Model.RarityPerMille)
	assert.Equal(t, 200, g.Symbol.RarityPerMille)
	assert.Equal(t, 300, g.Backdrop.RarityPerMille)
}

func TestUniqueGift_WithColors(t *testing.T) {
	raw := `{"base_name":"Gift","name":"Gift #2","number":2,"model":{"name":"M","sticker":{"file_id":"s","file_unique_id":"su","type":"regular","width":1,"height":1,"is_animated":false,"is_video":false},"rarity_per_mille":0},"symbol":{"name":"S","sticker":{"file_id":"s2","file_unique_id":"su2","type":"regular","width":1,"height":1,"is_animated":false,"is_video":false},"rarity_per_mille":0},"backdrop":{"name":"B","colors":{"center_color":0,"edge_color":0,"symbol_color":0,"text_color":0},"rarity_per_mille":0},"colors":{"model_custom_emoji_id":"5368324170671202286","symbol_custom_emoji_id":"5368324170671202287","light_theme_main_color":16711680,"light_theme_other_colors":[65280,255],"dark_theme_main_color":8388608,"dark_theme_other_colors":[32768,128,64]}}`
	var g tg.UniqueGift
	require.NoError(t, json.Unmarshal([]byte(raw), &g))
	// Colors is a singular pointer, not a slice
	require.NotNil(t, g.Colors)
	assert.Equal(t, "5368324170671202286", g.Colors.ModelCustomEmojiID)
	assert.Equal(t, 16711680, g.Colors.LightThemeMainColor) // 0xFF0000 (red)
	assert.Len(t, g.Colors.LightThemeOtherColors, 2)
	assert.Len(t, g.Colors.DarkThemeOtherColors, 3) // up to 3 allowed
}

func TestUniqueGift_WithoutColors(t *testing.T) {
	raw := `{"base_name":"Gift","name":"Gift #3","number":3,"model":{"name":"M","sticker":{"file_id":"s","file_unique_id":"su","type":"regular","width":1,"height":1,"is_animated":false,"is_video":false},"rarity_per_mille":0},"symbol":{"name":"S","sticker":{"file_id":"s2","file_unique_id":"su2","type":"regular","width":1,"height":1,"is_animated":false,"is_video":false},"rarity_per_mille":0},"backdrop":{"name":"B","colors":{"center_color":0,"edge_color":0,"symbol_color":0,"text_color":0},"rarity_per_mille":0}}`
	var g tg.UniqueGift
	require.NoError(t, json.Unmarshal([]byte(raw), &g))
	assert.Nil(t, g.Colors) // colors is optional
}
```

#### 5D. `sender/identity_test.go` — Profile photo methods

```go
func TestSetMyProfilePhoto_NilPhoto(t *testing.T) {
	c := newTestClient(t)
	err := c.SetMyProfilePhoto(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "photo")
}

func TestSetMyProfilePhoto_Static(t *testing.T) {
	c, transport := newTestClientWithTransport(t)
	transport.respondWith(`{"ok":true,"result":true}`)

	err := c.SetMyProfilePhoto(context.Background(), &sender.InputProfilePhotoStatic{
		Photo: sender.FromFileID("photo_123"),
	})
	assert.NoError(t, err)
	assert.Equal(t, "setMyProfilePhoto", transport.lastMethod)
}

func TestRemoveMyProfilePhoto(t *testing.T) {
	c, transport := newTestClientWithTransport(t)
	transport.respondWith(`{"ok":true,"result":true}`)

	err := c.RemoveMyProfilePhoto(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "removeMyProfilePhoto", transport.lastMethod)
}
```

#### 5E. `sender/methods_test.go` — getUserProfileAudios

```go
func TestGetUserProfileAudios(t *testing.T) {
	c, transport := newTestClientWithTransport(t)
	transport.respondWith(`{"ok":true,"result":{"total_count":1,"audios":[{"file_id":"a1","file_unique_id":"au1","duration":120}]}}`)

	audios, err := c.GetUserProfileAudios(context.Background(), 12345)
	require.NoError(t, err)
	assert.Equal(t, 1, audios.TotalCount)
	require.Len(t, audios.Audios, 1)
	assert.Equal(t, "a1", audios.Audios[0].FileID)
}

func TestGetUserProfileAudios_WithOptions(t *testing.T) {
	c, transport := newTestClientWithTransport(t)
	transport.respondWith(`{"ok":true,"result":{"total_count":5,"audios":[]}}`)

	_, err := c.GetUserProfileAudios(context.Background(), 12345,
		sender.WithAudiosOffset(10),
		sender.WithAudiosLimit(50),
	)
	require.NoError(t, err)
}
```

---

## File Change Summary

| File | Changes | Lines ±  |
|------|---------|---------|
| `tg/keyboard.go` | Add `Style`, `IconCustomEmojiID` to `InlineKeyboardButton`; add `ButtonStyle*` constants; add `KeyboardButton`, `ReplyKeyboardMarkup`, `ReplyKeyboardRemove`, sub-types; add `WithStyle()`, `WithIcon()` methods | ~+100 |
| `tg/types.go` | Add `AllowsUsersToCreateTopics` to `User`; add `ChatOwnerLeft`, `ChatOwnerChanged` to `Message`; add `Qualities` to `Video`; add `VideoQuality`, `UserProfileAudios`, `ChatOwnerLeft`, `ChatOwnerChanged` types | ~+35 |
| `tg/chat_full_info.go` | Add `FirstProfileAudio`, `UniqueGiftColors`, `PaidMessageStarCount` to `ChatFullInfo` | +3 |
| `tg/gifts.go` | Add `UniqueGiftModel`, `UniqueGiftSymbol`, `UniqueGiftBackdropColors`, `UniqueGiftBackdrop`, `UniqueGiftColors`, `UniqueGift` types; add `GiftRarity*` constants | ~+75 |
| `sender/identity.go` | Add `SetMyProfilePhoto`, `RemoveMyProfilePhoto` methods + payload builder | ~+65 |
| `sender/methods.go` | Add `GetUserProfileAudios` method + option funcs | ~+30 |
| `sender/requests.go` | Add `GetUserProfileAudiosRequest` | ~+6 |
| `sender/forum.go` | Update `CreateForumTopic` godoc (scope: private chats) | 1 line change |
| `tg/keyboard_test.go` | Button styling tests | ~+45 |
| `tg/types_test.go` | VideoQuality, ChatOwner*, UserProfileAudios, User field tests | ~+60 |
| `tg/gifts_test.go` | UniqueGiftModel rarity, UniqueGift is_burned, UniqueGiftColors round-trip tests | ~+55 |
| `sender/identity_test.go` | SetMyProfilePhoto, RemoveMyProfilePhoto tests | ~+25 |
| `sender/methods_test.go` | GetUserProfileAudios tests | ~+20 |
| **Total** | **13 files** | **~+510 lines** |

---

## Implementation Order (dependency-safe)

```
Step 1: tg/gifts.go          — UniqueGift* types (prerequisite for 9.4 fields)
Step 2: tg/types.go           — VideoQuality, UserProfileAudios, ChatOwnerLeft/Changed types
                              — User, Message, Video field additions
Step 3: tg/chat_full_info.go  — FirstProfileAudio (9.4), UniqueGiftColors, PaidMessageStarCount (9.3)
Step 4: tg/keyboard.go        — KeyboardButton type, button styling fields + constants + methods
Step 5: sender/requests.go    — GetUserProfileAudiosRequest
Step 6: sender/methods.go     — GetUserProfileAudios method + options
Step 7: sender/identity.go    — SetMyProfilePhoto, RemoveMyProfilePhoto
Step 8: sender/forum.go       — Godoc update
Step 9: Tests (all)            — Can run after each step for incremental validation
```

Steps 1–4 are independent of each other (all `tg/` package, no cross-dependencies). Steps 5–8 depend on the types from steps 1–4. Tests can be written alongside each step.

---

## What This Does NOT Cover (and why)

**Bot API 9.0–9.3 catch-up**: galigo is missing many types from recent API versions (OwnedGiftRegular, OwnedGiftUnique, ChecklistTask, DirectMessagesTopic, SuggestedPostParameters, etc.). This plan adds only what's needed for 9.4 + the minimal prerequisite types. A full catch-up would be a separate, larger effort.

**`CopyTextButton`**: Missing from `InlineKeyboardButton` since Bot API 7.x. Not part of 9.4. Can be added separately:
```go
CopyText *CopyTextButton `json:"copy_text,omitempty"`
```

**`CallbackGame`**: Also missing from `InlineKeyboardButton`. Not part of 9.4.

**Reply keyboard builder**: No `ReplyKeyboard` builder analogous to `Keyboard` for inline. Out of scope for 9.4 but would be a natural follow-up after adding `KeyboardButton`.

**Custom emoji eligibility check**: The API doesn't provide a method to check if a bot can use custom emoji. This is a runtime behavior — if the bot owner has Premium, custom emoji work. If not, the API returns an error. galigo should pass through this error naturally.

---

## Relevance to Science News Channel Bot

For your specific use case (channel posts via sendMessage + HTML + LinkPreviewOptions), the 9.4 changes that matter:

1. **Button `style`** — Color your inline buttons on news posts (green "Read Full Article", blue "Save"). Directly usable with the `WithStyle()` method.

2. **Custom emoji in messages** — If you have Telegram Premium, you can embed custom emoji in posts to private/group chats. **Channel eligibility needs testing** — the changelog says "private, group and supergroup chats" but doesn't mention channels.

3. **Everything else** (profile photos, video qualities, gift rarity, chat owner messages, profile audios) — Not relevant to channel posting but worth implementing for library completeness.

---

## Appendix A: Verification Log

All type definitions in this plan have been verified against multiple sources. This appendix documents the verification process for audit purposes.

### Sources Used

| Source | Method | Reliability |
|--------|--------|-------------|
| core.telegram.org/bots/api | web_fetch (partial — page truncates before gift types) | **Authoritative** (source of truth) |
| pyTelegramBotAPI v4.30.0 `types.py` | pip install + direct source inspection via grep | **High** — maintained, tracks Bot API closely |
| Consultant K (inline review) | Manual review of Bot API docs | **A-** — correct on all fields and typing |
| Consultant L (document review) | Manual review of Bot API docs | **B-** — correct on fields, WRONG on container types |

### Corrections Applied in v2

| What Changed | v1 (incorrect) | v2 (correct) | Verification Source |
|---|---|---|---|
| `UniqueGiftBackdropColors` field types | `string` (hex) | `int` (RGB24) | pyTelegramBotAPI line 12092-12096 |
| `UniqueGiftBackdropColors.SymbolColor` | Previously `PatternColor` | `SymbolColor` / `symbol_color` | pyTelegramBotAPI line 12095 |
| `UniqueGiftBackdrop.RarityPerMille` | Missing | Added as required `int` | pyTelegramBotAPI line 12126 |
| `UniqueGiftModel.RarityPerMille` | Missing | Added as required `int` | pyTelegramBotAPI line 13073 |
| `UniqueGiftSymbol.RarityPerMille` | Missing | Added as required `int` | pyTelegramBotAPI line 13053 |
| `UniqueGiftColors` fields | TODO (unknown) | 6 verified fields | pyTelegramBotAPI lines 13089-13119 + both consultants |
| `UniqueGift.Colors` type | Missing | `*UniqueGiftColors` (pointer, singular) | Bot API docs — no "Array of" prefix |
| `ChatFullInfo.UniqueGiftColors` | Missing | `*UniqueGiftColors` (pointer, singular) | Bot API docs — 9.3 addition |
| `ChatFullInfo.PaidMessageStarCount` | Missing | `int` with `omitempty` | Bot API docs — 9.3 addition |
| Test JSON for colors | `"#fff"` (hex strings) | `16777215` (integers) | Matches `int` field type |

### Consultant L Errors (rejected recommendations)

These specific claims from Consultant L were **verified as incorrect** and are NOT reflected in this plan:

1. `UniqueGift.colors → []UniqueGiftColors` — **WRONG**, it's `*UniqueGiftColors` (singular pointer)
2. `ChatFullInfo.unique_gift_colors → []UniqueGiftColors` — **WRONG**, it's `*UniqueGiftColors` (singular pointer)
3. `UniqueGiftBackdrop.colors → []UniqueGiftBackdropColors` — **WRONG**, it's `UniqueGiftBackdropColors` (singular value)

Implementing any of these as slices would cause `json.Unmarshal` to fail when Telegram sends a JSON object instead of a JSON array.

---

## Document Changelog

| Version | Date | Changes |
|---------|------|---------|
| v1 | 2026-02-10 | Initial implementation plan based on Bot API 9.4 changelog analysis |
| **v2** | **2026-02-10** | **Major corrections**: Fixed all gift type definitions (color types: string→int, added RarityPerMille to all 3 sub-types, complete UniqueGiftColors definition, added UniqueGift.Colors field). Added 9.3 lightweight fields (ChatFullInfo.UniqueGiftColors, ChatFullInfo.PaidMessageStarCount). Expanded test coverage. Added verification appendix. |