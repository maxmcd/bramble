package lang

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime/trace"
	"sort"
	"strings"
	"time"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/bramble/pkg/logger"
	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/maxmcd/bramble/pkg/store"
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
)

func init() {
	// It's easier to start giving away free coffee than it is to take away
	// free coffee.

	// I think this would allow storing arbitrary state in function closures
	// and make the codebase much harder to reason about. Maybe we want this
	// level of complexity at some point, but nice to avoid for now.
	resolve.AllowLambda = false
	resolve.AllowNestedDef = false

	// Recursion might make it easier to write long executing code.
	resolve.AllowRecursion = false

	// Sets seem harmless tho?
	resolve.AllowSet = true

	// See little need for this (currently), but open to allowing it. Are there
	// correctness issues here?
	resolve.AllowFloat = false
}

type Derivation struct {
	store.Derivation
	sources filesList
}

func valuesToDerivations(values starlark.Value) (derivations []*Derivation) {
	switch v := values.(type) {
	case *Derivation:
		return []*Derivation{v}
	case *starlark.List:
		for _, v := range starutil.ListToValueList(v) {
			derivations = append(derivations, valuesToDerivations(v)...)
		}
	case starlark.Tuple:
		for _, v := range v {
			derivations = append(derivations, valuesToDerivations(v)...)
		}
	}
	return
}

// derivationFunction is the function that creates derivations
type derivationFunction struct {
	runtime *Runtime
}

var (
	_ starlark.Value    = new(derivationFunction)
	_ starlark.Callable = new(derivationFunction)
)

func (f *derivationFunction) Freeze()               {}
func (f *derivationFunction) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("module") }
func (f *derivationFunction) Name() string          { return f.String() }
func (f *derivationFunction) String() string        { return `<built-in function derivation>` }
func (f *derivationFunction) Truth() starlark.Bool  { return true }
func (f *derivationFunction) Type() string          { return "module" }

// newDerivationFunction creates a new derivation function. When initialized this function checks if the
// bramble store exists and creates it if it does not.
func newDerivationFunction(runtime *Runtime) *derivationFunction {
	fn := &derivationFunction{
		runtime: runtime,
	}
	return fn
}

func isTopLevel(thread *starlark.Thread) bool {
	if thread.CallStackDepth() == 0 {
		// TODO: figure out what we should actually do here, so far this is
		// only for tests
		return false
	}
	return thread.CallStack().At(1).Name == "<toplevel>"
}

func (f *derivationFunction) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (v starlark.Value, err error) {
	// TODO: we should be able to cache derivation builds using some kind of hash
	// of the input values

	ctx, task := trace.NewTask(context.Background(), "derivation()")
	now := time.Now()
	defer task.End()
	if isTopLevel(thread) {
		return nil, errors.New("derivation call not within a function")
	}
	// Parse function arguments and assemble the basic derivation
	var drv *Derivation
	drv, err = f.newDerivationFromArgs(ctx, args, kwargs)
	if err != nil {
		return nil, err
	}

	defer func() {
		logger.Debugf("derivation() %s %s", time.Since(now), strings.TrimPrefix(
			drv.sources.location, f.runtime.project.Location))
	}()

	// find all source files that are used for this derivation
	if err = f.runtime.calculateDerivationInputSources(ctx, drv); err != nil {
		return
	}

	// Add this derivation to our internal store
	f.runtime.store.StoreDerivation(&drv.Derivation)
	return drv, nil
}

func (rt *Runtime) calculateDerivationInputSources(ctx context.Context, drv *Derivation) (err error) {
	region := trace.StartRegion(ctx, "calculateDerivationInputSources")
	defer region.End()

	if len(drv.sources.files) == 0 {
		return
	}

	// TODO: should extend reptar to handle hasing the files before moving
	// them to a tempdir
	tmpDir, err := rt.store.TempDir()
	if err != nil {
		return
	}

	sources := drv.sources
	drv.sources.files = []string{}
	absDir, err := filepath.Abs(drv.sources.location)
	if err != nil {
		return
	}

	// get absolute paths for all sources
	for i, src := range sources.files {
		sources.files[i] = filepath.Join(b.configLocation, src)
	}
	prefix := fileutil.CommonFilepathPrefix(append(sources.files, absDir))
	relBramblefileLocation, err := filepath.Rel(prefix, absDir)
	if err != nil {
		return
	}
	if err = fileutil.CopyFilesByPath(prefix, sources.files, tmpDir); err != nil {
		return
	}
	// sometimes the location the derivation runs from is not present
	// in the structure of the copied source files. ensure that we add it
	runLocation := filepath.Join(tmpDir, relBramblefileLocation)
	if err = os.MkdirAll(runLocation, 0755); err != nil {
		return
	}

	hshr := hasher.NewHasher()
	if err = reptar.Reptar(tmpDir, hshr); err != nil {
		return
	}
	storeLocation := rt.store.JoinStorePath(hshr.String())
	if fileutil.PathExists(storeLocation) {
		if err = os.RemoveAll(tmpDir); err != nil {
			return
		}
	} else {
		if err = os.Rename(tmpDir, storeLocation); err != nil {
			return
		}
	}
	drv.BuildContextSource = hshr.String()
	drv.BuildContextRelativePath = relBramblefileLocation
	drv.SourcePaths = append(drv.SourcePaths, hshr.String())
	sort.Strings(drv.SourcePaths)
	return
}

func (f *derivationFunction) newDerivationFromArgs(ctx context.Context, args starlark.Tuple, kwargs []starlark.Tuple) (drv *Derivation, err error) {
	region := trace.StartRegion(ctx, "newDerivationFromArgs")
	defer region.End()

	drv = &Derivation{
		Derivation: *f.runtime.store.NewDerivation(),
	}
	var (
		name      starlark.String
		builder   starlark.String
		argsParam *starlark.List
		sources   filesList
		env       *starlark.Dict
		outputs   *starlark.List
	)
	if err = starlark.UnpackArgs("derivation", args, kwargs,
		"name", &name,
		"builder", &builder,
		"args?", &argsParam,
		"sources?", &sources,
		"env?", &env,
		"outputs?", &outputs,
	); err != nil {
		return
	}

	drv.Name = name.GoString()

	if argsParam != nil {
		if drv.Args, err = starutil.IterableToGoList(argsParam); err != nil {
			return
		}
	}
	drv.sources = sources

	if env != nil {
		if drv.Env, err = starutil.DictToGoStringMap(env); err != nil {
			return
		}
	}

	if outputs != nil {
		outputsList, err := starutil.IterableToGoList(outputs)
		if err != nil {
			return nil, err
		}
		drv.Outputs = nil
		drv.OutputNames = outputsList
	}

	drv.Builder = builder.GoString()

	drv.PopulateUnbuiltInputDerivations()

	return drv, nil
}

// runErrorReporter reports errors during a run. These errors are just passed up the thread
type runErrorReporter struct{}

func (e runErrorReporter) Error(err error) {}
func (e runErrorReporter) FailNow() bool   { return true }
