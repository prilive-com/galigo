package tg

// RED STUB (B-25 10.2d TEST-PLAN §6): the planned types, fields, tags and markers; MarshalJSON writes {}; no depth
// guard. Not for the repository.

// InputRichBlock is a block of a rich message to send (Bot API 10.2).
type InputRichBlock interface {
	inputRichBlock()
}

// InputRichBlockParagraph is a text paragraph, corresponding to the HTML tag <p> (Bot API 10.2).
type InputRichBlockParagraph struct {
	Text RichText `json:"text"`
}

// InputRichBlockSectionHeading is a section heading, corresponding to the HTML tags <h1>-<h6> (Bot API 10.2). Size is
// the relative font size, 1-6 (1 is the largest).
type InputRichBlockSectionHeading struct {
	Text RichText `json:"text"`
	Size int      `json:"size"`
}

// InputRichBlockPreformatted is a preformatted text block, corresponding to the HTML tags <pre> and <code> (Bot API
// 10.2).
type InputRichBlockPreformatted struct {
	Text     RichText `json:"text"`
	Language string   `json:"language,omitempty"`
}

// InputRichBlockFooter is a footer, corresponding to the HTML tag <footer> (Bot API 10.2).
type InputRichBlockFooter struct {
	Text RichText `json:"text"`
}

// InputRichBlockDivider is a divider, corresponding to the HTML tag <hr/> (Bot API 10.2).
type InputRichBlockDivider struct{}

// InputRichBlockMathematicalExpression is a block with a mathematical expression in LaTeX format (Bot API 10.2).
type InputRichBlockMathematicalExpression struct {
	Expression string `json:"expression"`
}

// InputRichBlockAnchor is a block with an anchor, corresponding to the HTML tag <a> with the attribute name (Bot API
// 10.2).
type InputRichBlockAnchor struct {
	Name string `json:"name"`
}

// InputRichBlockListItem is an item of a list to send (Bot API 10.2). Value and Type are the label of an ordered list
// item: Value is not sent when nil (a pointer to 0 sends 0); Type is one of "a", "A", "i", "I", "1".
type InputRichBlockListItem struct {
	Blocks      []InputRichBlock `json:"blocks"`
	HasCheckbox bool             `json:"has_checkbox,omitempty"`
	IsChecked   bool             `json:"is_checked,omitempty"`
	Value       *int             `json:"value,omitempty"`
	Type        string           `json:"type,omitempty"`
}

// InputRichBlockList is a list of blocks, corresponding to the HTML tag <ul> or <ol> (Bot API 10.2).
type InputRichBlockList struct {
	Items []InputRichBlockListItem `json:"items"`
}

// InputRichBlockBlockQuotation is a block quotation, corresponding to the HTML tag <blockquote> (Bot API 10.2).
type InputRichBlockBlockQuotation struct {
	Blocks []InputRichBlock `json:"blocks"`
	Credit *RichText        `json:"credit,omitempty"`
}

// InputRichBlockPullQuotation is a quotation with centered text, loosely corresponding to the HTML tag <aside> (Bot
// API 10.2).
type InputRichBlockPullQuotation struct {
	Text   RichText  `json:"text"`
	Credit *RichText `json:"credit,omitempty"`
}

// InputRichBlockCollage is a collage, corresponding to the custom HTML tag <tg-collage> (Bot API 10.2).
type InputRichBlockCollage struct {
	Blocks  []InputRichBlock  `json:"blocks"`
	Caption *RichBlockCaption `json:"caption,omitempty"`
}

// InputRichBlockSlideshow is a slideshow, corresponding to the custom HTML tag <tg-slideshow> (Bot API 10.2).
type InputRichBlockSlideshow struct {
	Blocks  []InputRichBlock  `json:"blocks"`
	Caption *RichBlockCaption `json:"caption,omitempty"`
}

// InputRichBlockTable is a table, corresponding to the HTML tag <table> (Bot API 10.2).
type InputRichBlockTable struct {
	Cells      [][]RichBlockTableCell `json:"cells"`
	IsBordered bool                   `json:"is_bordered,omitempty"`
	IsStriped  bool                   `json:"is_striped,omitempty"`
	Caption    *RichText              `json:"caption,omitempty"`
}

// InputRichBlockDetails is an expandable block for details disclosure, corresponding to the HTML tag <details> (Bot
// API 10.2).
type InputRichBlockDetails struct {
	Summary RichText         `json:"summary"`
	Blocks  []InputRichBlock `json:"blocks"`
	IsOpen  bool             `json:"is_open,omitempty"`
}

// InputRichBlockMap is a block with a map, corresponding to the custom HTML tag <tg-map> (Bot API 10.2). Zoom (0-24),
// Width and Height (0-10000) are not sent when nil; a pointer to 0 sends 0.
type InputRichBlockMap struct {
	Location Location          `json:"location"`
	Zoom     *int              `json:"zoom,omitempty"`
	Width    *int              `json:"width,omitempty"`
	Height   *int              `json:"height,omitempty"`
	Caption  *RichBlockCaption `json:"caption,omitempty"`
}

// InputRichBlockThinking is a block with a "Thinking…" placeholder, corresponding to the custom HTML tag
// <tg-thinking> (Bot API 10.2). It may be used only in sendRichMessageDraft.
type InputRichBlockThinking struct {
	Text RichText `json:"text"`
}

// MarshalJSON is the red stub: it writes an empty object.
func (b InputRichBlockParagraph) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

// MarshalJSON is the red stub: it writes an empty object.
func (b InputRichBlockSectionHeading) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

// MarshalJSON is the red stub: it writes an empty object.
func (b InputRichBlockPreformatted) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

// MarshalJSON is the red stub: it writes an empty object.
func (b InputRichBlockFooter) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

// MarshalJSON is the red stub: it writes an empty object.
func (b InputRichBlockDivider) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

// MarshalJSON is the red stub: it writes an empty object.
func (b InputRichBlockMathematicalExpression) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

// MarshalJSON is the red stub: it writes an empty object.
func (b InputRichBlockAnchor) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

// MarshalJSON is the red stub: it writes an empty object.
func (b InputRichBlockList) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

// MarshalJSON is the red stub: it writes an empty object.
func (b InputRichBlockBlockQuotation) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

// MarshalJSON is the red stub: it writes an empty object.
func (b InputRichBlockPullQuotation) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

// MarshalJSON is the red stub: it writes an empty object.
func (b InputRichBlockCollage) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

// MarshalJSON is the red stub: it writes an empty object.
func (b InputRichBlockSlideshow) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

// MarshalJSON is the red stub: it writes an empty object.
func (b InputRichBlockTable) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

// MarshalJSON is the red stub: it writes an empty object.
func (b InputRichBlockDetails) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

// MarshalJSON is the red stub: it writes an empty object.
func (b InputRichBlockMap) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

// MarshalJSON is the red stub: it writes an empty object.
func (b InputRichBlockThinking) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

func (InputRichBlockParagraph) inputRichBlock()              {}
func (InputRichBlockSectionHeading) inputRichBlock()         {}
func (InputRichBlockPreformatted) inputRichBlock()           {}
func (InputRichBlockFooter) inputRichBlock()                 {}
func (InputRichBlockDivider) inputRichBlock()                {}
func (InputRichBlockMathematicalExpression) inputRichBlock() {}
func (InputRichBlockAnchor) inputRichBlock()                 {}
func (InputRichBlockList) inputRichBlock()                   {}
func (InputRichBlockBlockQuotation) inputRichBlock()         {}
func (InputRichBlockPullQuotation) inputRichBlock()          {}
func (InputRichBlockCollage) inputRichBlock()                {}
func (InputRichBlockSlideshow) inputRichBlock()              {}
func (InputRichBlockTable) inputRichBlock()                  {}
func (InputRichBlockDetails) inputRichBlock()                {}
func (InputRichBlockMap) inputRichBlock()                    {}
func (InputRichBlockThinking) inputRichBlock()               {}
