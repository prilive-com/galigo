package suites

import (
	"context"
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
)

// S6_MediaUploads tests basic media upload methods.
func S6_MediaUploads() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S6_MediaUploads",
		ScenarioDescription: "Test media upload methods (photo, document, animation)",
		CoveredMethods:      []string{"sendPhoto", "sendDocument", "sendAnimation"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			// Send photo from URL
			&engine.SendPhotoStep{
				Photo:   engine.MediaFromURL(fixtures.TestURLs.Photo),
				Caption: "galigo test photo",
			},
			// Send document with file upload
			&engine.SendDocumentStep{
				Document: engine.MediaFromBytes(fixtures.DocumentBytes(), "test.txt", "document"),
				Caption:  "galigo test document",
			},
			// Send animation from URL (if available)
			&conditionalStep{
				condition: fixtures.HasAnimation,
				step: &engine.SendAnimationStep{
					Animation: engine.MediaFromURL(fixtures.TestURLs.Animation),
					Caption:   "galigo test animation",
				},
				skipName: "sendAnimation (skipped - no URL)",
			},
			// Cleanup
			&engine.CleanupStep{},
		},
	}
}

// S7_MediaGroups tests media group (album) functionality.
func S7_MediaGroups() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S7_MediaGroups",
		ScenarioDescription: "Test sending media groups (albums)",
		CoveredMethods:      []string{"sendMediaGroup"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			// Send a media group with multiple photos
			&engine.SendMediaGroupStep{
				Media: []engine.MediaInput{
					{
						URL:  fixtures.TestURLs.Photo,
						Type: "photo",
					},
					{
						URL:  fixtures.TestURLs.Photo,
						Type: "photo",
					},
				},
			},
			// Cleanup
			&engine.CleanupStep{},
		},
	}
}

// S8_EditMedia tests editing media captions.
func S8_EditMedia() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S8_EditMedia",
		ScenarioDescription: "Test editing media message captions",
		CoveredMethods:      []string{"sendPhoto", "editMessageCaption"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			// Send photo with caption
			&engine.SendPhotoStep{
				Photo:   engine.MediaFromURL(fixtures.TestURLs.Photo),
				Caption: "Original caption",
			},
			// Edit caption
			&engine.EditMessageCaptionStep{
				Caption: "Edited caption by galigo-testbot",
			},
			// Cleanup
			&engine.CleanupStep{},
		},
	}
}

// S9_GetFile tests file download info retrieval.
func S9_GetFile() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S9_GetFile",
		ScenarioDescription: "Test getFile to retrieve file download info",
		CoveredMethods:      []string{"sendDocument", "getFile"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			// Send document to get a file_id
			&engine.SendDocumentStep{
				Document: engine.MediaFromBytes(fixtures.DocumentBytes(), "test.txt", "document"),
				Caption:  "File for getFile test",
			},
			// Get file info
			&engine.GetFileStep{
				FileKey: "document",
			},
			// Cleanup
			&engine.CleanupStep{},
		},
	}
}

// AllPhaseBScenarios returns all Phase B (media) scenarios.
func AllPhaseBScenarios() []engine.Scenario {
	return []engine.Scenario{
		S6_MediaUploads(),
		S7_MediaGroups(),
		S8_EditMedia(),
		S9_GetFile(),
	}
}

// conditionalStep wraps a step that may be skipped based on a condition.
type conditionalStep struct {
	condition func() bool
	step      engine.Step
	skipName  string
}

func (s *conditionalStep) Name() string {
	if !s.condition() {
		return s.skipName
	}
	return s.step.Name()
}

func (s *conditionalStep) Execute(ctx context.Context, rt *engine.Runtime) (*engine.StepResult, error) {
	if !s.condition() {
		return &engine.StepResult{
			StepName: s.skipName,
			Success:  true,
			Evidence: map[string]any{
				"skipped": true,
				"reason":  "condition not met",
			},
		}, nil
	}
	return s.step.Execute(ctx, rt)
}
