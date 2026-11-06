package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/sender"
)

// ==================== Bot API 10.1 Scenarios ====================

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

// AllAPI101Scenarios returns the Bot API 10.1 scenarios.
func AllAPI101Scenarios() []engine.Scenario {
	return []engine.Scenario{S66_PollOptionLink()}
}
