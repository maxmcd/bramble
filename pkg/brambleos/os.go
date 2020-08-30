package brambleos

import (
	"bufio"
	goos "os"

	"github.com/maxmcd/bramble/pkg/starutil"
	"go.starlark.net/starlark"
)

type OS struct{}

var (
	_ starlark.Value    = OS{}
	_ starlark.HasAttrs = OS{}
)

func (os OS) String() string        { return "<module 'os'>" }
func (os OS) Freeze()               {}
func (os OS) Type() string          { return "module" }
func (os OS) Truth() starlark.Bool  { return starlark.True }
func (os OS) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("os") }
func (os OS) AttrNames() []string {
	return []string{
		"args",
		"error",
		"input",
	}
}

func makeArgs() (starlark.Value, error) {
	out := []starlark.Value{}
	if len(goos.Args) >= 3 {
		for _, arg := range goos.Args[3:] {
			out = append(out, starlark.String(arg))
		}
	}
	return starlark.NewList(out), nil
}

func (os OS) Attr(name string) (val starlark.Value, err error) {
	switch name {
	case "args":
		return makeArgs()
	case "error":
		return starutil.Callable{ThisName: "error", ParentName: "os", Callable: os.error}, nil
	case "input":
		return starutil.Callable{ThisName: "input", ParentName: "os", Callable: os.input}, nil
	}
	return nil, nil
}

func (os OS) error(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	if err = starlark.UnpackArgs("args", args, kwargs, "val", &val); err != nil {
		return
	}
	panic(val) //TODO
}

func (os OS) input(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	reader := bufio.NewReader(goos.Stdin)
	text, err := reader.ReadString('\n')
	return starlark.String(text), err
}
