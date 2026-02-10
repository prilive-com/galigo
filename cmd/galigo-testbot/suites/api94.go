package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
)

// ==================== Bot API 9.4 Scenarios ====================

// S38_StyledButtons tests 9.4 button styling (style, icon_custom_emoji_id).
// NOTE: icon_custom_emoji_id requires Premium. Style works for all bots.
func S38_StyledButtons() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S38-StyledButtons",
		ScenarioDescription: "Send message with 9.4 styled buttons (danger/success/primary)",
		CoveredMethods:      []string{"sendMessage"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			// Send message with styled buttons (style only, no icon - works without Premium)
			&engine.SendMessageWithStyledButtonsStep{
				Text: "[galigo 9.4] Styled buttons test",
				Buttons: []engine.ButtonDef{
					{Text: "Confirm", CallbackData: "action:confirm", Style: "success"},
					{Text: "Delete", CallbackData: "action:delete", Style: "danger"},
					{Text: "Info", CallbackData: "action:info", Style: "primary"},
				},
			},
			// Cleanup
			&engine.CleanupStep{},
		},
	}
}

// S39_ProfileAudios tests 9.4 getUserProfileAudios.
func S39_ProfileAudios() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S39-ProfileAudios",
		ScenarioDescription: "Get user profile audios (9.4)",
		CoveredMethods:      []string{"getUserProfileAudios"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.GetUserProfileAudiosStep{},
		},
	}
}

// S40_ChatInfo94 tests 9.4 ChatFullInfo fields (first_profile_audio, unique_gift_colors, paid_message_star_count).
func S40_ChatInfo94() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S40-ChatInfo94",
		ScenarioDescription: "Verify 9.4 ChatFullInfo fields deserialize correctly",
		CoveredMethods:      []string{"getChat"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.VerifyChatInfo94Step{},
		},
	}
}

// S42_VideoQualities tests 9.4 Video.Qualities field deserialization.
func S42_VideoQualities() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S42-VideoQualities",
		ScenarioDescription: "Send video and verify qualities field deserialization (9.4)",
		CoveredMethods:      []string{"sendVideo"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			// Send a test video
			&engine.SendVideoStep{
				Video:   engine.MediaFromBytes(fixtures.VideoBytes(), "test.mp4", "video"),
				Caption: "[galigo 9.4] video qualities test",
			},
			// Verify the response contains Video field and qualities deserialized
			&engine.VerifyVideoQualitiesStep{},
			// Cleanup
			&engine.CleanupStep{},
		},
	}
}

// AllAPI94Scenarios returns all Bot API 9.4 test scenarios.
// Note: S41 (profile photo) is excluded - it's destructive and manual-only.
func AllAPI94Scenarios() []engine.Scenario {
	return []engine.Scenario{
		S38_StyledButtons(),
		S39_ProfileAudios(),
		S40_ChatInfo94(),
		S42_VideoQualities(),
	}
}
