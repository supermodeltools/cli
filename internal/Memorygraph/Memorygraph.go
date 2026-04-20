 Package memorygraph implements a persistent memory graph for interlinked RAG.
// Nodes represent typed knowledge units; edges are weighted, typed relations.
// The graph is stored as a single JSON file under rootDir/.supermodel/memory-graph.json
// and is safe for concurrent reads within a process (writes hold a mutex).
package Memorygraph

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// --- Types -------------------------------------------------------------------

// NodeType classifies what kind of knowledge a node represents.
type NodeType string

const (
	NodeTypeFact      NodeType = "fact"
	NodeTypeConcept   NodeType = "concept"
	NodeTypeEntity    NodeType = "entity"
	NodeTypeEvent     NodeType = "event"
	NodeTypeProcedure NodeType = "procedure"
	NodeTypeContext   NodeType = "context"
)

// RelationType classifies the semantic relationship between two nodes.
type RelationType string

const (
	RelationRelatedTo   RelationType = "related_to"
	RelationDependsOn   RelationType = "depends_on"
	RelationPartOf      RelationType = "part_of"
	RelationLeadsTo     RelationType = "leads_to"
	RelationContrasts   RelationType = "contrasts"
	RelationSimilarTo   RelationType = "similar_to"
	RelationInstantiates RelationType = "instantiates"
)

// Node is a single knowledge unit in the memory graph.
type Node struct {
	ID          string            `json:"id"`
	Type        NodeType          `json:"type"`
	Label       string            `json:"label"`
	Content     string            `json:"content"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	AccessCount int               `json:"accessCount"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

// Edge is a directed, weighted relation between two nodes.
type Edge struct {
	ID       string            `json:"id"`
	Source   string            `json:"source"`
	Target   string            `json:"target"`
	Relation RelationType      `json:"relation"`
	Weight   float64           `json:"weight"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// TraversalResult is a node reached during graph traversal, enriched with
// path context and a relevance score.
type TraversalResult struct {
	Node          Node
	Depth         int
	RelevanceScore float64
	PathRelations  []string // relation labels along the path from the start node
}

// GraphStats summarises the current state of the graph.
type GraphStats struct {
	Nodes int
	Edges int
}

// graphData is the on-disk format.
type graphData struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// --- Storage -----------------------------------------------------------------

const graphFile = ".supermodel/memory-graph.json"

var (
	mu    sync.RWMutex
	cache = map[string]*graphData{} // rootDir → loaded graph
)

func graphPath(rootDir string) string {
	return filepath.Join(rootDir, graphFile)
}

// load reads the graph for rootDir from disk (or returns the in-memory cache).
func load(rootDir string) (*graphData, error) {
	mu.RLock()
	if g, ok := cache[rootDir]; ok {
		mu.RUnlock()
		return g, nil
	}
	mu.RUnlock()

	mu.Lock()
	defer mu.Unlock()

	// Double-checked locking.
	if g, ok := cache[rootDir]; ok {
		return g, nil
	}

	path := graphPath(rootDir)
	g := &graphData{}

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("memorygraph: read %s: %w", path, err)
	}
	if err == nil {
		if err := json.Unmarshal(data, g); err != nil {
			return nil, fmt.Errorf("memorygraph: parse %s: %w", path, err)
		}
	}

	cache[rootDir] = g
	return g, nil
}

// save persists g to disk. Caller must hold mu (write lock).
func save(rootDir string, g *graphData) error {
	path := graphPath(rootDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("memorygraph: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return fmt.Errorf("memorygraph: marshal: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".memory-graph-*.json.tmp")
	if err != nil {
		return fmt.Errorf("memorygraph: tempfile: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("memorygraph: write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("memorygraph: fsync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("memorygraph: close: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("memorygraph: rename: %w", err)
	}
	return nil
}

// nodeID derives a stable deterministic ID from type+label.
func nodeID(t NodeType, label string) string {
	return fmt.Sprintf("%s:%s", t, strings.ToLower(strings.ReplaceAll(label, " ", "_")))
}

// edgeID derives a stable deterministic ID from endpoints+relation.
func edgeID(source, target string, relation RelationType) string {
	return fmt.Sprintf("%s--%s-->%s", source, relation, target)
}

// --- Core operations ---------------------------------------------------------

// UpsertNode creates or updates a node identified by (type, label).
// If the node already exists its content and metadata are updated in-place.
func UpsertNode(rootDir string, t NodeType, label, content string, metadata map[string]string) (*Node, error) {
	mu.Lock()
	defer mu.Unlock()

	g, err := loadLocked(rootDir)
	if err != nil {
		return nil, err
	}

	id := nodeID(t, label)
	now := time.Now().UTC()

	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			g.Nodes[i].Content = content
			g.Nodes[i].Metadata = metadata
			g.Nodes[i].UpdatedAt = now
			g.Nodes[i].AccessCount++
			node := g.Nodes[i]
			if err := save(rootDir, g); err != nil {
				return nil, err
			}
			return &node, nil
		}
	}

	node := Node{
		ID:        id,
		Type:      t,
		Label:     label,
		Content:   content,
		Metadata:  metadata,
		CreatedAt: now,
		UpdatedAt: now,
	}
	g.Nodes = append(g.Nodes, node)
	if err := save(rootDir, g); err != nil {
		return nil, err
	}
	return &node, nil
}

// CreateRelation adds a directed edge between two existing nodes.
// Returns nil if either node ID is not found.
func CreateRelation(rootDir, sourceID, targetID string, relation RelationType, weight float64, metadata map[string]string) (*Edge, error) {
	mu.Lock()
	defer mu.Unlock()

	g, err := loadLocked(rootDir)
	if err != nil {
		return nil, err
	}

	if !nodeExists(g, sourceID) || !nodeExists(g, targetID) {
		return nil, nil //nolint:nilnil // caller checks nil to detect missing nodes
	}

	if weight <= 0 {
		weight = 1.0
	}

	id := edgeID(sourceID, targetID, relation)

	// Upsert: update weight if edge already exists.
	for i := range g.Edges {
		if g.Edges[i].ID == id {
			g.Edges[i].Weight = weight
			g.Edges[i].Metadata = metadata
			edge := g.Edges[i]
			if err := save(rootDir, g); err != nil {
				return nil, err
			}
			return &edge, nil
		}
	}

	edge := Edge{
		ID:       id,
		Source:   sourceID,
		Target:   targetID,
		Relation: relation,
		Weight:   weight,
		Metadata: metadata,
	}
	g.Edges = append(g.Edges, edge)
	if err := save(rootDir, g); err != nil {
		return nil, err
	}
	return &edge, nil
}

// GetGraphStats returns a snapshot of node and edge counts.
func GetGraphStats(rootDir string) (GraphStats, error) {
	g, err := load(rootDir)
	if err != nil {
		return GraphStats{}, err
	}
	return GraphStats{Nodes: len(g.Nodes), Edges: len(g.Edges)}, nil
}

// PruneResult reports what was removed during a prune pass.
type PruneResult struct {
	Removed   int
	Remaining int
}

// PruneStaleLinks removes edges whose weight falls below threshold and then
// removes any nodes that have become fully orphaned (no edges in or out).
func PruneStaleLinks(rootDir string, threshold float64) (PruneResult, error) {
	if threshold <= 0 {
		threshold = 0.1
	}

	mu.Lock()
	defer mu.Unlock()

	g, err := loadLocked(rootDir)
	if err != nil {
		return PruneResult{}, err
	}

	removed := 0

	// Remove weak edges.
	live := g.Edges[:0]
	for _, e := range g.Edges {
		if e.Weight >= threshold {
			live = append(live, e)
		} else {
			removed++
		}
	}
	g.Edges = live

	// Remove orphaned nodes (no remaining edges reference them).
	connected := make(map[string]bool, len(g.Nodes))
	for _, e := range g.Edges {
		connected[e.Source] = true
		connected[e.Target] = true
	}
	liveNodes := g.Nodes[:0]
	for _, n := range g.Nodes {
		if connected[n.ID] {
			liveNodes = append(liveNodes, n)
		} else {
			removed++
		}
	}
	g.Nodes = liveNodes

< truncated lines 316-378 >
}

// SearchGraph finds nodes whose label or content matches query, then expands
// one hop to collect linked neighbors. Results are scored by relevance.
func SearchGraph(rootDir, query string, maxDepth, topK int, edgeFilter []RelationType) (*SearchResult, error) {
	if maxDepth <= 0 {
		maxDepth = 2
	}
	if topK <= 0 {
		topK = 10
	}

	g, err := load(rootDir)
	if err != nil {
		return nil, err
	}

	queryLower := strings.ToLower(query)
	nodeByID := indexNodes(g)
	adjOut := buildAdjacency(g, edgeFilter) // nodeID → []Edge

	// Score all nodes against the query.
	type scored struct {
		node  Node
		score float64
	}
	var candidates []scored
	for _, n := range g.Nodes {
		score := scoreNode(n, queryLower)
		if score > 0 {
			candidates = append(candidates, scored{node: n, score: score})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
	if len(candidates) > topK {
		candidates = candidates[:topK]
	}

	result := &SearchResult{
		TotalNodes: len(g.Nodes),
		TotalEdges: len(g.Edges),
	}
	directIDs := make(map[string]bool)
	for _, c := range candidates {
		result.Direct = append(result.Direct, TraversalResult{
			Node:          c.node,
			Depth:         0,
			RelevanceScore: c.score,
			PathRelations:  []string{},
		})
		directIDs[c.node.ID] = true
	}

	// Expand one hop of neighbors.
	neighborIDs := make(map[string]bool)
	for _, c := range candidates {
		for _, edge := range adjOut[c.node.ID] {
			if directIDs[edge.Target] || neighborIDs[edge.Target] {
				continue
			}
			if n, ok := nodeByID[edge.Target]; ok {
				neighborIDs[edge.Target] = true
				score := scoreNode(n, queryLower) * edge.Weight * 0.5
				result.Neighbors = append(result.Neighbors, TraversalResult{
					Node:          n,
					Depth:         1,
					RelevanceScore: score,
					PathRelations:  []string{string(edge.Relation)},
				})
			}
		}
	}
	sort.Slice(result.Neighbors, func(i, j int) bool {
		return result.Neighbors[i].RelevanceScore > result.Neighbors[j].RelevanceScore
	})

	// Bump access counts for returned nodes.
	go func() { _ = bumpAccess(rootDir, directIDs) }()

	return result, nil
}

// RetrieveWithTraversal performs a BFS/depth-limited walk starting from
// startNodeID, returning all reachable nodes up to maxDepth hops away.
// edgeFilter restricts which relation types are followed; nil follows all.
func RetrieveWithTraversal(rootDir, startNodeID string, maxDepth int, edgeFilter []RelationType) ([]TraversalResult, error) {
	if maxDepth <= 0 {
		maxDepth = 2
	}

	g, err := load(rootDir)
	if err != nil {
		return nil, err
	}

	nodeByID := indexNodes(g)
	startNode, ok := nodeByID[startNodeID]
	if !ok {
		return nil, nil
	}

	adjOut := buildAdjacency(g, edgeFilter)

	type queueItem struct {
		nodeID        string
		depth         int
		pathRelations []string
		score         float64
	}

	visited := map[string]bool{startNodeID: true}
	queue := []queueItem{{nodeID: startNodeID, depth: 0, pathRelations: []string{}, score: 1.0}}
	var results []TraversalResult

	results = append(results, TraversalResult{
		Node:          startNode,
		Depth:         0,
		RelevanceScore: 1.0,
		PathRelations:  []string{},
	})

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if item.depth >= maxDepth {
			continue
		}

		for _, edge := range adjOut[item.nodeID] {
			if visited[edge.Target] {
				continue
			}
			visited[edge.Target] = true

			n, ok := nodeByID[edge.Target]
			if !ok {
				continue
			}

			// Decay relevance with depth and edge weight.
			score := item.score * edge.Weight * math.Pow(0.8, float64(item.depth+1))
			pathRels := append(append([]string(nil), item.pathRelations...), string(edge.Relation))

			results = append(results, TraversalResult{
				Node:          n,
				Depth:         item.depth + 1,
				RelevanceScore: score,
				PathRelations:  pathRels,
			})
			queue = append(queue, queueItem{
				nodeID:        edge.Target,
				depth:         item.depth + 1,
				pathRelations: pathRels,
				score:         score,
			})
		}
	}

	// Sort by depth first, then descending relevance.
	sort.Slice(results, func(i, j int) bool {
		if results[i].Depth != results[j].Depth {
			return results[i].Depth < results[j].Depth
		}
		return results[i].RelevanceScore > results[j].RelevanceScore
	})

	go func() {
		ids := make(map[string]bool, len(results))
		for _, r := range results {
			ids[r.Node.ID] = true
		}
		_ = bumpAccess(rootDir, ids)
	}()

	return results, nil
}

// --- Internal helpers --------------------------------------------------------

// loadLocked loads the graph assuming the caller already holds mu (write lock).
// It reads directly from the in-memory cache or from disk without acquiring
// any additional locks (caller already holds the write lock).
func loadLocked(rootDir string) (*graphData, error) {
	if g, ok := cache[rootDir]; ok {
		return g, nil
	}
	path := graphPath(rootDir)
	g := &graphData{}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("memorygraph: read %s: %w", path, err)
	}
	if err == nil {
		if err := json.Unmarshal(data, g); err != nil {
			return nil, fmt.Errorf("memorygraph: parse %s: %w", path, err)
		}
	}
	cache[rootDir] = g
	return g, nil
}

func nodeExists(g *graphData, id string) bool {
	for _, n := range g.Nodes {
		if n.ID == id {
			return true
		}
	}
	return false
}

func indexNodes(g *graphData) map[string]Node {
	m := make(map[string]Node, len(g.Nodes))
	for _, n := range g.Nodes {
		m[n.ID] = n
	}
	return m
}

// buildAdjacency returns a map of nodeID → outgoing edges, optionally filtered
// by relation type.
func buildAdjacency(g *graphData, filter []RelationType) map[string][]Edge {
	allowed := make(map[RelationType]bool, len(filter))
	for _, r := range filter {
		allowed[r] = true
	}

	adj := make(map[string][]Edge)
	for _, e := range g.Edges {
		if len(filter) > 0 && !allowed[e.Relation] {
			continue
		}
		adj[e.Source] = append(adj[e.Source], e)
	}
	return adj
}

// scoreNode scores a node against a lower-cased query string.
// Label matches are weighted more heavily than content matches.
func scoreNode(n Node, queryLower string) float64 {
	labelLower := strings.ToLower(n.Label)
	contentLower := strings.ToLower(n.Content)

	var score float64
	if strings.Contains(labelLower, queryLower) {
		score += 1.0
	}
	if strings.Contains(contentLower, queryLower) {
		// Partial overlap: proportion of query tokens found in content.
		score += tokenOverlap(queryLower, contentLower) * 0.6
	}
	// Popularity bias: nodes accessed frequently are slightly preferred.
	if n.AccessCount > 0 {
		score += math.Log1p(float64(n.AccessCount)) * 0.05
	}
	return score
}

// tokenOverlap returns the fraction of query tokens present in text.
func tokenOverlap(query, text string) float64 {
	queryTokens := strings.Fields(query)
	if len(queryTokens) == 0 {
		return 0
	}
	found := 0
	for _, t := range queryTokens {
		if strings.Contains(text, t) {
			found++
		}
	}
	return float64(found) / float64(len(queryTokens))
}

// jaccardSimilarity approximates cosine similarity via token-set Jaccard.
func jaccardSimilarity(a, b string) float64 {
	ta := tokenSet(a)
	tb := tokenSet(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	intersection := 0
	for t := range ta {
		if tb[t] {
			intersection++
		}
	}
	return float64(intersection) / float64(len(ta)+len(tb)-intersection)
}

func tokenSet(s string) map[string]bool {
	m := make(map[string]bool)
	for _, t := range strings.Fields(strings.ToLower(s)) {
		m[t] = true
	}
	return m
}

// bumpAccess increments the AccessCount for each node in ids and persists.
func bumpAccess(rootDir string, ids map[string]bool) error {
	mu.Lock()
	defer mu.Unlock()

	g, ok := cache[rootDir]
	if !ok {
		return nil
	}
	for i := range g.Nodes {
		if ids[g.Nodes[i].ID] {
			g.Nodes[i].AccessCount++
		}
	}
	return save(rootDir, g)
}
