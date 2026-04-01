package blastradius

import (
	"testing"

	"github.com/supermodeltools/cli/internal/api"
)

func TestPathMatches(t *testing.T) {
	tests := []struct {
		nodePath string
		target   string
		want     bool
	}{
		{"internal/api/client.go", "internal/api/client.go", true},
		{"./internal/api/client.go", "internal/api/client.go", true},
		{"/repo/internal/api/client.go", "internal/api/client.go", true},
		{"internal/auth/handler.go", "internal/api/client.go", false},
		{"internal/apifoo/client.go", "internal/api/client.go", false},
	}
	for _, tt := range tests {
		t.Run(tt.nodePath+"→"+tt.target, func(t *testing.T) {
			got := pathMatches(tt.nodePath, tt.target)
			if got != tt.want {
				t.Errorf("pathMatches(%q, %q) = %v, want %v", tt.nodePath, tt.target, got, tt.want)
			}
		})
	}
}

func TestFindBlastRadius(t *testing.T) {
	// Dependency chain: c → a → b (a imports b, c imports a)
	g := &api.Graph{
		Nodes: []api.Node{
			fileNode("a", "internal/a/a.go"),
			fileNode("b", "internal/b/b.go"),
			fileNode("c", "internal/c/c.go"),
		},
		Relationships: []api.Relationship{
			{ID: "r1", Type: "IMPORTS", StartNode: "a", EndNode: "b"},
			{ID: "r2", Type: "IMPORTS", StartNode: "c", EndNode: "a"},
		},
	}

	t.Run("blast radius of b", func(t *testing.T) {
		results, err := findBlastRadius(g, ".", "internal/b/b.go", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 affected files, got %d: %v", len(results), results)
		}
		if results[0].File != "internal/a/a.go" || results[0].Depth != 1 {
			t.Errorf("expected a at depth 1, got %+v", results[0])
		}
		if results[1].File != "internal/c/c.go" || results[1].Depth != 2 {
			t.Errorf("expected c at depth 2, got %+v", results[1])
		}
	})

	t.Run("depth cap", func(t *testing.T) {
		results, err := findBlastRadius(g, ".", "internal/b/b.go", 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result at depth 1, got %d", len(results))
		}
		if results[0].File != "internal/a/a.go" {
			t.Errorf("expected a, got %q", results[0].File)
		}
	})

	t.Run("unknown file", func(t *testing.T) {
		_, err := findBlastRadius(g, ".", "internal/z/z.go", 0)
		if err == nil {
			t.Fatal("expected error for unknown file")
		}
	})
}

func fileNode(id, path string) api.Node {
	return api.Node{
		ID:         id,
		Labels:     []string{"File"},
		Properties: map[string]any{"path": path},
	}
}
