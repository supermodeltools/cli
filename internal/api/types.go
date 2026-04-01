package api

import "fmt"

// Node represents a graph node returned by the Supermodel API.
type Node struct {
	ID         string         `json:"id"`
	Labels     []string       `json:"labels"`
	Properties map[string]any `json:"properties"`
}

// Prop returns the first non-empty string value from the node's properties.
func (n Node) Prop(keys ...string) string {
	for _, k := range keys {
		if v, ok := n.Properties[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// HasLabel reports whether the node carries the given label.
func (n Node) HasLabel(label string) bool {
	for _, l := range n.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// Relationship is a directed edge between two graph nodes.
type Relationship struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	StartNode  string         `json:"startNode"`
	EndNode    string         `json:"endNode"`
	Properties map[string]any `json:"properties,omitempty"`
}

// Graph is the unified response type for /v1/supermodel and
// /v1/repos/{id}/graph/display. The API serialises relationships as either
// "edges" or "relationships" depending on the endpoint; Rels() unifies both.
type Graph struct {
	Nodes         []Node         `json:"nodes"`
	Edges         []Relationship `json:"edges"`
	Relationships []Relationship `json:"relationships"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// Rels returns all relationships regardless of which JSON field they came from.
func (g *Graph) Rels() []Relationship {
	if len(g.Relationships) > 0 {
		return g.Relationships
	}
	return g.Edges
}

// RepoID returns the repoId from graph metadata, or "".
func (g *Graph) RepoID() string {
	if g.Metadata == nil {
		return ""
	}
	if id, ok := g.Metadata["repoId"].(string); ok {
		return id
	}
	return ""
}

// NodesByLabel returns all nodes that carry the given label.
func (g *Graph) NodesByLabel(label string) []Node {
	var out []Node
	for _, n := range g.Nodes {
		if n.HasLabel(label) {
			out = append(out, n)
		}
	}
	return out
}

// NodeByID returns the node with the given ID, if present.
func (g *Graph) NodeByID(id string) (Node, bool) {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}

// Error represents a non-2xx response from the API.
type Error struct {
	StatusCode int    `json:"-"`
	Status     int    `json:"status"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *Error) Error() string {
	code := e.StatusCode
	if code == 0 {
		code = e.Status
	}
	if e.Code != "" {
		return fmt.Sprintf("API error %d (%s): %s", code, e.Code, e.Message)
	}
	return fmt.Sprintf("API error %d: %s", code, e.Message)
}
