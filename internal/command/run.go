package command

import (
	"context"
	"errors"
	"os"

	"github.com/maxmcd/bramble/internal/store"
	project "github.com/maxmcd/bramble/internal/project"
)

type runOptions struct {
	paths         []string
	readOnlyPaths []string
	hiddenPaths   []string
	network       bool
}

func (b bramble) run(ctx context.Context, args []string, ro runOptions) (err error) {
	output, err := b.execModule(ctx, "run", args, execModuleOptions{})
	if err != nil {
		return
	}
	outputDerivations, err := b.runBuild(ctx, output, runBuildOptions{})
	if err != nil {
		return err
	}

	var run *project.Run
	if len(output.Run) > 1 {
		return errors.New("multiple run commands, not sure how to proceed")
	}
	if len(output.Run) == 1 {
		run = &output.Run[0]
	}

	if len(outputDerivations) != 1 {
		return errors.New("can't run a starlark function if it doesn't return a single derivation")
	}

	// use args after the module location
	args = args[1:]

	if run != nil {
		ro.paths = run.Paths
		ro.readOnlyPaths = run.ReadOnlyPaths
		ro.hiddenPaths = run.HiddenPaths
		ro.network = run.Network
		args = append([]string{run.Cmd}, run.Args...)
	}
	if len(ro.paths) == 0 {
		ro.paths = []string{b.project.Location()}
	}
	return b.store.RunDerivation(ctx, outputDerivations[0], store.RunDerivationOptions{
		Stdin: os.Stdin,
		Args:  args,
		Dir:   b.project.WD(),

		Network: ro.network,

		Mounts:        ro.paths,
		HiddenPaths:   ro.hiddenPaths,
		ReadOnlyPaths: ro.readOnlyPaths,
	})
}
