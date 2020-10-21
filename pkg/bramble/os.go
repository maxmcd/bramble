package bramble

import (
	"bufio"
	"fmt"
	goos "os"
	"path/filepath"

	"github.com/maxmcd/bramble/pkg/assert"
	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

type OS struct {
	bramble *Bramble

	cmdFunction *CmdFunction
}

var (
	_ starlark.Value    = OS{}
	_ starlark.HasAttrs = OS{}
)

func NewOS(bramble *Bramble, cmdFunction *CmdFunction) OS {
	return OS{bramble: bramble, cmdFunction: cmdFunction}
}

func (os OS) String() string        { return "<module 'os'>" }
func (os OS) Freeze()               {}
func (os OS) Type() string          { return "module" }
func (os OS) Truth() starlark.Bool  { return starlark.True }
func (os OS) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("os") }
func (os OS) AttrNames() []string {
	return []string{
		"args",
		"cd",
		"cmd",
		"cp",
		"create",
		"error",
		"expand",
		"getenv",
		"input",
		"mkdir",
		"setenv",
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
	case "cp":
		return starutil.Callable{ThisName: "cp", ParentName: "os", Callable: os.cp}, nil
	case "cmd":
		return os.cmdFunction, nil
	case "create":
		return starutil.Callable{ThisName: "create", ParentName: "os", Callable: os.create}, nil
	case "error":
		return starutil.Callable{ThisName: "error", ParentName: "os", Callable: os.error}, nil
	case "input":
		return starutil.Callable{ThisName: "input", ParentName: "os", Callable: os.input}, nil
	case "mkdir":
		return starutil.Callable{ThisName: "mkdir", ParentName: "os", Callable: os.mkdir}, nil
	case "session":
		return starutil.Callable{ThisName: "session", ParentName: "os", Callable: os.session}, nil
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
	return starlark.None, goos.Mkdir(path.GoString(), 0755)
}

func (os OS) session(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	if err = starlark.UnpackArgs("session", args, kwargs); err != nil {
		return
	}
	return newSession("/", map[string]string{})
}

func (os OS) cp(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	if err = starlark.UnpackArgs("cp", nil, kwargs); err != nil {
		return
	}
	paths := make([]string, len(args))
	for i, arg := range args {
		str, err := starutil.ValueToString(arg)
		if err != nil {
			return nil, err
		}
		if !filepath.IsAbs(str) {
			return nil, errors.New("cp doesn't support relative paths yet")
		}
		paths[i] = str
	}
	err = cp("", paths...)
	fmt.Printf("%+v", err)
	return starlark.None, err
}

func (os OS) create(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	var path starlark.String
	if err = starlark.UnpackArgs("create", args, kwargs, "path", &path); err != nil {
		return
	}
	f, err := goos.Create(path.GoString())
	return File{file: f}, err
}

type File struct {
	name string
	file *goos.File
}

var (
	_ starlark.Value    = File{}
	_ starlark.HasAttrs = File{}
)

func (f File) String() string        { return fmt.Sprintf("<file '%s'>", f.name) }
func (f File) Freeze()               {}
func (f File) Type() string          { return "file" }
func (f File) Truth() starlark.Bool  { return starlark.True }
func (f File) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("os") }
func (f File) AttrNames() []string {
	return []string{
		"write",
		"close",
	}
}

func (f File) Attr(name string) (val starlark.Value, err error) {
	switch name {
	case "write":
		return starutil.Callable{ThisName: "write", ParentName: "file", Callable: f.write}, nil
	case "close":
		return starutil.Callable{ThisName: "close", ParentName: "file", Callable: f.close}, nil
	}
	return nil, nil
}

func (f File) close(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	if err = starlark.UnpackArgs("close", args, kwargs); err != nil {
		return
	}
	return starlark.None, f.file.Close()
}

func (f File) write(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	var content starlark.Value
	if err = starlark.UnpackArgs("write", args, kwargs, "content", &content); err != nil {
		return
	}
	s, err := starutil.ValueToString(content)
	if err != nil {
		return
	}
	_, err = f.file.Write([]byte(s))
	return starlark.None, err
}
