package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// S25_GeoLocation tests sendLocation.
func S25_GeoLocation() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S25-GeoLocation",
		ScenarioDescription: "Tests sendLocation",
		CoveredMethods:      []string{"sendLocation"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendLocationStep{},
			&engine.CleanupStep{},
		},
	}
}

// S26_GeoVenue tests sendVenue.
func S26_GeoVenue() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S26-GeoVenue",
		ScenarioDescription: "Tests sendVenue",
		CoveredMethods:      []string{"sendVenue"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendVenueStep{
				Title:   "Eiffel Tower",
				Address: "Paris, France",
			},
			&engine.CleanupStep{},
		},
	}
}

// S27_ContactAndDice tests sendContact and sendDice.
func S27_ContactAndDice() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S27-ContactAndDice",
		ScenarioDescription: "Tests sendContact and sendDice",
		CoveredMethods:      []string{"sendContact", "sendDice"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendContactStep{},
			&engine.SendDiceStep{Emoji: "🎲"},
			&engine.SendDiceStep{Emoji: "🎯"},
			&engine.CleanupStep{},
		},
	}
}

// S28_BulkOps tests bulk message operations.
func S28_BulkOps() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S28-BulkOps",
		ScenarioDescription: "Tests forwardMessages, copyMessages, deleteMessages",
		CoveredMethods:      []string{"forwardMessages", "copyMessages", "deleteMessages"},
		ScenarioTimeout:     2 * time.Minute,
		ScenarioSteps: []engine.Step{
			&engine.SeedMessagesStep{Count: 3},
			&engine.ForwardMessagesStep{},
			&engine.CopyMessagesStep{},
			&engine.DeleteMessagesStep{},
		},
	}
}

// S29_Reactions tests setMessageReaction.
func S29_Reactions() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S29-Reactions",
		ScenarioDescription: "Tests setMessageReaction",
		CoveredMethods:      []string{"setMessageReaction"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendMessageStep{Text: "React to this!"},
			&engine.SetMessageReactionStep{Emoji: "👍"},
			&engine.CleanupStep{},
		},
	}
}

// S30_UserInfo tests getUserProfilePhotos and getUserChatBoosts.
func S30_UserInfo() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S30-UserInfo",
		ScenarioDescription: "Tests getUserProfilePhotos, getUserChatBoosts",
		CoveredMethods:      []string{"getUserProfilePhotos", "getUserChatBoosts"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.GetUserProfilePhotosStep{},
			&engine.GetUserChatBoostsStep{},
		},
	}
}

// S31_ChatPhotoLifecycle tests setChatPhoto and deleteChatPhoto.
func S31_ChatPhotoLifecycle() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S31-ChatPhotoLifecycle",
		ScenarioDescription: "Tests chat photo lifecycle (requires admin + can_change_info)",
		CoveredMethods:      []string{"setChatPhoto", "deleteChatPhoto"},
		ScenarioTimeout:     time.Minute,
		ScenarioSteps: []engine.Step{
			&engine.SaveChatPhotoStep{},
			&engine.SetChatPhotoStep{},
			&engine.RestoreChatPhotoStep{},
		},
	}
}

// S32_ChatPermissionsLifecycle tests setChatPermissions.
func S32_ChatPermissionsLifecycle() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S32-ChatPermissionsLifecycle",
		ScenarioDescription: "Tests chat permissions lifecycle (requires admin + can_restrict_members)",
		CoveredMethods:      []string{"setChatPermissions"},
		ScenarioTimeout:     time.Minute,
		ScenarioSteps: []engine.Step{
			&engine.SaveChatPermissionsStep{},
			&engine.SetChatPermissionsStep{},
			&engine.RestoreChatPermissionsStep{},
		},
	}
}

// AllExtrasScenarios returns all extras scenarios.
func AllExtrasScenarios() []engine.Scenario {
	return []engine.Scenario{
		S25_GeoLocation(),
		S26_GeoVenue(),
		S27_ContactAndDice(),
		S28_BulkOps(),
		S29_Reactions(),
		S30_UserInfo(),
		S31_ChatPhotoLifecycle(),
		S32_ChatPermissionsLifecycle(),
	}
}
