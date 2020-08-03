package bramblescript

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/errors"

	"github.com/kballard/go-shellquote"
	"github.com/moby/moby/pkg/stdcopy"
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

// NewCmd creates a new cmd instance given args and kwargs. NewCmd will error
// immediately if it can't find the cmd
func NewCmd(args starlark.Tuple, kwargsList []starlark.Tuple, stdin *io.Reader) (v *Cmd, err error) {
	// if input is an array we use the first item as the cmd
	// if input is just args we use them as cmd+args
	// if input is just a string we parse it as a shell command

	cmd := Cmd{}
	kwargs, err := kwargsToStringDict(kwargsList)
	if err != nil {
		return nil, err
	}
	// TODO
	_ = kwargs

	// cmd() isn't allowed
	if args.Len() == 0 {
		return nil, errors.New("cmd() missing 1 required positional argument")
	}

	// it's cmd(["grep", "-v"])
	if args.Len() == 1 {
		if args.Index(0).Type() == "list" {
			cmd.Args, err = starlarkListToListOfStrings(args.Index(0))
			if err != nil {
				return nil, err
			}
			if len(cmd.Args) == 0 {
				return nil, errors.New("if the first argument is a list it can't be empty")
			}
		} else if args.Index(0).Type() == "string" {
			starlarkCmd := args.Index(0).(starlark.String).GoString()
			if starlarkCmd == "" {
				return nil, errors.New("if the first argument is a string it can't be empty")
			}
			cmd.Args, err = shellquote.Split(starlarkCmd)
			if err != nil {
				return
			}
			if len(cmd.Args) == 0 {
				// whitespace bash characters will be removed by shellquote,
				// add them back for correct error message
				cmd.Args = []string{starlarkCmd}
			}
		}
	} else {
		iterator := args.Iterate()
		defer iterator.Done()
		var val starlark.Value
		for iterator.Next(&val) {
			if err := cmd.addArgumentToCmd(val); err != nil {
				return nil, err
			}
		}
	}

	// kwargs:
	// stdin
	// dir
	// env
	name := cmd.name()
	if filepath.Base(name) == name {
		var lp string
		if lp, err = exec.LookPath(name); err != nil {
			return nil, err
		}
		cmd.Path = lp
	}

	cmd.wg = &sync.WaitGroup{}
	cmd.wg.Add(1)

	buffPipe := newBufferedPipe(4096 * 16)
	buffPipe.cmd = &cmd
	cmd.out = buffPipe
	if stdin != nil {
		cmd.Stdin = *stdin
	}
	cmd.Stdout = stdcopy.NewStdWriter(buffPipe, stdcopy.Stdout)
	cmd.Stderr = stdcopy.NewStdWriter(buffPipe, stdcopy.Stderr)
	err = cmd.Start()
	go func() {
		cmd.err = cmd.Cmd.Wait()
		cmd.finished = true
		cmd.out.Close()
		cmd.wg.Done()
	}()
	return &cmd, err
}

// StarlarkCmd defines the cmd() starlark function.
func StarlarkCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargsList []starlark.Tuple) (v starlark.Value, err error) {
	return NewCmd(args, kwargsList, nil)
}
