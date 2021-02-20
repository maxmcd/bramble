package bramble

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"runtime/trace"
	"strings"
	"text/tabwriter"

	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/mitchellh/cli"
	"github.com/peterbourgon/ff/v3/ffcli"
)

var (
	BrambleFunctionBuildHiddenCommand = "__bramble-function-build"
)

func createCLI(b *Bramble) *ffcli.Command {
	var (
		run             *ffcli.Command
		root            *ffcli.Command
		repl            *ffcli.Command
		test            *ffcli.Command
		store           *ffcli.Command
		storeGC         *ffcli.Command
		derivation      *ffcli.Command
		derivationBuild *ffcli.Command
		rootFlagSet     = flag.NewFlagSet("bramble", flag.ExitOnError)
		version         = rootFlagSet.Bool("version", false, "version")
	)

	run = &ffcli.Command{
		Name:       "run",
		ShortUsage: "bramble run [options] [module]:<function> [args...]",
		ShortHelp:  "Run a function",
		LongHelp:   "Run a function",
		Exec:       func(ctx context.Context, args []string) error { return b.run(args) },
	}

	repl = &ffcli.Command{
		Name:       "repl",
		ShortUsage: "bramble repl",
		ShortHelp:  "Run an interactive shell",
		Exec:       func(ctx context.Context, args []string) error { return b.repl(args) },
	}
	test = &ffcli.Command{
		Name:       "test",
		ShortUsage: "bramble test [path]",
		ShortHelp:  "Run tests",
		LongHelp: `    Run tests

	Bramble test parses all bramble files recursively and runs all global
	functions that begin with 'test_'`,
		Exec: func(ctx context.Context, args []string) error { return b.test(args) },
	}

	storeGC = &ffcli.Command{
		Name:       "gc",
		ShortUsage: "bramble store gc",
		ShortHelp:  "Run the bramble garbage collector against the store",
		LongHelp: `    Collect garbage

	'Bramble gc' will clean up unused files and dependencies from the bramble
	store. This includes cache files, artifacts, and derivations that were used as
	build inputs that are no longer needed to run resulting programs.
	`,
		Exec: func(ctx context.Context, args []string) error {
			return b.gc(args)
		},
	}

	store = &ffcli.Command{
		Name:        "store",
		ShortUsage:  "bramble store <subcommand>",
		ShortHelp:   "Interact with the store",
		Exec:        func(ctx context.Context, args []string) error { return flag.ErrHelp },
		Subcommands: []*ffcli.Command{storeGC},
	}

	derivationBuild = &ffcli.Command{
		Name:       "build",
		ShortUsage: "bramble derivation build ~/bramble/store/3orpqhjdgtvfbqbhpecro3qe6heb3jvq-simple.drv",
		Exec:       func(ctx context.Context, args []string) error { return b.derivationBuild(args) },
	}
	derivation = &ffcli.Command{
		Name:        "derivation",
		ShortUsage:  "bramble derivation <subcomand>",
		ShortHelp:   "Work with derivations directly",
		Subcommands: []*ffcli.Command{derivationBuild},
		Exec:        func(ctx context.Context, args []string) error { return flag.ErrHelp },
	}

	root = &ffcli.Command{
		ShortUsage:  "bramble [--version] [--help] <command> [<args>]",
		Subcommands: []*ffcli.Command{run, repl, test, store, derivation},
		FlagSet:     rootFlagSet,
		UsageFunc:   DefaultUsageFunc,
		Exec: func(ctx context.Context, args []string) error {
			if *version {
				fmt.Println("0.0.1")
				return nil
			}
			return flag.ErrHelp
		},
	}

	// Recursively patch all commands
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
	return root
}

// RunCLI runs the cli with os.Args
func RunCLI() {
	log.SetOutput(ioutil.Discard)
	b := &Bramble{}
	command := createCLI(b)
	if err := command.ParseAndRun(context.Background(), os.Args[1:]); err != nil {
		if err == errQuiet {
			os.Exit(1)
		}
		if err == flag.ErrHelp {
			os.Exit(127)
		}
		fmt.Fprint(os.Stderr, starutil.AnnotateError(err))
		os.Exit(1)
	}
	os.Exit(0)
}

type CLI struct {
	stderr io.Writer
	stdout io.Writer

	exit func(int)

	cli.CLI
}

func (ci *CLI) run(args []string) {
	ci.Name = "bramble"
	ci.HelpFunc = cli.BasicHelpFunc(ci.Name)
	ci.Version = "0.0.1"
	ci.Args = args
	_, withinDocker := os.LookupEnv("BRAMBLE_WITHIN_DOCKER")
	if !withinDocker {
		b := Bramble{}
		if err := b.init(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if err := b.runDockerRun(context.Background(), args); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
		return
	}
	if len(ci.Args) >= 1 && !ci.containsHelp() {
		if ci.Args[0] == "run" {
			// we must run this one manually so that cli doesn't parse [args] and
			// [options] for -v and -h
			v := ci.Commands["run"]
			c, _ := v()
			ci.exit(c.Run(args[1:]))
			return
		} else if ci.Args[0] == BrambleFunctionBuildHiddenCommand {
			if err := brambleFunctionBuildSingleton(); err != nil {
				fmt.Fprint(ci.stderr, starutil.AnnotateError(err))
				trace.Stop()
				ci.exit(1)
			}
			ci.exit(0)
			return
		}
	}

	exitCode, err := ci.Run()
	if err != nil {
		fmt.Fprintln(ci.stderr, err)
	}
	ci.exit(exitCode)
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
