package tg

import (
	"bytes"
	"encoding/json"
)

// Bot API 10.1 rich messages, read side: Message.RichMessage and the RichText and RichBlock types.
// Kinds added in later Bot API versions decode as RichTextUnknown or RichBlockUnknown and keep their raw JSON.

// RichText is rich formatted text (Bot API 10.1): a JSON string for plain text, a JSON array of RichText, or one of
// the RichText* kinds. Exactly one form is set when decoded: Plain for a string (also the zero value), Parts for an
// array (non-nil, may be empty), Node for an object. When encoding, Node wins over Parts, and Parts over Plain.
type RichText struct {
	Plain string
	Parts []RichText
	Node  RichTextNode
}

// RichTextNode is one of the RichText* kinds, or RichTextUnknown.
type RichTextNode interface {
	richTextNode()
}

// RichTextUnknown is a RichText value this version of galigo does not model: a kind added in a later Bot API
// version, a known kind that does not decode, or a JSON value that is no string, array or object. Raw holds the
// value's JSON bytes as received; Type is its "type" when it has one.
type RichTextUnknown struct {
	Type string
	Raw  json.RawMessage
}

func (RichTextUnknown) richTextNode() {}

// MarshalJSON returns Raw, or {"type":Type} when Raw is empty.
func (u RichTextUnknown) MarshalJSON() ([]byte, error) {
	return unknownJSON(u.Type, u.Raw)
}

// UnmarshalJSON decodes any JSON value. It never returns an error: a value it cannot decode is kept as
// RichTextUnknown, so a message is never lost because of its text.
func (r *RichText) UnmarshalJSON(data []byte) error {
	*r = RichText{}
	switch firstByte(data) {
	case '"':
		if err := json.Unmarshal(data, &r.Plain); err != nil {
			*r = RichText{Node: RichTextUnknown{Raw: copyRaw(data)}}
		}
	case '[':
		var raws []json.RawMessage
		if err := json.Unmarshal(data, &raws); err != nil {
			*r = RichText{Node: RichTextUnknown{Raw: copyRaw(data)}}
			return nil //nolint:nilerr // Intentional: a value that does not decode is kept as RichTextUnknown
		}
		r.Parts = make([]RichText, len(raws))
		for i, raw := range raws {
			_ = r.Parts[i].UnmarshalJSON(raw)
		}
	case '{':
		r.Node = unmarshalRichTextNode(data)
	case 'n':
		if string(bytes.TrimSpace(data)) != "null" {
			r.Node = RichTextUnknown{Raw: copyRaw(data)}
		}
	default:
		r.Node = RichTextUnknown{Raw: copyRaw(data)}
	}
	return nil
}

// MarshalJSON encodes the set form: Node, then Parts, then Plain.
func (r RichText) MarshalJSON() ([]byte, error) {
	switch {
	case r.Node != nil:
		return json.Marshal(r.Node)
	case r.Parts != nil:
		return json.Marshal(r.Parts)
	default:
		return json.Marshal(r.Plain)
	}
}

// RichBlock is one of the RichBlock* kinds, or RichBlockUnknown.
type RichBlock interface {
	richBlock()
}

// RichBlocks is a list of rich blocks. Every field that holds blocks has this type, so each element decodes into
// its kind.
type RichBlocks []RichBlock

// RichBlockUnknown is a block this version of galigo does not model: a kind added in a later Bot API version, a
// known kind that does not decode, or a value that is no object. Raw holds the JSON bytes as received; Type is its
// "type" when it has one.
type RichBlockUnknown struct {
	Type string
	Raw  json.RawMessage
}

func (RichBlockUnknown) richBlock() {}

// MarshalJSON returns Raw, or {"type":Type} when Raw is empty.
func (u RichBlockUnknown) MarshalJSON() ([]byte, error) {
	return unknownJSON(u.Type, u.Raw)
}

// UnmarshalJSON decodes a JSON array of blocks. It never returns an error: null gives nil, a value that is no array
// gives one RichBlockUnknown, and each element that is not a known block gives a RichBlockUnknown.
func (b *RichBlocks) UnmarshalJSON(data []byte) error {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		*b = RichBlocks{RichBlockUnknown{Raw: copyRaw(data)}}
		return nil //nolint:nilerr // Intentional: a value that is no array is kept as one RichBlockUnknown
	}
	if raws == nil {
		*b = nil
		return nil
	}
	out := make(RichBlocks, len(raws))
	for i, raw := range raws {
		out[i] = unmarshalRichBlock(raw)
	}
	*b = out
	return nil
}

func unmarshalRichTextNode(data []byte) RichTextNode {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return RichTextUnknown{Raw: copyRaw(data)}
	}
	switch probe.Type {
	case "bold":
		return decodeRichTextNode[RichTextBold](data, probe.Type)
	case "italic":
		return decodeRichTextNode[RichTextItalic](data, probe.Type)
	case "underline":
		return decodeRichTextNode[RichTextUnderline](data, probe.Type)
	case "strikethrough":
		return decodeRichTextNode[RichTextStrikethrough](data, probe.Type)
	case "spoiler":
		return decodeRichTextNode[RichTextSpoiler](data, probe.Type)
	case "date_time":
		return decodeRichTextNode[RichTextDateTime](data, probe.Type)
	case "text_mention":
		return decodeRichTextNode[RichTextTextMention](data, probe.Type)
	case "subscript":
		return decodeRichTextNode[RichTextSubscript](data, probe.Type)
	case "superscript":
		return decodeRichTextNode[RichTextSuperscript](data, probe.Type)
	case "marked":
		return decodeRichTextNode[RichTextMarked](data, probe.Type)
	case "code":
		return decodeRichTextNode[RichTextCode](data, probe.Type)
	case "custom_emoji":
		return decodeRichTextNode[RichTextCustomEmoji](data, probe.Type)
	case "mathematical_expression":
		return decodeRichTextNode[RichTextMathematicalExpression](data, probe.Type)
	case "url":
		return decodeRichTextNode[RichTextURL](data, probe.Type)
	case "email_address":
		return decodeRichTextNode[RichTextEmailAddress](data, probe.Type)
	case "phone_number":
		return decodeRichTextNode[RichTextPhoneNumber](data, probe.Type)
	case "bank_card_number":
		return decodeRichTextNode[RichTextBankCardNumber](data, probe.Type)
	case "mention":
		return decodeRichTextNode[RichTextMention](data, probe.Type)
	case "hashtag":
		return decodeRichTextNode[RichTextHashtag](data, probe.Type)
	case "cashtag":
		return decodeRichTextNode[RichTextCashtag](data, probe.Type)
	case "bot_command":
		return decodeRichTextNode[RichTextBotCommand](data, probe.Type)
	case "anchor":
		return decodeRichTextNode[RichTextAnchor](data, probe.Type)
	case "anchor_link":
		return decodeRichTextNode[RichTextAnchorLink](data, probe.Type)
	case "reference":
		return decodeRichTextNode[RichTextReference](data, probe.Type)
	case "reference_link":
		return decodeRichTextNode[RichTextReferenceLink](data, probe.Type)
	default:
		return RichTextUnknown{Type: probe.Type, Raw: copyRaw(data)}
	}
}

// unmarshalRichBlock keeps data without a copy: its only caller, (*RichBlocks).UnmarshalJSON, passes elements that
// json.RawMessage has already copied.
func unmarshalRichBlock(data json.RawMessage) RichBlock {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return RichBlockUnknown{Raw: data}
	}
	switch probe.Type {
	case "paragraph":
		return decodeRichBlock[RichBlockParagraph](data, probe.Type)
	case "heading":
		return decodeRichBlock[RichBlockSectionHeading](data, probe.Type)
	case "pre":
		return decodeRichBlock[RichBlockPreformatted](data, probe.Type)
	case "footer":
		return decodeRichBlock[RichBlockFooter](data, probe.Type)
	case "divider":
		return decodeRichBlock[RichBlockDivider](data, probe.Type)
	case "mathematical_expression":
		return decodeRichBlock[RichBlockMathematicalExpression](data, probe.Type)
	case "anchor":
		return decodeRichBlock[RichBlockAnchor](data, probe.Type)
	case "list":
		return decodeRichBlock[RichBlockList](data, probe.Type)
	case "blockquote":
		return decodeRichBlock[RichBlockBlockQuotation](data, probe.Type)
	case "pullquote":
		return decodeRichBlock[RichBlockPullQuotation](data, probe.Type)
	case "collage":
		return decodeRichBlock[RichBlockCollage](data, probe.Type)
	case "slideshow":
		return decodeRichBlock[RichBlockSlideshow](data, probe.Type)
	case "table":
		return decodeRichBlock[RichBlockTable](data, probe.Type)
	case "details":
		return decodeRichBlock[RichBlockDetails](data, probe.Type)
	case "map":
		return decodeRichBlock[RichBlockMap](data, probe.Type)
	case "animation":
		return decodeRichBlock[RichBlockAnimation](data, probe.Type)
	case "audio":
		return decodeRichBlock[RichBlockAudio](data, probe.Type)
	case "photo":
		return decodeRichBlock[RichBlockPhoto](data, probe.Type)
	case "video":
		return decodeRichBlock[RichBlockVideo](data, probe.Type)
	case "voice_note":
		return decodeRichBlock[RichBlockVoiceNote](data, probe.Type)
	case "thinking":
		return decodeRichBlock[RichBlockThinking](data, probe.Type)
	default:
		return RichBlockUnknown{Type: probe.Type, Raw: data}
	}
}

func decodeRichTextNode[T RichTextNode](data []byte, typ string) RichTextNode {
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return RichTextUnknown{Type: typ, Raw: copyRaw(data)}
	}
	return v
}

func decodeRichBlock[T RichBlock](data json.RawMessage, typ string) RichBlock {
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return RichBlockUnknown{Type: typ, Raw: data}
	}
	return v
}

// copyRaw copies data: encoding/json passes UnmarshalJSON a slice of the caller's buffer, which the caller may
// reuse (the webhook handler decodes from a pooled buffer).
func copyRaw(data []byte) json.RawMessage {
	return append(json.RawMessage(nil), data...)
}

func unknownJSON(typ string, raw json.RawMessage) ([]byte, error) {
	if len(raw) > 0 {
		return raw, nil
	}
	return json.Marshal(struct {
		Type string `json:"type"`
	}{typ})
}

func firstByte(data []byte) byte {
	if d := bytes.TrimSpace(data); len(d) > 0 {
		return d[0]
	}
	return 0
}

// RichTextBold is a bold text (Bot API 10.1).
type RichTextBold struct {
	Type string   `json:"type"`
	Text RichText `json:"text"`
}

// RichTextItalic is an italicized text (Bot API 10.1).
type RichTextItalic struct {
	Type string   `json:"type"`
	Text RichText `json:"text"`
}

// RichTextUnderline is an underlined text (Bot API 10.1).
type RichTextUnderline struct {
	Type string   `json:"type"`
	Text RichText `json:"text"`
}

// RichTextStrikethrough is a strikethrough text (Bot API 10.1).
type RichTextStrikethrough struct {
	Type string   `json:"type"`
	Text RichText `json:"text"`
}

// RichTextSpoiler is a text covered by a spoiler (Bot API 10.1).
type RichTextSpoiler struct {
	Type string   `json:"type"`
	Text RichText `json:"text"`
}

// RichTextDateTime is formatted date and time (Bot API 10.1).
type RichTextDateTime struct {
	Type           string   `json:"type"`
	Text           RichText `json:"text"`
	UnixTime       int64    `json:"unix_time"`
	DateTimeFormat string   `json:"date_time_format"`
}

// RichTextTextMention is a mention of a Telegram user by their identifier (Bot API 10.1).
type RichTextTextMention struct {
	Type string   `json:"type"`
	Text RichText `json:"text"`
	User User     `json:"user"`
}

// RichTextSubscript is a subscript text (Bot API 10.1).
type RichTextSubscript struct {
	Type string   `json:"type"`
	Text RichText `json:"text"`
}

// RichTextSuperscript is a superscript text (Bot API 10.1).
type RichTextSuperscript struct {
	Type string   `json:"type"`
	Text RichText `json:"text"`
}

// RichTextMarked is a marked text (Bot API 10.1).
type RichTextMarked struct {
	Type string   `json:"type"`
	Text RichText `json:"text"`
}

// RichTextCode is a monowidth text (Bot API 10.1).
type RichTextCode struct {
	Type string   `json:"type"`
	Text RichText `json:"text"`
}

// RichTextCustomEmoji is a custom emoji (Bot API 10.1).
type RichTextCustomEmoji struct {
	Type            string `json:"type"`
	CustomEmojiID   string `json:"custom_emoji_id"`
	AlternativeText string `json:"alternative_text"`
}

// RichTextMathematicalExpression is a mathematical expression (Bot API 10.1).
type RichTextMathematicalExpression struct {
	Type       string `json:"type"`
	Expression string `json:"expression"`
}

// RichTextURL is a text with a link (Bot API 10.1).
type RichTextURL struct {
	Type string   `json:"type"`
	Text RichText `json:"text"`
	URL  string   `json:"url"`
}

// RichTextEmailAddress is a text with an email address (Bot API 10.1).
type RichTextEmailAddress struct {
	Type         string   `json:"type"`
	Text         RichText `json:"text"`
	EmailAddress string   `json:"email_address"`
}

// RichTextPhoneNumber is a text with a phone number (Bot API 10.1).
type RichTextPhoneNumber struct {
	Type        string   `json:"type"`
	Text        RichText `json:"text"`
	PhoneNumber string   `json:"phone_number"`
}

// RichTextBankCardNumber is a text with a bank card number (Bot API 10.1).
type RichTextBankCardNumber struct {
	Type           string   `json:"type"`
	Text           RichText `json:"text"`
	BankCardNumber string   `json:"bank_card_number"`
}

// RichTextMention is a mention by a username (Bot API 10.1).
type RichTextMention struct {
	Type     string   `json:"type"`
	Text     RichText `json:"text"`
	Username string   `json:"username"`
}

// RichTextHashtag is a hashtag (Bot API 10.1).
type RichTextHashtag struct {
	Type    string   `json:"type"`
	Text    RichText `json:"text"`
	Hashtag string   `json:"hashtag"`
}

// RichTextCashtag is a cashtag (Bot API 10.1).
type RichTextCashtag struct {
	Type    string   `json:"type"`
	Text    RichText `json:"text"`
	Cashtag string   `json:"cashtag"`
}

// RichTextBotCommand is a bot command (Bot API 10.1).
type RichTextBotCommand struct {
	Type       string   `json:"type"`
	Text       RichText `json:"text"`
	BotCommand string   `json:"bot_command"`
}

// RichTextAnchor is an anchor (Bot API 10.1).
type RichTextAnchor struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// RichTextAnchorLink is a link to an anchor (Bot API 10.1).
type RichTextAnchorLink struct {
	Type       string   `json:"type"`
	Text       RichText `json:"text"`
	AnchorName string   `json:"anchor_name"`
}

// RichTextReference is a reference (Bot API 10.1).
type RichTextReference struct {
	Type string   `json:"type"`
	Text RichText `json:"text"`
	Name string   `json:"name"`
}

// RichTextReferenceLink is a link to a reference (Bot API 10.1).
type RichTextReferenceLink struct {
	Type          string   `json:"type"`
	Text          RichText `json:"text"`
	ReferenceName string   `json:"reference_name"`
}

// RichBlockCaption is a caption of a rich formatted block (Bot API 10.1).
type RichBlockCaption struct {
	Text   RichText  `json:"text"`
	Credit *RichText `json:"credit,omitempty"`
}

// RichBlockTableCell is a cell in a table (Bot API 10.1).
type RichBlockTableCell struct {
	Text     *RichText `json:"text,omitempty"`
	IsHeader bool      `json:"is_header,omitempty"`
	Colspan  int       `json:"colspan,omitempty"`
	Rowspan  int       `json:"rowspan,omitempty"`
	Align    string    `json:"align"`
	VAlign   string    `json:"valign"`
}

// RichBlockListItem is an item of a list (Bot API 10.1).
type RichBlockListItem struct {
	Label       string     `json:"label"`
	Blocks      RichBlocks `json:"blocks"`
	HasCheckbox bool       `json:"has_checkbox,omitempty"`
	IsChecked   bool       `json:"is_checked,omitempty"`
	Value       int        `json:"value,omitempty"`
	Type        string     `json:"type,omitempty"`
}

// RichBlockParagraph is a text paragraph, corresponding to the HTML tag <p> (Bot API 10.1).
type RichBlockParagraph struct {
	Type string   `json:"type"`
	Text RichText `json:"text"`
}

// RichBlockSectionHeading is a section heading, corresponding to the HTML tags <h1>, <h2>, <h3>, <h4>, <h5>, or <h6> (Bot API 10.1).
type RichBlockSectionHeading struct {
	Type string   `json:"type"`
	Text RichText `json:"text"`
	Size int      `json:"size"`
}

// RichBlockPreformatted is a preformatted text block, corresponding to the nested HTML tags <pre> and <code> (Bot API 10.1).
type RichBlockPreformatted struct {
	Type     string   `json:"type"`
	Text     RichText `json:"text"`
	Language string   `json:"language,omitempty"`
}

// RichBlockFooter is a footer, corresponding to the HTML tag <footer> (Bot API 10.1).
type RichBlockFooter struct {
	Type string   `json:"type"`
	Text RichText `json:"text"`
}

// RichBlockDivider is a divider, corresponding to the HTML tag <hr/> (Bot API 10.1).
type RichBlockDivider struct {
	Type string `json:"type"`
}

// RichBlockMathematicalExpression is a block with a mathematical expression in LaTeX format, corresponding to the custom HTML tag <tg-math-block> (Bot API 10.1).
type RichBlockMathematicalExpression struct {
	Type       string `json:"type"`
	Expression string `json:"expression"`
}

// RichBlockAnchor is a block with an anchor, corresponding to the HTML tag <a> with the attribute name (Bot API 10.1).
type RichBlockAnchor struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// RichBlockList is a list of blocks, corresponding to the HTML tag <ul> or <ol> with multiple nested tags <li> (Bot API 10.1).
type RichBlockList struct {
	Type  string              `json:"type"`
	Items []RichBlockListItem `json:"items"`
}

// RichBlockBlockQuotation is a block quotation, corresponding to the HTML tag <blockquote> (Bot API 10.1).
type RichBlockBlockQuotation struct {
	Type   string     `json:"type"`
	Blocks RichBlocks `json:"blocks"`
	Credit *RichText  `json:"credit,omitempty"`
}

// RichBlockPullQuotation is a quotation with centered text, loosely corresponding to the HTML tag <aside> (Bot API 10.1).
type RichBlockPullQuotation struct {
	Type   string    `json:"type"`
	Text   RichText  `json:"text"`
	Credit *RichText `json:"credit,omitempty"`
}

// RichBlockCollage is a collage, corresponding to the custom HTML tag <tg-collage> (Bot API 10.1).
type RichBlockCollage struct {
	Type    string            `json:"type"`
	Blocks  RichBlocks        `json:"blocks"`
	Caption *RichBlockCaption `json:"caption,omitempty"`
}

// RichBlockSlideshow is a slideshow, corresponding to the custom HTML tag <tg-slideshow> (Bot API 10.1).
type RichBlockSlideshow struct {
	Type    string            `json:"type"`
	Blocks  RichBlocks        `json:"blocks"`
	Caption *RichBlockCaption `json:"caption,omitempty"`
}

// RichBlockTable is a table, corresponding to the HTML tag <table> (Bot API 10.1).
type RichBlockTable struct {
	Type       string                 `json:"type"`
	Cells      [][]RichBlockTableCell `json:"cells"`
	IsBordered bool                   `json:"is_bordered,omitempty"`
	IsStriped  bool                   `json:"is_striped,omitempty"`
	Caption    *RichText              `json:"caption,omitempty"`
}

// RichBlockDetails is an expandable block for details disclosure, corresponding to the HTML tag <details> (Bot API 10.1).
type RichBlockDetails struct {
	Type    string     `json:"type"`
	Summary RichText   `json:"summary"`
	Blocks  RichBlocks `json:"blocks"`
	IsOpen  bool       `json:"is_open,omitempty"`
}

// RichBlockMap is a block with a map, corresponding to the custom HTML tag <tg-map> (Bot API 10.1).
type RichBlockMap struct {
	Type     string            `json:"type"`
	Location Location          `json:"location"`
	Zoom     int               `json:"zoom"`
	Width    int               `json:"width"`
	Height   int               `json:"height"`
	Caption  *RichBlockCaption `json:"caption,omitempty"`
}

// RichBlockAnimation is a block with an animation, corresponding to the HTML tag <video> (Bot API 10.1).
type RichBlockAnimation struct {
	Type       string            `json:"type"`
	Animation  Animation         `json:"animation"`
	HasSpoiler bool              `json:"has_spoiler,omitempty"`
	Caption    *RichBlockCaption `json:"caption,omitempty"`
}

// RichBlockAudio is a block with a music file, corresponding to the HTML tag <audio> (Bot API 10.1).
type RichBlockAudio struct {
	Type    string            `json:"type"`
	Audio   Audio             `json:"audio"`
	Caption *RichBlockCaption `json:"caption,omitempty"`
}

// RichBlockPhoto is a block with a photo, corresponding to the HTML tag <img> (Bot API 10.1).
type RichBlockPhoto struct {
	Type       string            `json:"type"`
	Photo      []PhotoSize       `json:"photo"`
	HasSpoiler bool              `json:"has_spoiler,omitempty"`
	Caption    *RichBlockCaption `json:"caption,omitempty"`
}

// RichBlockVideo is a block with a video, corresponding to the HTML tag <video> (Bot API 10.1).
type RichBlockVideo struct {
	Type       string            `json:"type"`
	Video      Video             `json:"video"`
	HasSpoiler bool              `json:"has_spoiler,omitempty"`
	Caption    *RichBlockCaption `json:"caption,omitempty"`
}

// RichBlockVoiceNote is a block with a voice note, corresponding to the HTML tag <audio> (Bot API 10.1).
type RichBlockVoiceNote struct {
	Type      string            `json:"type"`
	VoiceNote Voice             `json:"voice_note"`
	Caption   *RichBlockCaption `json:"caption,omitempty"`
}

// RichBlockThinking is a block with a “Thinking…” placeholder, corresponding to the custom HTML tag <tg-thinking> (Bot API 10.1).
type RichBlockThinking struct {
	Type string   `json:"type"`
	Text RichText `json:"text"`
}

// RichMessage is a rich formatted message (Bot API 10.1).
type RichMessage struct {
	Blocks RichBlocks `json:"blocks"`
	IsRTL  bool       `json:"is_rtl,omitempty"`
}

func (RichTextBold) richTextNode()                   {}
func (RichTextItalic) richTextNode()                 {}
func (RichTextUnderline) richTextNode()              {}
func (RichTextStrikethrough) richTextNode()          {}
func (RichTextSpoiler) richTextNode()                {}
func (RichTextDateTime) richTextNode()               {}
func (RichTextTextMention) richTextNode()            {}
func (RichTextSubscript) richTextNode()              {}
func (RichTextSuperscript) richTextNode()            {}
func (RichTextMarked) richTextNode()                 {}
func (RichTextCode) richTextNode()                   {}
func (RichTextCustomEmoji) richTextNode()            {}
func (RichTextMathematicalExpression) richTextNode() {}
func (RichTextURL) richTextNode()                    {}
func (RichTextEmailAddress) richTextNode()           {}
func (RichTextPhoneNumber) richTextNode()            {}
func (RichTextBankCardNumber) richTextNode()         {}
func (RichTextMention) richTextNode()                {}
func (RichTextHashtag) richTextNode()                {}
func (RichTextCashtag) richTextNode()                {}
func (RichTextBotCommand) richTextNode()             {}
func (RichTextAnchor) richTextNode()                 {}
func (RichTextAnchorLink) richTextNode()             {}
func (RichTextReference) richTextNode()              {}
func (RichTextReferenceLink) richTextNode()          {}

func (RichBlockParagraph) richBlock()              {}
func (RichBlockSectionHeading) richBlock()         {}
func (RichBlockPreformatted) richBlock()           {}
func (RichBlockFooter) richBlock()                 {}
func (RichBlockDivider) richBlock()                {}
func (RichBlockMathematicalExpression) richBlock() {}
func (RichBlockAnchor) richBlock()                 {}
func (RichBlockList) richBlock()                   {}
func (RichBlockBlockQuotation) richBlock()         {}
func (RichBlockPullQuotation) richBlock()          {}
func (RichBlockCollage) richBlock()                {}
func (RichBlockSlideshow) richBlock()              {}
func (RichBlockTable) richBlock()                  {}
func (RichBlockDetails) richBlock()                {}
func (RichBlockMap) richBlock()                    {}
func (RichBlockAnimation) richBlock()              {}
func (RichBlockAudio) richBlock()                  {}
func (RichBlockPhoto) richBlock()                  {}
func (RichBlockVideo) richBlock()                  {}
func (RichBlockVoiceNote) richBlock()              {}
func (RichBlockThinking) richBlock()               {}
