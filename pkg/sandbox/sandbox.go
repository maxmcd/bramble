// Ever been to a playground? It's pretty easy to step in and out of a sandbox.
package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/maxmcd/bramble/pkg/logger"
	"github.com/pkg/errors"
)

const (
	execStepArg = "exec"
)

// Entrypoint must be run at the beginning of your executable. When the sandbox
// runs it re-runs the same binary with various arguments to indicate that we
// want the process to be run as a sandbox. If this function detects that it
// is needed it will run what it needs and then os.Exit the process, otherwise
// it will be a no-op.
func Entrypoint() {
	if !firstArgMatchesStep() {
		return
	}
	if err := entrypoint(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}

// Sandbox defines a command or function that you want to run in a sandbox
type Sandbox struct {
	Stdin      io.Reader `json:"-"`
	Stdout     io.Writer `json:"-"`
	Stderr     io.Writer `json:"-"`
	ChrootPath string
	Path       string
	Args       []string
	Dir        string
	Env        []string

	UserID  int
	GroupID int

	// Bind mounts or directories the process should have access too. These
	// should be absolute paths. If a mount is intended to be readonly add
	// ":ro" to the end of the path like `/tmp:ro`
	Mounts []string
	// DisableNetwork will remove network access within the sandbox process
	DisableNetwork bool
	// SetUIDBinary can be used if you want the parent process to call out
	// first to a different binary
	SetUIDBinary string // TODO
}

func (s Sandbox) serializeArg() (string, error) {
	byt, err := json.Marshal(s)
	return string(byt), err
}

func parseSerializedArg(arg string) (s Sandbox, err error) {
	return s, json.Unmarshal([]byte(arg), &s)
}

// Run runs the sandbox until execution has been completed
func (s Sandbox) Run(ctx context.Context) (err error) {
	serialized, err := s.serializeArg()
	if err != nil {
		return err
	}
	// TODO: allow reference to self
	// TODO: figure out what ^ means
	path, err := runExecPath()
	if err != nil {
		return err
	}
	logger.Debugw("newSanbox", "execpath", path)
	// interrupt will be caught be the child process and the process
	// will exiting, causing this process to exit
	ignoreInterrupt()
	cmd := &exec.Cmd{
		Path:   path,
		Args:   runFirstArgs(),
		Stdin:  s.Stdin,
		Stdout: s.Stdout,
		Stderr: s.Stderr,
	}
	errChan := make(chan error)
	go func() {
		if err := cmd.Run(); err != nil {
			errChan <- fmt.Errorf("error running newSandbox - %w", err)
		}
		close(errChan)
	}()
	select {
	case <-ctx.Done():
		if cmd.Process != nil {
			if err := cmd.Process.Signal(os.Interrupt); err != nil {
				return err
			}
		}
		// TODO: do this for all of them? Stop ignoring the interrupt in the children?
	case err = <-errChan:
		if err == nil && cmd.ProcessState != nil && cmd.ProcessState.ExitCode() != 0 {
			return errors.New("ah!a")
		}
		return err
	}
	return nil // Start the command with a pty.
}

func ignoreInterrupt() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		for {
			<-c
		}
	}()
}

func interruptContext() context.Context {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-c
		cancel()
	}()
	return ctx
}

func (s Sandbox) runExecStep() {
	cmd := exec.Cmd{
		Path: s.Path,
		Dir:  s.Dir,
		Args: append([]string{s.Path}, s.Args...),
		Env:  os.Environ(),

		// We don't use the passed sandbox stdio because
		// it's been passed to the very first run command
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	if err := cmd.Run(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
