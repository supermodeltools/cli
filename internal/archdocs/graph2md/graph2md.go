package graph2md

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// toSlug converts a string to a URL-safe slug.
func toSlug(s string) string {
	s = strings.ToLower(s)
	s = nonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// Graph JSON structures matching Supermodel API response

type APIResponse struct {
	Status string          `json:"status"`
	JobID  string          `json:"jobId"`
	Error  json.RawMessage `json:"error"`
	Result *GraphResult    `json:"result"`
}

type GraphResult struct {
	GeneratedAt string          `json:"generatedAt"`
	Message     string          `json:"message"`
	Stats       GraphStats      `json:"stats"`
	Graph       Graph           `json:"graph"`
	Metadata    json.RawMessage `json:"metadata"`
	Domains     []DomainSummary `json:"domains"`
	Artifacts   []Artifact      `json:"artifacts"`
}

type GraphStats struct {
	NodeCount         int            `json:"nodeCount"`
	RelationshipCount int            `json:"relationshipCount"`
	NodeTypes         map[string]int `json:"nodeTypes"`
	RelationshipTypes map[string]int `json:"relationshipTypes"`
}

type Graph struct {
	Nodes         []Node         `json:"nodes"`
	Relationships []Relationship `json:"relationships"`
}

type Node struct {
	ID         string                 `json:"id"`
	Labels     []string               `json:"labels"`
	Properties map[string]interface{} `json:"properties"`
}

type Relationship struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	StartNode  string                 `json:"startNode"`
	EndNode    string                 `json:"endNode"`
	Properties map[string]interface{} `json:"properties"`
}

type DomainSummary struct {
	Name       string             `json:"name"`
	Subdomains []SubdomainSummary `json:"subdomains"`
	Files      []string           `json:"files"`
}

type SubdomainSummary struct {
	Name               string   `json:"name"`
	DescriptionSummary string   `json:"descriptionSummary"`
	Files              []string `json:"files"`
	Functions          []string `json:"functions"`
	Classes            []string `json:"classes"`
}

type Artifact struct {
	Type  string          `json:"type"`
	Name  string          `json:"name"`
	Graph *Graph          `json:"graph"`
	Stats json.RawMessage `json:"stats"`
}

// Run executes graph2md conversion. inputFiles is a comma-separated list of
// paths to graph JSON files. outputDir is the directory for markdown output.
func Run(inputFiles, outputDir, repoName, repoURL string, maxEntities int) error {
	if inputFiles == "" {
		return fmt.Errorf("input is required (comma-separated paths to graph JSON files)")
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	// Load and merge all graphs
	var allNodes []Node
	var allRels []Relationship
	nodeMap := make(map[string]bool)

	for _, path := range strings.Split(inputFiles, ",") {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		log.Printf("Loading graph from %s...", path)
		nodes, rels, err := loadGraph(path)
		if err != nil {
			log.Printf("Warning: failed to load %s: %v", path, err)
			continue
		}
		for _, n := range nodes {
			if !nodeMap[n.ID] {
				nodeMap[n.ID] = true
				allNodes = append(allNodes, n)
			}
		}
		allRels = append(allRels, rels...)
		log.Printf("  Loaded %d nodes, %d relationships", len(nodes), len(rels))
	}

	log.Printf("Total: %d unique nodes, %d relationships", len(allNodes), len(allRels))

	// Build node lookup: id -> node
	nodeLookup := make(map[string]*Node)
	for i := range allNodes {
		nodeLookup[allNodes[i].ID] = &allNodes[i]
	}

	// Build relationship indices
	imports := make(map[string][]string)
	importedBy := make(map[string][]string)
	callsRel := make(map[string][]string)
	calledByRel := make(map[string][]string)
	containsFile := make(map[string][]string)     // directory -> files
	definesFunc := make(map[string][]string)      // file -> functions
	declaresClass := make(map[string][]string)    // file -> classes
	definesType := make(map[string][]string)      // file -> types
	childDir := make(map[string][]string)         // directory -> subdirectories
	belongsToDomain := make(map[string]string)    // node -> domain name
	belongsToSubdomain := make(map[string]string) // node -> subdomain name
	partOfDomain := make(map[string]string)       // subdomain node ID -> domain name
	extendsRel := make(map[string][]string)       // class -> parent classes

	// Reverse lookups for "Defined In"
	fileOfFunc := make(map[string]string)  // function nodeID -> file nodeID
	fileOfClass := make(map[string]string) // class nodeID -> file nodeID
	fileOfType := make(map[string]string)  // type nodeID -> file nodeID

	// Domain/subdomain node lookups by name
	domainNodeByName := make(map[string]string)    // domain name -> domain node ID
	subdomainNodeByName := make(map[string]string) // subdomain name -> subdomain node ID

	// Domain -> subdomain mappings
	domainSubdomains := make(map[string][]string) // domain name -> subdomain node IDs

	// Subdomain -> functions/classes
	subdomainFuncs := make(map[string][]string)   // subdomain name -> function node IDs
	subdomainClasses := make(map[string][]string) // subdomain name -> class node IDs

	for _, rel := range allRels {
		switch rel.Type {
		case "IMPORTS":
			imports[rel.StartNode] = append(imports[rel.StartNode], rel.EndNode)
			importedBy[rel.EndNode] = append(importedBy[rel.EndNode], rel.StartNode)
		case "calls":
			callsRel[rel.StartNode] = append(callsRel[rel.StartNode], rel.EndNode)
			calledByRel[rel.EndNode] = append(calledByRel[rel.EndNode], rel.StartNode)
		case "CONTAINS_FILE":
			containsFile[rel.StartNode] = append(containsFile[rel.StartNode], rel.EndNode)
		case "DEFINES_FUNCTION":
			definesFunc[rel.StartNode] = append(definesFunc[rel.StartNode], rel.EndNode)
			fileOfFunc[rel.EndNode] = rel.StartNode
		case "DECLARES_CLASS":
			declaresClass[rel.StartNode] = append(declaresClass[rel.StartNode], rel.EndNode)
			fileOfClass[rel.EndNode] = rel.StartNode
		case "DEFINES":
			definesType[rel.StartNode] = append(definesType[rel.StartNode], rel.EndNode)
			fileOfType[rel.EndNode] = rel.StartNode
		case "CHILD_DIRECTORY":
			childDir[rel.StartNode] = append(childDir[rel.StartNode], rel.EndNode)
		case "EXTENDS":
			extendsRel[rel.StartNode] = append(extendsRel[rel.StartNode], rel.EndNode)
		case "belongsTo":
			endNode := nodeLookup[rel.EndNode]
			if endNode == nil {
				continue
			}
			name := getStr(endNode.Properties, "name")
			if hasLabel(endNode, "Domain") {
				belongsToDomain[rel.StartNode] = name
			} else if hasLabel(endNode, "Subdomain") {
				belongsToSubdomain[rel.StartNode] = name
			}
		case "partOf":
			endNode := nodeLookup[rel.EndNode]
			if endNode != nil {
				partOfDomain[rel.StartNode] = getStr(endNode.Properties, "name")
			}
		}
	}

	// Build domain/subdomain node-by-name lookups
	for _, node := range allNodes {
		if hasLabel(&node, "Domain") {
			name := getStr(node.Properties, "name")
			if name != "" {
				domainNodeByName[name] = node.ID
			}
		} else if hasLabel(&node, "Subdomain") {
			name := getStr(node.Properties, "name")
			if name != "" {
				subdomainNodeByName[name] = node.ID
			}
		}
	}

	// Build domain -> subdomain mapping from partOf relationships
	for subNodeID, domName := range partOfDomain {
		domainSubdomains[domName] = append(domainSubdomains[domName], subNodeID)
	}

	// Build subdomain -> functions/classes from belongsToSubdomain
	for nodeID, subName := range belongsToSubdomain {
		n := nodeLookup[nodeID]
		if n == nil {
			continue
		}
		if hasLabel(n, "Function") {
			subdomainFuncs[subName] = append(subdomainFuncs[subName], nodeID)
		} else if hasLabel(n, "Class") {
			subdomainClasses[subName] = append(subdomainClasses[subName], nodeID)
		}
	}

	// Resolve domain for files via belongsTo on their functions/classes
	for _, node := range allNodes {
		if !hasLabel(&node, "File") {
			continue
		}
		if _, ok := belongsToDomain[node.ID]; ok {
			continue
		}
		for _, fnID := range definesFunc[node.ID] {
			if d, ok := belongsToDomain[fnID]; ok {
				belongsToDomain[node.ID] = d
				break
			}
		}
		if _, ok := belongsToDomain[node.ID]; ok {
			continue
		}
		for _, clsID := range declaresClass[node.ID] {
			if d, ok := belongsToDomain[clsID]; ok {
				belongsToDomain[node.ID] = d
				break
			}
			for _, fnID := range definesFunc[clsID] {
				if d, ok := belongsToDomain[fnID]; ok {
					belongsToDomain[node.ID] = d
					break
				}
			}
			if _, ok := belongsToDomain[node.ID]; ok {
				break
			}
		}
	}

	// Similarly resolve subdomain for files
	for _, node := range allNodes {
		if !hasLabel(&node, "File") {
			continue
		}
		if _, ok := belongsToSubdomain[node.ID]; ok {
			continue
		}
		for _, fnID := range definesFunc[node.ID] {
			if s, ok := belongsToSubdomain[fnID]; ok {
				belongsToSubdomain[node.ID] = s
				break
			}
		}
		if _, ok := belongsToSubdomain[node.ID]; ok {
			continue
		}
		for _, clsID := range declaresClass[node.ID] {
			if s, ok := belongsToSubdomain[clsID]; ok {
				belongsToSubdomain[node.ID] = s
				break
			}
			for _, fnID := range definesFunc[clsID] {
				if s, ok := belongsToSubdomain[fnID]; ok {
					belongsToSubdomain[node.ID] = s
					break
				}
			}
			if _, ok := belongsToSubdomain[node.ID]; ok {
				break
			}
		}
	}

	// Propagate domain from subdomain's partOf for any node that has a
	// subdomain but no direct domain assignment.
	for nodeID, subName := range belongsToSubdomain {
		if _, ok := belongsToDomain[nodeID]; ok {
			continue
		}
		subNodeID, ok := subdomainNodeByName[subName]
		if !ok {
			continue
		}
		if domName, ok := partOfDomain[subNodeID]; ok && domName != "" {
			belongsToDomain[nodeID] = domName
		}
	}

	// Collect all domain members for Domain/Subdomain body sections
	domainFiles := make(map[string][]string)    // domain name -> file node IDs
	subdomainFiles := make(map[string][]string) // subdomain name -> file node IDs
	for nodeID, domName := range belongsToDomain {
		n := nodeLookup[nodeID]
		if n != nil && hasLabel(n, "File") {
			domainFiles[domName] = append(domainFiles[domName], nodeID)
		}
	}
	for nodeID, subName := range belongsToSubdomain {
		n := nodeLookup[nodeID]
		if n != nil && hasLabel(n, "File") {
			subdomainFiles[subName] = append(subdomainFiles[subName], nodeID)
		}
	}

	// Which node types to generate pages for
	generateLabels := map[string]bool{
		"File": true, "Function": true, "Class": true, "Type": true,
		"Domain": true, "Subdomain": true, "Directory": true,
	}

	// --- Pass 1: Generate all slugs and build nodeID -> slug lookup ---
	slugLookup := make(map[string]string)
	usedSlugs := make(map[string]int)

	type nodeEntry struct {
		node  Node
		label string
		slug  string
	}
	var entries []nodeEntry

	for _, node := range allNodes {
		if len(node.Labels) == 0 {
			continue
		}
		primaryLabel := node.Labels[0]
		if !generateLabels[primaryLabel] {
			continue
		}

		slug := generateSlug(node, primaryLabel)
		if slug == "" {
			continue
		}

		// Handle slug collisions
		if n, ok := usedSlugs[slug]; ok {
			usedSlugs[slug] = n + 1
			slug = fmt.Sprintf("%s-%d", slug, n+1)
			usedSlugs[slug] = 1 // register the disambiguated slug so a later node can't claim it
		} else {
			usedSlugs[slug] = 1
		}

		slugLookup[node.ID] = slug
		entries = append(entries, nodeEntry{node: node, label: primaryLabel, slug: slug})
	}

	log.Printf("Pass 1 complete: %d slugs generated", len(entries))

	// Cap entities by priority + connectivity to stay within CF Pages 20k file limit.
	if maxEntities > 0 && len(entries) > maxEntities {
		// Score each node by relationship degree (higher = more architecturally central)
		degree := make(map[string]int)
		for _, rel := range allRels {
			degree[rel.StartNode]++
			degree[rel.EndNode]++
		}
		// Label priority: structural first, then files, then symbols
		labelPriority := map[string]int{
			"Domain": 0, "Subdomain": 1, "Directory": 2,
			"File": 3, "Class": 4, "Type": 5, "Function": 6,
		}
		sort.SliceStable(entries, func(i, j int) bool {
			pi := labelPriority[entries[i].label]
			pj := labelPriority[entries[j].label]
			if pi != pj {
				return pi < pj
			}
			return degree[entries[i].node.ID] > degree[entries[j].node.ID]
		})
		log.Printf("Capping entities from %d to %d (max-entities limit)", len(entries), maxEntities)
		entries = entries[:maxEntities]
		// Rebuild slugLookup to only reference kept entries; dropped entities
		// will degrade to plain text in internal links.
		newSlugLookup := make(map[string]string, len(entries))
		for _, e := range entries {
			newSlugLookup[e.node.ID] = e.slug
		}
		slugLookup = newSlugLookup
	}

	// --- Pass 2: Generate markdown with internal links ---
	var count int
	for _, e := range entries {
		ctx := &renderContext{
			node:                &e.node,
			label:               e.label,
			slug:                e.slug,
			repoName:            repoName,
			repoURL:             repoURL,
			nodeLookup:          nodeLookup,
			slugLookup:          slugLookup,
			imports:             imports,
			importedBy:          importedBy,
			calls:               callsRel,
			calledBy:            calledByRel,
			containsFile:        containsFile,
			definesFunc:         definesFunc,
			declaresClass:       declaresClass,
			definesType:         definesType,
			childDir:            childDir,
			extendsRel:          extendsRel,
			belongsToDomain:     belongsToDomain,
			belongsToSubdomain:  belongsToSubdomain,
			partOfDomain:        partOfDomain,
			domainFiles:         domainFiles,
			subdomainFiles:      subdomainFiles,
			fileOfFunc:          fileOfFunc,
			fileOfClass:         fileOfClass,
			fileOfType:          fileOfType,
			domainNodeByName:    domainNodeByName,
			subdomainNodeByName: subdomainNodeByName,
			domainSubdomains:    domainSubdomains,
			subdomainFuncs:      subdomainFuncs,
			subdomainClasses:    subdomainClasses,
		}

		md := ctx.generateMarkdown()
		outPath := filepath.Join(outputDir, e.slug+".md")
		if err := os.WriteFile(outPath, []byte(md), 0644); err != nil {
			log.Printf("Warning: failed to write %s: %v", outPath, err)
			continue
		}
		count++
	}

	log.Printf("Generated %d entity files in %s", count, outputDir)
	return nil
}

type renderContext struct {
	node                                     *Node
	label, slug, repoName, repoURL           string
	nodeLookup                               map[string]*Node
	slugLookup                               map[string]string
	imports, importedBy                      map[string][]string
	calls, calledBy                          map[string][]string
	containsFile, definesFunc, declaresClass map[string][]string
	definesType, childDir, extendsRel        map[string][]string
	belongsToDomain, belongsToSubdomain      map[string]string
	partOfDomain                             map[string]string
	domainFiles, subdomainFiles              map[string][]string
	fileOfFunc, fileOfClass, fileOfType      map[string]string
	domainNodeByName, subdomainNodeByName    map[string]string
	domainSubdomains                         map[string][]string
	subdomainFuncs, subdomainClasses         map[string][]string
}

// internalLink returns an HTML <a> tag linking to the entity page for nodeID,
// or plain-text label if no slug is found.
func (c *renderContext) internalLink(nodeID, label string) string {
	slug, ok := c.slugLookup[nodeID]
	if !ok {
		return html.EscapeString(label)
	}
	return fmt.Sprintf(`<a href="/%s.html">%s</a>`, slug, html.EscapeString(label))
}

// internalLinkByName looks up a domain/subdomain node by name, then links to it.
func (c *renderContext) domainLink(domainName string) string {
	nodeID, ok := c.domainNodeByName[domainName]
	if !ok {
		return html.EscapeString(domainName)
	}
	return c.internalLink(nodeID, domainName)
}

func (c *renderContext) subdomainLink(subdomainName string) string {
	nodeID, ok := c.subdomainNodeByName[subdomainName]
	if !ok {
		return html.EscapeString(subdomainName)
	}
	return c.internalLink(nodeID, subdomainName)
}

func (c *renderContext) generateMarkdown() string {
	var sb strings.Builder

	sb.WriteString("---\n")

	switch c.label {
	case "File":
		c.writeFileFrontmatter(&sb)
	case "Function":
		c.writeFunctionFrontmatter(&sb)
	case "Class":
		c.writeClassFrontmatter(&sb)
	case "Type":
		c.writeTypeFrontmatter(&sb)
	case "Domain":
		c.writeDomainFrontmatter(&sb)
	case "Subdomain":
		c.writeSubdomainFrontmatter(&sb)
	case "Directory":
		c.writeDirectoryFrontmatter(&sb)
	}

	// Write graph_data, mermaid_diagram, arch_map frontmatter fields
	c.writeGraphData(&sb)
	c.writeMermaidDiagram(&sb)
	c.writeArchMap(&sb)

	sb.WriteString("---\n\n")

	switch c.label {
	case "File":
		c.writeFileBody(&sb)
	case "Function":
		c.writeFunctionBody(&sb)
	case "Class":
		c.writeClassBody(&sb)
	case "Type":
		c.writeTypeBody(&sb)
	case "Domain":
		c.writeDomainBody(&sb)
	case "Subdomain":
		c.writeSubdomainBody(&sb)
	case "Directory":
		c.writeDirectoryBody(&sb)
	}

	// FAQ section at the end of body
	c.writeFAQSection(&sb)

	return sb.String()
}

// --- Frontmatter writers ---

func (c *renderContext) writeFileFrontmatter(sb *strings.Builder) {
	props := c.node.Properties
	path := getStr(props, "path")
	name := getStr(props, "name")
	lang := getStr(props, "language")
	if name == "" {
		name = filepath.Base(path)
	}

	title := fmt.Sprintf("%s — %s Source File", name, c.repoName)
	desc := fmt.Sprintf("Architecture documentation for %s", name)
	if lang != "" {
		desc += fmt.Sprintf(", a %s file", lang)
	}
	desc += fmt.Sprintf(" in the %s codebase.", c.repoName)

	depCount := len(c.imports[c.node.ID])
	ibCount := len(c.importedBy[c.node.ID])
	if depCount > 0 || ibCount > 0 {
		desc += fmt.Sprintf(" %d imports, %d dependents.", depCount, ibCount)
	}

	fmt.Fprintf(sb, "title: %q\n", title)
	fmt.Fprintf(sb, "description: %q\n", desc)
	sb.WriteString("node_type: \"File\"\n")
	fmt.Fprintf(sb, "file_path: %q\n", path)
	fmt.Fprintf(sb, "file_name: %q\n", name)
	if lang != "" {
		fmt.Fprintf(sb, "language: %q\n", lang)
	}
	fmt.Fprintf(sb, "repo: %q\n", c.repoName)
	fmt.Fprintf(sb, "repo_url: %q\n", c.repoURL)

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		fmt.Fprintf(sb, "directory: %q\n", dir)
		parts := strings.Split(dir, "/")
		if len(parts) > 0 {
			fmt.Fprintf(sb, "top_directory: %q\n", parts[0])
		}
	}

	ext := filepath.Ext(name)
	if ext != "" {
		fmt.Fprintf(sb, "extension: %q\n", ext)
	}

	if d, ok := c.belongsToDomain[c.node.ID]; ok {
		fmt.Fprintf(sb, "domain: %q\n", d)
	}
	if s, ok := c.belongsToSubdomain[c.node.ID]; ok {
		fmt.Fprintf(sb, "subdomain: %q\n", s)
	}

	// File line count: use lineCount property, or compute from endLine
	if lc := getNum(props, "lineCount"); lc > 0 {
		fmt.Fprintf(sb, "line_count: %d\n", lc)
	} else if endLine := getNum(props, "endLine"); endLine > 0 {
		startLine := getNum(props, "startLine")
		if startLine <= 0 {
			startLine = 1
		}
		fmt.Fprintf(sb, "start_line: %d\n", startLine)
		fmt.Fprintf(sb, "end_line: %d\n", endLine)
		fmt.Fprintf(sb, "line_count: %d\n", endLine-startLine+1)
	}

	fmt.Fprintf(sb, "import_count: %d\n", depCount)
	fmt.Fprintf(sb, "imported_by_count: %d\n", ibCount)

	funcCount := len(c.definesFunc[c.node.ID])
	classCount := len(c.declaresClass[c.node.ID])
	typeCount := len(c.definesType[c.node.ID])
	fmt.Fprintf(sb, "function_count: %d\n", funcCount)
	fmt.Fprintf(sb, "class_count: %d\n", classCount)
	fmt.Fprintf(sb, "type_count: %d\n", typeCount)

	c.writeTags(sb)
}

func (c *renderContext) writeFunctionFrontmatter(sb *strings.Builder) {
	props := c.node.Properties
	name := getStr(props, "name")
	filePath := getStr(props, "filePath")
	lang := getStr(props, "language")
	startLine := getNum(props, "startLine")
	endLine := getNum(props, "endLine")

	title := fmt.Sprintf("%s() — %s Function Reference", name, c.repoName)
	desc := fmt.Sprintf("Architecture documentation for the %s() function", name)
	if filePath != "" {
		desc += fmt.Sprintf(" in %s", filepath.Base(filePath))
	}
	desc += fmt.Sprintf(" from the %s codebase.", c.repoName)

	fmt.Fprintf(sb, "title: %q\n", title)
	fmt.Fprintf(sb, "description: %q\n", desc)
	sb.WriteString("node_type: \"Function\"\n")
	fmt.Fprintf(sb, "function_name: %q\n", name)
	if filePath != "" {
		fmt.Fprintf(sb, "file_path: %q\n", filePath)
		dir := filepath.Dir(filePath)
		if dir != "" && dir != "." {
			fmt.Fprintf(sb, "directory: %q\n", dir)
		}
	}
	if lang != "" {
		fmt.Fprintf(sb, "language: %q\n", lang)
	}
	if startLine > 0 {
		fmt.Fprintf(sb, "start_line: %d\n", startLine)
	}
	if endLine > 0 {
		effectiveStart := startLine
		if effectiveStart <= 0 {
			effectiveStart = 1
		}
		fmt.Fprintf(sb, "end_line: %d\n", endLine)
		fmt.Fprintf(sb, "line_count: %d\n", endLine-effectiveStart+1)
	}
	fmt.Fprintf(sb, "repo: %q\n", c.repoName)
	fmt.Fprintf(sb, "call_count: %d\n", len(c.calls[c.node.ID]))
	fmt.Fprintf(sb, "called_by_count: %d\n", len(c.calledBy[c.node.ID]))

	if d, ok := c.belongsToDomain[c.node.ID]; ok {
		fmt.Fprintf(sb, "domain: %q\n", d)
	}
	if s, ok := c.belongsToSubdomain[c.node.ID]; ok {
		fmt.Fprintf(sb, "subdomain: %q\n", s)
	}

	c.writeTags(sb)
}

func (c *renderContext) writeClassFrontmatter(sb *strings.Builder) {
	props := c.node.Properties
	name := getStr(props, "name")
	filePath := getStr(props, "filePath")
	lang := getStr(props, "language")
	startLine := getNum(props, "startLine")
	endLine := getNum(props, "endLine")

	title := fmt.Sprintf("%s Class — %s Architecture", name, c.repoName)
	desc := fmt.Sprintf("Architecture documentation for the %s class", name)
	if filePath != "" {
		desc += fmt.Sprintf(" in %s", filepath.Base(filePath))
	}
	desc += fmt.Sprintf(" from the %s codebase.", c.repoName)

	fmt.Fprintf(sb, "title: %q\n", title)
	fmt.Fprintf(sb, "description: %q\n", desc)
	sb.WriteString("node_type: \"Class\"\n")
	fmt.Fprintf(sb, "class_name: %q\n", name)
	if filePath != "" {
		fmt.Fprintf(sb, "file_path: %q\n", filePath)
		dir := filepath.Dir(filePath)
		if dir != "" && dir != "." {
			fmt.Fprintf(sb, "directory: %q\n", dir)
		}
	}
	if lang != "" {
		fmt.Fprintf(sb, "language: %q\n", lang)
	}
	if startLine > 0 {
		fmt.Fprintf(sb, "start_line: %d\n", startLine)
	}
	if endLine > 0 {
		effectiveStart := startLine
		if effectiveStart <= 0 {
			effectiveStart = 1
		}
		fmt.Fprintf(sb, "end_line: %d\n", endLine)
		fmt.Fprintf(sb, "line_count: %d\n", endLine-effectiveStart+1)
	}
	fmt.Fprintf(sb, "repo: %q\n", c.repoName)

	if d, ok := c.belongsToDomain[c.node.ID]; ok {
		fmt.Fprintf(sb, "domain: %q\n", d)
	}
	if s, ok := c.belongsToSubdomain[c.node.ID]; ok {
		fmt.Fprintf(sb, "subdomain: %q\n", s)
	}

	extends := c.extendsRel[c.node.ID]
	if len(extends) > 0 {
		names := c.resolveNames(extends)
		fmt.Fprintf(sb, "extends: %q\n", strings.Join(names, ", "))
	}

	c.writeTags(sb)
}

func (c *renderContext) writeTypeFrontmatter(sb *strings.Builder) {
	props := c.node.Properties
	name := getStr(props, "name")
	filePath := getStr(props, "filePath")
	lang := getStr(props, "language")
	startLine := getNum(props, "startLine")
	endLine := getNum(props, "endLine")

	title := fmt.Sprintf("%s Type — %s Architecture", name, c.repoName)
	desc := fmt.Sprintf("Architecture documentation for the %s type/interface", name)
	if filePath != "" {
		desc += fmt.Sprintf(" in %s", filepath.Base(filePath))
	}
	desc += fmt.Sprintf(" from the %s codebase.", c.repoName)

	fmt.Fprintf(sb, "title: %q\n", title)
	fmt.Fprintf(sb, "description: %q\n", desc)
	sb.WriteString("node_type: \"Type\"\n")
	fmt.Fprintf(sb, "type_name: %q\n", name)
	if filePath != "" {
		fmt.Fprintf(sb, "file_path: %q\n", filePath)
		dir := filepath.Dir(filePath)
		if dir != "" && dir != "." {
			fmt.Fprintf(sb, "directory: %q\n", dir)
		}
	}
	if lang != "" {
		fmt.Fprintf(sb, "language: %q\n", lang)
	}
	if startLine > 0 {
		fmt.Fprintf(sb, "start_line: %d\n", startLine)
	}
	if endLine > 0 {
		effectiveStart := startLine
		if effectiveStart <= 0 {
			effectiveStart = 1
		}
		fmt.Fprintf(sb, "end_line: %d\n", endLine)
		fmt.Fprintf(sb, "line_count: %d\n", endLine-effectiveStart+1)
	}
	fmt.Fprintf(sb, "repo: %q\n", c.repoName)

	if d, ok := c.belongsToDomain[c.node.ID]; ok {
		fmt.Fprintf(sb, "domain: %q\n", d)
	}
	if s, ok := c.belongsToSubdomain[c.node.ID]; ok {
		fmt.Fprintf(sb, "subdomain: %q\n", s)
	}

	c.writeTags(sb)
}

func (c *renderContext) writeDomainFrontmatter(sb *strings.Builder) {
	name := getStr(c.node.Properties, "name")
	if name == "" {
		name = c.node.ID
	}

	nodeDesc := getStr(c.node.Properties, "description")
	fileCount := len(c.domainFiles[name])
	title := fmt.Sprintf("%s Domain — %s Architecture", name, c.repoName)
	desc := ""
	if nodeDesc != "" {
		desc = nodeDesc + " "
	}
	desc += fmt.Sprintf("Architectural overview of the %s domain in the %s codebase. Contains %d source files.", name, c.repoName, fileCount)

	fmt.Fprintf(sb, "title: %q\n", title)
	fmt.Fprintf(sb, "description: %q\n", desc)
	sb.WriteString("node_type: \"Domain\"\n")
	fmt.Fprintf(sb, "domain: %q\n", name)
	fmt.Fprintf(sb, "repo: %q\n", c.repoName)
	fmt.Fprintf(sb, "file_count: %d\n", fileCount)
	if nodeDesc != "" {
		fmt.Fprintf(sb, "summary: %q\n", nodeDesc)
	}

	c.writeTags(sb)
}

func (c *renderContext) writeSubdomainFrontmatter(sb *strings.Builder) {
	name := getStr(c.node.Properties, "name")
	if name == "" {
		name = c.node.ID
	}

	nodeDesc := getStr(c.node.Properties, "description")
	parentDomain := c.partOfDomain[c.node.ID]
	fileCount := len(c.subdomainFiles[name])

	title := fmt.Sprintf("%s — %s Architecture", name, c.repoName)
	desc := ""
	if nodeDesc != "" {
		desc = nodeDesc + " "
	}
	desc += fmt.Sprintf("Architecture documentation for the %s subdomain", name)
	if parentDomain != "" {
		desc += fmt.Sprintf(" (part of %s domain)", parentDomain)
	}
	desc += fmt.Sprintf(" in the %s codebase. Contains %d source files.", c.repoName, fileCount)

	fmt.Fprintf(sb, "title: %q\n", title)
	fmt.Fprintf(sb, "description: %q\n", desc)
	sb.WriteString("node_type: \"Subdomain\"\n")
	fmt.Fprintf(sb, "subdomain: %q\n", name)
	if parentDomain != "" {
		fmt.Fprintf(sb, "domain: %q\n", parentDomain)
	}
	fmt.Fprintf(sb, "repo: %q\n", c.repoName)
	fmt.Fprintf(sb, "file_count: %d\n", fileCount)
	if nodeDesc != "" {
		fmt.Fprintf(sb, "summary: %q\n", nodeDesc)
	}

	c.writeTags(sb)
}

func (c *renderContext) writeDirectoryFrontmatter(sb *strings.Builder) {
	props := c.node.Properties
	name := getStr(props, "name")
	path := getStr(props, "path")
	if name == "" {
		name = filepath.Base(path)
	}
	if path == "" {
		path = name
	}

	// Skip root directory
	if strings.Contains(path, "/app/repo-root/") {
		return
	}

	fileCount := len(c.containsFile[c.node.ID])
	subdirCount := len(c.childDir[c.node.ID])

	// Aggregate function/class/type counts from contained files
	funcCount := 0
	classCount := 0
	typeCount := 0
	for _, fileID := range c.containsFile[c.node.ID] {
		funcCount += len(c.definesFunc[fileID])
		classCount += len(c.declaresClass[fileID])
		typeCount += len(c.definesType[fileID])
	}

	title := fmt.Sprintf("%s/ — %s Directory Structure", path, c.repoName)
	desc := fmt.Sprintf("Directory listing for %s/ in the %s codebase. Contains %d files and %d subdirectories.", path, c.repoName, fileCount, subdirCount)

	fmt.Fprintf(sb, "title: %q\n", title)
	fmt.Fprintf(sb, "description: %q\n", desc)
	sb.WriteString("node_type: \"Directory\"\n")
	fmt.Fprintf(sb, "dir_name: %q\n", name)
	fmt.Fprintf(sb, "dir_path: %q\n", path)
	fmt.Fprintf(sb, "repo: %q\n", c.repoName)
	fmt.Fprintf(sb, "file_count: %d\n", fileCount)
	fmt.Fprintf(sb, "subdir_count: %d\n", subdirCount)
	fmt.Fprintf(sb, "function_count: %d\n", funcCount)
	fmt.Fprintf(sb, "class_count: %d\n", classCount)
	fmt.Fprintf(sb, "type_count: %d\n", typeCount)

	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		fmt.Fprintf(sb, "top_directory: %q\n", parts[0])
	}

	c.writeTags(sb)
}

// --- Body writers ---

func (c *renderContext) writeFileBody(sb *strings.Builder) {
	props := c.node.Properties
	path := getStr(props, "path")

	// Domain link
	if d, ok := c.belongsToDomain[c.node.ID]; ok {
		sb.WriteString("## Domain\n\n")
		fmt.Fprintf(sb, "- %s\n", c.domainLink(d))
		sb.WriteString("\n")

		// Subdomain link (only show if domain exists)
		if s, ok := c.belongsToSubdomain[c.node.ID]; ok {
			sb.WriteString("## Subdomains\n\n")
			fmt.Fprintf(sb, "- %s\n", c.subdomainLink(s))
			sb.WriteString("\n")
		}
	}

	// Functions defined in this file
	funcs := c.definesFunc[c.node.ID]
	if len(funcs) > 0 {
		sb.WriteString("## Functions\n\n")
		c.writeLinkedList(sb, funcs, func(id string) string {
			name := c.resolveName(id)
			return c.internalLink(id, name+"()")
		})
	}

	// Classes defined in this file
	classes := c.declaresClass[c.node.ID]
	if len(classes) > 0 {
		sb.WriteString("## Classes\n\n")
		c.writeLinkedList(sb, classes, func(id string) string {
			return c.internalLink(id, c.resolveName(id))
		})
	}

	// Types defined in this file
	types := c.definesType[c.node.ID]
	if len(types) > 0 {
		sb.WriteString("## Types\n\n")
		c.writeLinkedList(sb, types, func(id string) string {
			return c.internalLink(id, c.resolveName(id))
		})
	}

	// Dependencies
	deps := c.imports[c.node.ID]
	if len(deps) > 0 {
		sb.WriteString("## Dependencies\n\n")
		c.writeLinkedList(sb, deps, func(id string) string {
			return c.internalLink(id, c.resolveName(id))
		})
	}

	// Imported By
	ib := c.importedBy[c.node.ID]
	if len(ib) > 0 {
		sb.WriteString("## Imported By\n\n")
		c.writeLinkedList(sb, ib, func(id string) string {
			return c.internalLink(id, c.resolveNameWithPath(id))
		})
	}

	// Source link
	if path != "" && c.repoURL != "" {
		sb.WriteString("## Source\n\n")
		fmt.Fprintf(sb, "- <a href=\"%s/blob/main/%s\">View on GitHub</a>\n\n", c.repoURL, path)
	}
}

func (c *renderContext) writeFunctionBody(sb *strings.Builder) {
	props := c.node.Properties
	filePath := getStr(props, "filePath")
	startLine := getNum(props, "startLine")

	// Defined In
	if fileID, ok := c.fileOfFunc[c.node.ID]; ok {
		sb.WriteString("## Defined In\n\n")
		fmt.Fprintf(sb, "- %s\n", c.internalLink(fileID, c.resolveNameWithPath(fileID)))
		sb.WriteString("\n")
	}

	// Domain link
	if d, ok := c.belongsToDomain[c.node.ID]; ok {
		sb.WriteString("## Domain\n\n")
		fmt.Fprintf(sb, "- %s\n", c.domainLink(d))
		sb.WriteString("\n")

		if s, ok := c.belongsToSubdomain[c.node.ID]; ok {
			sb.WriteString("## Subdomains\n\n")
			fmt.Fprintf(sb, "- %s\n", c.subdomainLink(s))
			sb.WriteString("\n")
		}
	}

	// Calls
	called := c.calls[c.node.ID]
	if len(called) > 0 {
		sb.WriteString("## Calls\n\n")
		c.writeLinkedList(sb, called, func(id string) string {
			name := c.resolveName(id)
			return c.internalLink(id, name+"()")
		})
	}

	// Called By
	callers := c.calledBy[c.node.ID]
	if len(callers) > 0 {
		sb.WriteString("## Called By\n\n")
		c.writeLinkedList(sb, callers, func(id string) string {
			name := c.resolveName(id)
			return c.internalLink(id, name+"()")
		})
	}

	// Source
	if filePath != "" && c.repoURL != "" {
		sb.WriteString("## Source\n\n")
		link := fmt.Sprintf("%s/blob/main/%s", c.repoURL, filePath)
		if startLine > 0 {
			link += fmt.Sprintf("#L%d", startLine)
		}
		fmt.Fprintf(sb, "- <a href=\"%s\">View on GitHub</a>\n\n", link)
	}
}

func (c *renderContext) writeClassBody(sb *strings.Builder) {
	props := c.node.Properties
	filePath := getStr(props, "filePath")
	startLine := getNum(props, "startLine")

	// Defined In
	if fileID, ok := c.fileOfClass[c.node.ID]; ok {
		sb.WriteString("## Defined In\n\n")
		fmt.Fprintf(sb, "- %s\n", c.internalLink(fileID, c.resolveNameWithPath(fileID)))
		sb.WriteString("\n")
	}

	// Domain link
	if d, ok := c.belongsToDomain[c.node.ID]; ok {
		sb.WriteString("## Domain\n\n")
		fmt.Fprintf(sb, "- %s\n", c.domainLink(d))
		sb.WriteString("\n")

		if s, ok := c.belongsToSubdomain[c.node.ID]; ok {
			sb.WriteString("## Subdomains\n\n")
			fmt.Fprintf(sb, "- %s\n", c.subdomainLink(s))
			sb.WriteString("\n")
		}
	}

	// Extends
	extends := c.extendsRel[c.node.ID]
	if len(extends) > 0 {
		sb.WriteString("## Extends\n\n")
		for _, id := range extends {
			fmt.Fprintf(sb, "- %s\n", c.internalLink(id, c.resolveName(id)))
		}
		sb.WriteString("\n")
	}

	// Source
	if filePath != "" && c.repoURL != "" {
		sb.WriteString("## Source\n\n")
		link := fmt.Sprintf("%s/blob/main/%s", c.repoURL, filePath)
		if startLine > 0 {
			link += fmt.Sprintf("#L%d", startLine)
		}
		fmt.Fprintf(sb, "- <a href=\"%s\">View on GitHub</a>\n\n", link)
	}
}

func (c *renderContext) writeTypeBody(sb *strings.Builder) {
	props := c.node.Properties
	filePath := getStr(props, "filePath")
	startLine := getNum(props, "startLine")

	// Defined In
	if fileID, ok := c.fileOfType[c.node.ID]; ok {
		sb.WriteString("## Defined In\n\n")
		fmt.Fprintf(sb, "- %s\n", c.internalLink(fileID, c.resolveNameWithPath(fileID)))
		sb.WriteString("\n")
	}

	// Domain link
	if d, ok := c.belongsToDomain[c.node.ID]; ok {
		sb.WriteString("## Domain\n\n")
		fmt.Fprintf(sb, "- %s\n", c.domainLink(d))
		sb.WriteString("\n")

		if s, ok := c.belongsToSubdomain[c.node.ID]; ok {
			sb.WriteString("## Subdomains\n\n")
			fmt.Fprintf(sb, "- %s\n", c.subdomainLink(s))
			sb.WriteString("\n")
		}
	}

	if filePath != "" && c.repoURL != "" {
		sb.WriteString("## Source\n\n")
		link := fmt.Sprintf("%s/blob/main/%s", c.repoURL, filePath)
		if startLine > 0 {
			link += fmt.Sprintf("#L%d", startLine)
		}
		fmt.Fprintf(sb, "- <a href=\"%s\">View on GitHub</a>\n\n", link)
	}
}

func (c *renderContext) writeDomainBody(sb *strings.Builder) {
	name := getStr(c.node.Properties, "name")

	// Subdomains
	subs := c.domainSubdomains[name]
	if len(subs) > 0 {
		sb.WriteString("## Subdomains\n\n")
		c.writeLinkedList(sb, subs, func(id string) string {
			return c.internalLink(id, c.resolveName(id))
		})
	}

	// Source Files
	files := c.domainFiles[name]
	if len(files) > 0 {
		sb.WriteString("## Source Files\n\n")
		c.writeLinkedList(sb, files, func(id string) string {
			return c.internalLink(id, c.resolveNameWithPath(id))
		})
	}
}

func (c *renderContext) writeSubdomainBody(sb *strings.Builder) {
	name := getStr(c.node.Properties, "name")

	// Domain link
	if parentDomain := c.partOfDomain[c.node.ID]; parentDomain != "" {
		sb.WriteString("## Domain\n\n")
		fmt.Fprintf(sb, "- %s\n", c.domainLink(parentDomain))
		sb.WriteString("\n")
	}

	// Functions in this subdomain
	funcs := c.subdomainFuncs[name]
	if len(funcs) > 0 {
		sb.WriteString("## Functions\n\n")
		c.writeLinkedList(sb, funcs, func(id string) string {
			fnName := c.resolveName(id)
			return c.internalLink(id, fnName+"()")
		})
	}

	// Classes in this subdomain
	classes := c.subdomainClasses[name]
	if len(classes) > 0 {
		sb.WriteString("## Classes\n\n")
		c.writeLinkedList(sb, classes, func(id string) string {
			return c.internalLink(id, c.resolveName(id))
		})
	}

	// Source Files
	files := c.subdomainFiles[name]
	if len(files) > 0 {
		sb.WriteString("## Source Files\n\n")
		c.writeLinkedList(sb, files, func(id string) string {
			return c.internalLink(id, c.resolveNameWithPath(id))
		})
	}
}

func (c *renderContext) writeDirectoryBody(sb *strings.Builder) {
	// Subdirectories
	subdirs := c.childDir[c.node.ID]
	if len(subdirs) > 0 {
		sb.WriteString("## Subdirectories\n\n")
		c.writeLinkedList(sb, subdirs, func(id string) string {
			label := c.resolveNameWithPath(id) + "/"
			return c.internalLink(id, label)
		})
	}

	// Files
	files := c.containsFile[c.node.ID]
	if len(files) > 0 {
		sb.WriteString("## Files\n\n")
		c.writeLinkedList(sb, files, func(id string) string {
			return c.internalLink(id, c.resolveName(id))
		})
	}
}

// --- FAQ Section ---

func (c *renderContext) writeFAQSection(sb *strings.Builder) {
	type faqEntry struct{ q, a string }
	var faqs []faqEntry

	name := getStr(c.node.Properties, "name")
	if name == "" {
		name = c.node.ID
	}

	switch c.label {
	case "File":
		path := getStr(c.node.Properties, "path")
		fileName := getStr(c.node.Properties, "name")
		if fileName == "" {
			fileName = filepath.Base(path)
		}
		lang := getStr(c.node.Properties, "language")

		// What does file do?
		desc := fmt.Sprintf("%s is a source file in the %s codebase", fileName, c.repoName)
		if lang != "" {
			desc += fmt.Sprintf(", written in %s", lang)
		}
		desc += "."
		if d, ok := c.belongsToDomain[c.node.ID]; ok {
			desc += fmt.Sprintf(" It belongs to the %s domain", d)
			if s, ok := c.belongsToSubdomain[c.node.ID]; ok {
				desc += fmt.Sprintf(", %s subdomain", s)
			}
			desc += "."
		}
		faqs = append(faqs, faqEntry{fmt.Sprintf("What does %s do?", fileName), desc})

		// Functions defined
		funcs := c.definesFunc[c.node.ID]
		if len(funcs) > 0 {
			names := c.resolveNames(funcs)
			sort.Strings(names)
			listed := names
			if len(listed) > 10 {
				listed = listed[:10]
			}
			a := fmt.Sprintf("%s defines %d function(s): %s", fileName, len(funcs), strings.Join(listed, ", "))
			if len(funcs) > 10 {
				a += fmt.Sprintf(", and %d more", len(funcs)-10)
			}
			a += "."
			faqs = append(faqs, faqEntry{fmt.Sprintf("What functions are defined in %s?", fileName), a})
		}

		// Dependencies
		deps := c.imports[c.node.ID]
		if len(deps) > 0 {
			names := c.resolveNames(deps)
			sort.Strings(names)
			listed := names
			if len(listed) > 8 {
				listed = listed[:8]
			}
			a := fmt.Sprintf("%s imports %d module(s): %s", fileName, len(deps), strings.Join(listed, ", "))
			if len(deps) > 8 {
				a += fmt.Sprintf(", and %d more", len(deps)-8)
			}
			a += "."
			faqs = append(faqs, faqEntry{fmt.Sprintf("What does %s depend on?", fileName), a})
		}

		// Imported by
		ib := c.importedBy[c.node.ID]
		if len(ib) > 0 {
			names := c.resolveNames(ib)
			sort.Strings(names)
			listed := names
			if len(listed) > 8 {
				listed = listed[:8]
			}
			a := fmt.Sprintf("%s is imported by %d file(s): %s", fileName, len(ib), strings.Join(listed, ", "))
			if len(ib) > 8 {
				a += fmt.Sprintf(", and %d more", len(ib)-8)
			}
			a += "."
			faqs = append(faqs, faqEntry{fmt.Sprintf("What files import %s?", fileName), a})
		}

		// Architecture position
		archParts := []string{}
		if d, ok := c.belongsToDomain[c.node.ID]; ok {
			archParts = append(archParts, fmt.Sprintf("domain: %s", d))
		}
		if s, ok := c.belongsToSubdomain[c.node.ID]; ok {
			archParts = append(archParts, fmt.Sprintf("subdomain: %s", s))
		}
		dir := filepath.Dir(path)
		if dir != "" && dir != "." {
			archParts = append(archParts, fmt.Sprintf("directory: %s", dir))
		}
		if len(archParts) > 0 {
			faqs = append(faqs, faqEntry{
				fmt.Sprintf("Where is %s in the architecture?", fileName),
				fmt.Sprintf("%s is located at %s (%s).", fileName, path, strings.Join(archParts, ", ")),
			})
		}

	case "Function":
		funcName := name + "()"

		// What does it do?
		desc := fmt.Sprintf("%s is a function in the %s codebase", funcName, c.repoName)
		if fileID, ok := c.fileOfFunc[c.node.ID]; ok {
			desc += fmt.Sprintf(", defined in %s", c.resolveNameWithPath(fileID))
		}
		desc += "."
		faqs = append(faqs, faqEntry{fmt.Sprintf("What does %s do?", funcName), desc})

		// Where defined
		if fileID, ok := c.fileOfFunc[c.node.ID]; ok {
			filePath := c.resolveNameWithPath(fileID)
			startLine := getNum(c.node.Properties, "startLine")
			a := fmt.Sprintf("%s is defined in %s", funcName, filePath)
			if startLine > 0 {
				a += fmt.Sprintf(" at line %d", startLine)
			}
			a += "."
			faqs = append(faqs, faqEntry{fmt.Sprintf("Where is %s defined?", funcName), a})
		}

		// What does it call?
		called := c.calls[c.node.ID]
		if len(called) > 0 {
			names := c.resolveNames(called)
			sort.Strings(names)
			listed := names
			if len(listed) > 8 {
				listed = listed[:8]
			}
			a := fmt.Sprintf("%s calls %d function(s): %s", funcName, len(called), strings.Join(listed, ", "))
			if len(called) > 8 {
				a += fmt.Sprintf(", and %d more", len(called)-8)
			}
			a += "."
			faqs = append(faqs, faqEntry{fmt.Sprintf("What does %s call?", funcName), a})
		}

		// What calls it?
		callers := c.calledBy[c.node.ID]
		if len(callers) > 0 {
			names := c.resolveNames(callers)
			sort.Strings(names)
			listed := names
			if len(listed) > 8 {
				listed = listed[:8]
			}
			a := fmt.Sprintf("%s is called by %d function(s): %s", funcName, len(callers), strings.Join(listed, ", "))
			if len(callers) > 8 {
				a += fmt.Sprintf(", and %d more", len(callers)-8)
			}
			a += "."
			faqs = append(faqs, faqEntry{fmt.Sprintf("What calls %s?", funcName), a})
		}

	case "Class":
		className := name

		desc := fmt.Sprintf("%s is a class in the %s codebase", className, c.repoName)
		if fileID, ok := c.fileOfClass[c.node.ID]; ok {
			desc += fmt.Sprintf(", defined in %s", c.resolveNameWithPath(fileID))
		}
		desc += "."
		faqs = append(faqs, faqEntry{fmt.Sprintf("What is the %s class?", className), desc})

		if fileID, ok := c.fileOfClass[c.node.ID]; ok {
			filePath := c.resolveNameWithPath(fileID)
			startLine := getNum(c.node.Properties, "startLine")
			a := fmt.Sprintf("%s is defined in %s", className, filePath)
			if startLine > 0 {
				a += fmt.Sprintf(" at line %d", startLine)
			}
			a += "."
			faqs = append(faqs, faqEntry{fmt.Sprintf("Where is %s defined?", className), a})
		}

		extends := c.extendsRel[c.node.ID]
		if len(extends) > 0 {
			names := c.resolveNames(extends)
			faqs = append(faqs, faqEntry{
				fmt.Sprintf("What does %s extend?", className),
				fmt.Sprintf("%s extends %s.", className, strings.Join(names, ", ")),
			})
		}

	case "Type":
		typeName := name

		desc := fmt.Sprintf("%s is a type/interface in the %s codebase", typeName, c.repoName)
		if fileID, ok := c.fileOfType[c.node.ID]; ok {
			desc += fmt.Sprintf(", defined in %s", c.resolveNameWithPath(fileID))
		}
		desc += "."
		faqs = append(faqs, faqEntry{fmt.Sprintf("What is the %s type?", typeName), desc})

		if fileID, ok := c.fileOfType[c.node.ID]; ok {
			filePath := c.resolveNameWithPath(fileID)
			startLine := getNum(c.node.Properties, "startLine")
			a := fmt.Sprintf("%s is defined in %s", typeName, filePath)
			if startLine > 0 {
				a += fmt.Sprintf(" at line %d", startLine)
			}
			a += "."
			faqs = append(faqs, faqEntry{fmt.Sprintf("Where is %s defined?", typeName), a})
		}

	case "Domain":
		domainName := name
		fileCount := len(c.domainFiles[domainName])
		subs := c.domainSubdomains[domainName]

		nodeDesc := getStr(c.node.Properties, "description")
		desc := fmt.Sprintf("The %s domain is an architectural grouping in the %s codebase", domainName, c.repoName)
		if nodeDesc != "" {
			desc += ". " + nodeDesc
		}
		desc += fmt.Sprintf(" It contains %d source files.", fileCount)
		faqs = append(faqs, faqEntry{fmt.Sprintf("What is the %s domain?", domainName), desc})

		if len(subs) > 0 {
			names := c.resolveNames(subs)
			sort.Strings(names)
			faqs = append(faqs, faqEntry{
				fmt.Sprintf("What subdomains are in %s?", domainName),
				fmt.Sprintf("The %s domain contains %d subdomain(s): %s.", domainName, len(subs), strings.Join(names, ", ")),
			})
		}

		faqs = append(faqs, faqEntry{
			fmt.Sprintf("How many files are in %s?", domainName),
			fmt.Sprintf("The %s domain contains %d source files.", domainName, fileCount),
		})

	case "Subdomain":
		subName := name
		parentDomain := c.partOfDomain[c.node.ID]
		fileCount := len(c.subdomainFiles[subName])
		funcs := c.subdomainFuncs[subName]

		nodeDesc := getStr(c.node.Properties, "description")
		desc := fmt.Sprintf("%s is a subdomain in the %s codebase", subName, c.repoName)
		if parentDomain != "" {
			desc += fmt.Sprintf(", part of the %s domain", parentDomain)
		}
		if nodeDesc != "" {
			desc += ". " + nodeDesc
		}
		desc += fmt.Sprintf(" It contains %d source files.", fileCount)
		faqs = append(faqs, faqEntry{fmt.Sprintf("What is the %s subdomain?", subName), desc})

		if parentDomain != "" {
			faqs = append(faqs, faqEntry{
				fmt.Sprintf("Which domain does %s belong to?", subName),
				fmt.Sprintf("%s belongs to the %s domain.", subName, parentDomain),
			})
		}

		if len(funcs) > 0 {
			names := c.resolveNames(funcs)
			sort.Strings(names)
			listed := names
			if len(listed) > 8 {
				listed = listed[:8]
			}
			a := fmt.Sprintf("The %s subdomain contains %d function(s): %s", subName, len(funcs), strings.Join(listed, ", "))
			if len(funcs) > 8 {
				a += fmt.Sprintf(", and %d more", len(funcs)-8)
			}
			a += "."
			faqs = append(faqs, faqEntry{fmt.Sprintf("What functions are in %s?", subName), a})
		}

	case "Directory":
		dirName := getStr(c.node.Properties, "name")
		if dirName == "" {
			dirName = filepath.Base(getStr(c.node.Properties, "path"))
		}
		files := c.containsFile[c.node.ID]
		subdirs := c.childDir[c.node.ID]

		desc := fmt.Sprintf("The %s/ directory contains %d files and %d subdirectories in the %s codebase.", dirName, len(files), len(subdirs), c.repoName)
		faqs = append(faqs, faqEntry{fmt.Sprintf("What's in the %s/ directory?", dirName), desc})

		if len(subdirs) > 0 {
			names := c.resolveNames(subdirs)
			sort.Strings(names)
			faqs = append(faqs, faqEntry{
				fmt.Sprintf("What subdirectories does %s/ contain?", dirName),
				fmt.Sprintf("%s/ contains %d subdirectory(ies): %s.", dirName, len(subdirs), strings.Join(names, ", ")),
			})
		}
	}

	// Require minimum 2 FAQs
	if len(faqs) < 2 {
		return
	}

	sb.WriteString("## FAQs\n\n")
	for _, faq := range faqs {
		fmt.Fprintf(sb, "### %s\n\n%s\n\n", faq.q, faq.a)
	}
}

// --- Graph Data (frontmatter) ---

type graphNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Type  string `json:"type"`
	Slug  string `json:"slug"`
	LC    int    `json:"lc,omitempty"`   // line count
	Lang  string `json:"lang,omitempty"` // language
	CC    int    `json:"cc,omitempty"`   // call count (calls out)
	CBC   int    `json:"cbc,omitempty"`  // called by count
}

type graphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

type graphData struct {
	Nodes []graphNode `json:"nodes"`
	Edges []graphEdge `json:"edges"`
}

func (c *renderContext) writeGraphData(sb *strings.Builder) {
	var nodes []graphNode
	var edges []graphEdge
	seen := make(map[string]bool)

	addNode := func(nodeID string) {
		if seen[nodeID] || len(seen) >= 31 { // center + 30 neighbors
			return
		}
		n := c.nodeLookup[nodeID]
		if n == nil {
			return
		}
		seen[nodeID] = true
		label := getStr(n.Properties, "name")
		if label == "" {
			label = nodeID
		}
		nodeType := ""
		if len(n.Labels) > 0 {
			nodeType = n.Labels[0]
		}
		// Enrichment data
		lineCount := 0
		startLine := getNum(n.Properties, "startLine")
		endLine := getNum(n.Properties, "endLine")
		if endLine > 0 {
			effectiveStart := startLine
			if effectiveStart <= 0 {
				effectiveStart = 1
			}
			lineCount = endLine - effectiveStart + 1
		}
		lang := getStr(n.Properties, "language")
		callCount := len(c.calls[nodeID])
		calledByCount := len(c.calledBy[nodeID])

		nodes = append(nodes, graphNode{
			ID:    nodeID,
			Label: label,
			Type:  nodeType,
			Slug:  c.slugLookup[nodeID],
			LC:    lineCount,
			Lang:  lang,
			CC:    callCount,
			CBC:   calledByCount,
		})
	}

	addEdge := func(from, to, relType string) {
		edges = append(edges, graphEdge{Source: from, Target: to, Type: relType})
	}

	// Add center node
	addNode(c.node.ID)

	// Collect neighbor relationships
	relSets := []struct {
		ids     []string
		relType string
		reverse bool // if true, edge goes neighbor -> center
	}{
		{c.imports[c.node.ID], "imports", false},
		{c.importedBy[c.node.ID], "imports", true},
		{c.calls[c.node.ID], "calls", false},
		{c.calledBy[c.node.ID], "calls", true},
		{c.definesFunc[c.node.ID], "defines", false},
		{c.declaresClass[c.node.ID], "defines", false},
		{c.definesType[c.node.ID], "defines", false},
		{c.extendsRel[c.node.ID], "extends", false},
		{c.containsFile[c.node.ID], "contains", false},
		{c.childDir[c.node.ID], "contains", false},
	}

	// Add file-of reverse lookups
	if fileID, ok := c.fileOfFunc[c.node.ID]; ok {
		relSets = append(relSets, struct {
			ids     []string
			relType string
			reverse bool
		}{[]string{fileID}, "defines", true})
	}
	if fileID, ok := c.fileOfClass[c.node.ID]; ok {
		relSets = append(relSets, struct {
			ids     []string
			relType string
			reverse bool
		}{[]string{fileID}, "defines", true})
	}
	if fileID, ok := c.fileOfType[c.node.ID]; ok {
		relSets = append(relSets, struct {
			ids     []string
			relType string
			reverse bool
		}{[]string{fileID}, "defines", true})
	}

	// Domain/subdomain neighbors
	if domName, ok := c.belongsToDomain[c.node.ID]; ok {
		if domNodeID, ok := c.domainNodeByName[domName]; ok {
			relSets = append(relSets, struct {
				ids     []string
				relType string
				reverse bool
			}{[]string{domNodeID}, "belongsTo", false})
		}
	}
	if subName, ok := c.belongsToSubdomain[c.node.ID]; ok {
		if subNodeID, ok := c.subdomainNodeByName[subName]; ok {
			relSets = append(relSets, struct {
				ids     []string
				relType string
				reverse bool
			}{[]string{subNodeID}, "belongsTo", false})
		}
	}

	// For domains: add subdomain children
	if c.label == "Domain" {
		domName := getStr(c.node.Properties, "name")
		relSets = append(relSets, struct {
			ids     []string
			relType string
			reverse bool
		}{c.domainSubdomains[domName], "contains", false})
	}
	// For subdomains: add domain parent
	if c.label == "Subdomain" {
		if parentDom := c.partOfDomain[c.node.ID]; parentDom != "" {
			if domNodeID, ok := c.domainNodeByName[parentDom]; ok {
				relSets = append(relSets, struct {
					ids     []string
					relType string
					reverse bool
				}{[]string{domNodeID}, "partOf", false})
			}
		}
	}

	for _, rs := range relSets {
		for _, id := range rs.ids {
			if len(seen) >= 31 {
				break
			}
			addNode(id)
			if !seen[id] {
				continue // node wasn't added (cap reached before)
			}
			if rs.reverse {
				addEdge(id, c.node.ID, rs.relType)
			} else {
				addEdge(c.node.ID, id, rs.relType)
			}
		}
	}

	if len(nodes) < 2 {
		return // no neighbors, skip
	}

	gd := graphData{Nodes: nodes, Edges: edges}
	data, err := json.Marshal(gd)
	if err != nil {
		return
	}
	fmt.Fprintf(sb, "graph_data: %q\n", string(data))
}

// --- Mermaid Diagram (frontmatter) ---

func mermaidEscape(s string) string {
	// Escape special chars for Mermaid node labels
	s = strings.ReplaceAll(s, `"`, "#quot;")
	s = strings.ReplaceAll(s, `<`, "#lt;")
	s = strings.ReplaceAll(s, `>`, "#gt;")
	return s
}

func mermaidID(nodeID string) string {
	// Create valid Mermaid node ID from arbitrary string
	id := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, nodeID)
	if id == "" {
		id = "node"
	}
	return id
}

func (c *renderContext) writeMermaidDiagram(sb *strings.Builder) {
	var lines []string
	centerID := mermaidID(c.node.ID)
	centerLabel := mermaidEscape(getStr(c.node.Properties, "name"))
	if centerLabel == "" {
		centerLabel = mermaidEscape(c.node.ID)
	}
	nodeCount := 0
	maxNodes := 15

	addedNodes := make(map[string]bool)

	addNode := func(nodeID, label string) string {
		mid := mermaidID(nodeID)
		if !addedNodes[mid] {
			addedNodes[mid] = true
			nodeCount++
		}
		return mid
	}

	switch c.label {
	case "File":
		lines = append(lines, "graph LR")
		lines = append(lines, fmt.Sprintf("  %s[\"%s\"]", centerID, centerLabel))
		addedNodes[centerID] = true
		nodeCount++

		// Imports
		for _, id := range c.imports[c.node.ID] {
			if nodeCount >= maxNodes {
				break
			}
			label := mermaidEscape(c.resolveName(id))
			mid := addNode(id, label)
			lines = append(lines, fmt.Sprintf("  %s[\"%s\"]", mid, label))
			lines = append(lines, fmt.Sprintf("  %s --> %s", centerID, mid))
		}
		// ImportedBy
		for _, id := range c.importedBy[c.node.ID] {
			if nodeCount >= maxNodes {
				break
			}
			label := mermaidEscape(c.resolveName(id))
			mid := addNode(id, label)
			lines = append(lines, fmt.Sprintf("  %s[\"%s\"]", mid, label))
			lines = append(lines, fmt.Sprintf("  %s --> %s", mid, centerID))
		}

	case "Function":
		lines = append(lines, "graph TD")
		lines = append(lines, fmt.Sprintf("  %s[\"%s()\"]", centerID, centerLabel))
		addedNodes[centerID] = true
		nodeCount++

		// File it's defined in
		if fileID, ok := c.fileOfFunc[c.node.ID]; ok {
			if nodeCount < maxNodes {
				label := mermaidEscape(c.resolveName(fileID))
				mid := addNode(fileID, label)
				lines = append(lines, fmt.Sprintf("  %s[\"%s\"]", mid, label))
				lines = append(lines, fmt.Sprintf("  %s -->|defined in| %s", centerID, mid))
			}
		}

		for _, id := range c.calledBy[c.node.ID] {
			if nodeCount >= maxNodes {
				break
			}
			label := mermaidEscape(c.resolveName(id))
			mid := addNode(id, label)
			lines = append(lines, fmt.Sprintf("  %s[\"%s()\"]", mid, label))
			lines = append(lines, fmt.Sprintf("  %s -->|calls| %s", mid, centerID))
		}
		for _, id := range c.calls[c.node.ID] {
			if nodeCount >= maxNodes {
				break
			}
			label := mermaidEscape(c.resolveName(id))
			mid := addNode(id, label)
			lines = append(lines, fmt.Sprintf("  %s[\"%s()\"]", mid, label))
			lines = append(lines, fmt.Sprintf("  %s -->|calls| %s", centerID, mid))
		}

	case "Type":
		lines = append(lines, "graph TD")
		lines = append(lines, fmt.Sprintf("  %s[\"%s\"]", centerID, centerLabel))
		addedNodes[centerID] = true
		nodeCount++

		// File it's defined in
		if fileID, ok := c.fileOfType[c.node.ID]; ok {
			if nodeCount < maxNodes {
				label := mermaidEscape(c.resolveName(fileID))
				mid := addNode(fileID, label)
				lines = append(lines, fmt.Sprintf("  %s[\"%s\"]", mid, label))
				lines = append(lines, fmt.Sprintf("  %s -->|defined in| %s", centerID, mid))
			}
		}

	case "Class":
		lines = append(lines, "graph TD")
		lines = append(lines, fmt.Sprintf("  %s[\"%s\"]", centerID, centerLabel))
		addedNodes[centerID] = true
		nodeCount++

		// Parent classes (extends)
		for _, id := range c.extendsRel[c.node.ID] {
			if nodeCount >= maxNodes {
				break
			}
			label := mermaidEscape(c.resolveName(id))
			mid := addNode(id, label)
			lines = append(lines, fmt.Sprintf("  %s[\"%s\"]", mid, label))
			lines = append(lines, fmt.Sprintf("  %s -->|extends| %s", centerID, mid))
		}

		// File it's defined in
		if fileID, ok := c.fileOfClass[c.node.ID]; ok {
			if nodeCount < maxNodes {
				label := mermaidEscape(c.resolveName(fileID))
				mid := addNode(fileID, label)
				lines = append(lines, fmt.Sprintf("  %s[\"%s\"]", mid, label))
				lines = append(lines, fmt.Sprintf("  %s -->|defined in| %s", centerID, mid))
			}
		}

		// Methods defined on this class
		for _, id := range c.definesFunc[c.node.ID] {
			if nodeCount >= maxNodes {
				break
			}
			label := mermaidEscape(c.resolveName(id))
			mid := addNode(id, label)
			lines = append(lines, fmt.Sprintf("  %s[\"%s()\"]", mid, label))
			lines = append(lines, fmt.Sprintf("  %s -->|method| %s", centerID, mid))
		}

	case "Domain":
		lines = append(lines, "graph TD")
		domName := getStr(c.node.Properties, "name")
		lines = append(lines, fmt.Sprintf("  %s[\"%s\"]", centerID, mermaidEscape(domName)))
		addedNodes[centerID] = true
		nodeCount++

		for _, subID := range c.domainSubdomains[domName] {
			if nodeCount >= maxNodes {
				break
			}
			label := mermaidEscape(c.resolveName(subID))
			mid := addNode(subID, label)
			lines = append(lines, fmt.Sprintf("  %s[\"%s\"]", mid, label))
			lines = append(lines, fmt.Sprintf("  %s --> %s", centerID, mid))
		}

	case "Subdomain":
		lines = append(lines, "graph TD")
		subName := getStr(c.node.Properties, "name")
		lines = append(lines, fmt.Sprintf("  %s[\"%s\"]", centerID, mermaidEscape(subName)))
		addedNodes[centerID] = true
		nodeCount++

		files := c.subdomainFiles[subName]
		for _, fID := range files {
			if nodeCount >= maxNodes {
				break
			}
			label := mermaidEscape(c.resolveName(fID))
			mid := addNode(fID, label)
			lines = append(lines, fmt.Sprintf("  %s[\"%s\"]", mid, label))
			lines = append(lines, fmt.Sprintf("  %s --> %s", centerID, mid))
		}

	case "Directory":
		lines = append(lines, "graph TD")
		dirName := getStr(c.node.Properties, "name")
		if dirName == "" {
			dirName = filepath.Base(getStr(c.node.Properties, "path"))
		}
		lines = append(lines, fmt.Sprintf("  %s[\"%s/\"]", centerID, mermaidEscape(dirName)))
		addedNodes[centerID] = true
		nodeCount++

		for _, id := range c.childDir[c.node.ID] {
			if nodeCount >= maxNodes {
				break
			}
			label := mermaidEscape(c.resolveName(id))
			mid := addNode(id, label)
			lines = append(lines, fmt.Sprintf("  %s[\"%s/\"]", mid, label))
			lines = append(lines, fmt.Sprintf("  %s --> %s", centerID, mid))
		}
		for _, id := range c.containsFile[c.node.ID] {
			if nodeCount >= maxNodes {
				break
			}
			label := mermaidEscape(c.resolveName(id))
			mid := addNode(id, label)
			lines = append(lines, fmt.Sprintf("  %s[\"%s\"]", mid, label))
			lines = append(lines, fmt.Sprintf("  %s --> %s", centerID, mid))
		}

	default:
		return
	}

	// Style the center node
	if len(lines) > 1 && c.label != "Class" {
		lines = append(lines, fmt.Sprintf("  style %s fill:#6366f1,stroke:#818cf8,color:#fff", centerID))
	}

	if nodeCount < 2 {
		return
	}

	diagram := strings.Join(lines, "\n")
	fmt.Fprintf(sb, "mermaid_diagram: %q\n", diagram)
}

// --- Architecture Map (frontmatter) ---

func (c *renderContext) writeArchMap(sb *strings.Builder) {
	archMap := make(map[string]interface{})

	// Domain
	if domName, ok := c.belongsToDomain[c.node.ID]; ok && domName != "" {
		entry := map[string]string{"name": domName}
		if domNodeID, ok := c.domainNodeByName[domName]; ok {
			entry["slug"] = c.slugLookup[domNodeID]
		}
		archMap["domain"] = entry
	}

	// Subdomain
	if subName, ok := c.belongsToSubdomain[c.node.ID]; ok && subName != "" {
		entry := map[string]string{"name": subName}
		if subNodeID, ok := c.subdomainNodeByName[subName]; ok {
			entry["slug"] = c.slugLookup[subNodeID]
		}
		archMap["subdomain"] = entry
	}

	// File (for functions/classes/types)
	switch c.label {
	case "Function":
		if fileID, ok := c.fileOfFunc[c.node.ID]; ok {
			archMap["file"] = map[string]string{
				"name": c.resolveName(fileID),
				"slug": c.slugLookup[fileID],
			}
		}
	case "Class":
		if fileID, ok := c.fileOfClass[c.node.ID]; ok {
			archMap["file"] = map[string]string{
				"name": c.resolveName(fileID),
				"slug": c.slugLookup[fileID],
			}
		}
	case "Type":
		if fileID, ok := c.fileOfType[c.node.ID]; ok {
			archMap["file"] = map[string]string{
				"name": c.resolveName(fileID),
				"slug": c.slugLookup[fileID],
			}
		}
	}

	// Entity itself
	name := getStr(c.node.Properties, "name")
	if name == "" {
		name = c.node.ID
	}
	archMap["entity"] = map[string]string{
		"name": name,
		"type": c.label,
		"slug": c.slug,
	}

	if len(archMap) < 2 {
		return // just the entity itself, not useful
	}

	data, err := json.Marshal(archMap)
	if err != nil {
		return
	}
	fmt.Fprintf(sb, "arch_map: %q\n", string(data))
}

// writeLinkedList writes a sorted list of linked items.
func (c *renderContext) writeLinkedList(sb *strings.Builder, nodeIDs []string, linkFn func(string) string) {
	type sortItem struct {
		label string
		id    string
	}
	items := make([]sortItem, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		items = append(items, sortItem{label: c.resolveName(id), id: id})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].label < items[j].label
	})
	for _, item := range items {
		fmt.Fprintf(sb, "- %s\n", linkFn(item.id))
	}
	sb.WriteString("\n")
}

// --- Tag generation ---

func (c *renderContext) writeTags(sb *strings.Builder) {
	var tags []string

	for _, label := range c.node.Labels {
		tags = append(tags, label)
	}

	if lang := getStr(c.node.Properties, "language"); lang != "" {
		tags = append(tags, lang)
	}

	ibCount := len(c.importedBy[c.node.ID])
	impCount := len(c.imports[c.node.ID])
	cbCount := len(c.calledBy[c.node.ID])

	if ibCount >= 5 || cbCount >= 5 {
		tags = append(tags, "High-Dependency")
	}
	if impCount >= 5 {
		tags = append(tags, "Many-Imports")
	}

	funcCount := len(c.definesFunc[c.node.ID])
	classCount := len(c.declaresClass[c.node.ID])
	if funcCount >= 10 || classCount >= 5 {
		tags = append(tags, "Complex")
	}

	if ibCount == 0 && impCount == 0 && cbCount == 0 && c.label == "File" {
		tags = append(tags, "Isolated")
	}

	if len(tags) > 0 {
		sb.WriteString("tags:\n")
		for _, t := range tags {
			fmt.Fprintf(sb, "  - %q\n", t)
		}
	}
}

// --- Helpers ---

func (c *renderContext) resolveName(nodeID string) string {
	n := c.nodeLookup[nodeID]
	if n == nil {
		return nodeID
	}
	name := getStr(n.Properties, "name")
	if name == "" {
		return nodeID
	}
	return name
}

func (c *renderContext) resolveNames(nodeIDs []string) []string {
	result := make([]string, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		result = append(result, c.resolveName(id))
	}
	return result
}

func (c *renderContext) resolveNameWithPath(nodeID string) string {
	n := c.nodeLookup[nodeID]
	if n == nil {
		return nodeID
	}
	path := getStr(n.Properties, "path")
	if path == "" {
		path = getStr(n.Properties, "filePath")
	}
	name := getStr(n.Properties, "name")
	if path != "" {
		return path
	} else if name != "" {
		return name
	}
	return nodeID
}

func loadGraph(path string) ([]Node, []Relationship, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	log.Printf("  File size: %d bytes", len(data))

	var resp APIResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		log.Printf("  APIResponse unmarshal error: %v", err)
	} else if resp.Result == nil {
		log.Printf("  APIResponse parsed but Result is nil (status=%s)", resp.Status)
	} else {
		g := resp.Result.Graph
		log.Printf("  APIResponse parsed: %d nodes, %d rels", len(g.Nodes), len(g.Relationships))
		return g.Nodes, g.Relationships, nil
	}

	var result GraphResult
	if err := json.Unmarshal(data, &result); err == nil && len(result.Graph.Nodes) > 0 {
		return result.Graph.Nodes, result.Graph.Relationships, nil
	}

	var graph Graph
	if err := json.Unmarshal(data, &graph); err == nil && len(graph.Nodes) > 0 {
		return graph.Nodes, graph.Relationships, nil
	}

	return nil, nil, fmt.Errorf("unrecognized graph format")
}

func generateSlug(node Node, label string) string {
	props := node.Properties

	switch label {
	case "File":
		path := getStr(props, "path")
		if path == "" {
			return ""
		}
		return toSlug("file-" + path)
	case "Function":
		name := getStr(props, "name")
		filePath := getStr(props, "filePath")
		if name == "" {
			return ""
		}
		if filePath != "" {
			return toSlug("fn-" + filepath.Base(filePath) + "-" + name)
		}
		return toSlug("fn-" + name)
	case "Class":
		name := getStr(props, "name")
		filePath := getStr(props, "filePath")
		if name == "" {
			return ""
		}
		if filePath != "" {
			return toSlug("class-" + filepath.Base(filePath) + "-" + name)
		}
		return toSlug("class-" + name)
	case "Type":
		name := getStr(props, "name")
		filePath := getStr(props, "filePath")
		if name == "" {
			return ""
		}
		if filePath != "" {
			return toSlug("type-" + filepath.Base(filePath) + "-" + name)
		}
		return toSlug("type-" + name)
	case "Domain":
		name := getStr(props, "name")
		if name == "" {
			return ""
		}
		return toSlug("domain-" + name)
	case "Subdomain":
		name := getStr(props, "name")
		if name == "" {
			return ""
		}
		return toSlug("subdomain-" + name)
	case "Directory":
		path := getStr(props, "path")
		if path == "" || strings.Contains(path, "/app/repo-root/") {
			return ""
		}
		return toSlug("dir-" + path)
	default:
		return ""
	}
}

func hasLabel(node *Node, label string) bool {
	for _, l := range node.Labels {
		if l == label {
			return true
		}
	}
	return false
}

func getStr(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func getNum(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}
