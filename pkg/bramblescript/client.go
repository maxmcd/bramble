package bramblescript

import (
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/kballard/go-shellquote"
	"github.com/moby/moby/pkg/stdcopy"
	"go.starlark.net/starlark"
)

type Client struct {
	dir string
}

func NewClient(dir string) *Client {
	dir, err := filepath.Abs(dir)
	if err != nil {
		// TODO
		panic(err)
	}
	return &Client{dir: dir}
}

// NewCmd creates a new cmd instance given args and kwargs. NewCmd will error
// immediately if it can't find the cmd
func newCmd(args starlark.Tuple, kwargsList []starlark.Tuple, stdin *io.Reader, dir string) (v *Cmd, err error) {
	// if input is an array we use the first item as the cmd
	// if input is just args we use them as cmd+args
	// if input is just a string we parse it as a shell command

	cmd := Cmd{}
	cmd.Dir = dir
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
func (client Client) StarlarkCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargsList []starlark.Tuple) (v starlark.Value, err error) {
	return newCmd(args, kwargsList, nil, client.dir)
}
