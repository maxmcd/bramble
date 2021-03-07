package bramble

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/maxmcd/bramble/pkg/logger"
	"github.com/maxmcd/bramble/pkg/sandbox"
	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/peterbourgon/ff/v3/ffcli"
)

var (
	BrambleFunctionBuildHiddenCommand = "__bramble-function-build"
)

func (b *Bramble) createCLI() *ffcli.Command {
	var (
		root            *ffcli.Command
		repl            *ffcli.Command
		build           *ffcli.Command
		store           *ffcli.Command
		storeGC         *ffcli.Command
		derivation      *ffcli.Command
		derivationBuild *ffcli.Command
		rootFlagSet     = flag.NewFlagSet("bramble", flag.ExitOnError)
		version         = rootFlagSet.Bool("version", false, "version")
		verbose         = rootFlagSet.Bool("v", false, "print verbose logs")
	)

	build = &ffcli.Command{
		Name:       "build",
		ShortUsage: "bramble build [options] [module]:<function> [args...]",
		ShortHelp:  "Build a function",
		LongHelp:   "Build a function",
		Exec:       func(ctx context.Context, args []string) error { return b.build(args) },
	}

	repl = &ffcli.Command{
		Name:       "repl",
		ShortUsage: "bramble repl",
		ShortHelp:  "Run an interactive shell",
		Exec:       func(ctx context.Context, args []string) error { return b.repl(args) },
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
		Subcommands: []*ffcli.Command{build, repl, store, derivation},
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

	if *verbose {
		logger.SetDebugLogger()
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
	return root
}

// RunCLI runs the cli with os.Args
func RunCLI() {
	sandbox.Entrypoint()

	log.SetOutput(ioutil.Discard)
	b := &Bramble{}
	command := b.createCLI()
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
