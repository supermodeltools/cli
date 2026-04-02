package deadcode

import (
	"bytes"
	"testing"

	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/ui"
)

func TestPrintResults_Human(t *testing.T) {
	result := &api.DeadCodeResult{
		Metadata: api.DeadCodeMetadata{
			TotalDeclarations:  100,
			DeadCodeCandidates: 2,
		},
		DeadCodeCandidates: []api.DeadCodeCandidate{
			{File: "src/utils.ts", Line: 8, Name: "unusedHelper", Confidence: "high", Reason: "No callers found"},
			{File: "src/old.ts", Line: 42, Name: "deprecated", Confidence: "medium", Reason: "Only called from dead code"},
		},
	}

	var buf bytes.Buffer
	err := printResults(&buf, result, ui.FormatHuman)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("unusedHelper")) {
		t.Errorf("expected unusedHelper in output, got:\n%s", out)
	}
	if !bytes.Contains([]byte(out), []byte("2 dead code candidate(s)")) {
		t.Errorf("expected summary line, got:\n%s", out)
	}
}

func TestPrintResults_Empty(t *testing.T) {
	result := &api.DeadCodeResult{
		Metadata:           api.DeadCodeMetadata{TotalDeclarations: 50},
		DeadCodeCandidates: nil,
	}

	var buf bytes.Buffer
	err := printResults(&buf, result, ui.FormatHuman)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("No dead code detected")) {
		t.Errorf("expected 'No dead code detected', got:\n%s", buf.String())
	}
}
