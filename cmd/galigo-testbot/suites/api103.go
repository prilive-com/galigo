package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// s74Text is a plain RichText.
func s74Text(s string) tg.RichText { return tg.RichText{Plain: s} }

// s74Blocks are the 10.3 blocks S74 sends without media: a row of four buttons (callback, URL, copy_text, disabled),
// a paragraph with a button node, a compact table and an expandable quotation. engine.VerifyRichAPI103Step checks
// the reply against them.
func s74Blocks() []tg.InputRichBlock {
	cell := func(s string) tg.RichBlockTableCell {
		t := s74Text(s)
		return tg.RichBlockTableCell{Text: &t, Align: "left", VAlign: "top"}
	}
	return []tg.InputRichBlock{
		tg.InputRichBlockButtons{Align: "center", Buttons: []tg.RichMessageButton{
			{Text: s74Text("callback"), Style: tg.ButtonStylePrimary, CallbackData: "s74"},
			{Text: s74Text("url"), URL: "https://t.me/"},
			{Text: s74Text("copy"), CopyText: &tg.CopyTextButton{Text: "galigo"}},
			{Text: s74Text("off"), Disabled: &tg.DisabledButton{}},
		}},
		tg.InputRichBlockParagraph{Text: tg.RichText{Parts: []tg.RichText{s74Text("galigo 10.3 "),
			{Node: tg.RichTextButton{Type: "button",
				Button: tg.RichMessageButton{Text: s74Text("node"), CallbackData: "s74-node"}}}}}},
		tg.InputRichBlockTable{Cells: [][]tg.RichBlockTableCell{{cell("a"), cell("b")}}, IsCompact: true},
		tg.InputRichBlockExpandableBlockQuotation{Text: s74Text("galigo 10.3 expandable"), Credit: &tg.RichText{Plain: "galigo"}},
	}
}

// S74_RichAPI103 sends the Bot API 10.3 rich message kinds and checks how they come back: a buttons block, a button
// node, a compact table and an expandable quotation; then a document upload by a tg://document link and a document
// block by the file_id the server returned.
func S74_RichAPI103() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S74-RichAPI103",
		ScenarioDescription: "sendRichMessage with the Bot API 10.3 rich message kinds and the document block",
		CoveredMethods:      []string{"sendRichMessage"},
		ScenarioTimeout:     90 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendRichMessageStep{RichMessage: tg.InputRichMessage{Blocks: s74Blocks()},
				WantBlocks: []string{"buttons", "paragraph", "table", "expandable_blockquote"}},
			&engine.VerifyRichAPI103Step{},
			&engine.SendRichMessageStep{RichMessage: tg.InputRichMessage{
				Markdown: "galigo 10.3 document\n\n![](tg://document?id=d)",
				Media: []tg.InputRichMessageMedia{
					{ID: "d", Media: sender.NewInputMediaDocument(sender.FromBytes(fixtures.DocumentBytes(), "document.txt"))}}},
				WantBlocks: []string{"paragraph", "document"}},
			&engine.SendRichDocumentBlockStep{WantBlocks: []string{"document"}},
			&engine.CleanupStep{},
		},
	}
}

// AllAPI103Scenarios returns the Bot API 10.3 rich message scenarios (the 10.3 ephemeral changes are in
// AllEphemeralScenarios, S73).
func AllAPI103Scenarios() []engine.Scenario {
	return []engine.Scenario{S74_RichAPI103()}
}
