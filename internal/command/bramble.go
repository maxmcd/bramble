package command

import (
	build "github.com/maxmcd/bramble/internal/build"
	project "github.com/maxmcd/bramble/internal/project"
)

type bramble struct {
	store   *build.Store
	project *project.Project
}

func newBramble(wd string, bramblePath string) (b bramble, err error) {
	if b.project, err = project.NewProject(wd); err != nil {
		return
	}
	if b.store, err = build.NewStore(bramblePath); err != nil {
		return
	}

	return b, nil
}
