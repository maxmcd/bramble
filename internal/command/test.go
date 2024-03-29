package command

import (
	"context"
	"fmt"

	"github.com/maxmcd/bramble/internal/project"
	"github.com/maxmcd/bramble/internal/store"
)

func (b bramble) test(ctx context.Context) (err error) {
	output, err := b.execModule(ctx, nil, execModuleOptions{
		includeTests: true,
	})
	if err != nil {
		return
	}

	type buildOutput struct {
		dep      project.Dependency
		drv      project.Derivation
		buildDrv store.Derivation
	}

	finishedBuild := make(chan buildOutput, 100)
	errChan := make(chan error)
	go func() {
		if _, err := b.runBuild(ctx, output, runBuildOptions{
			callback: func(dep project.Dependency, drv project.Derivation, buildDrv store.Derivation) {
				finishedBuild <- buildOutput{
					dep:      dep,
					drv:      drv,
					buildDrv: buildDrv,
				}
			},
		}); err != nil {
			errChan <- err
		}
	}()
	for {
		select {
		case err := <-errChan:
			return err
		case bo := <-finishedBuild:
			fmt.Println(output.Tests)
			// TODO: ensure tests aren't run twice
			for _, test := range output.Tests[bo.dep.Hash] {
				return b.store.RunDerivation(ctx, bo.buildDrv, store.RunDerivationOptions{
					Args: test.Args,
				})
			}
		}
	}
}
