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

var (
	ErrRequiredFunctionArgument = errors.New("bramble run takes a required positional argument \"function\"")
	ErrModuleDoesNotExist       = errors.New("module doesn't exist")
)

// RunCLI runs the cli with os.Args
func RunCLI() {
	c := cli.NewCLI("bramble", "0.0.1")
	c.Args = os.Args[1:]
	b := Bramble{}
	c.Commands = map[string]cli.CommandFactory{
		"run": command{
			help: `Usage: bramble run [options] [module]:<function> [args...]

  Run a function
			`,
			synopsis: "Run a bramble function",
			run:      b.run,
		}.factory(),
		"test": command{
			help:     `Usage: bramble test [path]`,
			synopsis: "Run bramble tests",
			run:      b.test,
		}.factory(),
	}
	exitStatus, err := c.Run()
	if err != nil {
		valueErr, ok := err.(*starlark.EvalError)
		if ok {
			fmt.Println(valueErr.Backtrace())
		} else {
			fmt.Printf("%+v\n", err)
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
			fmt.Printf("%+v\n", err)
		}
		return 1
	}
	return 0
}

type Bramble struct {
	thread         *starlark.Thread
	predeclared    starlark.StringDict
	config         Config
	configLocation string
	derivation     *derivation.Function
}

func (b *Bramble) init() (err error) {
	if b.configLocation != "" {
		return errors.New("can't initialize Bramble twice")
	}

	// ensures we have a bramble.toml in the current or parent dir
	b.config, b.configLocation, err = findConfig()
	if err != nil {
		return
	}

	b.thread = &starlark.Thread{
		Name: "main",
		Load: b.load,
	}
	starlarktest.SetReporter(b.thread, ErrorReporter{})

	// creates the derivation function and checks we have a valid bramble path and store
	b.derivation, err = derivation.NewFunction(b.thread)
	if err != nil {
		return
	}

	assertGlobals, err := starlarktest.LoadAssertModule()
	if err != nil {
		return
	}

	b.predeclared = starlark.StringDict{
		"derivation": b.derivation,
		"cmd":        bramblecmd.NewFunction(),
		"os":         brambleos.OS{},
		"assert":     assertGlobals["assert"],
	}

	return
}

func (b *Bramble) load(thread *starlark.Thread, module string) (globals starlark.StringDict, err error) {
	globals, err = b.resolveModule(module)
	return
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

func findTestFiles(path string) (testFiles []string, err error) {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return
	}
	for _, file := range files {
		name := file.Name()
		if filepath.Ext(name) != extension {
			continue
		}
		if !isTestFile(name) {
			continue
		}
		testFiles = append(testFiles, filepath.Join(path, name))
	}

	return
}

type ErrorReporter struct {
}

func (e ErrorReporter) Error(args ...interface{}) {
	fmt.Println(fmt.Sprint(args...))
	os.Exit(1)
}

func (b *Bramble) test(args []string) (err error) {
	if err = b.init(); err != nil {
		return
	}
	testFiles, err := findTestFiles(".")
	if err != nil {
		return errors.Wrap(err, "error finding test files")
	}
	for _, filename := range testFiles {
		globals, err := starlark.ExecFile(b.thread, filename, nil, b.predeclared)
		if err != nil {
			return err
		}
		for name, fn := range globals {
			starFn, ok := fn.(*starlark.Function)
			if !ok {
				continue
			}
			fmt.Printf("running test %q\n", name)
			_, err = starlark.Call(b.thread, starFn, nil, nil)
			if err != nil {
				return err
			}
		}
	}
	return
}

func (b *Bramble) resolveModule(module string) (globals starlark.StringDict, err error) {
	if !strings.HasPrefix(module, b.config.Module.Name) {
		// TODO: support other modules
		err = errors.Errorf("can't find module %s", module)
		return
	}

	path := module[len(b.config.Module.Name):]
	path = filepath.Join(b.configLocation, path)

	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		path += ".bramble"
		fi, err = os.Stat(path)
		if os.IsNotExist(err) {
			return nil, ErrModuleDoesNotExist
		}
		if err != nil {
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		path += "/default.bramble"
		_, err = os.Stat(path)
		if os.IsNotExist(err) {
			return nil, ErrModuleDoesNotExist
		}
		if err != nil {
			return nil, err
		}
	}
	return starlark.ExecFile(b.thread, path, nil, b.predeclared)
}

func (b *Bramble) run(args []string) (err error) {
	if len(args) == 0 {
		err = ErrRequiredFunctionArgument
		return
	}
	if err = b.init(); err != nil {
		return
	}
	module, fn, err := b.argsToImport(args)
	if err != nil {
		return
	}
	globals, err := b.resolveModule(module)
	if err != nil {
		return
	}
	toCall, ok := globals[fn]
	if !ok {
		return errors.Errorf("global function %q not found", fn)
	}

	_, err = starlark.Call(&starlark.Thread{}, toCall, nil, nil)
	err = errors.Wrap(err, "error running")
	return
}

func (b *Bramble) argsToImport(args []string) (module, function string, err error) {
	if len(args) == 0 {
		return "", "", ErrRequiredFunctionArgument
	}
	wd, _ := os.Getwd()
	path, _ := filepath.Rel(b.configLocation, wd)

	functionArg := args[0]
	if strings.Contains(functionArg, ":") {
		parts := strings.Split(functionArg, ":")
		if len(parts) != 2 {
			return "", "", errors.New("function name has too many colons")
		}
		filename, fn := parts[0], parts[1]
		fullName := filename + extension
		_, err = os.Stat(fullName)
		if os.IsNotExist(err) {
			_, err = os.Stat(filename + "/default.bramble")
			if os.IsNotExist(err) {
				return "", "", errors.Errorf(
					"tried to find %q in the current directory to run %q, but the file doesn't exist", fullName, functionArg)
			}
		}
		if err != nil {
			return
		}
		functionArg = fn
		path += ("/" + filename)
	}

	return b.config.Module.Name + "/" + path, functionArg, nil
}
