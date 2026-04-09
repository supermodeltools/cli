package output

import (
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
)

func TestGenerateFeedNoDoubleEscape(t *testing.T) {
	entities := []*entity.Entity{
		{
			Slug:   "beef-rice",
			Fields: map[string]interface{}{"title": "Beef & Rice", "description": "A <simple> recipe"},
		},
	}
	out := generateFeed("Site & Blog", "https://example.com", "A <test> site", "en", "Mon, 01 Jan 2024 00:00:00 +0000", entities, "https://example.com")

	// xml.MarshalIndent produces single-encoded entities; double-encoded would show &amp;amp;
	if strings.Contains(out, "&amp;amp;") {
		t.Errorf("double-escaped ampersand in output:\n%s", out)
	}
	if strings.Contains(out, "&lt;lt;") || strings.Contains(out, "&amp;lt;") {
		t.Errorf("double-escaped less-than in output:\n%s", out)
	}
	// The correctly single-encoded forms must be present
	if !strings.Contains(out, "Beef &amp; Rice") {
		t.Errorf("expected 'Beef &amp; Rice' in output:\n%s", out)
	}
	if !strings.Contains(out, "A &lt;simple&gt; recipe") {
		t.Errorf("expected 'A &lt;simple&gt; recipe' in output:\n%s", out)
	}
	if !strings.Contains(out, "Site &amp; Blog") {
		t.Errorf("expected 'Site &amp; Blog' in output:\n%s", out)
	}
}
