package bramblescript

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"
)

// IfErr is a Value and Callable that is returned when calling cmd("foo").if_err
type IfErr struct {
	cmd *Cmd
}

var (
	_ starlark.Value    = IfErr{}
	_ starlark.Callable = IfErr{}
)

func (ie IfErr) String() string {
	return fmt.Sprintf("<built-in method %s of cmd object>", ie.Name())
}
func (ie IfErr) Type() string          { return ie.Name() }
func (ie IfErr) Freeze()               { /*TODO*/ }
func (ie IfErr) Truth() starlark.Bool  { return ie.cmd.Truth() }
func (ie IfErr) Hash() (uint32, error) { return 0, errors.New("if_err is unhashable") }
func (ie IfErr) Name() string          { return "if_err" }

func (ie IfErr) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	// If there's no error we ignore the 'or' call
	if err := ie.cmd.Wait(); err == nil {
		return ie.cmd, nil
	}

	// if there is an error we run the command in or instead
	return newCmd(args, kwargs, nil, ie.cmd.Dir)
}
