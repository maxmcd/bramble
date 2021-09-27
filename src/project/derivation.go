package project

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

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
	// Args are arguments that are passed to the builder
	Args []string
	// Builder will either be set to a string constant to signify an internal builder (like
	// "fetch_url"), or it will be set to the path of an executable in the bramble store
	Builder string

	Dependencies []Dependency

	// Env are environment variables set during the build
	Env map[string]string

	Name     string
	Network  bool `json:",omitempty"`
	Outputs  []string
	Platform string

	Sources FilesList
}

var (
	_ starlark.Value    = Derivation{}
	_ starlark.HasAttrs = Derivation{}
)

func (drv Derivation) String() string        { return drv.templateString(drv.defaultOutput()) }
func (drv Derivation) Type() string          { return "derivation" }
func (drv Derivation) Freeze()               {}
func (drv Derivation) Truth() starlark.Bool  { return starlark.True }
func (drv Derivation) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("derivation") }

func (drv Derivation) prettyJSON() string {
	b, _ := json.MarshalIndent(drv, "", "  ")
	return string(b)
}

func (drv Derivation) PrettyJSON() string {
	return drv.prettyJSON()
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

func (drv Derivation) outputsAsDependencies() (deps []Dependency) {
	for _, o := range drv.Outputs {
		deps = append(deps, Dependency{
			Hash:   drv.hash(),
			Output: o,
		})
	}
	return
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

func (drv Derivation) patchDependencyReferences(buildOutputs []BuildOutput) Derivation {
	j := drv.json()
	for _, bo := range buildOutputs {
		j = strings.ReplaceAll(j, fmt.Sprintf(derivationTemplate, bo.Dep.Hash, bo.Dep.Output), bo.OutputPath)
	}

	var out Derivation
	_ = json.Unmarshal([]byte(j), &out)
	return out
}

// patchDerivationReferences replaces references to one derivation with the
// other. We explicitly pass the old hash in case the derivations content has
// been changed and the originally referenced hash has changed.
func (drv Derivation) patchDerivationReferences(oldHash string, old, new Derivation) Derivation {
	if !(len(old.Outputs) == 1 &&
		len(new.Outputs) == 1 &&
		new.Outputs[0] == old.Outputs[0] &&
		old.Outputs[0] == "out") {
		// Just to be sure this isn't used incorrectly, but we currently validate that this is true
		// TODO: validate that this is true
		panic("can't patch a derivation with another derivation unless they only have the default outputs")
	}
	j := drv.json()
	j = strings.ReplaceAll(j,
		fmt.Sprintf(derivationTemplate, new.hash(), new.Outputs[0]),
		fmt.Sprintf(derivationTemplate, oldHash, new.Outputs[0]))

	var out Derivation
	_ = json.Unmarshal([]byte(j), &out)
	return out
}

type Dependency struct {
	Hash   string
	Output string
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
	if thread.CallStackDepth() <= 1 {
		// TODO: figure out what we should actually do here, so far this is only
		// for tests
		return false
	}
	// TODO: instead of always assuming an additional layer for the derivation
	// wrapper function we should validate the function with a builtin within
	// the derivation file. maybe
	return thread.CallStack().At(2).Name == "<toplevel>" || thread.CallStack().At(2).Name == "<expr>"
}

func (rt *runtime) derivationFunction(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if thread.Name != "repl" && isTopLevel(thread) {
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

func (rt *runtime) newDerivationFromArgs(args starlark.Tuple, kwargs []starlark.Tuple) (drv Derivation, err error) {
	drv = Derivation{
		Outputs: []string{"out"},
	}
	var (
		name        starlark.String
		builder     starlark.String
		argsParam   *starlark.List
		env         *starlark.Dict
		outputs     *starlark.List
		internalKey starlark.Int
	)
	if err = starlark.UnpackArgs("derivation", args, kwargs,
		"name", &name,
		"builder", &builder,
		"args?", &argsParam,
		"sources?", &drv.Sources,
		"env?", &env,
		"outputs?", &outputs,
		"platform?", &drv.Platform,
		"network?", &drv.Network,
		"_internal_key?", &internalKey,
	); err != nil {
		return
	}

	if (rt.internalKey != internalKey.BigInt().Int64()) && drv.Network {
		return drv, errors.New("derivations aren't allowed to use the network")
	}

	drv.Name = name.GoString()
	if len(drv.Name) == 0 {
		return drv, errors.New("derivation must have a name")
	}

	if argsParam != nil {
		if drv.Args, err = starutil.IterableToStringSlice(argsParam); err != nil {
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
		outputsList, err = starutil.IterableToStringSlice(outputs)
		if err != nil {
			return
		}
		if len(outputsList) == 0 {
			return drv, errors.New("derivation output must contain at least 1 value, set to None to use the default output")
		}
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
func makeConsistentNullJSONValues(drv Derivation) Derivation {
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

func (rt *runtime) findDependencies(drv Derivation) []Dependency {
	s := string(drv.json())
	out := []Dependency{}
	for _, match := range derivationTemplateRegexp.FindAllStringSubmatch(s, -1) {
		// We must validate that the derivation exists and this isn't just an
		// errant template string.

		if _, found := rt.allDerivations[match[1]]; found {
			out = append(out, Dependency{
				Hash:   match[1],
				Output: match[2],
			})
		}
	}
	return sortAndUniqueDependencies(out)
}

func sortAndUniqueDependencies(deps []Dependency) []Dependency {
	sort.Slice(deps, func(i, j int) bool {
		return deps[i].Hash+deps[i].Output < deps[j].Hash+deps[j].Output
	})

	// deduplicate
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
