package command

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	_ "net/http/pprof"

	"github.com/maxmcd/bramble/internal/dependency"
	"github.com/maxmcd/bramble/internal/logger"
	"github.com/maxmcd/bramble/internal/project"
	"github.com/maxmcd/bramble/internal/store"
	"github.com/maxmcd/bramble/internal/tracing"
	"github.com/maxmcd/bramble/internal/types"
	"github.com/maxmcd/bramble/pkg/sandbox"
	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/mitchellh/go-wordwrap"
	"github.com/pkg/errors"
	cli "github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/trace"
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
	{{end}}{{$option}}{{end}}
`
)

var tracer trace.Tracer

func init() {
	tracer = tracing.Tracer("command")
}

func cliApp(wd string) *cli.App {
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
bramble build [options] [modules]

The build command is used to build derivations returned by bramble functions.
Calling build with a module location and function will call that function, take
any derivations that are returned, and build that derivation and its
dependencies.

bramble build ./tests/basic:self_reference
bramble build ./...
bramble build ./tests/...
bramble build github.com/maxmcd/bramble/...
bramble build github.com/maxmcd/bramble:bash github.com/maxmcd/bramble:all
bramble build github.com/username/repo/subdirectory:function

Calls to build with a path argument will build everything in that directory and
all of its subdirectories. This is done by searching for all bramble files and
calling all of their public functions. Any derivations that are returned by
these functions are built along with all of their dependencies. Call to build
without a path will run all builds from the current directory and its
subdirectories.
`,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "check",
						Value: false,
						Usage: "verify that builds are reproducible by running them twice and comparing their output",
					},
					&cli.StringFlag{
						Name:  "target",
						Value: "",
						Usage: "the target that you'd like to build for",
					},
					&cli.BoolFlag{
						Name:  "just-parse",
						Value: false,
						Usage: "only parse and run bramble files, don't build",
					},
					&cli.BoolFlag{
						Name:    "verbose",
						Aliases: []string{"v"},
						Value:   false,
						Usage:   "print build logs",
					},
				},
				Action: func(c *cli.Context) error {
					ctx, span := tracer.Start(c.Context, "bramble build "+fmt.Sprintf("%q", c.Args().Slice()))
					defer span.End()

					if c.Args().Len() == 0 {
						return cli.ShowCommandHelp(c, "build")
					}

					b, err := newBramble(wd, "")
					if err != nil {
						return err
					}
					output, err := b.execModule(ctx, c.Args().Slice(), execModuleOptions{
						target: c.String("target"),
					})
					if err != nil {
						return err
					}
					if c.Bool("just-parse") {
						return nil
					}
					_, err = b.runBuild(ctx, output, runBuildOptions{
						check:   c.Bool("check"),
						verbose: c.Bool("verbose"),
					})
					return err
				},
			},
			{
				Name:      "run",
				Usage:     "Run an executable in a derivation output",
				UsageText: "bramble run [options] [module]:<function> [args...]",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:  "paths",
						Usage: "paths that the process has access to, to pass multiple paths use this flag multiple times",
					},
					&cli.StringSliceFlag{
						Name:  "read_only_paths",
						Usage: "paths the process can't write to, to pass multiple paths use this flag multiple times",
					},
					&cli.StringSliceFlag{
						Name:  "hidden_paths",
						Usage: "paths that are hidden from the process, to pass multiple paths use this flag multiple times",
					},
					&cli.BoolFlag{
						Name:  "just-parse",
						Value: false,
						Usage: "only parse and run bramble files, don't build",
					},
					&cli.BoolFlag{
						Name:  "network",
						Usage: "allow network access",
					},
				},
				Action: func(c *cli.Context) error {
					b, err := newBramble(wd, "")
					if err != nil {
						return err
					}
					absoluteSlicePaths := func(v []string) error {
						for i, p := range v {
							a, err := filepath.Abs(p)
							if err != nil {
								return err
							}
							v[i] = a
						}
						return nil
					}

					paths := c.StringSlice("paths")
					readOnlyPaths := c.StringSlice("read_only_paths")
					hiddenPaths := c.StringSlice("hidden_paths")
					network := c.Bool("network")
					for _, err := range []error{
						absoluteSlicePaths(paths),
						absoluteSlicePaths(readOnlyPaths),
						absoluteSlicePaths(hiddenPaths),
					} {
						if err != nil {
							return err
						}
					}

					return b.run(c.Context, c.Args().Slice(), runOptions{
						paths:         paths,
						readOnlyPaths: readOnlyPaths,
						hiddenPaths:   hiddenPaths,
						network:       network,
						justParse:     c.Bool("just-parse"),
					})
				},
			},
			{
				Name:      "test",
				UsageText: "bramble test",
				Action: func(c *cli.Context) error {
					b, err := newBramble(wd, "")
					if err != nil {
						return err
					}
					return b.test(c.Context)
				},
			},
			{
				Name:      "magic",
				UsageText: "bramble magic",
				Action: func(c *cli.Context) error {
					b, err := newBramble(wd, "")
					if err != nil {
						return err
					}
					return b.project.CalculateDependencies()
				},
			},
			{
				Name:      "config",
				UsageText: "bramble config",
				Action:    cli.ShowAppHelp,
				Subcommands: []*cli.Command{
					{
						Name:      "version",
						UsageText: "bramble config version",
						Action: func(c *cli.Context) error {
							project, err := project.NewProject(".")
							if err != nil {
								return err
							}
							fmt.Println(project.Version())
							return nil
						},
						Subcommands: []*cli.Command{},
					},
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
					ctx, span := tracer.Start(c.Context, "bramble shell")
					defer span.End()
					b, err := newBramble(wd, "")
					if err != nil {
						return err
					}
					output, err := b.execModule(ctx, c.Args().Slice(), execModuleOptions{})
					if err != nil {
						return err
					}
					_, err = b.runBuild(ctx, output, runBuildOptions{
						shell: true,
					})
					return err
				},
			},
			{
				Name:      "init",
				Usage:     "Initialize a new directory as a bramble project",
				UsageText: `bramble init [name]`,
				Action: func(c *cli.Context) error {
					panic("unimplemented")
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
					project, err := project.NewProject(wd)
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
					project, err := project.NewProject(wd)
					if err != nil {
						return err
					}
					wd := project.WD()
					args := c.Args().Slice()
					if len(args) > 1 {
						return errors.New("bramble ls takes one or zero arguments")
					}
					if len(args) == 1 {
						wd = args[0]
					}
					modules, err := project.ListModuleDoc(wd)
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
			{
				Name:      "publish",
				UsageText: `bramble publish <package>`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "url",
						Value: "",
						Usage: "The url (schema+host) of the package cache server. Eg: \"https://cache.bramble.bramble\"",
					},
					&cli.BoolFlag{
						Name:  "local",
						Value: false,
						Usage: "Build locally, don't send to a build server.",
					},
					&cli.BoolFlag{
						Name:  "upload",
						Value: false,
						Usage: "Upload resulting build artifacts to an object store.",
					},
				},
				Action: func(c *cli.Context) error {
					args := c.Args().Slice()
					if len(args) == 0 {
						return errors.New("bramble publish takes at least one argument: \"module\"")
					}
					if len(args) > 2 {
						return errors.New("bramble publish takes at most two arguments")
					}
					pkg := args[0]
					return publish(c.Context, publishOptions{
						pkg:    pkg,
						local:  c.Bool("local"),
						upload: c.Bool("upload"),
						url:    c.String("url"),
					}, dependency.DownloadGithubRepo, nil)
				},
			},
			{
				Name:      "add",
				UsageText: `bramble add module@version`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "url",
						Value: "",
						Usage: "The url (schema+host) of the module cache server. Eg: \"https://cache.bramble.bramble\"",
					},
				},
				Action: func(c *cli.Context) error {
					b, err := newBramble(wd, "")
					if err != nil {
						return err
					}
					cut := func(s, sep string) (before, after string, ok bool) {
						if i := strings.Index(s, sep); i >= 0 {
							return s[:i], s[i+len(sep):], true
						}
						return s, "", false
					}
					name, version, _ := cut(c.Args().First(), "@")
					return b.project.AddDependency(c.Context, types.Package{Name: name, Version: version})
				},
			},
			{
				Name: "server",
				UsageText: `bramble server

server starts a server instance. The server can act as a build cache and a
module cache.
`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "port",
						Value: "2726",
						Usage: "the port that the server will listen on",
					},
					&cli.StringFlag{
						Name:  "host",
						Value: "localhost",
						Usage: "the host that the server will listen on",
					},
				},
				Action: func(c *cli.Context) error {
					listenOn := fmt.Sprintf("%s:%s", c.String("host"), c.String("port"))
					fmt.Printf("Server listening on: %s\n", listenOn)

					// TODO: add build cache handler to this server
					store, err := store.NewStore("")
					if err != nil {
						return err
					}

					srv := &http.Server{
						Addr: listenOn,
						Handler: dependency.ServerHandler(
							filepath.Join(store.BramblePath, "var/dependencies"),
							newBuilder(store),
							dependency.DownloadGithubRepo,
						),
					}
					errChan := make(chan error)
					go func() {
						if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
							errChan <- err
						}
					}()
					select {
					case err := <-errChan:
						return err
					case <-c.Context.Done():
						fmt.Println("Shutting down server.")
						ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
						defer cancel()
						return srv.Shutdown(ctx)
					}
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
			case *cli.StringSliceFlag:
				c.Usage = formatFlag(c.Usage, longest)
			}
		}
	}
	return app
}

// RunCLI runs the cli with os.Args
func RunCLI() {
	go func() {
		s := make(chan os.Signal, 1)
		signal.Notify(s, syscall.SIGQUIT)
		<-s
		panic("give me the stack")
	}()
	sandbox.Entrypoint()

	go func() { _ = http.ListenAndServe(":6060", nil) }()

	defer tracing.Stop()

	// Patch cli lib to remove bool default
	oldFlagStringer := cli.FlagStringer
	cli.FlagStringer = func(f cli.Flag) string {
		return strings.TrimSuffix(oldFlagStringer(f), " (default: false)")
	}

	app := cliApp(".")
	log.SetOutput(ioutil.Discard)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		s := make(chan os.Signal, 5)
		count := 0
		// handle all signals for the process.
		signal.Notify(s, syscall.SIGINT, syscall.SIGTERM)
		for {
			<-s
			count++
			cancel()
			if count == 3 {
				fmt.Println("Three interrupt attempts, exiting immediately")
				os.Exit(1)
			}
			fmt.Println("Got interrupt, shutting down")
		}
	}()
	var exitCode int
	if err := app.RunContext(ctx, os.Args); err != nil {
		if er, ok := errors.Cause(err).(store.ExecError); ok && er.Logs != nil {
			_, _ = er.Logs.Seek(0, 0)
			_, _ = io.Copy(os.Stdout, er.Logs)
			_ = er.Logs.Close()
		}
		if er, ok := errors.Cause(err).(sandbox.ExitError); ok {
			exitCode = er.ExitCode
		} else {
			logger.Print(starutil.AnnotateError(err))
			exitCode = 1
		}
	}
	if exitCode != 0 {
		// Explicitly call stop since the Exit will not call the defer
		tracing.Stop()
		os.Exit(exitCode)
	}
}

func formatFlag(usage string, longest int) string {
	return strings.ReplaceAll(
		wordwrap.WrapString(usage,
			uint(80-3-longest-3),
		), "\n", "\n\t")
}
