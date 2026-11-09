package tg

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
)

// Bot API 10.2 rich messages, send side: the blocks of InputRichMessage.Blocks.

// InputRichBlock is a block of a rich message to send (Bot API 10.2): one of the InputRichBlock* types. Each type
// writes its own "type" value when encoded; the caller does not set it.
type InputRichBlock interface {
	inputRichBlock()
}

// marshalInputRichBlock encodes v (a struct without a "type" field) as a JSON object whose first key is "type" with
// the value typ.
func marshalInputRichBlock(typ string, v any) ([]byte, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	t, err := json.Marshal(typ)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString(`{"type":`)
	buf.Write(t)
	if inner := body[1 : len(body)-1]; len(inner) > 0 {
		buf.WriteByte(',')
		buf.Write(inner)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// maxInputRichBlockDepth bounds how deep blocks may be nested inside each other. It is a safety bound, not a Bot API
// rule: each block encodes its children with a fresh json.Marshal, so the encoder's own cycle check cannot see a
// cycle through nested blocks, and without this bound a cyclic value would overflow the stack.
const maxInputRichBlockDepth = 100

var errInputRichBlockDepth = errors.New("tg: input rich blocks nested more than 100 levels deep (or in a cycle)")

// inputRichBlockParent is a block that holds other blocks. A type that embeds such a block inherits the method, so the
// walk below also sees a container wrapped in a caller's type.
type inputRichBlockParent interface {
	inputRichBlockChildren() [][]InputRichBlock
}

// checkInputRichBlockDepth returns an error when blocks, at nesting depth depth, hold blocks nested deeper than
// maxInputRichBlockDepth. It always ends, also on a cycle. A nil pointer has no children.
func checkInputRichBlockDepth(blocks []InputRichBlock, depth int) error {
	if depth > maxInputRichBlockDepth {
		return errInputRichBlockDepth
	}
	for _, b := range blocks {
		p, ok := b.(inputRichBlockParent)
		if !ok {
			continue
		}
		if v := reflect.ValueOf(p); v.Kind() == reflect.Pointer && v.IsNil() {
			continue
		}
		for _, c := range p.inputRichBlockChildren() {
			if err := checkInputRichBlockDepth(c, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// listItemBlocks returns the blocks of each list item.
func listItemBlocks(items []InputRichBlockListItem) [][]InputRichBlock {
	out := make([][]InputRichBlock, len(items))
	for i, it := range items {
		out[i] = it.Blocks
	}
	return out
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

// MarshalJSON encodes the block with "type":"paragraph".
func (b InputRichBlockParagraph) MarshalJSON() ([]byte, error) {
	type t InputRichBlockParagraph
	return marshalInputRichBlock("paragraph", t(b))
}

// MarshalJSON encodes the block with "type":"heading".
func (b InputRichBlockSectionHeading) MarshalJSON() ([]byte, error) {
	type t InputRichBlockSectionHeading
	return marshalInputRichBlock("heading", t(b))
}

// MarshalJSON encodes the block with "type":"pre".
func (b InputRichBlockPreformatted) MarshalJSON() ([]byte, error) {
	type t InputRichBlockPreformatted
	return marshalInputRichBlock("pre", t(b))
}

// MarshalJSON encodes the block with "type":"footer".
func (b InputRichBlockFooter) MarshalJSON() ([]byte, error) {
	type t InputRichBlockFooter
	return marshalInputRichBlock("footer", t(b))
}

// MarshalJSON encodes the block with "type":"divider".
func (b InputRichBlockDivider) MarshalJSON() ([]byte, error) {
	type t InputRichBlockDivider
	return marshalInputRichBlock("divider", t(b))
}

// MarshalJSON encodes the block with "type":"mathematical_expression".
func (b InputRichBlockMathematicalExpression) MarshalJSON() ([]byte, error) {
	type t InputRichBlockMathematicalExpression
	return marshalInputRichBlock("mathematical_expression", t(b))
}

// MarshalJSON encodes the block with "type":"anchor".
func (b InputRichBlockAnchor) MarshalJSON() ([]byte, error) {
	type t InputRichBlockAnchor
	return marshalInputRichBlock("anchor", t(b))
}

// MarshalJSON encodes the block with "type":"list".
func (b InputRichBlockList) MarshalJSON() ([]byte, error) {
	if err := checkInputRichBlockDepth([]InputRichBlock{b}, 0); err != nil {
		return nil, err
	}
	type t InputRichBlockList
	return marshalInputRichBlock("list", t(b))
}

// MarshalJSON encodes the block with "type":"blockquote".
func (b InputRichBlockBlockQuotation) MarshalJSON() ([]byte, error) {
	if err := checkInputRichBlockDepth([]InputRichBlock{b}, 0); err != nil {
		return nil, err
	}
	type t InputRichBlockBlockQuotation
	return marshalInputRichBlock("blockquote", t(b))
}

// MarshalJSON encodes the block with "type":"pullquote".
func (b InputRichBlockPullQuotation) MarshalJSON() ([]byte, error) {
	type t InputRichBlockPullQuotation
	return marshalInputRichBlock("pullquote", t(b))
}

// MarshalJSON encodes the block with "type":"collage".
func (b InputRichBlockCollage) MarshalJSON() ([]byte, error) {
	if err := checkInputRichBlockDepth([]InputRichBlock{b}, 0); err != nil {
		return nil, err
	}
	type t InputRichBlockCollage
	return marshalInputRichBlock("collage", t(b))
}

// MarshalJSON encodes the block with "type":"slideshow".
func (b InputRichBlockSlideshow) MarshalJSON() ([]byte, error) {
	if err := checkInputRichBlockDepth([]InputRichBlock{b}, 0); err != nil {
		return nil, err
	}
	type t InputRichBlockSlideshow
	return marshalInputRichBlock("slideshow", t(b))
}

// MarshalJSON encodes the block with "type":"table".
func (b InputRichBlockTable) MarshalJSON() ([]byte, error) {
	type t InputRichBlockTable
	return marshalInputRichBlock("table", t(b))
}

// MarshalJSON encodes the block with "type":"details".
func (b InputRichBlockDetails) MarshalJSON() ([]byte, error) {
	if err := checkInputRichBlockDepth([]InputRichBlock{b}, 0); err != nil {
		return nil, err
	}
	type t InputRichBlockDetails
	return marshalInputRichBlock("details", t(b))
}

// MarshalJSON encodes the block with "type":"map".
func (b InputRichBlockMap) MarshalJSON() ([]byte, error) {
	type t InputRichBlockMap
	return marshalInputRichBlock("map", t(b))
}

// MarshalJSON encodes the block with "type":"thinking".
func (b InputRichBlockThinking) MarshalJSON() ([]byte, error) {
	type t InputRichBlockThinking
	return marshalInputRichBlock("thinking", t(b))
}

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

func (b InputRichBlockList) inputRichBlockChildren() [][]InputRichBlock {
	return listItemBlocks(b.Items)
}
func (b InputRichBlockBlockQuotation) inputRichBlockChildren() [][]InputRichBlock {
	return [][]InputRichBlock{b.Blocks}
}
func (b InputRichBlockCollage) inputRichBlockChildren() [][]InputRichBlock {
	return [][]InputRichBlock{b.Blocks}
}
func (b InputRichBlockSlideshow) inputRichBlockChildren() [][]InputRichBlock {
	return [][]InputRichBlock{b.Blocks}
}
func (b InputRichBlockDetails) inputRichBlockChildren() [][]InputRichBlock {
	return [][]InputRichBlock{b.Blocks}
}
