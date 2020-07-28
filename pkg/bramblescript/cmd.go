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

func (c *Cmd) name() string {
	if len(c.Args) == 0 {
		return ""
	}
	return c.Args[0]
}

func (c *Cmd) String() string {
	s := fmt.Sprintf
	var sb strings.Builder
	sb.WriteString("<cmd")
	sb.WriteString(s(" '%s'", c.name()))
	if len(c.Args) > 1 {
		sb.WriteString(" ['")
		sb.WriteString(strings.Join(c.Args[1:], `', '`))
		sb.WriteString("']")
	}
	sb.WriteString(">")
	return sb.String()
}
func (c *Cmd) Type() string          { return "cmd" }
func (c *Cmd) Freeze()               { c.frozen = true }
func (c *Cmd) Truth() starlark.Bool  { return c != nil }
func (c *Cmd) Hash() (uint32, error) { return 0, errors.New("cmd is unhashable") }

func (c *Cmd) Attr(name string) (val starlark.Value, err error) {
	switch name {
	case "stdout":
		return ByteStream{stdout: true, cmd: c}, nil
	case "stderr":
		return ByteStream{stderr: true, cmd: c}, nil
	case "combined_output":
		return ByteStream{stdout: true, stderr: true, cmd: c}, nil
	case "if_err":
		return IfErr{cmd: c}, nil
	case "pipe":
		return Pipe{cmd: c}, nil
	}
	return nil, nil
}
func (c *Cmd) AttrNames() []string {
	return []string{"stdout", "stderr", "combined_output", "if_err", "pipe", "stdin"}
}

func (c *Cmd) addArgumentToCmd(value starlark.Value) (err error) {
	var stringValue string
	switch v := value.(type) {
	case starlark.String:
		stringValue = v.GoString()
	case starlark.Int:
		stringValue = v.String()
	default:
		return errors.Errorf("don't know how to cast type %q into a command argument", v.Type())
	}

	c.Args = append(c.Args, stringValue)
	return nil
}

// NewCmd creates a new cmd instance given args and kwargs. NewCmd will error
// immediately if it can't find the cmd
func NewCmd(args starlark.Tuple, kwargsList []starlark.Tuple, stdin *io.Reader) (v *Cmd, err error) {
	// if input is an array we use the first item as the cmd
	// if input is just args we use them as cmd+args
	// if input is just a string we parse it as a shell command

	cmd := Cmd{}
	// TODO: might want CommandContext

	kwargs := map[string]starlark.Value{}
	for _, kwarg := range kwargsList {
		kwargs[kwarg.Index(0).(*starlark.String).GoString()] = kwarg.Index(1)
	}

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
		cmd.err = cmd.Wait()
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
