package cmd

import "testing"

func TestPickRootAction(t *testing.T) {
	cases := []struct {
		name        string
		hasAPIKey   bool
		interactive bool
		want        rootAction
	}{
		{"key + tty starts watch", true, true, runWatch},
		{"key + non-tty starts watch (CI happy path)", true, false, runWatch},
		{"no key + tty drops into setup wizard", false, true, runSetup},
		{"no key + non-tty errors instead of hanging", false, false, errNotAuthenticated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pickRootAction(tc.hasAPIKey, tc.interactive)
			if got != tc.want {
				t.Errorf("pickRootAction(hasAPIKey=%v, interactive=%v) = %v, want %v",
					tc.hasAPIKey, tc.interactive, got, tc.want)
			}
		})
	}
}
