package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// ==================== Bot API 10.0 Scenarios (part a) ====================
//
// answerGuestQuery, getManagedBotAccessSettings, setManagedBotAccessSettings and getUserPersonalChatMessages have
// no scenario: they need a guest summon, a managed bot, or a user with a personal channel (mock tests cover them).

// S51_DeleteReactions sets the bot's reaction on a new message and removes it with deleteMessageReaction, then
// sets it again and removes it with deleteAllMessageReactions. The removal is not observable (no read API).
// deleteAllMessageReactions with the bot's id removes the bot's own reactions in the whole test chat.
func S51_DeleteReactions() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S51-DeleteReactions",
		ScenarioDescription: "deleteMessageReaction and deleteAllMessageReactions for the bot's own reaction (10.0)",
		CoveredMethods:      []string{"deleteMessageReaction", "deleteAllMessageReactions"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.RequireDeleteMessages100Step{},
			&engine.SendMessageStep{Text: "galigo 10.0 reactions"},
			&engine.SetMessageReactionStep{Emoji: "👍"},
			&engine.DeleteReactions100Step{Kind: "single"},
			&engine.SetMessageReactionStep{Emoji: "👍"},
			&engine.DeleteReactions100Step{Kind: "all"},
			&engine.CleanupStep{},
		},
	}
}

// S52_ChatPermissionsReact checks that getChat returns permissions with can_react_to_messages.
func S52_ChatPermissionsReact() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S52-ChatPermissionsReact",
		ScenarioDescription: "getChat permissions carry can_react_to_messages (10.0)",
		CoveredMethods:      []string{"getChat"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps:       []engine.Step{&engine.ChatPermissionsReact100Step{}},
	}
}

// S53_AdminsWithBots calls getChatAdministrators with return_bots (a smoke check: the current bot is always
// listed, so the return_bots effect is proved only by the mock tests).
func S53_AdminsWithBots() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S53-AdminsWithBots",
		ScenarioDescription: "getChatAdministrators with return_bots (10.0)",
		CoveredMethods:      []string{"getChatAdministrators"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps:       []engine.Step{&engine.GetAdminsWithBots100Step{}},
	}
}

// S54_EmptyDraft sends a message draft with an empty text to the admin's private chat.
func S54_EmptyDraft() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S54-EmptyDraft",
		ScenarioDescription: "sendMessageDraft with an empty text (10.0)",
		CoveredMethods:      []string{"sendMessageDraft"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps:       []engine.Step{&engine.EmptyDraft100Step{}},
	}
}

// AllAPI100Scenarios returns the Bot API 10.0 (part a) scenarios.
func AllAPI100Scenarios() []engine.Scenario {
	return []engine.Scenario{
		S51_DeleteReactions(),
		S52_ChatPermissionsReact(),
		S53_AdminsWithBots(),
		S54_EmptyDraft(),
	}
}
