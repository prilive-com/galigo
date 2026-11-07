package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// ==================== Bot API 10.1 Scenarios ====================

// answerChatJoinRequestQuery and sendChatJoinRequestWebApp (10.1) have no scenario: they need a join request
// query, which only a bot assigned as a chat's guard bot receives. Mock tests cover them
// (sender/joinreq101b_test.go).

// S66_PollOptionLink sends a poll whose options have links (no file, multipart without files), then a poll with a
// link and a photo upload; each option's media kind and link URL in the reply is checked. The URLs have a path and
// no fragment, so the server keeps them as sent.
func S66_PollOptionLink() engine.Scenario {
	link1 := sender.NewInputMediaLink("https://example.com/1")
	link2 := sender.NewInputMediaLink("https://example.com/2")
	link := sender.NewInputMediaLink("https://example.com/l")
	photo := sender.NewInputMediaPhoto(sender.FromBytes(fixtures.PhotoBytes(), "photo.jpg"))
	return &engine.BaseScenario{
		ScenarioName:        "S66-PollOptionLink",
		ScenarioDescription: "sendPoll with links as option media (10.1)",
		CoveredMethods:      []string{"sendPoll"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendPollMediaStep{Req: sender.SendPollRequest{Question: "galigo 10.1 poll, option links",
				Options: []sender.InputPollOption{{Text: "link 1", Media: &link1}, {Text: "link 2", Media: &link2},
					{Text: "plain"}}},
				WantOptions: []string{"link", "link", ""},
				WantLinks:   []string{"https://example.com/1", "https://example.com/2", ""}},
			&engine.SendPollMediaStep{Req: sender.SendPollRequest{Question: "galigo 10.1 poll, link and upload",
				Options: []sender.InputPollOption{{Text: "link", Media: &link}, {Text: "photo", Media: &photo}}},
				WantOptions: []string{"link", "photo"}, WantLinks: []string{"https://example.com/l", ""}},
			&engine.CleanupStep{},
		},
	}
}

// S67_RichMessage sends a rich message (markdown), edits it to rich content (html) with editMessageText, and streams a
// rich draft. The draft works in private chats only: in a group the server refuses it and the step is skipped.
func S67_RichMessage() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S67-RichMessage",
		ScenarioDescription: "sendRichMessage, editMessageText with rich_message, sendRichMessageDraft (10.1)",
		CoveredMethods:      []string{"sendRichMessage", "editMessageText", "sendRichMessageDraft"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendRichMessageStep{RichMessage: tg.InputRichMessage{Markdown: "galigo 10.1 **rich** message"},
				WantBlocks: []string{"paragraph"}},
			&engine.EditMessageRichStep{RichMessage: tg.InputRichMessage{HTML: "<p>edited <b>rich</b></p>"},
				WantBlocks: []string{"paragraph"}},
			// Cleanup runs before the draft: in a group the draft is skipped, and a Skip ends the scenario.
			&engine.CleanupStep{},
			&engine.SendRichMessageDraftStep{DraftID: 1, RichMessage: tg.InputRichMessage{Markdown: "galigo 10.1 draft"}},
		},
	}
}

// AllAPI101Scenarios returns the Bot API 10.1 scenarios.
func AllAPI101Scenarios() []engine.Scenario {
	return []engine.Scenario{S66_PollOptionLink(), S67_RichMessage()}
}
