package derivation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/maxmcd/bramble/pkg/bramblecmd"
	"github.com/mitchellh/cli"
	"go.starlark.net/repl"
	"go.starlark.net/starlark"
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
			run:      client.buildCommand,
		}.factory(),
		"test": command{
			help:     `Usage: bramble test [path]`,
			synopsis: "Run bramble tests",
			run:      client.scriptCommand,
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

func (m *Module) shellCommand(args []string) (err error) {
	panic("unimplemented")
}

func (m *Module) scriptCommand(args []string) (err error) {
	thread := &starlark.Thread{Name: ""}
	// TODO: run from location of script
	wd, err := os.Getwd()
	if err != nil {
		return
	}
	builtins := bramblecmd.Builtins(wd)
	if len(args) != 0 {
		if _, err := starlark.ExecFile(thread, args[0], nil, builtins); err != nil {
			return err
		}
		return nil
	}
	repl.REPL(thread, builtins)
	return nil
}

func (m *Module) buildCommand(args []string) (err error) {
	if len(args) == 0 {
		return errors.New("the build command takes a positional argument")
	}
	_, err = m.Run(args[0])
	return
}

// Run runs a file given a path. Returns the global variable values from that
// file. Run will recursively run imported files.
func (m *Module) Run(file string) (globals starlark.StringDict, err error) {
	m.log.Debug("running file ", file)
	m.scriptLocation.Push(filepath.Dir(file))
	globals, err = starlark.ExecFile(m.thread, file, nil, starlark.StringDict{
		"derivation": starlark.NewBuiltin("derivation", m.starlarkDerivation),
	})
	if err != nil {
		return
	}
	// clear the context of this Run as it might be on an import
	m.scriptLocation.Pop()
	return
}
