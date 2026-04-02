package deadcode

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/ui"
)

// ---- glob matching ----

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		// exact
		{"src/utils.ts", "src/utils.ts", true},
		{"src/utils.ts", "src/other.ts", false},
		// single-segment wildcard
		{"src/*.ts", "src/utils.ts", true},
		{"src/*.ts", "src/nested/utils.ts", false},
		// ** at end
		{"dist/**", "dist/index.js", true},
		{"dist/**", "dist/sub/index.js", true},
		{"dist/**", "src/index.js", false},
		// ** at start
		{"**/generated/**", "src/generated/api.go", true},
		{"**/generated/**", "generated/api.go", true},
		{"**/generated/**", "src/other/api.go", false},
		// ** matching extension
		{"**/*.test.ts", "src/foo.test.ts", true},
		{"**/*.test.ts", "src/nested/foo.test.ts", true},
		{"**/*.test.ts", "src/foo.ts", false},
		// no match on empty path tail
		{"src/**", "src", false},
		{"src/**", "src/a", true},
	}

	for _, tc := range cases {
		got := matchGlob(tc.pattern, tc.path)
		if got != tc.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

// ---- filterIgnored ----

func TestFilterIgnored_NoPatterns(t *testing.T) {
	candidates := sampleResult().DeadCodeCandidates
	got := filterIgnored(candidates, nil)
	if len(got) != len(candidates) {
		t.Errorf("expected %d candidates, got %d", len(candidates), len(got))
	}
}

func TestFilterIgnored_MatchesPattern(t *testing.T) {
	candidates := []api.DeadCodeCandidate{
		{File: "src/generated/api.ts", Name: "fn1", Confidence: "high"},
		{File: "src/utils.ts", Name: "fn2", Confidence: "high"},
	}
	got := filterIgnored(candidates, []string{"**/generated/**"})
	if len(got) != 1 || got[0].Name != "fn2" {
		t.Errorf("unexpected filtered result: %+v", got)
	}
}

func TestFilterIgnored_MultiplePatterns(t *testing.T) {
	candidates := []api.DeadCodeCandidate{
		{File: "src/generated/api.ts", Name: "fn1", Confidence: "high"},
		{File: "src/migrations/001.ts", Name: "fn2", Confidence: "high"},
		{File: "src/utils.ts", Name: "fn3", Confidence: "high"},
	}
	got := filterIgnored(candidates, []string{"**/generated/**", "**/migrations/**"})
	if len(got) != 1 || got[0].Name != "fn3" {
		t.Errorf("unexpected filtered result: %+v", got)
	}
}

func sampleResult() *api.DeadCodeResult {
	return &api.DeadCodeResult{
		Metadata: api.DeadCodeMetadata{
			TotalDeclarations:  100,
			DeadCodeCandidates: 3,
			AliveCode:          80,
			AnalysisMethod:     "symbol_level_import_analysis",
		},
		DeadCodeCandidates: []api.DeadCodeCandidate{
			{File: "src/utils.ts", Line: 8, Name: "unusedHelper", Type: "function", Confidence: "high", Reason: "No callers found"},
			{File: "src/old.ts", Line: 42, Name: "deprecated", Type: "function", Confidence: "medium", Reason: "Only called from dead code"},
			{File: "src/types.ts", Line: 0, Name: "OldInterface", Type: "type", Confidence: "low", Reason: "Type with no references"},
		},
	}
}

func TestPrintResults_Human(t *testing.T) {
	var buf bytes.Buffer
	if err := printResults(&buf, sampleResult(), ui.FormatHuman); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		"unusedHelper", "deprecated", "OldInterface",
		"high", "medium", "low",
		"No callers found",
		"3 dead code candidate(s) out of 100 total declarations",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestPrintResults_HumanLineNumbers(t *testing.T) {
	var buf bytes.Buffer
	if err := printResults(&buf, sampleResult(), ui.FormatHuman); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	// Line 8 and 42 should appear; line 0 should be blank.
	if !strings.Contains(out, "8") {
		t.Error("expected line number 8 in output")
	}
	if !strings.Contains(out, "42") {
		t.Error("expected line number 42 in output")
	}
}

func TestPrintResults_Empty(t *testing.T) {
	result := &api.DeadCodeResult{
		Metadata:           api.DeadCodeMetadata{TotalDeclarations: 50},
		DeadCodeCandidates: nil,
	}

	var buf bytes.Buffer
	if err := printResults(&buf, result, ui.FormatHuman); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No dead code detected") {
		t.Errorf("expected 'No dead code detected', got:\n%s", buf.String())
	}
}

func TestPrintResults_JSON(t *testing.T) {
	var buf bytes.Buffer
	if err := printResults(&buf, sampleResult(), ui.FormatJSON); err != nil {
		t.Fatal(err)
	}

	var decoded api.DeadCodeResult
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if len(decoded.DeadCodeCandidates) != 3 {
		t.Errorf("expected 3 candidates in JSON, got %d", len(decoded.DeadCodeCandidates))
	}
	if decoded.Metadata.TotalDeclarations != 100 {
		t.Errorf("expected totalDeclarations=100, got %d", decoded.Metadata.TotalDeclarations)
	}
}
