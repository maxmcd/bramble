package bramble

import (
	"fmt"
	"os"

	"github.com/maxmcd/tester"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

func Run() (err error) {
	thread := &starlark.Thread{Name: ""}
	globals, err := starlark.ExecFile(thread, os.Args[1], nil, starlark.StringDict{
		"load":  starlark.NewBuiltin("load", StarlarkLoad),
		"build": starlark.NewBuiltin("build", StarlarkBuild),
	})
	if err != nil {
		return
	}
	fmt.Println(globals)
	return
	// main := globals["main"]
	// _, err = starlark.Call(thread, main, starlark.Tuple{}, nil)
	// if err != nil {
	// 	if er, ok := err.(*starlark.EvalError); ok {
	// 		fmt.Println(er.Backtrace())
	// 	}
	// 	return
	// }
	// return
}

func StarlarkLoad(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (v starlark.Value, err error) {
	return
}

type Build struct {
	Name        string
	Outputs     []Output
	Builder     string
	Platform    string
	Args        []string
	Environment map[string]string
}

type Output struct {
	Name          string
	Path          string
	HashAlgorithm string
	Hash          string
}

type typeError struct {
	funcName   string
	argument   string
	wantedType string
}

func (te typeError) Error() string {
	return fmt.Sprintf("%s() keyword argument '%s' must be of type '%s'", te.funcName, te.argument, te.wantedType)
}

func newBuildFromKWArgs(kwargs []starlark.Tuple) (build Build, err error) {
	te := typeError{
		funcName: "build",
	}
	for _, kwarg := range kwargs {
		key := kwarg.Index(0).(starlark.String).GoString()
		value := kwarg.Index(1)
		switch key {
		case "name":
			name, ok := value.(starlark.String)
			if !ok {
				te.argument = "name"
				te.wantedType = "string"
				return build, te
			}
			build.Name = name.GoString()
		case "builder":
			name, ok := value.(starlark.String)
			if !ok {
				te.argument = "builder"
				te.wantedType = "string"
				return build, te
			}
			build.Builder = name.GoString()
		case "args":
		case "environment":
			build.Environment, err = valueToStringMap(value, "build", "environment")
			if err != nil {
				return
			}
		default:
			err = errors.Errorf("build() got an unexpected keyword argument '%s'", key)
			return
		}
	}
	return build, nil
}

func valueToStringMap(val starlark.Value, function, param string) (out map[string]string, err error) {
	out = map[string]string{}
	maybeErr := errors.Errorf(
		"%s parameter '%s' expects type 'dict' instead got '%s'",
		function, param, val.String())
	if val.Type() != "dict" {
		err = maybeErr
		return
	}
	dict, ok := val.(starlark.IterableMapping)
	if !ok {
		err = maybeErr
		return
	}
	items := dict.Items()
	for _, item := range items {
		key := item.Index(0)
		value := item.Index(1)
		ks, ok := key.(starlark.String)
		if !ok {
			err = errors.Errorf("%s %s expects a dictionary of strings, but got value %s", function, param, key.String())
			return
		}
		vs, ok := value.(starlark.String)
		if !ok {
			err = errors.Errorf("%s %s expects a dictionary of strings, but got value %s", function, param, value.String())
			return
		}
		out[ks.GoString()] = vs.GoString()
	}
	return
}

func StarlarkBuild(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (v starlark.Value, err error) {
	if args.Len() > 0 {
		return nil, errors.New("builtin function build() takes no positional arguments")
	}
	build, err := newBuildFromKWArgs(kwargs)
	if err != nil {
		return nil, err
	}
	tester.Print(build)
	return starlark.None, nil
}
