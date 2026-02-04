package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/tg"
)

// S33_BotCommands tests bot command lifecycle.
func S33_BotCommands() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S33-BotCommands",
		ScenarioDescription: "Set, get, and delete bot commands",
		CoveredMethods:      []string{"setMyCommands", "getMyCommands", "deleteMyCommands"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			// Set commands
			&engine.SetMyCommandsStep{
				Commands: []tg.BotCommand{
					{Command: "start", Description: "Start the bot"},
					{Command: "help", Description: "Get help"},
					{Command: "settings", Description: "Open settings"},
				},
			},
			// Verify commands were set
			&engine.GetMyCommandsStep{ExpectedCount: 3},
			// Delete commands
			&engine.DeleteMyCommandsStep{},
			// Verify commands were deleted
			&engine.GetMyCommandsStep{ExpectedCount: 0},
		},
	}
}

// S34_BotProfile tests bot profile management.
func S34_BotProfile() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S34-BotProfile",
		ScenarioDescription: "Set and get bot name, description, and short description",
		CoveredMethods:      []string{"setMyName", "getMyName", "setMyDescription", "getMyDescription", "setMyShortDescription", "getMyShortDescription"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			// Set name
			&engine.SetMyNameStep{BotName: "[galigo-test] TestBot"},
			// Verify name
			&engine.GetMyNameStep{ExpectedName: "[galigo-test] TestBot"},
			// Set description
			&engine.SetMyDescriptionStep{Description: "[galigo-test] This is a test bot for galigo library."},
			// Verify description
			&engine.GetMyDescriptionStep{},
			// Set short description
			&engine.SetMyShortDescriptionStep{ShortDescription: "[galigo-test] galigo test bot"},
			// Verify short description
			&engine.GetMyShortDescriptionStep{},
			// Reset to empty (cleanup) - note: bot name cannot be cleared, only descriptions
			&engine.SetMyDescriptionStep{Description: ""},
			&engine.SetMyShortDescriptionStep{ShortDescription: ""},
		},
	}
}

// S35_BotAdminDefaults tests default administrator rights management.
func S35_BotAdminDefaults() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S35-BotAdminDefaults",
		ScenarioDescription: "Set and get default administrator rights",
		CoveredMethods:      []string{"setMyDefaultAdministratorRights", "getMyDefaultAdministratorRights"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			// Set default rights for groups
			&engine.SetMyDefaultAdministratorRightsStep{
				Rights: &tg.ChatAdministratorRights{
					CanDeleteMessages:  true,
					CanInviteUsers:     true,
					CanRestrictMembers: true,
				},
				ForChannels: false,
			},
			// Verify rights for groups
			&engine.GetMyDefaultAdministratorRightsStep{ForChannels: false},
			// Set default rights for channels
			&engine.SetMyDefaultAdministratorRightsStep{
				Rights: &tg.ChatAdministratorRights{
					CanDeleteMessages: true,
				},
				ForChannels: true,
			},
			// Verify rights for channels
			&engine.GetMyDefaultAdministratorRightsStep{ForChannels: true},
			// Reset to no rights (cleanup)
			&engine.SetMyDefaultAdministratorRightsStep{Rights: nil, ForChannels: false},
			&engine.SetMyDefaultAdministratorRightsStep{Rights: nil, ForChannels: true},
		},
	}
}

// AllBotConfigScenarios returns all bot config scenarios.
func AllBotConfigScenarios() []engine.Scenario {
	return []engine.Scenario{
		S33_BotCommands(),
		S34_BotProfile(),
		S35_BotAdminDefaults(),
	}
}
