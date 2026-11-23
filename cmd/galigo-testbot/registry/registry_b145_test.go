package registry

import "testing"

// B-145 C2: getUserGifts, getChatGifts and getBusinessAccountGifts are each in AllMethods exactly once, as extended
// methods; the business method is a refusal-only row; getOwnedGifts (not a Bot API method) and getChat (S80's chat
// type fallback) are not added.
func TestB145_Registry(t *testing.T) {
	want := map[string]string{
		"getUserGifts":            "",
		"getChatGifts":            "",
		"getBusinessAccountGifts": "S80: refusal only (fake business connection)",
	}
	got := map[string][]Method{}
	for _, m := range AllMethods {
		got[m.Name] = append(got[m.Name], m)
	}
	for name, notes := range want {
		if len(got[name]) != 1 {
			t.Fatalf("%s entries: %d, want 1", name, len(got[name]))
		}
		m := got[name][0]
		if m.Category != CategoryExtended {
			t.Errorf("%s category %q, want %q", name, m.Category, CategoryExtended)
		}
		if m.Notes != notes {
			t.Errorf("%s notes %q, want %q", name, m.Notes, notes)
		}
	}
	if n := len(got["getOwnedGifts"]); n != 0 {
		t.Errorf("getOwnedGifts entries: %d, want 0 (not a Bot API method)", n)
	}
	if n := len(got["getChat"]); n != 1 {
		t.Errorf("getChat entries: %d, want 1 (unchanged)", n)
	}
}
