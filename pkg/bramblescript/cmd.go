package bramblescript

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/pkg/errors"

	"go.starlark.net/starlark"
)

type Cmd struct {
	exec.Cmd
	frozen   bool
	finished bool
	err      error
	out      io.ReadCloser

	wg      *sync.WaitGroup
	reading bool
}

var (
	_ starlark.Value    = new(Cmd)
	_ starlark.HasAttrs = new(Cmd)
)

func (cmd *Cmd) name() string {
	if cmd == nil || len(cmd.Args) == 0 {
		return ""
	}
	return cmd.Args[0]
}

func (cmd *Cmd) String() string {
	s := fmt.Sprintf
	var sb strings.Builder
	sb.WriteString("<cmd")
	sb.WriteString(s(" '%s'", cmd.name()))
	if len(cmd.Args) > 1 {
		sb.WriteString(" ['")
		sb.WriteString(strings.Join(cmd.Args[1:], `', '`))
		sb.WriteString("']")
	}
	sb.WriteString(">")
	return sb.String()
}
func (cmd *Cmd) Freeze() {
	// TODO: don't implement functionality that does nothing
	if cmd != nil {
		cmd.frozen = true
	}
}
func (cmd *Cmd) Type() string          { return "cmd" }
func (cmd *Cmd) Truth() starlark.Bool  { return cmd != nil }
func (cmd *Cmd) Hash() (uint32, error) { return 0, errors.New("cmd is unhashable") }

func (cmd *Cmd) Attr(name string) (val starlark.Value, err error) {
	switch name {
	case "stdout":
		return ByteStream{stdout: true, cmd: cmd}, nil
	case "stderr":
		return ByteStream{stderr: true, cmd: cmd}, nil
	case "combined_output":
		return ByteStream{stdout: true, stderr: true, cmd: cmd}, nil
	case "if_err":
		return IfErr{cmd: cmd}, nil
	case "pipe":
		return Pipe{cmd: cmd}, nil
	case "kill":
		return Callable{ThisName: "kill", ParentName: "cmd", Callable: cmd.Kill}, nil
	case "wait":
		return Callable{ThisName: "wait", ParentName: "cmd", Callable: cmd.starlarkWait}, nil
	}
	return nil, nil
}
func (cmd *Cmd) AttrNames() []string {
	return []string{"stdout", "stderr", "combined_output", "if_err", "pipe", "kill", "wait"}
}

func (cmd *Cmd) Wait() error {
	cmd.wg.Wait()
	return cmd.err
}

func (cmd *Cmd) addArgumentToCmd(value starlark.Value) (err error) {
	val, err := valueToString(value)
	if err != nil {
		return
	}
	cmd.Args = append(cmd.Args, val)
	return
}

func (cmd *Cmd) starlarkWait(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	return starlark.None, cmd.Wait()
}

func (cmd *Cmd) Kill(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	if err = starlark.UnpackArgs("kill", args, kwargs); err != nil {
		return
	}
	val = starlark.None
	if cmd.finished {
		return
	}
	if cmd.Process != nil {
		if err = cmd.Process.Kill(); err != nil {
			return
		}
	}
	return
}
