package bramblescript

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

// CmdClient is the value for the builtin "cmd", calling it as a function
// creates a new cmd instance, it also has various other attributes and methods
type CmdClient struct {
	dir string
}

var (
	_ starlark.Value    = new(CmdClient)
	_ starlark.Callable = new(CmdClient)
	_ starlark.HasAttrs = new(CmdClient)
)

func NewClient(dir string) *CmdClient {
	dir, err := filepath.Abs(dir)
	if err != nil {
		// TODO
		panic(err)
	}
	return &CmdClient{dir: dir}
}

func (client *CmdClient) Type() string         { return "builtin_function_cmd" }
func (client *CmdClient) Truth() starlark.Bool { return starlark.True }
func (client *CmdClient) Name() string         { return client.Type() }
func (client *CmdClient) String() string       { return "<built-in function cmd>" }
func (client *CmdClient) Freeze()              {}
func (client *CmdClient) AttrNames() []string  { return []string{"cd", "debug"} }

func (client *CmdClient) Hash() (uint32, error) {
	return 0, errors.Errorf("%s is unhashable", client.Type())
}

func (client *CmdClient) Attr(name string) (val starlark.Value, err error) {
	switch name {
	case "cd":
		return Callable{ThisName: "cmd", ParentName: "cmd", Callable: client.CD}, nil
	case "debug":
		return Callable{ThisName: "debug", ParentName: "debug", Callable: client.Debug}, nil
	}
	return nil, nil
}

func (client *CmdClient) CD(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	var dir starlark.String
	if err = starlark.UnpackArgs("cmd", args, kwargs, "dir", &dir); err != nil {
		return
	}
	client.dir = filepath.Join(client.dir, dir.GoString())
	return dir, nil
}
func (client *CmdClient) Debug(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (val starlark.Value, err error) {
	logger.SetOutput(os.Stdout)
	return starlark.None, nil
}

// CallInternal defines the cmd() starlark function.
func (client *CmdClient) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (v starlark.Value, err error) {
	return newCmd(thread, args, kwargs, nil, client.dir)
}
