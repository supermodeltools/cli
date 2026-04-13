package cmd

import (
	"strings"
	"testing"
)

func TestSkillPrompt_ContainsKeyElements(t *testing.T) {
	required := []struct {
		substr string
		reason string
	}{
		{".graph.", "must reference graph file extension"},
		{"[deps]", "must document deps section"},
		{"[calls]", "must document calls section"},
		{"[impact]", "must document impact section"},
		{".graph.py", "must show naming convention with concrete example"},
		{"before the source file", "must instruct read-order (graph first)"},
	}

	for _, r := range required {
		if !strings.Contains(skillPrompt, r.substr) {
			t.Errorf("skill prompt missing %q — %s", r.substr, r.reason)
		}
	}
}

func TestSkillPrompt_NotEmpty(t *testing.T) {
	if len(strings.TrimSpace(skillPrompt)) < 100 {
		t.Error("skill prompt is suspiciously short")
	}
}
