package bramble

import (
	"bufio"
	goos "os"

	"github.com/maxmcd/bramble/pkg/assert"
	"github.com/maxmcd/bramble/pkg/starutil"
	"go.starlark.net/starlark"
)

type OS struct {
	bramble *Bramble
	session *session
}

var (
	_ starlark.Value    = OS{}
	_ starlark.HasAttrs = OS{}
)

func NewOS(bramble *Bramble, session *session) OS {
	return OS{bramble: bramble, session: session}
}

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
		"mkdir",
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
	os.bramble.AfterDerivation()
	switch name {
	case "args":
		return makeArgs()
	case "error":
		return starutil.Callable{ThisName: "error", ParentName: "os", Callable: os.error}, nil
	case "cd":
		return starutil.Callable{ThisName: "cd", ParentName: "os", Callable: os.cd}, nil
	case "getenv":
		return starutil.Callable{ThisName: "getenv", ParentName: "os", Callable: os.getenv}, nil
	case "setenv":
		return starutil.Callable{ThisName: "setenv", ParentName: "os", Callable: os.setenv}, nil
	case "input":
		return starutil.Callable{ThisName: "input", ParentName: "os", Callable: os.input}, nil
	case "mkdir":
		return starutil.Callable{ThisName: "mkdir", ParentName: "os", Callable: os.mkdir}, nil
	}
	return nil, nil
}

func (os OS) error(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	return assert.Error(thread, nil, args, kwargs)
}

func (os OS) input(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	reader := bufio.NewReader(goos.Stdin)
	text, err := reader.ReadString('\n')
	return starlark.String(text), err
}

func (os OS) mkdir(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	var path starlark.String
	if err = starlark.UnpackArgs("mkdir", args, kwargs, "path", &path); err != nil {
		return
	}
	return starlark.None, goos.Mkdir(os.session.expand(path.GoString()), 0755)
}

func (os OS) getenv(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	var key starlark.String
	if err = starlark.UnpackArgs("getenv", args, kwargs, "key", &key); err != nil {
		return
	}
	return starlark.String(os.session.getEnv(key.GoString())), nil
}

func (os OS) setenv(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	var key starlark.String
	var value starlark.String
	if err = starlark.UnpackArgs("setenv", args, kwargs, "key", &key, "value", &value); err != nil {
		return
	}
	os.session.setEnv(
		key.GoString(),
		value.GoString(),
	)
	return starlark.None, nil
}

func (os OS) cd(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	var path starlark.String
	if err = starlark.UnpackArgs("cd", args, kwargs, "path", &path); err != nil {
		return
	}
	return starlark.None, os.session.cd(os.session.expand(path.GoString()))
}
