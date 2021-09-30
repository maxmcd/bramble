package command

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/maxmcd/bramble/pkg/sandbox"
	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/maxmcd/bramble/src/build"
	"github.com/maxmcd/bramble/src/logger"
	"github.com/maxmcd/bramble/src/project"
	"github.com/mitchellh/go-wordwrap"

	"github.com/pkg/errors"
	cli "github.com/urfave/cli/v2"
)

var (
	commandHelpTemplate = `Usage: {{if .UsageText}}{{.UsageText}}{{else}}{{.HelpName}}{{if .VisibleFlags}} [command options]{{end}} {{if .ArgsUsage}}{{.ArgsUsage}}{{else}}[arguments...]{{end}}{{end}}{{if .Category}}

Category:
   {{.Category}}{{end}}{{if .Description}}

Description:
   {{.Description | nindent 3 | trim}}{{end}}{{if .VisibleFlags}}

Options:{{range .VisibleFlags}}
   {{.}}{{end}}{{end}}
`

	appHelpTemplate = `Usage: {{.Usage}}
	{{.Description | nindent 3 | trim}}
Commands:{{range .VisibleCategories}}{{if .Name}}
	{{.Name}}:{{range .VisibleCommands}}
	  {{join .Names ", "}}{{"\t"}}{{.Usage}}{{end}}{{else}}{{range .VisibleCommands}}
	{{join .Names ", "}}{{"\t"}}{{.Usage}}{{end}}{{end}}{{end}}

Options:
	{{range $index, $option := .VisibleFlags}}{{if $index}}
	{{end}}{{$option}}{{end}}`
)

// RunCLI runs the cli with os.Args
func RunCLI() {
	// Patch cli lib to remove bool default
	oldFlagStringer := cli.FlagStringer
	cli.FlagStringer = func(f cli.Flag) string {
		return strings.TrimSuffix(oldFlagStringer(f), " (default: false)")
	}

	go func() {
		s := make(chan os.Signal, 1)
		signal.Notify(s, syscall.SIGQUIT)
		<-s
		panic("give me the stack")
	}()
	sandbox.Entrypoint()

	app := &cli.App{
		Name:                  "bramble",
		Usage:                 "bramble [--version] [--help] <command> [args]",
		Version:               "0.1.0",
		HideHelpCommand:       true,
		CustomAppHelpTemplate: appHelpTemplate,
		Commands: []*cli.Command{
			{
				Name:  "build",
				Usage: "Build derivations",
				UsageText: `

bramble build [options] [module]:<function>
bramble build [options] [path]

The build command is used to build derivations returned by bramble functions.
Calling build with a module location and function will call that function, take
any derivations that are returned, and build that derivation and its
dependencies.

bramble build ./tests/basic:self_reference
bramble build github.com/maxmcd/bramble:all
bramble build github.com/username/repo/subdirectory:all

Calls to build with a path argument will build everything in that directory and
all of its subdirectories. This is done by searching for all bramble files and
calling all of their public functions. Any derivations that are returned by
these functions are built along with all of their dependencies. Call to build
without a path will run all builds from the current directory and its
subdirectories.

bramble build
bramble build ./tests
`,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "check",
						Value: false,
						Usage: "verify that builds are reproducible by running them twice and comparing their output",
					},
				},
				Action: func(c *cli.Context) error {
					b, err := newBramble()
					if err != nil {
						return err
					}
					output, err := b.execModule("build", c.Args().Slice(), execModuleOptions{})
					if err != nil {
						return err
					}
					_, err = b.runBuild(c.Context, output, buildOptions{
						check: c.Bool("check"),
					})
					return err
				},
			},
			{
				Name:      "run",
				Usage:     "Run an executable in a derivation output",
				UsageText: "bramble run [options] [module]:<function> [args...]",
				Action: func(c *cli.Context) error {
					b, err := newBramble()
					if err != nil {
						return err
					}
					return b.run(c.Context, c.Args().Slice())
				},
			},
			{
				Name:      "test",
				UsageText: "bramble test",
				Action: func(c *cli.Context) error {
					b, err := newBramble()
					if err != nil {
						return err
					}
					return b.test(c.Context)
				},
			},
			{
				Name:  "shell",
				Usage: "Open a shell within a derivation build context",
				UsageText: `bramble shell [options] [module]:<function>

shell takes the same arguments as "bramble build" but instead of building the
final derivation it opens up a terminal into the build environment within a
build directory with environment variables and dependencies populated. This is a
good way to debug a derivation that you're building.`,
				Action: func(c *cli.Context) error {
					b, err := newBramble()
					if err != nil {
						return err
					}
					output, err := b.execModule("shell", c.Args().Slice(), execModuleOptions{})
					if err != nil {
						return err
					}
					_, err = b.runBuild(c.Context, output, buildOptions{
						shell: true,
					})
					return err
				},
			},
			{
				Name:  "repl",
				Usage: "Open a read-eval-print-loop to interact with the Bramble config language",
				UsageText: `bramble repl

repl opens up a read-eval-print-loop for interacting with the bramble config
language. You can make derivations and call other built-in functions. The repl
has limited use because you can't build anything that you create, but it's a
good place to get familiar with how the built-in modules and functions work.
				`,
				Action: func(c *cli.Context) error {
					project, err := project.NewProject(".")
					if err != nil {
						return err
					}
					project.REPL()
					return nil
				},
			},
			{
				Name:  "ls",
				Usage: "List functions and documentation",
				UsageText: `bramble ls [path]

Calls to "ls" will search the current directory for bramble files and print
their public functions with documentation. If an immediate subdirectory has a
"default.bramble" documentation will be printed for those functions as well.
				`,
				Action: func(c *cli.Context) error {
					project, err := project.NewProject(".")
					if err != nil {
						return err
					}
					modules, err := project.ListModuleDoc()
					if err != nil {
						return err
					}
					for _, m := range modules {
						fmt.Printf("Module: %s\n", m.Name)
						fmt.Println(m.Docstring)
						if m.Docstring != "" {
							fmt.Println()
						}
						for _, fn := range m.Functions {
							fmt.Println("    " + fn.Definition)
							fmt.Println(strings.ReplaceAll("        "+fn.Docstring, "\n", "\n    "))
							if fn.Docstring != "" {
								fmt.Println()
							}
						}
						fmt.Println()
					}
					return nil
				},
			},
		},
	}

	for _, c := range app.Commands {
		c.CustomHelpTemplate = commandHelpTemplate

		// Wrap the options help to 80 width. Requires knowledge of the longest
		// flag length. Assumes there are never aliases.
		longest := 0
		for _, flag := range c.Flags {
			for _, name := range flag.Names() {
				if len(name) > longest {
					longest = len(name)
				}
			}
		}
		for _, flag := range c.Flags {
			switch c := flag.(type) {
			case *cli.BoolFlag:
				c.Usage = formatFlag(c.Usage, longest)
			case *cli.StringFlag:
				c.Usage = formatFlag(c.Usage, longest)
			}
		}
	}

	log.SetOutput(ioutil.Discard)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		s := make(chan os.Signal)
		count := 0
		// handle all signals for the process.
		signal.Notify(s, syscall.SIGINT, syscall.SIGTERM)
		for {
			_ = <-s
			count++
			cancel()
			if count == 3 {
				fmt.Println("Three interrupt attempts, exiting")
				os.Exit(1)
			}
		}
	}()

	if err := app.RunContext(ctx, os.Args); err != nil {
		if er, ok := errors.Cause(err).(build.ExecError); ok {
			fmt.Println(er.Logs.Len())
			_, _ = io.Copy(os.Stdout, er.Logs)
		}
		if er, ok := errors.Cause(err).(sandbox.ExitError); ok {
			os.Exit(er.ExitCode)
		}
		logger.Print(starutil.AnnotateError(err))
		os.Exit(1)
	}
}

func formatFlag(usage string, longest int) string {
	return strings.ReplaceAll(
		wordwrap.WrapString(usage,
			uint(80-3-longest-3),
		), "\n", "\n\t")
}
