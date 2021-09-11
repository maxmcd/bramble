package bramble

import (
	"context"
	"errors"
	"os"

	"github.com/maxmcd/bramble/pkg/bramblebuild"
)

func (b bramble) run(args []string) (err error) {
	output, err := b.runBuildFromCLI("run", args)
	if err != nil {
		return err
	}
	if len(output) != 1 {
		return errors.New("can't run a starlark function if it doesn't return a single derivation")
	}

	return b.store.RunDerivation(context.Background(), output[0], bramblebuild.RunDerivationOptions{
		// Stdin:  io.MultiReader(os.Stdin),
		Stdin:  os.Stdin,
		Args:   args[1:],
		Dir:    b.project.WD(),
		Mounts: []string{b.project.Location()},
	})
}
