package bramble

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/maxmcd/bramble/pkg/brambleproject"
	"github.com/maxmcd/bramble/pkg/logger"
	"github.com/maxmcd/bramble/pkg/sandbox"
	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/pkg/errors"
)

var (
	BrambleFunctionBuildHiddenCommand = "__bramble-function-build"
)

func createAndParseCLI(args []string) (*ffcli.Command, error) {
	buildFlagSet := flag.NewFlagSet("build", flag.ContinueOnError)
	buildCheck := buildFlagSet.Bool("check", false, "verify that builds are reproducible by running them twice and comparing their output")
	subcommands := []*ffcli.Command{
		{
			Name:       "build",
			ShortUsage: "bramble build [options] [module]:<function> [args...]",
			ShortHelp:  "Build a function",
			Exec: func(ctx context.Context, args []string) error {
				b, err := newBramble()
				if err != nil {
					return err
				}
				_, err = b.runBuildFromCLI("build", args, buildOptions{
					Check: *buildCheck,
				})
				return err
			},
			FlagSet: buildFlagSet,
		},
		{
			Name:       "run",
			ShortUsage: "bramble run [options] [module]:<function> [args...]",
			ShortHelp:  "run a derivation",
			Exec: func(ctx context.Context, args []string) error {
				b, err := newBramble()
				if err != nil {
					return err
				}
				return b.run(args)
			},
		},

		{
			Name:       "shell",
			ShortUsage: "bramble shell [options] [module]:<function> [args...]",
			ShortHelp:  "Open a shell from a derivation",
			Exec: func(ctx context.Context, args []string) error {
				b, err := newBramble()
				if err != nil {
					return err
				}
				_, err = b.runBuildFromCLI("build", args, buildOptions{
					Shell: true,
				})
				return err
			},
		},
		{
			Name:       "repl",
			ShortUsage: "bramble repl",
			ShortHelp:  "Run an interactive shell",
			Exec:       func(ctx context.Context, args []string) error { return repl(args) },
		},
		{
			Name:       "print-build-input",
			ShortUsage: "bramble print-build-input",
			Exec:       func(ctx context.Context, args []string) error { return printBuildInput(args) },
		},
		{
			Name:       "derivation",
			ShortUsage: "bramble derivation <subcomand>",
			ShortHelp:  "Work with derivations directly",
			Subcommands: []*ffcli.Command{
				{
					Name:       "build",
					ShortUsage: "bramble derivation build ~/bramble/store/3orpqhjdgtvfbqbhpecro3qe6heb3jvq-simple.drv",
					Exec:       func(ctx context.Context, args []string) error { return nil },
				},
			},
			Exec: func(ctx context.Context, args []string) error { return flag.ErrHelp },
		},
		{
			Name:       "store",
			ShortUsage: "bramble store <subcommand>",
			ShortHelp:  "Interact with the store",
			Exec:       func(ctx context.Context, args []string) error { return flag.ErrHelp },
			// Subcommands: []*ffcli.Command{storeGC, storeAudit},
		},
	}

	// storeGC = &ffcli.Command{
	// 	Name:       "gc",
	// 	ShortUsage: "bramble gc",
	// 	ShortHelp:  "Run the bramble garbage collector against the store",
	// 	LongHelp: `    Collect garbage

	// 'Bramble gc' will clean up unused files and dependencies from the bramble
	// store. This includes cache files, artifacts, and derivations that were used as
	// build inputs that are no longer needed to run resulting programs.
	// `,
	// 	Exec: func(ctx context.Context, args []string) error {
	// 		return b.GC(args)
	// 	},
	// }

	// storeAudit = &ffcli.Command{
	// 	Name:       "audit",
	// 	ShortUsage: "bramble verify||check", #TODO
	// 	ShortHelp:  "",
	// 	LongHelp:   "",
	// 	Exec: func(ctx context.Context, args []string) error {
	// 		return b.GC(args)
	// 	},
	// }

	rootFlagSet := flag.NewFlagSet("bramble", flag.ExitOnError)
	version := rootFlagSet.Bool("version", false, "version")

	root := &ffcli.Command{
		ShortUsage:  "bramble [flags] <command> [<args>]",
		Subcommands: subcommands,
		FlagSet:     rootFlagSet,
		UsageFunc:   DefaultUsageFunc,
		Exec: func(ctx context.Context, args []string) error {
			if *version {
				logger.Print("0.0.1")
				return nil
			}
			return flag.ErrHelp
		},
	}

	// Recursively patch all command descriptions and usage functions
	var fixup func(*ffcli.Command)
	fixup = func(cmd *ffcli.Command) {
		for _, c := range cmd.Subcommands {
			c.UsageFunc = DefaultUsageFunc
			// replace tabs in help with 4 width spaces
			c.LongHelp = strings.ReplaceAll(c.LongHelp, "\t", "    ")
			fixup(c)
		}
	}
	fixup(root)

	if err := root.Parse(args); err != nil {
		return nil, err
	}

	return root, nil
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

	log.SetOutput(ioutil.Discard)
	handleErr := func(err error) {
		if err == flag.ErrHelp {
			os.Exit(127)
		}
		if er, ok := errors.Cause(err).(sandbox.ExitError); ok {
			os.Exit(er.ExitCode)
		}
		logger.Print(starutil.AnnotateError(err))
		os.Exit(1)
	}

	command, err := createAndParseCLI(os.Args[1:])
	if err != nil {
		handleErr(err)
	}
	if err := command.Run(context.Background()); err != nil {
		handleErr(err)
	}
}

func DefaultUsageFunc(c *ffcli.Command) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Usage: ")
	if c.ShortUsage != "" {
		fmt.Fprintf(&b, "%s\n", c.ShortUsage)
	} else {
		fmt.Fprintf(&b, "%s\n", c.Name)
	}
	fmt.Fprintf(&b, "\n")

	if c.LongHelp != "" {
		fmt.Fprintf(&b, "%s\n\n", c.LongHelp)
	}

	if len(c.Subcommands) > 0 {
		fmt.Fprintf(&b, "Commands:\n")
		tw := tabwriter.NewWriter(&b, 0, 4, 4, ' ', 0)
		for _, subcommand := range c.Subcommands {
			fmt.Fprintf(tw, "\t%s\t%s\n", subcommand.Name, subcommand.ShortHelp)
		}
		tw.Flush()
		fmt.Fprintf(&b, "\n")
	}

	if countFlags(c.FlagSet) > 0 {
		fmt.Fprintf(&b, "Flags:\n")
		tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
		c.FlagSet.VisitAll(func(f *flag.Flag) {
			fmt.Fprintf(tw, "  --%s\t%s\n", f.Name, f.Usage)
		})
		tw.Flush()
		fmt.Fprintf(&b, "\n")
	}

	return strings.TrimSpace(b.String())
}
func countFlags(fs *flag.FlagSet) (n int) {
	fs.VisitAll(func(*flag.Flag) { n++ })
	return n
}

func repl(_ []string) (err error) {
	project, err := brambleproject.NewProject(".")
	if err != nil {
		return err
	}
	project.REPL()
	return nil
}
