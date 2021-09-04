package brambleproject

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/pkg/errors"
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
)

var (
	derivationTemplate = "{{ %s:%s }}"
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

// fields are in alphabetical order to attempt to provide consistency to
// hashmap key ordering

// Derivation is the basic building block of a Bramble build
type Derivation struct {
	// Args are arguments that are passed to the builder
	Args []string
	// BuildContextSource is the source directory that
	BuildContextSource       string
	BuildContextRelativePath string
	// Builder will either be set to a string constant to signify an internal
	// builder (like "fetch_url"), or it will be set to the path of an
	// executable in the bramble store
	Builder string
	// Env are environment variables set during the build
	Env map[string]string

	Name     string
	Outputs  []string
	Platform string

	Sources FilesList
}

var (
	_ starlark.Value    = Derivation{}
	_ starlark.HasAttrs = Derivation{}
)

func (drv Derivation) Freeze()               {}
func (drv Derivation) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("derivation") }
func (drv Derivation) Truth() starlark.Bool  { return starlark.True }
func (drv Derivation) Type() string          { return "derivation" }
func (drv Derivation) String() string        { return drv.templateString(drv.defaultOutput()) }

func (drv Derivation) prettyJSON() string {
	b, _ := json.MarshalIndent(drv, "", "  ")
	return string(b)
}

func (drv Derivation) json() string {
	b, _ := json.Marshal(drv)
	return string(b)
}

func (drv Derivation) hash() string {
	return hasher.HashString(drv.json())
}

func (drv Derivation) defaultOutput() string {
	if len(drv.Outputs) > 0 {
		return drv.Outputs[0]
	}
	return "out"
}

func (drv Derivation) templateString(output string) string {
	return fmt.Sprintf(derivationTemplate, drv.hash(), output)
}

func (drv Derivation) hasOutput(name string) bool {
	for _, o := range drv.Outputs {
		if o == name {
			return true
		}
	}
	return false
}

func (drv Derivation) Attr(name string) (val starlark.Value, err error) {
	if !drv.hasOutput(name) {
		return nil, nil
	}
	return starlark.String(drv.templateString(name)), nil
}

func (drv Derivation) AttrNames() (out []string) {
	if len(drv.Outputs) == 0 {
		panic(drv.Outputs)
	}
	return drv.Outputs
}

func valuesToDerivations(values starlark.Value) (derivations []Derivation) {
	switch v := values.(type) {
	case Derivation:
		return []Derivation{v}
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

func isTopLevel(thread *starlark.Thread) bool {
	if thread.CallStackDepth() == 0 {
		// TODO: figure out what we should actually do here, so far this is
		// only for tests
		return false
	}
	return thread.CallStack().At(1).Name == "<toplevel>"
}

func (rt *runtime) derivationFunction(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// TODO: we should be able to cache derivation builds using some kind of hash
	// of the input values

	if isTopLevel(thread) {
		return nil, errors.New("derivation call not within a function")
	}
	// Parse function arguments and assemble the basic derivation
	drv, err := rt.newDerivationFromArgs(args, kwargs)
	if err != nil {
		return nil, err
	}

	return drv, nil
}

// REFAC, move to post-lang stage (??? check notes)
// func (rt *Runtime) calculateDerivationInputSources(ctx context.Context, drv *Derivation) (err error) {
// 	region := trace.StartRegion(ctx, "calculateDerivationInputSources")
// 	defer region.End()

// 	if len(drv.sources.files) == 0 {
// 		return
// 	}

// 	// TODO: should extend reptar to handle hasing the files before moving
// 	// them to a tempdir
// 	tmpDir, err := rt.store.TempDir()
// 	if err != nil {
// 		return
// 	}

// 	sources := drv.sources
// 	drv.sources.files = []string{}
// 	absDir, err := filepath.Abs(drv.sources.location)
// 	if err != nil {
// 		return
// 	}

// 	// get absolute paths for all sources
// 	for i, src := range sources.files {
// 		sources.files[i] = filepath.Join(rt.project.Location, src)
// 	}
// 	prefix := fileutil.CommonFilepathPrefix(append(sources.files, absDir))
// 	relBramblefileLocation, err := filepath.Rel(prefix, absDir)
// 	if err != nil {
// 		return
// 	}
// 	if err = fileutil.CopyFilesByPath(prefix, sources.files, tmpDir); err != nil {
// 		return
// 	}
// 	// sometimes the location the derivation runs from is not present
// 	// in the structure of the copied source files. ensure that we add it
// 	runLocation := filepath.Join(tmpDir, relBramblefileLocation)
// 	if err = os.MkdirAll(runLocation, 0755); err != nil {
// 		return
// 	}

// 	hshr := hasher.NewHasher()
// 	if err = reptar.Reptar(tmpDir, hshr); err != nil {
// 		return
// 	}
// 	storeLocation := rt.store.JoinStorePath(hshr.String())
// 	if fileutil.PathExists(storeLocation) {
// 		if err = os.RemoveAll(tmpDir); err != nil {
// 			return
// 		}
// 	} else {
// 		if err = os.Rename(tmpDir, storeLocation); err != nil {
// 			return
// 		}
// 	}
// 	drv.BuildContextSource = hshr.String()
// 	drv.BuildContextRelativePath = relBramblefileLocation
// 	drv.SourcePaths = append(drv.SourcePaths, hshr.String())
// 	sort.Strings(drv.SourcePaths)
// 	return
// }

func (rt *runtime) newDerivationFromArgs(args starlark.Tuple, kwargs []starlark.Tuple) (drv Derivation, err error) {
	drv = Derivation{Outputs: []string{"out"}}
	var (
		name      starlark.String
		builder   starlark.String
		argsParam *starlark.List
		sources   FilesList
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

	drv.Sources = sources
	for _, src := range drv.Sources.files {
		abs := filepath.Join(rt.projectLocation, src)
		if !fileutil.PathExists(abs) {
			return drv, errors.Errorf("Source file %q doesn't exit", abs)
		}
	}
	if env != nil {
		if drv.Env, err = starutil.DictToGoStringMap(env); err != nil {
			return
		}
	}

	if outputs != nil {
		var outputsList []string
		outputsList, err = starutil.IterableToGoList(outputs)
		if err != nil {
			return
		}
		// TODO: error if the array is empty?
		drv.Outputs = outputsList
	}

	drv.Builder = builder.GoString()
	drv = makeConsistentNullJSONValues(drv)
	return drv, nil
}

// makeConsistentNullJSONValues ensures that we null any empty arrays, some of
// these values will be initialized with zero-length arrays above, we want to
// make sure we remove this inconsistency from our hashed json output. To us
// an empty array is null.
func makeConsistentNullJSONValues(drv Derivation) Derivation {
	if len(drv.Args) == 0 {
		drv.Args = nil
	}
	if len(drv.Env) == 0 {
		drv.Env = nil
	}
	if len(drv.Outputs) == 0 {
		drv.Outputs = nil
	}
	if len(drv.Outputs) == 0 {
		drv.Outputs = nil
	}
	return drv
}

// runErrorReporter reports errors during a run. These errors are just passed up the thread
type runErrorReporter struct{}

func (e runErrorReporter) Error(err error) {}
func (e runErrorReporter) FailNow() bool   { return true }
