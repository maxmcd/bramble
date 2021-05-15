package bramble

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"runtime/trace"
	"sort"
	"strings"
	"time"

	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/bramble/pkg/logger"
	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

var (
	BuildDirPattern = "bramble_build_directory*"

	// BramblePrefixOfRecord is the prefix we use when hashing the build output
	// this allows us to get a consistent hash even if we're building in a
	// different location
	BramblePrefixOfRecord = "/home/bramble/bramble/bramble_store_padding/bramb"

	// DerivationOutputTemplate is the template string we use to write
	// derivation outputs into other derivations.
	DerivationOutputTemplate = "{{ %s:%s }}"

	// TemplateStringRegexp is the regular expression that matches template strings
	// in our derivations. I assume the ".*" parts won't run away too much because
	// of the earlier match on "{{ [0-9a-z]{32}" but might be worth further
	// investigation.
	//
	// TODO: should we limit the content of the derivation name? would at least
	// be limited by filesystem rules. If we're not eager about warning about this
	// we risk having derivation names only work on certain systems through that
	// limitation alone. Maybe this is ok?
	TemplateStringRegexp *regexp.Regexp = regexp.MustCompile(`\{\{ ([0-9a-z]{32}-.*?\.drv):(.+?) \}\}`)
)

// derivationFunction is the function that creates derivations
type derivationFunction struct {
	bramble *Bramble
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
func newDerivationFunction(bramble *Bramble) *derivationFunction {
	fn := &derivationFunction{
		bramble: bramble,
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
			drv.sources.location, f.bramble.configLocation))
	}()

	// find all source files that are used for this derivation
	if err = f.bramble.calculateDerivationInputSources(ctx, drv); err != nil {
		return
	}

	filename := drv.filename()
	f.bramble.derivations.Store(filename, drv)
	return drv, nil
}

func (f *derivationFunction) newDerivationFromArgs(ctx context.Context, args starlark.Tuple, kwargs []starlark.Tuple) (drv *Derivation, err error) {
	region := trace.StartRegion(ctx, "newDerivationFromArgs")
	defer region.End()

	drv = &Derivation{
		OutputNames: []string{"out"},
		Env:         map[string]string{},
		bramble:     f.bramble,
	}
	var (
		name      starlark.String
		builder   starlark.Value = starlark.None
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

	if err = f.bramble.setDerivationBuilder(drv, builder); err != nil {
		return
	}

	drv.InputDerivations = drv.searchForDerivationOutputs()

	return drv, nil
}

// Derivation is the basic building block of a Bramble build
type Derivation struct {
	// fields are in alphabetical order to attempt to provide consistency to
	// hashmap key ordering

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
	// InputDerivations are derivations that are using as imports to this build, outputs
	// dependencies are tracked in the outputs
	InputDerivations DerivationOutputs
	// Name is the name of the derivation
	Name string
	// Outputs are build outputs, a derivation can have many outputs, the
	// default output is called "out". Multiple outputs are useful when your
	// build process can produce multiple artifacts, but building them as a
	// standalone derivation would involve a complete rebuild.
	//
	// This attribute is removed when hashing the derivation.
	OutputNames []string
	Outputs     []Output
	// Platform is the platform we've built this derivation on
	Platform string
	// SourcePaths are all paths that must exist to support this build
	SourcePaths []string

	// internal fields
	sources filesList
	bramble *Bramble
}

// DerivationOutput tracks the build outputs. Outputs are not included in the
// Derivation hash. The path tracks the output location in the bramble store
// and Dependencies tracks the bramble outputs that are runtime dependencies.
type Output struct {
	Path         string
	Dependencies []string
}

func (o Output) Empty() bool {
	if o.Path == "" && len(o.Dependencies) == 0 {
		return true
	}
	return false
}

// DerivationOutput is one of the derivation inputs. Path is the location of
// the derivation, output is the name of the specific output this derivation
// uses for the build
type DerivationOutput struct {
	Filename   string
	OutputName string
}

func (do DerivationOutput) templateString() string {
	return fmt.Sprintf(DerivationOutputTemplate, do.Filename, do.OutputName)
}

type DerivationOutputs []DerivationOutput

func (dos DerivationOutputs) Len() int      { return len(dos) }
func (dos DerivationOutputs) Swap(i, j int) { dos[i], dos[j] = dos[j], dos[i] }
func (dos DerivationOutputs) Less(i, j int) bool {
	return dos[i].Filename+dos[i].OutputName < dos[j].Filename+dos[j].OutputName
}

func (drv *Derivation) DerivationOutputs() (dos DerivationOutputs) {
	filename := drv.filename()
	for _, name := range drv.OutputNames {
		dos = append(dos, DerivationOutput{Filename: filename, OutputName: name})
	}
	return
}

func sortAndUniqueInputDerivations(dos DerivationOutputs) DerivationOutputs {
	// sort
	if !sort.IsSorted(dos) {
		sort.Sort(dos)
	}
	if len(dos) == 0 {
		return dos
	}

	// dedupe
	j := 0
	for i := 1; i < len(dos); i++ {
		if dos[j] == dos[i] {
			continue
		}
		j++
		dos[j] = dos[i]
	}
	return dos[:j+1]
}

var (
	_ starlark.Value    = new(Derivation)
	_ starlark.HasAttrs = new(Derivation)
)

func (drv *Derivation) Freeze()               {}
func (drv *Derivation) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("cmd") }
func (drv *Derivation) Truth() starlark.Bool  { return starlark.True }
func (drv *Derivation) Type() string          { return "derivation" }

func (drv *Derivation) String() string {
	return drv.templateString(drv.mainOutput())
}

func (drv *Derivation) Attr(name string) (val starlark.Value, err error) {
	if !drv.HasOutput(name) {
		return nil, nil
	}
	return starlark.String(
		drv.templateString(name),
	), nil
}

func (drv *Derivation) AttrNames() (out []string) {
	return drv.OutputNames
}

func (drv *Derivation) MissingOutput() bool {
	if len(drv.Outputs) == 0 {
		return true
	}
	for _, v := range drv.Outputs {
		if v.Path == "" {
			return true
		}
	}
	return false
}

func (drv *Derivation) HasOutput(name string) bool {
	for _, o := range drv.OutputNames {
		if o == name {
			return true
		}
	}
	return false
}

func (drv *Derivation) Output(name string) Output {
	for i, o := range drv.OutputNames {
		if o == name {
			if len(drv.Outputs) > i {
				return drv.Outputs[i]
			}
		}
	}
	return Output{}
}

func (drv *Derivation) SetOutput(name string, o Output) {
	for i, on := range drv.OutputNames {
		if on == name {
			// grow if we need to
			for len(drv.Outputs) <= i {
				drv.Outputs = append(drv.Outputs, Output{})
			}
			drv.Outputs[i] = o
			return
		}
	}
	// TODO
	panic("unable to set output with name: " + name)
}

func (drv *Derivation) templateString(output string) string {
	outputPath := drv.Output(output).Path
	if drv.Output(output).Path != "" {
		return outputPath
	}
	fn := drv.filename()
	return fmt.Sprintf(DerivationOutputTemplate, fn, output)
}

func (drv *Derivation) mainOutput() string {
	if out := drv.Output("out"); out.Path != "" || len(drv.OutputNames) == 0 {
		return "out"
	}
	return drv.OutputNames[0]
}

func (drv *Derivation) env() (env []string) {
	for k, v := range drv.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return
}

func (drv *Derivation) prettyJSON() string {
	drv.makeConsistentNullJSONValues()
	b, _ := json.MarshalIndent(drv, "", "  ")
	return string(b)
}

func (drv *Derivation) searchForDerivationOutputs() DerivationOutputs {
	return searchForDerivationOutputs(string(drv.JSON()))
}

func searchForDerivationOutputs(s string) DerivationOutputs {
	out := DerivationOutputs{}
	for _, match := range TemplateStringRegexp.FindAllStringSubmatch(s, -1) {
		out = append(out, DerivationOutput{
			Filename:   match[1],
			OutputName: match[2],
		})
	}
	return sortAndUniqueInputDerivations(out)
}

func (drv *Derivation) makeConsistentNullJSONValues() {
	if len(drv.Args) == 0 {
		drv.Args = nil
	}
	if len(drv.Env) == 0 {
		drv.Env = nil
	}
	if len(drv.OutputNames) == 0 {
		drv.OutputNames = nil
	}
	if len(drv.Outputs) == 0 {
		drv.Outputs = nil
	}
	if len(drv.SourcePaths) == 0 {
		drv.SourcePaths = nil
	}
	if len(drv.InputDerivations) == 0 {
		drv.InputDerivations = nil
	}
}

func (drv *Derivation) JSON() []byte {
	drv.makeConsistentNullJSONValues()
	// This seems safe to ignore since we won't be updating the type signature
	// of Derivation. Is it?
	b, _ := json.Marshal(drv)
	return b
}

func (drv *Derivation) filename() (filename string) {
	// Content is hashed without derivation outputs.
	outputs := drv.Outputs
	drv.Outputs = nil

	jsonBytesForHashing := drv.JSON()

	drv.Outputs = outputs

	fileName := fmt.Sprintf("%s.drv", drv.Name)

	// We ignore this error, the errors would result from bad writes and all reads/writes are
	// in memory. Is this safe?
	_, filename, _ = hasher.HashFile(fileName, ioutil.NopCloser(bytes.NewBuffer(jsonBytesForHashing)))
	return
}
