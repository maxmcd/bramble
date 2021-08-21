package bramble

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"go.starlark.net/repl"
	"go.starlark.net/starlark"

	"github.com/maxmcd/bramble/pkg/logger"
	"github.com/maxmcd/bramble/pkg/project"
	"github.com/maxmcd/bramble/pkg/store"
)

// Bramble is the main bramble client. It has various caches and metadata
// associated with running bramble.
type Bramble struct {

	// don't use features that require root like setuid binaries
	noRoot bool

	project *project.Project
	store   *store.Store

	// working directory
	wd          string
	bramblePath string
}

// Option can be used to set various options when initializing bramble
type Option func(*Bramble)

// OptionNoRoot ensures bramble don't use features that require root like setuid binaries
func OptionNoRoot(b *Bramble) {
	b.noRoot = true
}

// OptionBramblePath allows the bramble path to be overridden
func OptionBramblePath(bramblePath string) Option {
	return func(b *Bramble) {
		b.bramblePath = bramblePath
	}
}

// NewBramble creates a new bramble instance. If the working directory passed is
// within a bramble project that projects configuration will be laoded
func NewBramble(wd string, opts ...Option) (b *Bramble, err error) {
	b = &Bramble{}
	b.wd = wd

	for _, opt := range opts {
		opt(b)
	}

	// Make b.wd absolute
	if !filepath.IsAbs(b.wd) {
		wd, _ := os.Getwd()
		b.wd = filepath.Join(wd, b.wd)
	}

	if b.store.IsEmpty() {
		bramblePath := os.Getenv("BRAMBLE_PATH")
		if b.bramblePath != "" {
			bramblePath = b.bramblePath
		}
		if b.store, err = store.NewStore(bramblePath); err != nil {
			return
		}
	}

	if b.project, err = project.NewProject(wd); err != nil {
		return nil, err
	}

	return b, nil
}

func findAllDerivationsInProject(loc string) (derivations []*Derivation, err error) {
	b, err := NewBramble(loc)
	if err != nil {
		return nil, err
	}

	if err := filepath.Walk(b.configLocation, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// TODO: ignore .git, ignore .gitignore?
		if strings.HasSuffix(path, ".bramble") {
			module, err := b.project.FilepathToModuleName(path)
			if err != nil {
				return err
			}
			globals, err := b.resolveModule(module)
			if err != nil {
				return err
			}
			for name, v := range globals {
				if fn, ok := v.(*starlark.Function); ok {
					if fn.NumParams()+fn.NumKwonlyParams() > 0 {
						continue
					}
					fn.NumParams()
					value, err := starlark.Call(b.thread, fn, nil, nil)
					if err != nil {
						return errors.Wrapf(err, "calling %q in %s", name, path)
					}
					derivations = append(derivations, valuesToDerivations(value)...)
				}
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return
}

func (b *Bramble) repl(_ []string) (err error) {
	repl.REPL(b.thread, b.predeclared)
	return nil
}

func (b *Bramble) parseAndCallBuildArg(cmd string, args []string) (derivations []*Derivation, err error) {
	if len(args) == 0 {
		logger.Printfln(`"bramble %s" requires 1 argument`, cmd)
		err = flag.ErrHelp
		return
	}

	// parse something like ./tests:foo into the correct module and function
	// name
	module, fn, err := b.parseModuleFuncArgument(args)
	if err != nil {
		return
	}
	logger.Debug("resolving module", module)
	// parse the module and all of its imports, return available functions
	globals, err := b.resolveModule(module)
	if err != nil {
		return
	}
	toCall, ok := globals[fn]
	if !ok {
		err = errors.Errorf("function %q not found in module %q", fn, module)
		return
	}

	logger.Debug("Calling function ", fn)
	values, err := starlark.Call(&starlark.Thread{}, toCall, nil, nil)
	if err != nil {
		err = errors.Wrap(err, "error running")
		return
	}

	// The function must return a single derivation or a list of derivations, or
	// a tuple of derivations. We turn them into an array.
	derivations = valuesToDerivations(values)
	return
}

func (b *Bramble) Shell(ctx context.Context, args []string) (result []BuildResult, err error) {
	if !b.withinProject() {
		return nil, ErrNotInProject
	}
	derivations, err := b.parseAndCallBuildArg("build", args)
	if err != nil {
		return nil, err
	}
	if len(derivations) > 1 {
		return nil, errors.New(`cannot run "bramble shell" with a function that returns multiple derivations`)
	}
	shellDerivation := derivations[0]

	if result, err = b.buildDerivations(ctx,
		derivations, shellDerivation); err != nil {
		return
	}

	if err := b.writeConfigMetadata(); err != nil {
		return nil, err
	}
	filename := shellDerivation.filename()
	logger.Print("Launching shell for derivation", filename)
	logger.Debugw(shellDerivation.PrettyJSON())
	if err = b.buildDerivation(ctx, shellDerivation, true); err != nil {
		return nil, errors.Wrap(err, "error spawning "+filename)
	}
	return result, nil
}

func (b *Bramble) withinProject() bool {
	return b.configLocation != ""
}

func (b *Bramble) Build(ctx context.Context, args []string) (derivations []*Derivation, buildResult []BuildResult, err error) {
	if !b.withinProject() {
		return nil, nil, ErrNotInProject
	}
	if derivations, err = b.parseAndCallBuildArg("build", args); err != nil {
		return nil, nil, err
	}
	if buildResult, err = b.buildDerivations(ctx, derivations, nil); err != nil {
		return
	}

	return derivations, buildResult, b.writeConfigMetadata()
}

func (b *Bramble) writeConfigMetadata() (err error) {
	if b.project == nil {
		return errors.New("no project to write config link for")
	}
	return b.store.WriteConfigLink(b.project.location)
}

func (b *Bramble) GC(_ []string) (err error) {

	return b.store.GC()

}

func (b *Bramble) derivationBuild(args []string) error {
	return nil
}

func (b *Bramble) parseModuleFuncArgument(args []string) (module, function string, err error) {
	if len(args) == 0 {
		logger.Print(`"bramble build" requires 1 argument`)
		return "", "", flag.ErrHelp
	}

	firstArgument := args[0]
	lastIndex := strings.LastIndex(firstArgument, ":")
	if lastIndex < 0 {
		logger.Print("module and function argument is not properly formatted")
		return "", "", flag.ErrHelp
	}
	path, function := firstArgument[:lastIndex], firstArgument[lastIndex+1:]
	module, err = b.moduleFromPath(path)
	return
}
