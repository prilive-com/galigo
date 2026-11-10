package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// s68Text is a plain RichText.
func s68Text(s string) tg.RichText { return tg.RichText{Plain: s} }

// s68Blocks are the blocks S68 sends: every block of Bot API 10.2 that can be sent in a message without media,
// except the collage and the slideshow (the server refuses them with text blocks only:
// RICH_MESSAGE_BLOCK_UNEXPECTED; S69 sends them with photo blocks) and the thinking block (drafts only).
func s68Blocks() []tg.InputRichBlock {
	zoom := 12
	cell := func(s string) tg.RichBlockTableCell {
		t := s68Text(s)
		return tg.RichBlockTableCell{Text: &t, Align: "left", VAlign: "top"}
	}
	return []tg.InputRichBlock{
		tg.InputRichBlockSectionHeading{Text: s68Text("galigo 10.2 blocks"), Size: 2},
		tg.InputRichBlockParagraph{Text: s68Text("paragraph")},
		tg.InputRichBlockPreformatted{Text: s68Text("x := 1"), Language: "go"},
		tg.InputRichBlockDivider{},
		tg.InputRichBlockMathematicalExpression{Expression: "a^2+b^2=c^2"},
		tg.InputRichBlockAnchor{Name: "galigo"},
		tg.InputRichBlockList{Items: []tg.InputRichBlockListItem{
			{Blocks: []tg.InputRichBlock{tg.InputRichBlockParagraph{Text: s68Text("item")}}}}},
		tg.InputRichBlockBlockQuotation{Blocks: []tg.InputRichBlock{tg.InputRichBlockParagraph{Text: s68Text("quote")}}},
		tg.InputRichBlockPullQuotation{Text: s68Text("pull")},
		tg.InputRichBlockTable{Cells: [][]tg.RichBlockTableCell{{cell("a"), cell("b")}}, IsBordered: true},
		tg.InputRichBlockDetails{Summary: s68Text("details"),
			Blocks: []tg.InputRichBlock{tg.InputRichBlockParagraph{Text: s68Text("inside")}}},
		tg.InputRichBlockMap{Location: tg.Location{Latitude: 52.37, Longitude: 4.89}, Zoom: &zoom},
		tg.InputRichBlockFooter{Text: s68Text("footer")},
	}
}

// S68_RichBlocks sends a rich message made of blocks and checks the block kinds the server returns, then streams a
// draft with a thinking block. The draft works in private chats only: in a group the server refuses it and the step
// is skipped.
func S68_RichBlocks() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S68-RichBlocks",
		ScenarioDescription: "sendRichMessage and sendRichMessageDraft with rich_message.blocks (10.2)",
		CoveredMethods:      []string{"sendRichMessage", "sendRichMessageDraft"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendRichMessageStep{RichMessage: tg.InputRichMessage{Blocks: s68Blocks()},
				WantBlocks: []string{"heading", "paragraph", "pre", "divider", "mathematical_expression", "anchor",
					"list", "blockquote", "pullquote", "table", "details", "map", "footer"}},
			// Cleanup runs before the draft: in a group the draft is skipped, and a Skip ends the scenario.
			&engine.CleanupStep{},
			&engine.SendRichMessageDraftStep{DraftID: 1, RichMessage: tg.InputRichMessage{
				Blocks: []tg.InputRichBlock{tg.InputRichBlockThinking{Text: s68Text("Thinking")}}}},
		},
	}
}

// s69Media is the media list S69 uploads, one file of each kind, for the links of s69Markdown. The animation is the
// MP4 fixture: the server refuses the GIF fixture as rich message media (RICH_MESSAGE_VIDEO_INVALID).
func s69Media() []tg.InputRichMessageMedia {
	return []tg.InputRichMessageMedia{
		{ID: "p", Media: sender.NewInputMediaPhoto(sender.FromBytes(fixtures.PhotoBytes(), "photo.jpg"))},
		{ID: "v", Media: sender.NewInputMediaVideo(sender.FromBytes(fixtures.VideoBytes(), "video.mp4"))},
		{ID: "a", Media: sender.NewInputMediaAudio(sender.FromBytes(fixtures.AudioBytes(), "audio.mp3"))},
		{ID: "g", Media: sender.NewInputMediaAnimation(sender.FromBytes(fixtures.VideoBytes(), "animation.mp4"))},
		{ID: "n", Media: sender.NewInputMediaVoiceNote(sender.FromBytes(fixtures.VoiceBytes(), "voice.ogg"))},
	}
}

// s69Markdown links the media of s69Media. The server takes tg://photo, tg://video (also an animation) and
// tg://audio (also a voice note) links; tg://animation, tg://voice_note and tg://voice links are refused.
const s69Markdown = "galigo 10.2 media\n\n![](tg://photo?id=p)\n\n![](tg://video?id=v)\n\n![](tg://audio?id=a)\n\n" +
	"![](tg://video?id=g)\n\n![](tg://audio?id=n)"

// S69_RichMedia uploads one file of each media kind in a rich message's media list (multipart), sends the five media
// blocks, a collage and a slideshow by the file_ids the server returned, and edits that message to a rich message with
// a new photo upload.
func S69_RichMedia() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S69-RichMedia",
		ScenarioDescription: "sendRichMessage and editMessageText with rich_message.media uploads and media blocks (10.2)",
		CoveredMethods:      []string{"sendRichMessage", "editMessageText"},
		ScenarioTimeout:     120 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendRichMessageStep{RichMessage: tg.InputRichMessage{Markdown: s69Markdown, Media: s69Media()},
				WantBlocks: []string{"paragraph", "photo", "video", "audio", "animation", "voice_note"}},
			&engine.SendRichMediaBlocksStep{
				WantBlocks: []string{"photo", "video", "audio", "animation", "voice_note", "collage", "slideshow"}},
			&engine.EditMessageRichStep{RichMessage: tg.InputRichMessage{Markdown: "galigo 10.2 edit\n\n![](tg://photo?id=q)",
				Media: []tg.InputRichMessageMedia{
					{ID: "q", Media: sender.NewInputMediaPhoto(sender.FromBytes(fixtures.PhotoBytes(), "photo.jpg"))}}},
				WantBlocks: []string{"paragraph", "photo"}},
			&engine.CleanupStep{},
		},
	}
}

// AllAPI102Scenarios returns the Bot API 10.2 scenarios.
func AllAPI102Scenarios() []engine.Scenario {
	return []engine.Scenario{S68_RichBlocks(), S69_RichMedia()}
}
