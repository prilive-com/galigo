package registry

import "testing"

// B-65 65i: getUniqueGift and deleteBusinessAccountGift are each in AllMethods exactly once, as extended methods;
// the delete is a refusal-only row; S79's guard call (getBusinessConnection) is not a registry method.
func TestB65I_Registry(t *testing.T) {
	want := map[string]string{
		"getUniqueGift":             "",
		"deleteBusinessAccountGift": "S79: refusal only (fake business connection)",
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
	if n := len(got["getBusinessConnection"]); n != 0 {
		t.Errorf("getBusinessConnection entries: %d, want 0 (S79's guard is not counted)", n)
	}
}
