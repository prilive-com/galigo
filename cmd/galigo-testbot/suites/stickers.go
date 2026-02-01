package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
)

// S20_StickerLifecycle tests the full sticker set lifecycle:
// create → get → addSticker → setPosition → setEmoji → setTitle → deleteSticker → delete set.
func S20_StickerLifecycle() engine.Scenario {
	stickerMedia := engine.MediaFromBytes(fixtures.StickerBytes(), "sticker.png", "sticker")

	return &engine.BaseScenario{
		ScenarioName:        "S20-StickerLifecycle",
		ScenarioDescription: "Full sticker set lifecycle: create, read, update, delete",
		CoveredMethods: []string{
			"createNewStickerSet", "getStickerSet", "addStickerToSet",
			"setStickerPositionInSet", "setStickerEmojiList",
			"setStickerSetTitle", "deleteStickerFromSet", "deleteStickerSet",
		},
		ScenarioTimeout: 2 * time.Minute,
		ScenarioSteps: []engine.Step{
			// Create a sticker set with one sticker
			&engine.CreateStickerSetStep{
				NameSuffix: "galigo_test_set",
				Title:      "[galigo-test] Test Stickers",
				Stickers: []engine.StickerInput{
					{
						Sticker:   stickerMedia,
						Format:    "static",
						EmojiList: []string{"\U0001F600"}, // 😀
					},
				},
			},
			// Get the created set
			&engine.GetStickerSetStep{},
			// Add a second sticker
			&engine.AddStickerToSetStep{
				Sticker: engine.StickerInput{
					Sticker:   stickerMedia,
					Format:    "static",
					EmojiList: []string{"\U0001F60E"}, // 😎
				},
			},
			// Re-fetch to get updated sticker list
			&engine.GetStickerSetStep{},
			// Move first sticker to position 1 (second)
			&engine.SetStickerPositionStep{Position: 1},
			// Change emoji list
			&engine.SetStickerEmojiListStep{EmojiList: []string{"\U0001F609"}}, // 😉
			// Rename the set
			&engine.SetStickerSetTitleStep{Title: "[galigo-test] Renamed Stickers"},
			// Delete one sticker
			&engine.DeleteStickerFromSetStep{},
			// Delete the entire set
			&engine.DeleteStickerSetStep{},
		},
	}
}

// AllStickerScenarios returns all sticker scenarios.
func AllStickerScenarios() []engine.Scenario {
	return []engine.Scenario{
		S20_StickerLifecycle(),
	}
}
