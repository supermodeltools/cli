package loader

import (
	"github.com/supermodeltools/cli/internal/archdocs/pssg/config"
	"github.com/supermodeltools/cli/internal/archdocs/pssg/entity"
)

// Loader is the interface for loading entities from data files.
type Loader interface {
	Load() ([]*entity.Entity, error)
}

// New creates a loader based on the config data format.
func New(cfg *config.Config) Loader {
	switch cfg.Data.Format {
	case "markdown":
		return &MarkdownLoader{Config: cfg}
	default:
		return &MarkdownLoader{Config: cfg}
	}
}
