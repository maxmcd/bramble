package command

import (
	"context"
	"errors"
	"os"

	"github.com/maxmcd/bramble/src/build"
)

func (b bramble) run(ctx context.Context, args []string) (err error) {
	output, err := b.runBuildFromCLI(ctx, "run", args, buildOptions{})
	if err != nil {
		return err
	}
	if len(output) != 1 {
		return errors.New("can't run a starlark function if it doesn't return a single derivation")
	}

	return b.store.RunDerivation(ctx, output[0], build.RunDerivationOptions{
		// Stdin:  io.MultiReader(os.Stdin),
		Stdin:  os.Stdin,
		Args:   args[1:],
		Dir:    b.project.WD(),
		Mounts: []string{b.project.Location()},

		HiddenPaths:   b.project.HiddenPaths(),
		ReadOnlyPaths: b.project.ReadOnlyPaths(),
	})
}
