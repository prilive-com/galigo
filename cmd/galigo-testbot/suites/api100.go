package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/sender"
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

// ==================== Bot API 10.0 Scenarios (part b: live photos) ====================

// S57_SendLivePhoto uploads a live photo (the fixture video and photo), then sends it again by the captured ids.
func S57_SendLivePhoto() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S57-SendLivePhoto",
		ScenarioDescription: "sendLivePhoto by upload and by file_id (10.0)",
		CoveredMethods:      []string{"sendLivePhoto"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendLivePhoto100Step{},
			&engine.SendLivePhoto100Step{ByFileID: true},
			&engine.CleanupStep{},
		},
	}
}

// S58_LivePhotoAlbum sends an album of two live photos by upload, then by the ids captured from a single upload.
func S58_LivePhotoAlbum() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S58-LivePhotoAlbum",
		ScenarioDescription: "sendMediaGroup with live photo items, by upload and by file_id (10.0)",
		CoveredMethods:      []string{"sendMediaGroup"},
		ScenarioTimeout:     90 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendLivePhoto100Step{},
			&engine.LivePhotoAlbum100Step{},
			&engine.LivePhotoAlbum100Step{ByFileID: true},
			&engine.CleanupStep{},
		},
	}
}

// S59_EditToLivePhoto edits a plain photo message into a live photo with editMessageMedia.
func S59_EditToLivePhoto() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S59-EditToLivePhoto",
		ScenarioDescription: "editMessageMedia from a photo into a live photo (10.0)",
		CoveredMethods:      []string{"editMessageMedia"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendLivePhoto100Step{},
			&engine.SendPhotoStep{
				Photo:   engine.MediaFromBytes(fixtures.PhotoBytes(), "galigo-live-edit.jpg", "photo"),
				Caption: "galigo 10.0 live photo: plain photo, will be replaced",
			},
			&engine.EditToLivePhoto100Step{},
			&engine.CleanupStep{},
		},
	}
}

// S60_PollOptionCount sends polls with 1 and with 12 options (Bot API 10.0: 1-12 options).
func S60_PollOptionCount() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S60-PollOptionCount",
		ScenarioDescription: "sendPoll with 1 and with 12 options (10.0)",
		CoveredMethods:      []string{"sendPoll"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendPollSimpleStep{Question: "galigo 10.0 poll, 1 option", Options: []string{"only"}},
			&engine.SendPollSimpleStep{Question: "galigo 10.0 poll, 12 options", Options: []string{
				"o1", "o2", "o3", "o4", "o5", "o6", "o7", "o8", "o9", "o10", "o11", "o12"}},
			&engine.CleanupStep{},
		},
	}
}

// S61_PollMembersOnly sends a poll with members_only. In a group the server refuses it (channel chats only),
// which the step accepts as a smoke check; in a channel the reply's members_only is checked.
func S61_PollMembersOnly() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S61-PollMembersOnly",
		ScenarioDescription: "sendPoll with members_only (10.0; channel chats only)",
		CoveredMethods:      []string{"sendPoll"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendPoll100Step{Req: sender.SendPollRequest{Question: "galigo 10.0 poll, members only",
				Options: []sender.InputPollOption{{Text: "a"}, {Text: "b"}}, MembersOnly: true}},
			&engine.CleanupStep{},
		},
	}
}

// S62_PollCountryCodes sends a poll with country_codes, like S61.
func S62_PollCountryCodes() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S62-PollCountryCodes",
		ScenarioDescription: "sendPoll with country_codes (10.0; channel chats only)",
		CoveredMethods:      []string{"sendPoll"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendPoll100Step{Req: sender.SendPollRequest{Question: "galigo 10.0 poll, country codes",
				Options: []sender.InputPollOption{{Text: "a"}, {Text: "b"}}, CountryCodes: []string{"DE", "FT"}}},
			&engine.CleanupStep{},
		},
	}
}

// AllLivePhotoScenarios returns the Bot API 10.0 live photo scenarios (the live-photos sub-suite).
func AllLivePhotoScenarios() []engine.Scenario {
	return []engine.Scenario{S57_SendLivePhoto(), S58_LivePhotoAlbum(), S59_EditToLivePhoto()}
}

// AllAPI100Scenarios returns the Bot API 10.0 scenarios (part a, live photos of part b, polls of part c).
func AllAPI100Scenarios() []engine.Scenario {
	s := append([]engine.Scenario{
		S51_DeleteReactions(),
		S52_ChatPermissionsReact(),
		S53_AdminsWithBots(),
		S54_EmptyDraft(),
	}, AllLivePhotoScenarios()...)
	return append(s, S60_PollOptionCount(), S61_PollMembersOnly(), S62_PollCountryCodes())
}
