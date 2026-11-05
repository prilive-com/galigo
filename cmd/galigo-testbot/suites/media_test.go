package suites

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
)

// mediaView is a MediaInput with its bytes read, so two inputs can be compared
// (a func field is never equal to another func).
type mediaView struct {
	Data     []byte
	FileName string
	FileID   string
	URL      string
	Type     string
}

func viewMedia(m engine.MediaInput) mediaView {
	v := mediaView{FileName: m.FileName, FileID: m.FileID, URL: m.URL, Type: m.Type}
	if m.Reader != nil {
		v.Data = m.Reader()
	}
	return v
}

// viewStep returns a comparable form of a step: its type and every input.
func viewStep(s engine.Step) any {
	switch st := s.(type) {
	case *engine.SendDocumentStep:
		return struct {
			Kind     string
			Document mediaView
			Caption  string
			StoreAs  string
		}{"SendDocumentStep", viewMedia(st.Document), st.Caption, st.StoreAs}
	case *engine.SendPhotoStep:
		return struct {
			Kind    string
			Photo   mediaView
			Caption string
		}{"SendPhotoStep", viewMedia(st.Photo), st.Caption}
	default:
		return s // a pointer: compared by the value it points to
	}
}

func viewSteps(steps []engine.Step) []any {
	out := make([]any, len(steps))
	for i, s := range steps {
		out[i] = viewStep(s)
	}
	return out
}

// T-tb-1 (B-45 TEST-PLAN §4): the file_id scenarios, their covered methods,
// their exact step sequences and inputs, and their place in Phase B.
func TestMediaFileIDScenarios(t *testing.T) {
	docA := fixtures.DocumentBytes()
	docB := append(fixtures.DocumentBytes(), []byte("second document of the file_id album\n")...)

	t.Run("two different documents", func(t *testing.T) {
		require.False(t, bytes.Equal(docA, docB))
		require.True(t, bytes.HasPrefix(docB, docA))
		assert.Equal(t, 1, bytes.Count(docB[len(docA):], []byte("\n")), "one extra line")
	})

	want := []struct {
		name   string
		covers []string
		steps  []engine.Step
	}{
		{"S55-MediaGroupByFileID", []string{"sendDocument", "sendMediaGroup"}, []engine.Step{
			&engine.SendDocumentStep{
				Document: engine.MediaFromBytes(docA, "b45-a.txt", "document"),
				Caption:  "galigo file_id album: document A",
				StoreAs:  "doc_a",
			},
			&engine.SendDocumentStep{
				Document: engine.MediaFromBytes(docB, "b45-b.txt", "document"),
				Caption:  "galigo file_id album: document B",
				StoreAs:  "doc_b",
			},
			&engine.SendMediaGroupByFileIDStep{FileKeys: []string{"doc_a", "doc_b"}, MediaType: "document"},
			&engine.CleanupStep{},
		}},
		{"S56-EditMessageMediaByFileID", []string{"sendDocument", "sendPhoto", "editMessageMedia"}, []engine.Step{
			&engine.SendDocumentStep{
				Document: engine.MediaFromBytes(docA, "b45-c.txt", "document"),
				Caption:  "galigo file_id edit: the document to reuse",
			},
			&engine.SendPhotoStep{
				Photo:   engine.MediaFromBytes(fixtures.PhotoBytes(), "b45-photo.jpg", "photo"),
				Caption: "galigo file_id edit: photo, will be replaced",
			},
			&engine.EditMessageMediaByFileIDStep{
				FileKey:   "document",
				MediaType: "document",
				Caption:   "Replaced by file_id via editMessageMedia",
			},
			&engine.CleanupStep{},
		}},
	}

	got := []engine.Scenario{S55_MediaGroupByFileID(), S56_EditMessageMediaByFileID()}
	for i, w := range want {
		t.Run(w.name, func(t *testing.T) {
			assert.Equal(t, w.name, got[i].Name())
			assert.Equal(t, w.covers, got[i].Covers())
			assert.Equal(t, viewSteps(w.steps), viewSteps(got[i].Steps()))
		})
	}

	t.Run("in Phase B", func(t *testing.T) {
		var names []string
		for _, s := range AllPhaseBScenarios() {
			names = append(names, s.Name())
		}
		assert.Equal(t, []string{
			"S6_MediaUploads", "S7_MediaGroups", "S8_EditMedia", "S9_GetFile", "S11_EditMessageMedia",
			"S36_MultipartParseMode", "S55-MediaGroupByFileID", "S56-EditMessageMediaByFileID",
		}, names)
	})
}
