package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
)

// S6_MediaUploads tests basic media upload methods.
// Focuses on document upload (multipart) since photo requires URL/file_id only.
func S6_MediaUploads() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S6_MediaUploads",
		ScenarioDescription: "Test media upload methods (document with multipart upload)",
		CoveredMethods:      []string{"sendDocument"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			// Send document with file upload (tests multipart encoding)
			&engine.SendDocumentStep{
				Document: engine.MediaFromBytes(fixtures.DocumentBytes(), "galigo-test.txt", "document"),
				Caption:  "galigo test document (multipart upload)",
			},
			// Send another document to verify consistency
			&engine.SendDocumentStep{
				Document: engine.MediaFromBytes(fixtures.PhotoBytes(), "galigo-test.jpg", "document"),
				Caption:  "galigo test image as document",
			},
			// Cleanup
			&engine.CleanupStep{},
		},
	}
}

// S7_MediaGroups tests media group (album) functionality.
// Uses document type since photo URLs are unreliable for testing.
func S7_MediaGroups() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S7_MediaGroups",
		ScenarioDescription: "Test sending media groups (document album)",
		CoveredMethods:      []string{"sendMediaGroup"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			// Send a media group with multiple documents
			&engine.SendMediaGroupStep{
				Media: []engine.MediaInput{
					engine.MediaFromBytes(fixtures.DocumentBytes(), "doc1.txt", "document"),
					engine.MediaFromBytes([]byte("Second test document content\n"), "doc2.txt", "document"),
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
		CoveredMethods:      []string{"sendDocument", "editMessageCaption"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			// Send document with caption
			&engine.SendDocumentStep{
				Document: engine.MediaFromBytes(fixtures.DocumentBytes(), "caption-test.txt", "document"),
				Caption:  "Original caption",
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
