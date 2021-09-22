package bramble

import (
	build "github.com/maxmcd/bramble/pkg/bramblebuild"
	project "github.com/maxmcd/bramble/pkg/brambleproject"
)

type bramble struct {
	store   *build.Store
	project *project.Project
}

func newBramble() (b bramble, err error) {
	if b.project, err = project.NewProject("."); err != nil {
		return
	}
	if b.store, err = build.NewStore(""); err != nil {
		return
	}

	b.store.RegisterGetGit(b.runGit)
	return b, nil
}
