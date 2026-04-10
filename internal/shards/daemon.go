package shards

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/supermodeltools/cli/internal/api"
)

// DaemonConfig holds watch daemon configuration.
type DaemonConfig struct {
	RepoDir      string
	CacheFile    string
	Debounce     time.Duration
	NotifyPort   int
	FSWatch      bool
	PollInterval time.Duration
	LogFunc      func(string, ...interface{})
	// OnReady is called once after the initial generate completes.
	OnReady func(GraphStats)
	// OnUpdate is called after each incremental update completes.
	OnUpdate func(GraphStats)
}

// Daemon watches for file changes and keeps shards fresh.
type Daemon struct {
	cfg    DaemonConfig
	client *api.Client
	cache  *Cache
	logf   func(string, ...interface{})

	mu          sync.Mutex
	ir          *api.ShardIR
	notifyCh    chan string
	loadedCache bool // true if startup data came from local cache
}

// NewDaemon creates a daemon with the given config and API client.
func NewDaemon(cfg DaemonConfig, client *api.Client) *Daemon { //nolint:gocritic // DaemonConfig is a value-semantic config struct; pointer would complicate call sites
	if cfg.Debounce <= 0 {
		cfg.Debounce = 2 * time.Second
	}
	if cfg.NotifyPort <= 0 {
		cfg.NotifyPort = 7734
	}
	if cfg.LogFunc == nil {
		cfg.LogFunc = func(f string, a ...interface{}) {
			fmt.Printf(f+"\n", a...)
		}
	}
	return &Daemon{
		cfg:      cfg,
		client:   client,
		cache:    NewCache(),
		logf:     cfg.LogFunc,
		notifyCh: make(chan string, 256),
	}
}

// Run starts the daemon. Blocks until context is cancelled.
// Loads existing cache if available, otherwise does a full generate.
// Then waits for triggers (UDP and/or fs-watch) to perform incremental updates.
func (d *Daemon) Run(ctx context.Context) error {
	d.logf("[step:1] Building code graph")
	if err := d.loadOrGenerate(ctx); err != nil {
		return fmt.Errorf("startup: %w", err)
	}

	d.mu.Lock()
	stats := computeStats(d.ir, d.cache)
	stats.FromCache = d.loadedCache
	d.mu.Unlock()
	d.writeStatus(fmt.Sprintf("ready — %s — %d files",
		time.Now().Format(time.RFC3339), len(d.ir.Graph.Nodes)))

	d.logf("[step:2] Starting listeners")
	if d.cfg.NotifyPort > 0 {
		udpReady := make(chan error, 1)
		go d.listenUDP(ctx, udpReady)
		if err := <-udpReady; err != nil {
			if !d.cfg.FSWatch {
				if errors.Is(err, syscall.EADDRINUSE) {
					return fmt.Errorf("UDP port %d already in use — is `supermodel watch` already running?", d.cfg.NotifyPort)
				}
				return fmt.Errorf("failed to start UDP listener on port %d: %w", d.cfg.NotifyPort, err)
			}
			d.logf("Warning: UDP listener failed (FSWatch active, continuing): %v", err)
		}
	}

	if d.cfg.FSWatch {
		w := NewWatcher(d.cfg.RepoDir, d.cfg.PollInterval)
		go w.Run(ctx)
		go d.forwardWatcherEvents(w)
	}

	if d.cfg.FSWatch {
		d.logf("[step:3] Ready — watching for file changes (git poll every %s)", d.cfg.PollInterval)
	} else {
		d.logf("[step:3] Ready — listening on UDP :%d (debounce %s)", d.cfg.NotifyPort, d.cfg.Debounce)
	}
	if d.cfg.OnReady != nil {
		d.cfg.OnReady(stats)
	}

	var (
		pendingFiles  = make(map[string]bool)
		debounceTimer *time.Timer
		debounceCh    <-chan time.Time
	)

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			d.logf("Shutting down...")
			d.logf("Cleaning shard files...")
			done := make(chan struct{})
			go func() { //nolint:gosec // ctx is already cancelled; background context is intentional for cleanup
				_ = Clean(context.Background(), nil, d.cfg.RepoDir, false)
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				d.logf("Warning: shard cleanup timed out")
			}
			return nil

		case filePath, ok := <-d.notifyCh:
			if !ok {
				return nil
			}
			pendingFiles[filePath] = true
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.NewTimer(d.cfg.Debounce)
			debounceCh = debounceTimer.C

		case <-debounceCh:
			debounceCh = nil
			if len(pendingFiles) == 0 {
				continue
			}
			files := daemonSortedKeys(pendingFiles)
			pendingFiles = make(map[string]bool)
			d.incrementalUpdate(ctx, files)
		}
	}
}

// loadOrGenerate loads an existing cache if available and re-renders shards.
// If no cache exists, it does a full API fetch.
func (d *Daemon) loadOrGenerate(ctx context.Context) error {
	data, err := os.ReadFile(d.cfg.CacheFile)
	if err == nil {
		var ir api.ShardIR
		if unmarshalErr := json.Unmarshal(data, &ir); unmarshalErr != nil {
			d.logf("Warning: cache file invalid, regenerating: %v", unmarshalErr)
		} else if len(ir.Graph.Nodes) > 0 {
			d.logf("Loaded existing cache (%d nodes, %d relationships)",
				len(ir.Graph.Nodes), len(ir.Graph.Relationships))

			d.mu.Lock()
			d.ir = &ir
			d.cache = NewCache()
			d.cache.Build(&ir)
			d.loadedCache = true
			d.mu.Unlock()

			files := d.cache.SourceFiles()
			written, renderErr := RenderAll(d.cfg.RepoDir, d.cache, files, false)
			if renderErr != nil {
				return renderErr
			}
			d.logf("Rendered %d shards for %d source files", written, len(files))
			return nil
		}
	}

	d.logf("No existing cache found — generating from scratch...")
	d.writeStatus("building graph...")
	return d.fullGenerate(ctx)
}

// fullGenerate does a complete fetch + render.
func (d *Daemon) fullGenerate(ctx context.Context) error {
	d.logf("Fetching full graph from Supermodel API...")
	idemKey := newUUID()

	if fileList, listErr := DryRunList(d.cfg.RepoDir); listErr == nil {
		stats := LanguageStats(fileList)
		PrintLanguageBarChart(stats, len(fileList))
	}

	zipPath, err := CreateZipFile(d.cfg.RepoDir, nil)
	if err != nil {
		return fmt.Errorf("creating zip: %w", err)
	}
	defer os.Remove(zipPath)

	ir, err := d.client.AnalyzeShards(ctx, zipPath, idemKey, nil)
	if err != nil {
		return err
	}

	d.mu.Lock()
	d.ir = ir
	d.cache = NewCache()
	d.cache.Build(ir)
	d.saveCache()
	d.mu.Unlock()

	files := d.cache.SourceFiles()
	written, err := RenderAll(d.cfg.RepoDir, d.cache, files, false)
	if err != nil {
		return err
	}
	d.logf("Rendered %d shards for %d source files", written, len(files))
	return nil
}

// incrementalUpdate fetches graph for only changed files and re-renders affected shards.
func (d *Daemon) incrementalUpdate(ctx context.Context, changedFiles []string) {
	d.logf("Incremental update: %d files changed [%s]",
		len(changedFiles), strings.Join(changedFiles, ", "))

	d.writeStatus(fmt.Sprintf("updating %d files — last ready %s",
		len(changedFiles), time.Now().Format(time.RFC3339)))

	idemKey := newUUID()

	zipPath, err := CreateZipFile(d.cfg.RepoDir, changedFiles)
	if err != nil {
		d.logf("Incremental zip error: %v", err)
		return
	}
	defer os.Remove(zipPath)

	ir, err := d.client.AnalyzeShards(ctx, zipPath, "incremental-"+idemKey[:8], nil)
	if err != nil {
		d.logf("Incremental API error: %v", err)
		return
	}

	// Snapshot old import and call relationships before the merge so we can
	// re-render files that lost a reference after A stops importing/calling B.
	oldImports := make(map[string][]string, len(changedFiles))
	oldCalleeFiles := make(map[string][]string) // funcID -> callee file paths
	func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		changedSet := make(map[string]bool, len(changedFiles))
		for _, f := range changedFiles {
			changedSet[f] = true
		}
		for _, f := range changedFiles {
			if deps := d.cache.Imports[f]; len(deps) > 0 {
				cp := make([]string, len(deps))
				copy(cp, deps)
				oldImports[f] = cp
			}
		}
		for id, fn := range d.cache.FnByID {
			if !changedSet[fn.File] {
				continue
			}
			for _, callee := range d.cache.Callees[id] {
				if callee.File != "" {
					oldCalleeFiles[id] = append(oldCalleeFiles[id], callee.File)
				}
			}
		}
	}()

	var affected []string
	var cacheSnapshot *Cache
	func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		d.mergeGraph(ir, changedFiles)
		d.cache = NewCache()
		d.cache.Build(d.ir)
		affected = d.computeAffectedFiles(changedFiles, oldImports, oldCalleeFiles)
		cacheSnapshot = d.cache
	}()

	d.logf("Re-rendering %d affected shards", len(affected))

	written, err := RenderAll(d.cfg.RepoDir, cacheSnapshot, affected, false)
	if err != nil {
		d.logf("Render error: %v", err)
		return
	}

	d.logf("Updated %d shards", written)

	var updateStats GraphStats
	func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		d.saveCache()
		updateStats = computeStats(d.ir, d.cache)
	}()

	d.writeStatus(fmt.Sprintf("ready — %s — %d files",
		time.Now().Format(time.RFC3339), updateStats.SourceFiles))

	if d.cfg.OnUpdate != nil {
		d.cfg.OnUpdate(updateStats)
	}
}

// saveCache writes the current merged ShardIR to the cache file. Must be called with d.mu held.
func (d *Daemon) saveCache() {
	if d.ir == nil {
		return
	}
	// Ensure cache directory exists
	if err := os.MkdirAll(filepath.Dir(d.cfg.CacheFile), 0o755); err != nil {
		d.logf("Error creating cache directory: %v", err)
		return
	}
	cacheJSON, err := json.MarshalIndent(d.ir, "", "  ")
	if err != nil {
		d.logf("Error marshaling cache: %v", err)
		return
	}
	tmp := d.cfg.CacheFile + ".tmp"
	if err := os.WriteFile(tmp, cacheJSON, 0o644); err != nil {
		d.logf("Error writing cache: %v", err)
		return
	}
	if err := os.Rename(tmp, d.cfg.CacheFile); err != nil {
		_ = os.Remove(tmp)
		d.logf("Error renaming cache: %v", err)
		return
	}
	d.logf("Saved merged cache (%d nodes, %d relationships)",
		len(d.ir.Graph.Nodes), len(d.ir.Graph.Relationships))
}

// mergeGraph integrates incremental API results into the existing ShardIR.
func (d *Daemon) mergeGraph(incremental *api.ShardIR, changedFiles []string) { //nolint:gocyclo // graph merge has inherent branching per node/rel type; splitting would obscure the algorithm
	if d.ir == nil {
		d.ir = incremental
		return
	}

	changedSet := make(map[string]bool, len(changedFiles))
	for _, f := range changedFiles {
		changedSet[f] = true
	}

	newNodeIDs := make(map[string]bool)
	for _, n := range incremental.Graph.Nodes {
		newNodeIDs[n.ID] = true
	}

	existingFileByPath := make(map[string]string)
	var existingFilePaths []struct {
		path string
		id   string
	}

	for _, n := range d.ir.Graph.Nodes {
		if n.HasLabel("File") {
			fp := n.Prop("filePath")
			if fp != "" {
				existingFileByPath[fp] = n.ID
				existingFilePaths = append(existingFilePaths, struct {
					path string
					id   string
				}{fp, n.ID})
			}
		}
	}

	extRemap := make(map[string]string)
	resolvedSet := make(map[string]bool)

	for _, n := range incremental.Graph.Nodes {
		if !n.HasLabel("LocalDependency") && !n.HasLabel("ExternalDependency") {
			continue
		}
		fp := n.Prop("filePath")
		if fp == "" {
			fp = n.Prop("name")
		}
		if fp == "" {
			fp = n.Prop("importPath")
		}
		if fp == "" {
			continue
		}

		if existing, ok := existingFileByPath[fp]; ok {
			extRemap[n.ID] = existing
			resolvedSet[n.ID] = true
			continue
		}

		importPath := fp
		if strings.HasPrefix(importPath, "@/") {
			importPath = importPath[2:]
		} else if strings.HasPrefix(importPath, "~/") {
			importPath = importPath[2:]
		}

		suffixes := []string{
			"/" + importPath + ".ts",
			"/" + importPath + ".tsx",
			"/" + importPath + ".js",
			"/" + importPath + ".jsx",
			"/" + importPath + "/index.ts",
			"/" + importPath + "/index.js",
			"/" + importPath + "/index.tsx",
			"/" + importPath + ".go",
			"/" + importPath + ".py",
			"/" + importPath + ".rs",
		}
		for _, ef := range existingFilePaths {
			matched := false
			for _, suffix := range suffixes {
				if strings.HasSuffix(ef.path, suffix) {
					matched = true
					break
				}
			}
			if matched {
				extRemap[n.ID] = ef.id
				resolvedSet[n.ID] = true
				break
			}
		}
	}

	incFileByPath := make(map[string]string)
	incFnByKey := make(map[string]string)
	for _, n := range incremental.Graph.Nodes {
		fp := n.Prop("filePath")
		if n.HasLabel("File") && fp != "" {
			incFileByPath[fp] = n.ID
		} else if n.HasLabel("Function") && fp != "" {
			name := n.Prop("name")
			if name != "" {
				incFnByKey[fp+":"+name] = n.ID
			}
		}
	}

	oldToNew := make(map[string]string)
	for _, n := range d.ir.Graph.Nodes {
		fp := n.Prop("filePath")
		if fp == "" {
			continue
		}
		if n.HasLabel("File") {
			if newID, ok := incFileByPath[fp]; ok && newID != n.ID {
				oldToNew[n.ID] = newID
			}
		} else if n.HasLabel("Function") {
			name := n.Prop("name")
			key := fp + ":" + name
			if newID, ok := incFnByKey[key]; ok && newID != n.ID {
				oldToNew[n.ID] = newID
			}
		}
	}

	var keptNodes []api.Node
	for _, n := range d.ir.Graph.Nodes {
		fp := n.Prop("filePath")
		if fp == "" {
			fp = n.Prop("path")
		}
		if changedSet[fp] || changedSet[n.ID] {
			continue
		}
		if newNodeIDs[n.ID] {
			continue
		}
		keptNodes = append(keptNodes, n)
	}

	// Build a set of node IDs that are still present in the graph (either kept as-is
	// or remapped to a new ID). Any relationship referencing a node outside this set
	// belongs to a deleted file and must be pruned.
	keptNodeIDs := make(map[string]bool, len(keptNodes))
	for _, n := range keptNodes {
		keptNodeIDs[n.ID] = true
	}

	replacedOldIDs := make(map[string]bool, len(oldToNew))
	for oldID := range oldToNew {
		replacedOldIDs[oldID] = true
	}

	var keptRels []api.Relationship
	for _, r := range d.ir.Graph.Relationships {
		// Prune relationships whose start or end node was deleted (not kept, not remapped).
		if !keptNodeIDs[r.StartNode] && !replacedOldIDs[r.StartNode] {
			continue
		}
		if !keptNodeIDs[r.EndNode] && !replacedOldIDs[r.EndNode] {
			continue
		}
		// Outgoing rels from replaced/changed nodes are superseded by newRels from the
		// incremental graph; skip them here.
		if replacedOldIDs[r.StartNode] {
			continue
		}

		rel := r
		if newID, ok := oldToNew[rel.EndNode]; ok {
			rel.EndNode = newID
		}
		keptRels = append(keptRels, rel)
	}

	var newNodes []api.Node
	for _, n := range incremental.Graph.Nodes {
		if resolvedSet[n.ID] {
			continue
		}
		newNodes = append(newNodes, n)
	}

	var newRels []api.Relationship
	for _, r := range incremental.Graph.Relationships {
		rel := r
		if mapped, ok := extRemap[rel.StartNode]; ok {
			rel.StartNode = mapped
		}
		if mapped, ok := extRemap[rel.EndNode]; ok {
			rel.EndNode = mapped
		}
		newRels = append(newRels, rel)
	}

	keptNodes = append(keptNodes, newNodes...)
	d.ir.Graph.Nodes = keptNodes
	keptRels = append(keptRels, newRels...)
	d.ir.Graph.Relationships = keptRels

	// Preserve domains from the last full generate. Incremental responses
	// contain domains classified from only the changed files, which are
	// incorrect for the repo as a whole. Domains only refresh on full generate.

	// Assign new files to existing domains by directory-prefix matching.
	d.assignNewFilesToDomains(newNodes)

	if len(extRemap) > 0 {
		d.logf("Resolved %d external references to internal nodes", len(extRemap))
	}
}

// assignNewFilesToDomains assigns newly merged File nodes to the best-matching
// existing domain using longest common directory-prefix heuristic.
func (d *Daemon) assignNewFilesToDomains(newNodes []api.Node) {
	if len(d.ir.Domains) == 0 {
		return
	}

	for _, n := range newNodes {
		if !n.HasLabel("File") {
			continue
		}
		fp := n.Prop("filePath")
		if fp == "" {
			continue
		}
		dir := filepath.Dir(fp)

		bestDomain := -1
		bestLen := -1
		for i, domain := range d.ir.Domains {
			for _, kf := range domain.KeyFiles {
				prefix := filepath.Dir(kf)
				if strings.HasPrefix(dir+"/", prefix+"/") && len(prefix) > bestLen {
					bestLen = len(prefix)
					bestDomain = i
				}
			}
		}
		if bestDomain >= 0 {
			d.ir.Domains[bestDomain].KeyFiles = append(d.ir.Domains[bestDomain].KeyFiles, fp)
		}
	}
}

// computeAffectedFiles returns changed files plus their 1-hop dependents.
// oldImports maps each changed file to the set of files it imported BEFORE
// the merge, so that files whose "imported-by" sections lost a reference
// are also re-rendered.
// oldCalleeFiles maps each function ID (in a changed file) to the set of files
// its callees lived in BEFORE the merge, so that callee files whose
// "← caller" entries were removed are also re-rendered.
func (d *Daemon) computeAffectedFiles(changedFiles []string, oldImports, oldCalleeFiles map[string][]string) []string {
	affected := make(map[string]bool)

	for _, f := range changedFiles {
		affected[f] = true

		// Files that currently import f: their [deps] imported-by section references f.
		for _, imp := range d.cache.Importers[f] {
			affected[imp] = true
		}

		// Files that f currently imports: their [deps] imported-by section needs to
		// include f (newly added imports gain an "imported-by" entry).
		for _, dep := range d.cache.Imports[f] {
			affected[dep] = true
		}

		// Files that f used to import but no longer does: their "imported-by" section
		// needs to drop f. These are absent from the post-merge cache so we use the
		// snapshot taken before mergeGraph was called.
		for _, dep := range oldImports[f] {
			affected[dep] = true
		}

		for id, fn := range d.cache.FnByID {
			if fn.File != f {
				continue
			}
			for _, caller := range d.cache.Callers[id] {
				if caller.File != "" {
					affected[caller.File] = true
				}
			}
			// Files that functions in f used to call (old callees): if the call was
			// removed, those files' shards still show "funcB ← funcA" and need
			// re-rendering to drop the stale back-reference.
			for _, calleeFile := range oldCalleeFiles[id] {
				affected[calleeFile] = true
			}
		}
	}

	return daemonSortedKeys(affected)
}

func (d *Daemon) listenUDP(ctx context.Context, ready chan<- error) {
	addr := fmt.Sprintf("127.0.0.1:%d", d.cfg.NotifyPort)
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		ready <- err
		return
	}
	defer conn.Close()
	ready <- nil
	d.logf("UDP listener on %s", addr)

	go func() {
		<-ctx.Done()
		_ = conn.SetReadDeadline(time.Now())
	}()

	buf := make([]byte, 4096)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		filePath := strings.TrimSpace(string(buf[:n]))
		if filePath != "" {
			d.logf("UDP trigger: %s", filePath)
			select {
			case d.notifyCh <- filePath:
			default:
				d.logf("Notify channel full, dropping: %s", filePath)
			}
		}
	}
}

func (d *Daemon) writeStatus(status string) {
	path := filepath.Join(d.cfg.RepoDir, ".supermodel", "status")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	if err := os.WriteFile(path, []byte(status+"\n"), 0o644); err != nil {
		d.logf("Warning: could not write status file %s: %v", path, err)
	}
}

func (d *Daemon) forwardWatcherEvents(w *Watcher) {
	for events := range w.Events() {
		for _, ev := range events {
			select {
			case d.notifyCh <- ev.Path:
			default:
				d.logf("Notify channel full, dropping: %s", ev.Path)
			}
		}
	}
}

func daemonSortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand unavailable: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
