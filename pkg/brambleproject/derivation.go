package brambleproject

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	ds "github.com/maxmcd/bramble/pkg/data_structures"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/pkg/errors"
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
)

var (
	derivationTemplate                      = "{{ %s:%s }}"
	derivationTemplateRegexp *regexp.Regexp = regexp.MustCompile(`\{\{ ([0-9a-z]{32}):(.+?) \}\}`)
)

func init() {
	// It's easier to start giving away free coffee than it is to take away
	// free coffee.

	// I think this would allow storing arbitrary state in function closures and make the codebase
	// much harder to reason about. Maybe we want this level of complexity at some point, but nice
	// to avoid for now.
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
	starDerivation
}

func (drv Derivation) Args() []string             { return drv.starDerivation.Args }
func (drv Derivation) Builder() string            { return drv.starDerivation.Builder }
func (drv Derivation) Dependencies() []Dependency { return drv.starDerivation.Dependencies }
func (drv Derivation) Env() map[string]string     { return drv.starDerivation.Env }
func (drv Derivation) Name() string               { return drv.starDerivation.Name }
func (drv Derivation) Outputs() []string          { return drv.starDerivation.Outputs }
func (drv Derivation) Platform() string           { return drv.starDerivation.Platform }
func (drv Derivation) Sources() FilesList         { return drv.starDerivation.Sources }

func (drv Derivation) JSON() string { return drv.starDerivation.prettyJSON() }

var _ ds.DrvReplacable = Derivation{}

func (drv Derivation) Hash() string { return drv.starDerivation.hash() }
func (drv Derivation) ReplaceHash(old, new string) {
	// REFAC: ensure we're replacing the entire pattern, eg: {{ hash:out }} and
	// not just every string instance of the hash
	_ = json.Unmarshal(
		[]byte(strings.ReplaceAll(drv.starDerivation.json(), old, new)),
		&drv.starDerivation)
}

type starDerivation struct {
	// Args are arguments that are passed to the builder
	Args []string
	// Builder will either be set to a string constant to signify an internal builder (like
	// "fetch_url"), or it will be set to the path of an executable in the bramble store
	Builder string

	Dependencies []Dependency

	// Env are environment variables set during the build
	Env map[string]string

	Name     string
	Outputs  []string
	Platform string

	Sources FilesList
}

var (
	_ starlark.Value    = starDerivation{}
	_ starlark.HasAttrs = starDerivation{}
)

func (drv starDerivation) String() string        { return drv.templateString(drv.defaultOutput()) }
func (drv starDerivation) Type() string          { return "derivation" }
func (drv starDerivation) Freeze()               {}
func (drv starDerivation) Truth() starlark.Bool  { return starlark.True }
func (drv starDerivation) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("derivation") }

func (drv starDerivation) prettyJSON() string {
	b, _ := json.MarshalIndent(drv, "", "  ")
	return string(b)
}

func (drv starDerivation) json() string {
	b, _ := json.Marshal(drv)
	return string(b)
}

func (drv starDerivation) hash() string {
	return hasher.HashString(drv.json())
}

func (drv starDerivation) defaultOutput() string {
	if len(drv.Outputs) > 0 {
		return drv.Outputs[0]
	}
	return "out"
}

func (drv starDerivation) outputsAsDependencies() (deps []Dependency) {
	for _, o := range drv.Outputs {
		deps = append(deps, Dependency{
			hash:   drv.hash(),
			output: o,
		})
	}
	return
}

func (drv starDerivation) templateString(output string) string {
	return fmt.Sprintf(derivationTemplate, drv.hash(), output)
}

func (drv starDerivation) hasOutput(name string) bool {
	for _, o := range drv.Outputs {
		if o == name {
			return true
		}
	}
	return false
}

func (drv starDerivation) Attr(name string) (val starlark.Value, err error) {
	if !drv.hasOutput(name) {
		return nil, nil
	}
	return starlark.String(drv.templateString(name)), nil
}

func (drv starDerivation) AttrNames() (out []string) {
	if len(drv.Outputs) == 0 {
		panic(drv.Outputs)
	}
	return drv.Outputs
}

type Dependency struct {
	hash   string
	output string
}

func (ds Dependency) MarshalJSON() ([]byte, error) {
	type dep struct {
		Hash   string
		Output string
	}
	return json.Marshal(dep{Hash: ds.hash, Output: ds.output})
}

func (ds Dependency) Hash() string {
	return ds.hash
}
func (ds Dependency) Output() string {
	return ds.output
}

var _ ds.DerivationOutput = Dependency{}

func valuesToDerivations(values starlark.Value) (derivations []Derivation) {
	switch v := values.(type) {
	case starDerivation:
		return []Derivation{{v}}
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
		// TODO: figure out what we should actually do here, so far this is only
		// for tests
		return false
	}
	return thread.CallStack().At(1).Name == "<toplevel>"
}

func (rt *runtime) derivationFunction(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// TODO: we should be able to cache derivation builds using some kind of
	// hash of the input values

	if isTopLevel(thread) {
		return nil, errors.New("derivation call not within a function")
	}
	// Parse function arguments and assemble the basic derivation
	drv, err := rt.newDerivationFromArgs(args, kwargs)
	if err != nil {
		return nil, err
	}
	rt.allDerivations[drv.hash()] = drv

	return drv, nil
}

func (rt *runtime) newDerivationFromArgs(args starlark.Tuple, kwargs []starlark.Tuple) (drv starDerivation, err error) {
	drv = starDerivation{Outputs: []string{"out"}}
	var (
		name      starlark.String
		builder   starlark.String
		argsParam *starlark.List
		env       *starlark.Dict
		outputs   *starlark.List
	)
	if err = starlark.UnpackArgs("derivation", args, kwargs,
		"name", &name,
		"builder", &builder,
		"args?", &argsParam,
		"sources?", &drv.Sources,
		"env?", &env,
		"outputs?", &outputs,
		"platform?", &drv.Platform,
	); err != nil {
		return
	}

	drv.Name = name.GoString()

	if argsParam != nil {
		if drv.Args, err = starutil.IterableToGoList(argsParam); err != nil {
			return
		}
	}

	for _, src := range drv.Sources.Files {
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

	// TODO: valide that the builder is either a built-in or looks like a real
	// builder?
	drv.Builder = builder.GoString()
	drv = makeConsistentNullJSONValues(drv)

	drv.Dependencies = rt.findDependencies(drv)
	return drv, nil
}

// makeConsistentNullJSONValues ensures that we null any empty arrays, some of
// these values will be initialized with zero-length arrays above, we want to
// make sure we remove this inconsistency from our hashed json output. To us an
// empty array is null.
func makeConsistentNullJSONValues(drv starDerivation) starDerivation {
	if len(drv.Args) == 0 {
		drv.Args = nil
	}
	if len(drv.Env) == 0 {
		drv.Env = nil
	}
	if len(drv.Dependencies) == 0 {
		drv.Dependencies = nil
	}
	if len(drv.Sources.Files) == 0 {
		drv.Sources.Files = nil
	}
	return drv
}

// runErrorReporter reports errors during a run. These errors are just passed up the thread
type runErrorReporter struct{}

func (e runErrorReporter) Error(err error) {}
func (e runErrorReporter) FailNow() bool   { return true }

func (rt *runtime) findDependencies(drv starDerivation) []Dependency {
	s := string(drv.json())
	out := []Dependency{}
	for _, match := range derivationTemplateRegexp.FindAllStringSubmatch(s, -1) {
		// We must validate that the derivation exists and this isn't just an
		// errant template string

		if _, found := rt.allDerivations[match[1]]; found {
			out = append(out, Dependency{
				hash:   match[1],
				output: match[2],
			})
		}
	}
	return sortAndUniqueDependencies(out)
}

func sortAndUniqueDependencies(deps []Dependency) []Dependency {
	sort.Slice(deps, func(i, j int) bool {
		return deps[i].hash+deps[i].output < deps[j].hash+deps[j].output
	})

	// dedupe
	if len(deps) == 0 {
		return nil
	}
	j := 0
	for i := 1; i < len(deps); i++ {
		if deps[j] == deps[i] {
			continue
		}
		j++
		deps[j] = deps[i]
	}
	return deps[:j+1]
}
