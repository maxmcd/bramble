package bramble

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/mitchellh/cli"
)

// RunCLI runs the cli with os.Args
func RunCLI() {
	ci := NewCLI()
	b := Bramble{}
	runCommand := command{
		help: `
Usage: bramble run [options] [module]:<function> [args...]

Run a function
		`,
		synopsis: "Run a bramble function",
		run:      b.run,
	}
	ci.AddCommand("run", runCommand)
	ci.AddCommand("repl", command{
		help: `
Usage: bramble repl

Open a Read Eval Print Loop
`,
		synopsis: "Run bramble tests",
		run:      b.repl,
	})
	ci.AddCommand("test", command{
		help: `
Usage: bramble test [path]

Run tests

'Bramble test' tests checks for files in the current directory that match the
pattern "*_test.bramble" or "test_*.bramble" and runs all global functions that
being with "test_".
`,
		synopsis: "Run bramble tests",
		run:      b.test,
	})
	ci.AddCommand("gc", command{
		help: `
Usage: bramble gc

Collect garbage

'Bramble gc' will clean up unused files and dependencies from the bramble
store. This includes cache files, artifacts, and derivations that were used as
build inputs that are no longer needed to run resulting programs.
`,
		synopsis: "Collect garbage",
		run:      b.gc,
	})
	ci.AddCommand("derivation", command{
		help: `
Usage: bramble derivation <subcommand> [args]

This command groups subcommands for interacting with derivations.`,
		synopsis: "Work with derivations directly",
		run: func([]string) error {
			return errHelp
		},
	})
	ci.AddCommand("derivation build", command{
		help: `
Usage: derivation build ~/bramble/store/3orpqhjdgtvfbqbhpecro3qe6heb3jvq-simple.drv
`,
		synopsis: "Build a specific derivation",
		run:      b.derivationBuild,
	})
	ci.run(os.Args[1:])
}

type command struct {
	cli      *CLI
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
		if err == errQuiet {
			return 1
		}
		if err == errHelp {
			return cli.RunResultHelp
		}
		fmt.Fprint(c.cli.stderr, starutil.AnnotateError(err))
	}
	return 0
}

type CLI struct {
	stderr io.Writer
	stdout io.Writer

	exit func(int)

	cli.CLI
}

func NewCLI() *CLI {
	return &CLI{
		stderr: os.Stderr,
		stdout: os.Stdout,
		exit:   os.Exit,
		CLI: cli.CLI{
			Autocomplete: true,
			Commands:     make(map[string]cli.CommandFactory),
		},
	}
}

func (ci *CLI) AddCommand(name string, cmd command) {
	cmd.cli = ci
	ci.Commands[name] = cmd.factory()
}

func (ci *CLI) containsHelp() bool {
	for _, arg := range ci.Args[1:] {
		if arg == "--" {
			break
		}
		if !strings.HasPrefix(arg, "-") {
			// exit the run command after the first non-argument command
			return false
		}
		if arg == "-h" || arg == "-help" || arg == "--help" {
			return true
		}
	}
	return false
}

func (ci *CLI) run(args []string) {
	ci.Name = "bramble"
	ci.HelpFunc = cli.BasicHelpFunc(ci.Name)
	ci.Version = "0.0.1"
	ci.Args = args

	if len(ci.Args) > 1 && ci.Args[0] == "run" && !ci.containsHelp() {
		// we must run this one manually so that cli doesn't parse [args] and
		// [options] for -v and -h
		v := ci.Commands["run"]
		c, _ := v()
		ci.exit(c.Run(args[1:]))
		return
	}
	exitCode, err := ci.Run()
	if err != nil {
		fmt.Fprintln(ci.stderr, err)
	}
	ci.exit(exitCode)
}
