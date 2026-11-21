package registry

import "testing"

// R1 (B-65 65f): each widget method is in AllMethods once, refusal-only.
func TestB65F_Registry(t *testing.T) {
	for _, name := range []string{"sendWidget", "sendSharedWidget", "answerWidgetQuery", "editMessageWidgetData", "editSharedWidgetData"} {
		var got []Method
		for _, m := range AllMethods {
			if m.Name == name {
				got = append(got, m)
			}
		}
		if len(got) != 1 {
			t.Errorf("%s entries: %d, want 1", name, len(got))
			continue
		}
		if w := (Method{Name: name, Category: CategoryMessaging, Notes: "S76: refusal only (the server refuses widgets for this bot)"}); got[0] != w {
			t.Errorf("%s entry %+v, want %+v", name, got[0], w)
		}
	}
}
