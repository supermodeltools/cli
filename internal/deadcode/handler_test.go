package deadcode

import (
	"testing"

	"github.com/supermodeltools/cli/internal/api"
)

func TestIsEntryPoint(t *testing.T) {
	tests := []struct {
		name           string
		fnName         string
		file           string
		includeExports bool
		want           bool
	}{
		{"main is entry", "main", "main.go", false, true},
		{"init is entry", "init", "foo.go", false, true},
		{"Test is entry", "TestFoo", "foo_test.go", false, true},
		{"Benchmark is entry", "BenchmarkBar", "foo_test.go", false, true},
		{"Fuzz is entry", "FuzzBaz", "foo_test.go", false, true},
		{"Example is entry", "ExampleThing", "foo_test.go", false, true},
		{"exported excluded by default", "Handler", "server.go", false, true},
		{"exported included when flag set", "Handler", "server.go", true, false},
		{"unexported not entry", "process", "server.go", false, false},
		{"test file unexported", "helper", "util_test.go", false, true},
		{"method receiver stripped", "(*Server).Start", "server.go", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEntryPoint(tt.fnName, tt.file, tt.includeExports)
			if got != tt.want {
				t.Errorf("isEntryPoint(%q, %q, %v) = %v, want %v",
					tt.fnName, tt.file, tt.includeExports, got, tt.want)
			}
		})
	}
}

func TestFindDeadCode(t *testing.T) {
	g := &api.Graph{
		Nodes: []api.Node{
			node("f1", "Function", "process", "server.go"),
			node("f2", "Function", "main", "main.go"),
			node("f3", "Function", "helper", "util.go"),
			node("f4", "Function", "Handler", "server.go"), // exported → excluded by default
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "CALLS", StartNode: "f2", EndNode: "f1"}, // main → process
		},
	}

	t.Run("default: excludes exports", func(t *testing.T) {
		results := findDeadCode(g, false)
		if len(results) != 1 {
			t.Fatalf("expected 1 dead fn, got %d: %v", len(results), results)
		}
		if results[0].Name != "helper" {
			t.Errorf("expected helper, got %q", results[0].Name)
		}
	})

	t.Run("include exports", func(t *testing.T) {
		results := findDeadCode(g, true)
		if len(results) != 2 {
			t.Fatalf("expected 2 dead fns, got %d: %v", len(results), results)
		}
	})
}

func node(id, label, name, file string) api.Node {
	return api.Node{
		ID:     id,
		Labels: []string{label},
		Properties: map[string]any{
			"name": name,
			"file": file,
		},
	}
}
