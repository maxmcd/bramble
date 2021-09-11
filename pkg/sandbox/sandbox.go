// Ever been to a playground? It's pretty easy to step in and out of a sandbox.
package sandbox

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"
)

const (
	initArg = "init"
)

var entrypoint func()

// Entrypoint must be run at the beginning of your executable. When the sandbox
// runs it re-runs the same binary with various arguments to indicate that we
// want the process to be run as a sandbox. If this function detects that it is
// needed it will run what it needs and then os.Exit the process, otherwise it
// will be a no-op.
func Entrypoint() {
	if entrypoint != nil {
		entrypoint()
	}
}

// Sandbox defines a command or function that you want to run in a sandbox
type Sandbox struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	Args []string

	// Dir specifies the working directory of the command. If Dir is the empty
	// string, Run runs the command in the calling process's current directory.
	Dir string

	// Env specifies the environment of the process. Each entry is of the form
	// "key=value".
	Env []string

	// Bind mounts or directories the process should have access too. These
	// should be absolute paths. If a mount is intended to be readonly add ":ro"
	// to the end of the path like `/tmp:ro`
	Mounts []string

	// DisableNetwork will remove network access within the sandbox process
	DisableNetwork bool

	ReadOnlyPaths []string
	HiddenPaths   []string
}

type ExitError struct {
	ExitCode int
}

func (ee ExitError) Error() string {
	return fmt.Sprintf("sandbox exited with code %d", ee.ExitCode)
}

// Run runs the sandbox until execution has been completed
func (s Sandbox) Run(ctx context.Context) (err error) {
	// fmt.Printf("%+v\n", s)
	container, err := newContainer(s)
	if err != nil {
		return err
	}
	errChan := make(chan error)
	go func() {
		if err := container.Run(); err != nil {
			errChan <- err
		}
		close(errChan)
	}()
	select {
	case <-ctx.Done():
		return combineErrors(
			container.Stop(),
			container.Destroy(),
		)
	case err = <-errChan:
		return errors.Wrap(err, "error running sandbox")
	}
}
func parseMount(mnt string) (src string, ro bool, valid bool) {
	parts := strings.Split(mnt, ":")
	switch len(parts) {
	case 1:
		return parts[0], false, true
	case 2:
		return parts[0], parts[1] == "ro", true
	}
	return "", false, false
}
