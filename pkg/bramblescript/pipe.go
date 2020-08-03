package bramblescript

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"
)

// Pipe is a Value and Callable that is returned when calling cmd("foo").pipe
type Pipe struct {
	cmd *Cmd
}

var (
	_ starlark.Value    = Pipe{}
	_ starlark.Callable = Pipe{}
)

func (pipe Pipe) String() string {
	return fmt.Sprintf("<built-in method %s of cmd object>", pipe.Name())
}
func (pipe Pipe) Name() string          { return "pipe" }
func (pipe Pipe) Type() string          { return pipe.Name() }
func (pipe Pipe) Freeze()               { /*TODO*/ }
func (pipe Pipe) Truth() starlark.Bool  { return pipe.cmd.Truth() }
func (pipe Pipe) Hash() (uint32, error) { return 0, errors.New("bytestream is unhashable") }
func (pipe Pipe) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	reader, err := pipe.cmd.Reader(true, false)
	if err != nil {
		return
	}
	// set the input of this command to the output of the previous command
	return newCmd(args, kwargs, &reader, pipe.cmd.Dir)
}
