package command

import (
	"path/filepath"

	"github.com/maxmcd/bramble/internal/dependency"
	"github.com/maxmcd/bramble/internal/project"
	"github.com/maxmcd/bramble/internal/store"
)

type bramble struct {
	store   *store.Store
	project *project.Project
}

func newBramble(wd string, bramblePath string) (b bramble, err error) {
	if b.store, err = store.NewStore(bramblePath); err != nil {
		return
	}
	if b.project, err = project.NewProject(wd); err != nil {
		return
	}

	b.project.AddModuleFetcher(
		dependency.NewManager(
			filepath.Join(b.store.BramblePath, "var/dependencies"),
			"https://store.bramble.run",
			nil,
		),
	)
	return b, nil
}
