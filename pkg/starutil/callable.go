package starutil

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"
)

type CallableFunc func(*starlark.Thread, starlark.Tuple, []starlark.Tuple) (starlark.Value, error)

type Callable struct {
	ParentName string
	ThisName   string
	Callable   CallableFunc
}

var (
	_ starlark.Callable = Callable{}
)

func (callable Callable) Name() string {
	return callable.ThisName
}
func (callable Callable) String() string {
	return fmt.Sprintf("<attribute '%s' of '%s'>", callable.Name(), callable.ParentName)
}

func (callable Callable) Type() string          { return "builtin_function_or_method" }
func (callable Callable) Freeze()               {}
func (callable Callable) Truth() starlark.Bool  { return true }
func (callable Callable) Hash() (uint32, error) { return 0, errors.New("unhashable") }
func (callable Callable) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	return callable.Callable(thread, args, kwargs)
}
