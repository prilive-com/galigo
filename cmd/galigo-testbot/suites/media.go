package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/sender"
)

// S6_MediaUploads tests basic media upload methods.
func S6_MediaUploads() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S6_MediaUploads",
		ScenarioDescription: "Test media upload methods (photo, document, animation, audio, voice, video, sticker, video note)",
		CoveredMethods:      []string{"sendPhoto", "sendDocument", "sendAnimation", "sendAudio", "sendVoice", "sendVideo", "sendSticker", "sendVideoNote"},
		ScenarioTimeout:     120 * time.Second,
		ScenarioSteps: []engine.Step{
			// Send photo
			&engine.SendPhotoStep{
				Photo:   engine.MediaFromBytes(fixtures.PhotoBytes(), "galigo-test.jpg", "photo"),
				Caption: "galigo test photo",
			},
			// Send document
			&engine.SendDocumentStep{
				Document: engine.MediaFromBytes(fixtures.DocumentBytes(), "galigo-test.txt", "document"),
				Caption:  "galigo test document",
			},
			// Send animation (GIF)
			&engine.SendAnimationStep{
				Animation: engine.MediaFromBytes(fixtures.AnimationBytes(), "galigo-test.gif", "animation"),
				Caption:   "galigo test animation",
			},
			// Send audio (MP3)
			&engine.SendAudioStep{
				Audio:   engine.MediaFromBytes(fixtures.AudioBytes(), "galigo-test.mp3", "audio"),
				Caption: "galigo test audio",
			},
			// Send voice (OGG Opus)
			&engine.SendVoiceStep{
				Voice:   engine.MediaFromBytes(fixtures.VoiceBytes(), "galigo-test.ogg", "voice"),
				Caption: "galigo test voice",
			},
			// Send video (MP4)
			&engine.SendVideoStep{
				Video:   engine.MediaFromBytes(fixtures.VideoBytes(), "galigo-test.mp4", "video"),
				Caption: "galigo test video",
			},
			// Send sticker (PNG)
			&engine.SendStickerStep{
				Sticker: engine.MediaFromBytes(fixtures.StickerBytes(), "galigo-test.png", "sticker"),
			},
			// Send video note (round MP4)
			&engine.SendVideoNoteStep{
				VideoNote: engine.MediaFromBytes(fixtures.VideoNoteBytes(), "galigo-test-note.mp4", "video_note"),
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
		CoveredMethods:      []string{"sendPhoto", "editMessageCaption"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			// Send photo with caption
			&engine.SendPhotoStep{
				Photo:   engine.MediaFromBytes(fixtures.PhotoBytes(), "caption-test.jpg", "photo"),
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

// S11_EditMessageMedia tests editing message media content.
func S11_EditMessageMedia() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S11_EditMessageMedia",
		ScenarioDescription: "Test editing message media (replace photo with document)",
		CoveredMethods:      []string{"sendPhoto", "editMessageMedia"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			// Send photo as initial media
			&engine.SendPhotoStep{
				Photo:   engine.MediaFromBytes(fixtures.PhotoBytes(), "original.jpg", "photo"),
				Caption: "Original photo - will be replaced",
			},
			// Replace with a document upload
			&engine.EditMessageMediaStep{
				Media: sender.NewInputMediaDocument(
					sender.FromReader(fixtures.Document(), "replaced.txt"),
				).WithCaption("Replaced via editMessageMedia"),
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
		S11_EditMessageMedia(),
	}
}
