package bramble

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"go.starlark.net/repl"

	"github.com/maxmcd/bramble/pkg/bramblebuild"
	"github.com/maxmcd/bramble/pkg/brambleproject"
	"github.com/maxmcd/bramble/pkg/logger"
)

// Bramble is the main bramble client. It has various caches and metadata
// associated with running bramble.
type Bramble struct {

	// don't use features that require root like setuid binaries
	noRoot bool

	project *brambleproject.Project
	store   *bramblebuild.Store

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

func (b *Bramble) repl(_ []string) (err error) {
	repl.REPL(b.thread, b.predeclared)
	return nil
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
