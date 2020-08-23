package bramble

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"github.com/maxmcd/bramble/pkg/bramblecmd"
	"github.com/maxmcd/bramble/pkg/brambleos"
	"github.com/maxmcd/bramble/pkg/derivation"
	"github.com/mitchellh/cli"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarktest"
)

// RunCLI runs the cli with os.Args
func RunCLI() {
	c := cli.NewCLI("bramble", "0.0.1")
	c.Args = os.Args[1:]
	c.Commands = map[string]cli.CommandFactory{
		"run": command{
			help: `Usage: bramble run [options] [module]:<function> [args...]

  Run a function
			`,
			synopsis: "Run a bramble function",
			run:      run,
		}.factory(),
		"test": command{
			help:     `Usage: bramble test [path]`,
			synopsis: "Run bramble tests",
			run:      test,
		}.factory(),
	}
	exitStatus, err := c.Run()
	if err != nil {
		valueErr, ok := err.(*starlark.EvalError)
		if ok {
			fmt.Println(valueErr.Backtrace())
		} else {
			fmt.Println(err)
		}
	}
	os.Exit(exitStatus)
}

type command struct {
	help     string
	synopsis string
	run      func(args []string) error
}

func (c command) factory() func() (cli.Command, error) {
	return func() (cli.Command, error) {
		return &c, nil
	}
}
func (c *command) Help() string     { return c.help }
func (c *command) Synopsis() string { return c.synopsis }
func (c *command) Run(args []string) int {
	if err := c.run(args); err != nil {
		valueErr, ok := err.(*starlark.EvalError)
		if ok {
			fmt.Println(valueErr.Backtrace())
		} else {
			fmt.Println(err)
		}
		return 1
	}
	return 0
}

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
	_, ok := map[string]struct{}{
		"print": {},
		"True":  {},
		"False": {},
	}[val]
	return !ok
}

type ErrorReporter struct {
}

func (e ErrorReporter) Error(args ...interface{}) {
	fmt.Println(fmt.Sprint(args...))
	os.Exit(1)
}

func test(args []string) (err error) {
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

	assertGlobals, err := starlarktest.LoadAssertModule()
	if err != nil {
		return
	}

	// TODO: use our own here, we need to be able to print code location
	// of various errors and the existing starlarktest lib doesn't easily
	// allow that
	starlarktest.SetReporter(thread, ErrorReporter{})

	derivation, err := derivation.NewModule()
	if err != nil {
		return
	}

	predeclared := starlark.StringDict{
		"derivation": derivation,
		"cmd":        bramblecmd.NewClient(),
		"os":         brambleos.OS{},
		"assert":     assertGlobals["assert"],
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

func run(args []string) (err error) {
	if len(args) == 0 {
		return errors.New("bramble run takes a required positional argument \"function\"")
	}
	function := args[0]
	brambles, err := findBrambles()
	if err != nil {
		return err
	}
	if len(brambles) == 0 {
		return errors.New("found no bramble files in this directory")
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
