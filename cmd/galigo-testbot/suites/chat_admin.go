package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// S15_ChatInfo tests chat information retrieval methods.
func S15_ChatInfo() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S15-ChatInfo",
		ScenarioDescription: "Get chat info, admins, member count, and member status",
		CoveredMethods:      []string{"getChat", "getChatAdministrators", "getChatMemberCount", "getChatMember"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.GetChatStep{},
			&engine.GetChatAdministratorsStep{},
			&engine.GetChatMemberCountStep{},
			&engine.GetChatMemberStep{}, // Gets bot's own member info
		},
	}
}

// S16_ChatSettings tests setting chat title and description (reversible).
func S16_ChatSettings() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S16-ChatSettings",
		ScenarioDescription: "Set and restore chat title and description",
		CoveredMethods:      []string{"setChatTitle", "setChatDescription"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SaveChatTitleStep{},
			&engine.SetChatTitleStep{Title: "[galigo-test] Settings Test"},
			&engine.SetChatDescriptionStep{Description: "galigo-testbot: testing setChatDescription"},
			&engine.RestoreChatTitleStep{},
		},
	}
}

// S17_PinMessages tests pin/unpin message operations.
func S17_PinMessages() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S17-PinMessages",
		ScenarioDescription: "Pin, unpin single, and unpin all messages",
		CoveredMethods:      []string{"pinChatMessage", "unpinChatMessage", "unpinAllChatMessages"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendMessageStep{Text: "[galigo-test] Pin test message"},
			&engine.PinChatMessageStep{Silent: true},
			&engine.UnpinChatMessageStep{},
			&engine.PinChatMessageStep{Silent: true},
			&engine.UnpinAllChatMessagesStep{},
			&engine.CleanupStep{},
		},
	}
}

// S18_Polls tests poll creation and stopping.
func S18_Polls() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S18-Polls",
		ScenarioDescription: "Send simple poll, quiz, and stop poll",
		CoveredMethods:      []string{"sendPoll", "stopPoll"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendPollSimpleStep{
				Question: "[galigo-test] Favorite color?",
				Options:  []string{"Red", "Green", "Blue"},
			},
			&engine.StopPollStep{},
			&engine.SendQuizStep{
				Question:        "[galigo-test] Capital of France?",
				Options:         []string{"London", "Paris", "Berlin"},
				CorrectOptionID: 1,
			},
			&engine.CleanupStep{},
		},
	}
}

// S19_ForumStickers tests retrieving forum topic icon stickers.
func S19_ForumStickers() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S19-ForumStickers",
		ScenarioDescription: "Get available forum topic icon stickers",
		CoveredMethods:      []string{"getForumTopicIconStickers"},
		ScenarioTimeout:     15 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.GetForumTopicIconStickersStep{},
		},
	}
}

// AllChatAdminScenarios returns all chat administration scenarios.
func AllChatAdminScenarios() []engine.Scenario {
	return []engine.Scenario{
		S15_ChatInfo(),
		S16_ChatSettings(),
		S17_PinMessages(),
		S18_Polls(),
		S19_ForumStickers(),
	}
}

// ChatAdminCoveredMethods returns all methods covered by chat admin scenarios.
func ChatAdminCoveredMethods() []string {
	var methods []string
	for _, s := range AllChatAdminScenarios() {
		methods = append(methods, s.Covers()...)
	}
	return methods
}
