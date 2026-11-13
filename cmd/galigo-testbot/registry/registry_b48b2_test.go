package registry

import "testing"

// B-48b2 T-reg (O-T1-05): sendPaidMedia is in AllMethods exactly once, as a messaging method. --status cannot show
// this: it also counts methods that only a scenario names, and its map hides a duplicate.
func TestB48B2_Registry(t *testing.T) {
	var got []Method
	for _, m := range AllMethods {
		if m.Name == "sendPaidMedia" {
			got = append(got, m)
		}
	}
	if len(got) != 1 {
		t.Fatalf("sendPaidMedia entries: %d, want 1", len(got))
	}
	if got[0].Category != CategoryMessaging {
		t.Errorf("sendPaidMedia category %q, want %q", got[0].Category, CategoryMessaging)
	}
	if len(AllMethods) != 101 {
		t.Errorf("AllMethods has %d entries, want 101 (98 before B-48b2, 99 before B-48c2)", len(AllMethods))
	}
}
