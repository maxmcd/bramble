package bramblescript

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/maxmcd/bramble/pkg/brambleos"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarktest"
)

var extension string = ".bramble"

func isTestFile(name string) bool {
	if !strings.HasSuffix(name, extension) {
		return false
	}
	nameWithoutExtension := name[:len(name)-len(extension)]
	return (strings.HasPrefix(nameWithoutExtension, "test_") ||
		strings.HasSuffix(nameWithoutExtension, "_test"))
}

func _findBrambles(withTest bool) (brambles []string, err error) {
	files, err := ioutil.ReadDir(".")
	if err != nil {
		return
	}
	for _, file := range files {
		name := file.Name()
		if filepath.Ext(name) != extension {
			continue
		}
		if !withTest && isTestFile(name) {
			continue
		}
		brambles = append(brambles, name)
	}
	return
}

func findBrambles() (brambles []string, err error) {
	return _findBrambles(false)
}

func findBramblesWithTest() (brambles []string, err error) {
	return _findBrambles(true)
}

func checkGlobals(val string) bool {
	return val != "print"
}

type ErrorReporter struct {
}

func (e ErrorReporter) Error(args ...interface{}) {
	fmt.Println(fmt.Sprint(args...))
	os.Exit(1)
}

func Test() (err error) {
	brambles, err := findBramblesWithTest()
	if err != nil {
		return
	}
	thread, testableGlobals, globals, err := resolveFiles(brambles)
	if err != nil {
		return
	}
	for _, toCall := range testableGlobals {
		funcToCall, ok := toCall.(*starlark.Function)
		if !ok {
			// global variables that start with test_ that are not functions are not run
			continue
		}
		// this is somewhat redundant, functions in a program share a module https://github.com/google/starlark-go/blob/949cc6f4b0/starlark/value.go#L579
		for key, val := range globals {
			funcToCall.AddPredeclared(key, val)
		}
		_, err = starlark.Call(thread, toCall, nil, nil)
		// TODO: run all tests before exiting
		if err != nil {
			err = errors.Wrap(err, "error running")
		}
	}
	return
}

func resolveFiles(brambles []string) (thread *starlark.Thread, testableGlobals starlark.StringDict, globals starlark.StringDict, err error) {
	thread = &starlark.Thread{Name: ""}
	client := NewClient(".")

	assertGlobals, err := starlarktest.LoadAssertModule()
	if err != nil {
		return
	}

	// TODO: use our own here, we need to be able to print code location
	// of various errors and the existing starlarktest lib doesn't easily
	// allow that
	starlarktest.SetReporter(thread, ErrorReporter{})

	predeclared := starlark.StringDict{
		"cmd":    client,
		"os":     brambleos.OS{},
		"assert": assertGlobals["assert"],
	}
	globals = starlark.StringDict{}
	testableGlobals = starlark.StringDict{}
	for _, bramble := range brambles {
		var prog *starlark.Program
		_, prog, err = starlark.SourceProgram(bramble, nil, checkGlobals)
		if err != nil {
			err = errors.Wrap(err, "error sourcing "+bramble)
			return
		}
		var theseGlobals starlark.StringDict
		theseGlobals, err = prog.Init(thread, predeclared)
		if err != nil {
			err = errors.Wrap(err, "error initializing "+bramble)
			return
		}
		for key, value := range theseGlobals {
			if _, ok := globals[key]; ok {
				if !ok {
					err = errors.Errorf("duplicate global value %q redeclared in %q", key, bramble)
					return
				}
			}
			globals[key] = value
			if strings.HasPrefix(key, "test_") {
				testableGlobals[key] = value
			}
		}
	}
	return
}

func Run(function string) (err error) {
	brambles, err := findBrambles()
	if err != nil {
		return err
	}
	thread, _, globals, err := resolveFiles(brambles)
	if err != nil {
		return
	}

	toCall, ok := globals[function]
	if !ok {
		return errors.Errorf("global function %q not found", function)
	}

	funcToCall, ok := toCall.(*starlark.Function)
	if !ok {
		return errors.Errorf("global value %q is not a function", function)
	}

	for key, val := range globals {
		funcToCall.AddPredeclared(key, val)
	}

	_, err = starlark.Call(thread, toCall, nil, nil)
	err = errors.Wrap(err, "error running")
	return
}
