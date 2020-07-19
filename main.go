package main

import (
	"fmt"
	"log"
	"os"

	"github.com/maxmcd/bramble/pkg/bramble"
	"github.com/mitchellh/cli"
	"go.starlark.net/starlark"
)

func main() {
	c := cli.NewCLI("bramble", "0.0.1")
	c.Args = os.Args[1:]
	client, err := bramble.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	c.Commands = map[string]cli.CommandFactory{
		"build": command{
			help: `Usage: bramble build [options] <path>

  Build packages from a file
			`,
			synopsis: "Build packages from a file",
			run:      client.BuildCommand,
		}.factory(),
		"shell": command{
			help: `Usage: bramble shell [options]

  Drop into a shell with selected packages loaded`,
			synopsis: "Drop into a shell with selected packages loaded",
			run:      client.ShellCommand,
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
