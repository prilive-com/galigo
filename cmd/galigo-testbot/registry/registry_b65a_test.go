package registry

import "testing"

// B-65 65a T-reg: deleteEphemeralMessage is covered by S73 (Notes name it); B-65 65b: so are the four edit methods.
// Each is in AllMethods exactly once.
func TestB65A_RegistryNotes(t *testing.T) {
	want := map[string]string{
		"deleteEphemeralMessage":          "S73: its cleanup deletes the ephemeral messages it sends",
		"editEphemeralMessageText":        "S73: edits the ephemeral messages it sends",
		"editEphemeralMessageMedia":       "S73: edits the ephemeral messages it sends",
		"editEphemeralMessageCaption":     "S73: edits the ephemeral messages it sends",
		"editEphemeralMessageReplyMarkup": "S73: edits the ephemeral messages it sends",
	}
	for name, notes := range want {
		var got []Method
		for _, m := range AllMethods {
			if m.Name == name {
				got = append(got, m)
			}
		}
		if len(got) != 1 {
			t.Fatalf("%s entries: %d, want 1", name, len(got))
		}
		if w := (Method{Name: name, Category: CategoryMessaging, Notes: notes}); got[0] != w {
			t.Errorf("%s entry %+v, want %+v", name, got[0], w)
		}
	}
}
