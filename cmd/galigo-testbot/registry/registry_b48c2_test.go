package registry

import "testing"

// B-48c2 S7: approveSuggestedPost and declineSuggestedPost are in AllMethods exactly once each, as chat admin
// methods with no Notes (a scenario covers them).
func TestB48C2_Registry(t *testing.T) {
	for _, name := range []string{"approveSuggestedPost", "declineSuggestedPost"} {
		var got []Method
		for _, m := range AllMethods {
			if m.Name == name {
				got = append(got, m)
			}
		}
		if len(got) != 1 {
			t.Fatalf("%s entries: %d, want 1", name, len(got))
		}
		if want := (Method{Name: name, Category: CategoryChatAdmin}); got[0] != want {
			t.Errorf("%s entry %+v, want %+v", name, got[0], want)
		}
	}
}
