package command

import (
	"github.com/maxmcd/bramble/internal/project"
	"github.com/maxmcd/bramble/internal/store"
)

type bramble struct {
	store   *store.Store
	project *project.Project
}

func newBramble(wd string, bramblePath string) (b bramble, err error) {
	if b.project, err = project.NewProject(wd); err != nil {
		return
	}
	if b.store, err = store.NewStore(bramblePath); err != nil {
		return
	}

	return b, nil
}
