package focus

import (
	"github.com/supermodeltools/cli/internal/api"
	"github.com/supermodeltools/cli/internal/archive"
	"github.com/supermodeltools/cli/internal/config"
)

func createZip(dir string) (string, error) {
	return archive.CreateZip(dir)
}

func newAPIClient(cfg *config.Config) *api.Client {
	return api.New(cfg)
}
