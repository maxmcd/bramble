// Ever been to a playground? It's pretty easy to step in and out of a sandbox.
package sandbox

import (
	"context"
	"io"
	"strings"

	_ "github.com/opencontainers/runc/libcontainer/nsenter"
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
	Path   string
	Args   []string

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
}

// Run runs the sandbox until execution has been completed
func (s Sandbox) Run(ctx context.Context) (err error) {
	container, err := newContainer(s)
	if err != nil {
		return err
	}
	errChan := make(chan error)
	go func() {
		if err := container.Run(); err != nil {
			errChan <- errors.Wrap(err, "error running sandbox")
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
		return err
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

// func (s Sandbox) newNamespaceStep() (err error) {
// 	selfExe, err := os.Readlink("/proc/self/exe")
// 	if err != nil {
// 		return err
// 	}
// 	defer func() {
// 		logger.Debugw("clean up chrootDir", "path", s.ChrootPath)
// 		if er := os.RemoveAll(s.ChrootPath); er != nil {
// 			logger.Debugw("error cleaning up", "err", er)
// 			if err == nil {
// 				err = errors.Wrap(er, "error removing all files in "+s.ChrootPath)
// 			}
// 		}
// 	}()
// 	serialized, err := s.serializeArg()
// 	if err != nil {
// 		return err
// 	}

// 	var cloneFlags uintptr = syscall.CLONE_NEWUTS |
// 		syscall.CLONE_NEWNS |
// 		syscall.CLONE_NEWPID

// 	if s.DisableNetwork {
// 		cloneFlags |= syscall.CLONE_NEWNET
// 	}

// 	// interrupt will be caught be the child process and the process will
// 	// exiting, causing this process to exit
// 	ignoreInterrupt()

// 	cmd := &exec.Cmd{
// 		Path: selfExe,
// 		Args: []string{setupStepArg, serialized},
// 		SysProcAttr: &syscall.SysProcAttr{
// 			// maybe sigint will allow the child more time to clean up its mounts????
// 			Pdeathsig:  unix.SIGINT,
// 			Cloneflags: cloneFlags,
// 		},
// 	}

// 	// We must use a pty here to enable interactive input. If we naively pass
// 	// os.Stdin to an exec.Cmd then we run into issues with the parent and child
// 	// terminals getting confused about who is supposed to process various
// 	// control signals.
// 	//
// 	// We can then just set to raw and copy the bytes across. We could remove
// 	// the pty entirely for jobs that don't pass a terminal as a stdin.
// 	ptmx, err := pty.Start(cmd)
// 	if err != nil {
// 		return errors.Wrap(err, "error starting pty")
// 	}
// 	defer func() { _ = ptmx.Close() }()

// 	// only handle stdin and set raw if it's an interactive terminal
// 	if os.Stdin != nil && term.IsTerminal(os.Stdin.Fd()) {
// 		// Handle pty resize
// 		ch := make(chan os.Signal, 1)
// 		signal.Notify(ch, syscall.SIGWINCH)
// 		go func() {
// 			for range ch {
// 				if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
// 					log.Printf("error resizing pty: %s", err)
// 				}
// 			}
// 		}()
// 		ch <- syscall.SIGWINCH // Initial resize.
// 		oldState, err := term.MakeRaw(os.Stdin.Fd())
// 		if err != nil {
// 			return err
// 		}
// 		// restore when complete
// 		defer func() { _ = term.RestoreTerminal(os.Stdin.Fd(), oldState) }()
// 		go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
// 	}
// 	_, _ = io.Copy(os.Stdout, ptmx)
// 	return errors.Wrap(cmd.Wait(), "error running newNamespace")
// }
