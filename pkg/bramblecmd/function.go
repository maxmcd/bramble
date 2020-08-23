package bramblecmd

import (
	"github.com/maxmcd/bramble/pkg/starutil"
	"go.starlark.net/starlark"
)

// Function is the value for the builtin "cmd", calling it as a function
// creates a new cmd instance, it also has various other attributes and methods
type Function struct{}

var (
	_ starlark.Value    = new(Function)
	_ starlark.Callable = new(Function)
)

func NewFunction() *Function {
	return &Function{}
}

func (fn *Function) Freeze()               {}
func (fn *Function) Hash() (uint32, error) { return 0, starutil.ErrUnhashable(fn.Type()) }
func (fn *Function) Name() string          { return fn.Type() }
func (fn *Function) String() string        { return "<built-in function cmd>" }
func (fn *Function) Type() string          { return "builtin_function_cmd" }
func (fn *Function) Truth() starlark.Bool  { return true }

// CallInternal defines the cmd() starlark function.
func (fn *Function) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (v starlark.Value, err error) {
	return newCmd(thread, args, kwargs, nil, "")
}
