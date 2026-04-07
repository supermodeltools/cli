package files

import (
	"path/filepath"
	"strings"

	"github.com/supermodeltools/cli/internal/api"
)

// SourceExtensions are the file extensions that get sidecars.
var SourceExtensions = map[string]bool{
	".ts": true, ".js": true, ".mjs": true, ".jsx": true, ".tsx": true,
	".go": true, ".py": true, ".rb": true, ".rs": true,
	".java": true, ".cs": true, ".cpp": true, ".c": true, ".h": true,
	".swift": true, ".kt": true,
}

// SidecarExt is the extension tag for the combined sidecar file.
const SidecarExt = "graph"

// FuncInfo holds metadata about a function node.
type FuncInfo struct {
	ID     string
	Name   string
	File   string
	Line   int
	Domain string
}

// CallerRef is a reference to a calling/called function.
type CallerRef struct {
	FuncID string
	File   string
	Line   int
}

// Cache holds the indexed graph data.
type Cache struct {
	FnByID     map[string]*FuncInfo
	IDToPath   map[string]string
	Callers    map[string][]CallerRef // fnID → callers
	Callees    map[string][]CallerRef // fnID → callees
	Imports    map[string][]string    // filePath → imported paths
	Importers  map[string][]string    // filePath → importer paths
	FileDomain map[string]string      // filePath → domain name
}

// GraphStats summarises what was mapped after a generate or incremental update.
type GraphStats struct {
	SourceFiles   int
	Functions     int
	Relationships int
	FromCache     bool // true when data was loaded from a local cache
}

// computeStats derives a GraphStats from a SidecarIR and its built Cache.
func computeStats(ir *api.SidecarIR, c *Cache) GraphStats {
	s := GraphStats{
		Relationships: len(ir.Graph.Relationships),
	}
	for _, n := range ir.Graph.Nodes {
		switch {
		case n.HasLabel("File"):
			s.SourceFiles++
		case n.HasLabel("Function"):
			s.Functions++
		}
	}
	_ = c // reserved for future per-file breakdown
	return s
}

// NewCache creates an empty Cache.
func NewCache() *Cache {
	return &Cache{
		FnByID:     make(map[string]*FuncInfo),
		IDToPath:   make(map[string]string),
		Callers:    make(map[string][]CallerRef),
		Callees:    make(map[string][]CallerRef),
		Imports:    make(map[string][]string),
		Importers:  make(map[string][]string),
		FileDomain: make(map[string]string),
	}
}

// Build populates the cache from a SidecarIR result.
// SidecarIR preserves the full Node/Relationship data (IDs, labels, properties)
// required for sidecar rendering.
func (c *Cache) Build(ir *api.SidecarIR) { //nolint:gocyclo // multi-pass graph indexing; each branch handles one node/rel label type
	nodes := ir.Graph.Nodes
	rels := ir.Graph.Relationships

	// Pass 1: index nodes
	for i := range nodes {
		n := nodes[i]
		props := n.Properties

		switch {
		case n.HasLabel("File"):
			c.IDToPath[n.ID] = firstString(props, "filePath", "path", "name", n.ID)
		case n.HasLabel("LocalDependency"):
			c.IDToPath[n.ID] = firstString(props, "filePath", "name", n.ID)
		case n.HasLabel("ExternalDependency"):
			name := n.Prop("name")
			if name == "" {
				name = n.ID
			}
			c.IDToPath[n.ID] = "[ext]" + name
		}

		// Any node with filePath gets registered
		if fp := n.Prop("filePath"); fp != "" {
			if _, ok := c.IDToPath[n.ID]; !ok {
				c.IDToPath[n.ID] = fp
			}
		}

		if n.HasLabel("Function") {
			filePath := firstString(props, "filePath", "file", "path", "")
			name := n.Prop("name")
			if name == "" {
				// Extract from ID like "fn:src/foo.ts:bar"
				parts := strings.Split(n.ID, ":")
				name = parts[len(parts)-1]
			}
			c.FnByID[n.ID] = &FuncInfo{
				ID:   n.ID,
				Name: name,
				File: filePath,
				Line: intProp(n, "startLine"),
			}
		}
	}

	// Pass 2: index relationships
	for i := range rels {
		rel := rels[i]

		switch rel.Type {
		case "calls":
			srcFn := c.FnByID[rel.StartNode]
			dstFn := c.FnByID[rel.EndNode]

			c.Callers[rel.EndNode] = append(c.Callers[rel.EndNode], CallerRef{
				FuncID: rel.StartNode,
				File:   fnFile(srcFn),
				Line:   fnLine(srcFn),
			})
			c.Callees[rel.StartNode] = append(c.Callees[rel.StartNode], CallerRef{
				FuncID: rel.EndNode,
				File:   fnFile(dstFn),
				Line:   fnLine(dstFn),
			})

		case "imports":
			srcPath := c.IDToPath[rel.StartNode]
			dstPath := c.IDToPath[rel.EndNode]
			if strings.HasPrefix(dstPath, "[ext]") {
				continue
			}
			if srcPath != "" && dstPath != "" {
				c.Imports[srcPath] = append(c.Imports[srcPath], dstPath)
				c.Importers[dstPath] = append(c.Importers[dstPath], srcPath)
			}

		case "defines_function":
			filePath := c.IDToPath[rel.StartNode]
			if fn, ok := c.FnByID[rel.EndNode]; ok && fn.File == "" && filePath != "" {
				fn.File = filePath
			}

		case "belongsTo":
			nodePath := c.IDToPath[rel.StartNode]
			if nodePath == "" {
				if fn, ok := c.FnByID[rel.StartNode]; ok {
					nodePath = fn.File
				}
			}
			if nodePath == "" {
				continue
			}
			// Find the domain node
			for j := range nodes {
				if nodes[j].ID == rel.EndNode {
					domainName := nodes[j].Prop("name")
					if domainName == "" {
						parts := strings.Split(rel.EndNode, ":")
						domainName = parts[len(parts)-1]
					}
					c.FileDomain[nodePath] = domainName
					break
				}
			}
		}
	}

	// Domain assignments from top-level domains[] array
	for _, domain := range ir.Domains {
		for _, kf := range domain.KeyFiles {
			if _, ok := c.FileDomain[kf]; !ok {
				c.FileDomain[kf] = domain.Name
			}
		}
		for _, sub := range domain.Subdomains {
			files := sub.Files
			if len(files) == 0 {
				files = sub.KeyFiles
			}
			for _, sf := range files {
				if _, ok := c.FileDomain[sf]; !ok {
					c.FileDomain[sf] = domain.Name + "/" + sub.Name
				}
			}
		}
	}
}

// SourceFiles returns all source file paths known to the graph.
func (c *Cache) SourceFiles() []string {
	seen := make(map[string]bool)

	for _, fn := range c.FnByID {
		if fn.File != "" {
			seen[fn.File] = true
		}
	}
	for p := range c.Imports {
		seen[p] = true
	}
	for p := range c.Importers {
		seen[p] = true
	}
	for _, p := range c.IDToPath {
		if !strings.HasPrefix(p, "[ext]") {
			seen[p] = true
		}
	}

	var files []string
	for f := range seen {
		ext := strings.ToLower(filepath.Ext(f))
		if !SourceExtensions[ext] {
			continue
		}
		if isSidecarPath(f) {
			continue
		}
		files = append(files, f)
	}
	return files
}

// FuncName returns the short name for a function ID.
func (c *Cache) FuncName(fnID string) string {
	if fn, ok := c.FnByID[fnID]; ok {
		return fn.Name
	}
	parts := strings.Split(fnID, ":")
	return parts[len(parts)-1]
}

// TransitiveDependents returns all files transitively importing the given file.
func (c *Cache) TransitiveDependents(filePath string) map[string]bool {
	visited := make(map[string]bool)
	c.walkImporters(filePath, visited)
	delete(visited, filePath)
	return visited
}

func (c *Cache) walkImporters(filePath string, visited map[string]bool) {
	if visited[filePath] {
		return
	}
	visited[filePath] = true
	for _, imp := range c.Importers[filePath] {
		c.walkImporters(imp, visited)
	}
}

func isSidecarPath(name string) bool {
	base := filepath.Base(name)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	stemExt := filepath.Ext(stem)
	if stemExt == "" {
		return false
	}
	tag := strings.TrimPrefix(stemExt, ".")
	return tag == SidecarExt
}

// firstString returns the first non-empty string value from props for the given keys.
// Returns the last key as a fallback string when none match.
func firstString(props map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := props[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return keys[len(keys)-1]
}

// intProp returns an integer property from a node or 0.
func intProp(n api.Node, key string) int {
	v, ok := n.Properties[key]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return 0
	}
}

func fnFile(fn *FuncInfo) string {
	if fn == nil {
		return ""
	}
	return fn.File
}

func fnLine(fn *FuncInfo) int {
	if fn == nil {
		return 0
	}
	return fn.Line
}
