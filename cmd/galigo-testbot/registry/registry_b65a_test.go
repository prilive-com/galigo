package registry

import "testing"

// B-65 65a T-reg: deleteEphemeralMessage is covered by S73 (Notes name it); the four edit methods stay without a
// live scenario. Each is in AllMethods exactly once.
func TestB65A_RegistryNotes(t *testing.T) {
	want := map[string]string{
		"deleteEphemeralMessage":          "S73: its cleanup deletes the ephemeral messages it sends",
		"editEphemeralMessageText":        "no live scenario yet (needs an ephemeral message; with the 10.3 changes)",
		"editEphemeralMessageMedia":       "no live scenario yet (needs an ephemeral message; with the 10.3 changes)",
		"editEphemeralMessageCaption":     "no live scenario yet (needs an ephemeral message; with the 10.3 changes)",
		"editEphemeralMessageReplyMarkup": "no live scenario yet (needs an ephemeral message; with the 10.3 changes)",
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
