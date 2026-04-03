package archdocs

import "github.com/supermodeltools/cli/internal/archive"

func createZip(dir string) (string, error) {
	return archive.CreateZip(dir)
}
