package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/tg"
)

// AllAPI104Scenarios returns the Bot API 10.4 scenarios: S76 widget methods, refusal only (B-65 65f), S77 rich
// quotes (B-65 65g) and S78 share theme backdrops (B-65 65h, read only). The 10.4 widget read side is decode-only
// (B-65 65e). S76's rows run only in a group or supergroup; S77 runs in an ordinary chat (private, group, supergroup),
// not in a channel direct messages chat; S78 runs in any chat (the method has no chat id).
func AllAPI104Scenarios() []engine.Scenario {
	return []engine.Scenario{S76_Widgets(), S77_RichQuote(), S78_ShareThemeBackdrops()}
}

// s77QuoteInvalid is the refusal of a quote that is not in the message (measured 2026-11-22 in the test group).
const s77QuoteInvalid = "QUOTE_TEXT_INVALID"

// S77_RichQuote sends a rich message with two paragraphs (the first with a bold node) and replies to it three times
// with ReplyParameters.QuoteRichTexts (Bot API 10.4): plain text, a bold node, and two quoted texts that span both
// paragraphs. Each reply's quote must come back with the same rich texts, position and is_manual. A fourth reply
// quotes text that is not in the message and must get the measured 400. The cleanup deletes the four messages.
// Positions are UTF-16 indexes in the joined plain text "galigo quote bold end\nsecond para".
func S77_RichQuote() engine.Scenario {
	bold := tg.RichText{Node: tg.RichTextBold{Type: "bold", Text: tg.RichText{Plain: "bold"}}}
	return &engine.BaseScenario{
		ScenarioName:        "S77-RichQuote",
		ScenarioDescription: "Replies quote parts of a rich message with quote_rich_texts (Bot API 10.4)",
		CoveredMethods:      []string{"sendRichMessage", "sendMessage"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendRichMessageStep{RichMessage: tg.InputRichMessage{Blocks: []tg.InputRichBlock{
				tg.InputRichBlockParagraph{Text: tg.RichText{Parts: []tg.RichText{{Plain: "galigo quote "}, bold, {Plain: " end"}}}},
				tg.InputRichBlockParagraph{Text: tg.RichText{Plain: "second para"}},
			}}, WantBlocks: []string{"paragraph", "paragraph"}},
			&engine.ReplyRichQuoteStep{Label: "plain", Texts: []tg.RichText{{Plain: "quote"}}, Position: 7},
			&engine.ReplyRichQuoteStep{Label: "bold", Texts: []tg.RichText{{Parts: []tg.RichText{{Plain: "quote "}, bold}}}, Position: 7},
			&engine.ReplyRichQuoteStep{Label: "two texts", Texts: []tg.RichText{{Plain: "end"}, {Plain: "second"}}, Position: 18},
			&engine.ReplyRichQuoteStep{Label: "not in the message", Texts: []tg.RichText{{Plain: "absent text"}}, WantError: s77QuoteInvalid},
			&engine.CleanupStep{},
		},
	}
}

// S76_Widgets calls the five widget methods (Bot API 10.4), refusal only: the server refuses sendWidget for this bot
// (measured 2026-11-21 in the test group and in a private chat), so each row must get its measured 400 text. A
// refusal shows that the method reached the server and took that refusal branch; it does not show that every field
// was read (the same text comes without them), which the mock tests prove. The steps run only in a group or
// supergroup and skip elsewhere. The plain message is the target of the two editMessageWidgetData chat rows and is
// deleted by the cleanup.
func S76_Widgets() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S76-Widgets",
		ScenarioDescription: "Widget methods reach the server and get their measured refusals (Bot API 10.4)",
		CoveredMethods: []string{"sendWidget", "sendSharedWidget", "answerWidgetQuery", "editMessageWidgetData",
			"editSharedWidgetData", "sendMessage"},
		ScenarioTimeout: 60 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.WidgetRefusalStep{Method: "sendWidget", WantError: engine.WidgetNotAllowed},
			&engine.SendMessageStep{Text: "galigo 10.4 widget target"},
			&engine.WidgetRefusalStep{Method: "sendSharedWidget", WantError: engine.WidgetIDInvalid},
			&engine.WidgetRefusalStep{Method: "answerWidgetQuery", WantError: engine.WidgetQueryInvalid},
			&engine.WidgetRefusalStep{Method: "editMessageWidgetData", Variant: "target", WantError: engine.WidgetMessageInvalid},
			&engine.WidgetRefusalStep{Method: "editMessageWidgetData", Variant: "missing", WantError: engine.WidgetMessageNotFound},
			&engine.WidgetRefusalStep{Method: "editMessageWidgetData", Variant: "inline", WantError: engine.WidgetInlineInvalid},
			&engine.WidgetRefusalStep{Method: "editSharedWidgetData", WantError: engine.WidgetIDInvalid},
			&engine.CleanupStep{},
		},
	}
}

// S78_ShareThemeBackdrops gets the chat share theme backdrops (Bot API 10.4). Read only: no message, no cleanup.
func S78_ShareThemeBackdrops() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S78-ShareThemeBackdrops",
		ScenarioDescription: "Get the chat share theme backdrops (Bot API 10.4, read only)",
		CoveredMethods:      []string{"getChatShareThemeBackdrops"},
		ScenarioSteps:       []engine.Step{&engine.GetChatShareThemeBackdropsStep{}},
		ScenarioTimeout:     30 * time.Second,
	}
}
