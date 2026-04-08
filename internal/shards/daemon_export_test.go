package shards

import "github.com/supermodeltools/cli/internal/api"

// NewTestDaemon creates a daemon preloaded with an existing ShardIR for testing merge logic.
func NewTestDaemon(ir *api.ShardIR) *Daemon {
	return &Daemon{
		ir:   ir,
		logf: func(string, ...interface{}) {},
	}
}

// MergeGraph exposes mergeGraph for testing.
func (d *Daemon) MergeGraph(incremental *api.ShardIR, changedFiles []string) {
	d.mergeGraph(incremental, changedFiles)
}

// GetIR returns the current merged ShardIR.
func (d *Daemon) GetIR() *api.ShardIR {
	return d.ir
}
