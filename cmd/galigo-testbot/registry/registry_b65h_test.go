package registry

import "testing"

// B-65 65h: getChatShareThemeBackdrops is in AllMethods exactly once, as an extended method.
func TestB65H_Registry(t *testing.T) {
	var got []Method
	for _, m := range AllMethods {
		if m.Name == "getChatShareThemeBackdrops" {
			got = append(got, m)
		}
	}
	if len(got) != 1 {
		t.Fatalf("getChatShareThemeBackdrops entries: %d, want 1", len(got))
	}
	if got[0].Category != CategoryExtended {
		t.Errorf("getChatShareThemeBackdrops category %q, want %q", got[0].Category, CategoryExtended)
	}
}
