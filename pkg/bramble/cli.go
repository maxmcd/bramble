package bramble

import (
	"fmt"
	"log"
	"os"

	"github.com/mitchellh/cli"
	"go.starlark.net/starlark"
)

// RunCLI runs the cli with os.Args
func RunCLI() {
	c := cli.NewCLI("bramble", "0.0.1")
	c.Args = os.Args[1:]
	client, err := NewClient()
	if err != nil {
		log.Fatal(err)
	}
	c.Commands = map[string]cli.CommandFactory{
		"build": command{
			help: `Usage: bramble build [options] <path>

  Build packages from a file
			`,
			synopsis: "Build packages from a file",
			run:      client.buildCommand,
		}.factory(),
		"shell": command{
			help: `Usage: bramble shell [options]

  Drop into a shell with selected packages loaded`,
			synopsis: "Drop into a shell with selected packages loaded",
			run:      client.shellCommand,
		}.factory(),
		"script": command{
			help:     `Usage: bramble script [options] <path>`,
			synopsis: "Bramblescript",
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
