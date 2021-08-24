package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/maxmcd/bramble/pkg/dstruct"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/dag"
	"github.com/pkg/errors"
)

var (
	// BramblePrefixOfRecord is the prefix we use when hashing the build output
	// this allows us to get a consistent hash even if we're building in a
	// different location
	BramblePrefixOfRecord = "/home/bramble/bramble/bramble_store_padding/bramb"

	// UnbuiltDerivationOutputTemplate is the template string we use to write
	// derivation outputs into other derivations.
	UnbuiltDerivationOutputTemplate = "{{ %s:%s }}"
	BuiltDerivationOutputTemplate   = "{{ %s }}"

	// UnbuiltTemplateStringRegexp is the regular expression that matches template strings
	// in our derivations. I assume the ".*" parts won't run away too much because
	// of the earlier match on "{{ [0-9a-z]{32}" but might be worth further
	// investigation.
	//
	// TODO: should we limit the content of the derivation name? non-latin would
	// be good for users but bad for filesystems. What's a sensible limiation
	UnbuiltTemplateStringRegexp *regexp.Regexp = regexp.MustCompile(`\{\{ ([0-9a-z]{32}-.*?\.drv):(.+?) \}\}`)
	BuiltTemplateStringRegexp   *regexp.Regexp = regexp.MustCompile(`\{\{ ([0-9a-z]{32}) \}\}`)
)

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
	store *Store
	lock  sync.Mutex
}

func (store *Store) NewDerivation() *Derivation {
	return &Derivation{
		OutputNames: []string{"out"},
		Env:         map[string]string{},
		store:       store,
	}
}

func (drv *Derivation) TemplateString() string {
	return drv.OutputTemplateString(drv.MainOutput())
}

func (drv *Derivation) DerivationOutputs() (dos DerivationOutputs) {
	filename := drv.Filename()
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

func (drv *Derivation) OutputTemplateString(output string) string {
	outputPath := drv.Output(output).Path
	if drv.Output(output).Path != "" {
		return fmt.Sprintf(BuiltDerivationOutputTemplate, outputPath)
	}
	fn := drv.Filename()
	return fmt.Sprintf(UnbuiltDerivationOutputTemplate, fn, output)
}

func (drv *Derivation) MainOutput() string {
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

func (drv *Derivation) PrettyJSON() string {
	drv.makeConsistentNullJSONValues()
	b, _ := json.MarshalIndent(drv, "", "  ")
	return string(b)
}

// REFAC, this was used in lang to populate the inputDerivations, this needs to be added in a post-lang process
// PopulateUnbuiltInputDerivations matches on the derivation input template and
// adds those candidates to the inputderivations
func (drv *Derivation) PopulateUnbuiltInputDerivations() {
	drv.InputDerivations = drv._matchedUnbuiltDerivationInputs()
}

func (drv *Derivation) _matchedUnbuiltDerivationInputs() DerivationOutputs {
	s := string(drv.JSON())
	out := DerivationOutputs{}
	for _, match := range UnbuiltTemplateStringRegexp.FindAllStringSubmatch(s, -1) {
		// We must validate that the derivation exists and this isn't just an
		// errant template string
		if drv.store.derivations.Load(match[1]) != nil {
			out = append(out, DerivationOutput{
				Filename:   match[1],
				OutputName: match[2],
			})
		}
	}
	return sortAndUniqueInputDerivations(out)
}

func (drv *Derivation) containsUnbuiltDerivationTemplateStrings() bool {
	return len(drv._matchedUnbuiltDerivationInputs()) > 0
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
	b, err := json.Marshal(drv)
	if err != nil {
		panic(err) // Shouldn't ever happen
	}
	return b
}

func (drv *Derivation) copy() *Derivation {
	out := &Derivation{}
	if err := json.Unmarshal(drv.JSON(), &out); err != nil {
		panic(err)
	}
	return out
}
func (drv *Derivation) Filename() (filename string) {
	// Content is hashed without derivation outputs.

	// TODO: why is this here?
	drv.CopyWithOutputValuesReplaced()
	copy := drv.copy()
	copy.Outputs = nil
	for i, input := range copy.InputDerivations {
		// Only use the output name and value when hashing and the output is available
		if input.Output != "" {
			copy.InputDerivations[i].Filename = ""
		}
	}
	jsonBytesForHashing := copy.JSON()

	fileName := fmt.Sprintf("%s.drv", drv.Name)
	_, filename, err := hasher.HashFile(fileName, ioutil.NopCloser(bytes.NewBuffer(jsonBytesForHashing)))
	if err != nil {
		panic(err) // shouldn't ever happen
	}
	return
}

func (drv *Derivation) BuildDependencyGraph() (graph *dstruct.AcyclicGraph, err error) {
	graph = dstruct.NewAcyclicGraph()
	var processInputDerivations func(drv *Derivation, do DerivationOutput) error
	processInputDerivations = func(drv *Derivation, do DerivationOutput) error {
		graph.Add(do)
		for _, id := range drv.InputDerivations {
			inputDrv, err := drv.store.LoadDerivation(id.Filename)
			if err != nil {
				return err
			}
			graph.Add(id)
			graph.Connect(dag.BasicEdge(do, id))
			if err := processInputDerivations(inputDrv, id); err != nil {
				return err
			}
		}
		return nil
	}
	dos := drv.DerivationOutputs()

	// If there are multiple build outputs we'll need to create a fake root and
	// connect all of the build outputs to our fake root.
	if len(dos) > 1 {
		graph.Add(dstruct.FakeDAGRoot)
		for _, do := range dos {
			graph.Connect(dag.BasicEdge(dstruct.FakeDAGRoot, do))
		}
	}
	for _, do := range dos {
		if err = processInputDerivations(drv, do); err != nil {
			return
		}
	}
	return
}

// RuntimeDependencyGraph graphs the full dependency graph needed at runtime for
// all outputs. Includes all immediate dependencies and their dependencies
func (drv *Derivation) RuntimeDependencyGraph() (graph *dstruct.AcyclicGraph, err error) {
	graph = dstruct.NewAcyclicGraph()
	noOutput := errors.New("outputs missing on derivation when searching for runtime dependencies")
	if drv.MissingOutput() {
		return nil, noOutput
	}
	var processDerivationOutputs func(do DerivationOutput) error
	processDerivationOutputs = func(do DerivationOutput) error {
		drv, err := drv.store.LoadDerivation(do.Filename)
		if err != nil {
			return err
		}
		if drv.MissingOutput() {
			return noOutput
		}
		dependencies, err := drv.runtimeDependencies()
		if err != nil {
			return err
		}
		graph.Add(do)
		for _, dependency := range dependencies[do.OutputName] {
			graph.Add(dependency)
			graph.Connect(dag.BasicEdge(do, dependency))
			if err := processDerivationOutputs(dependency); err != nil {
				return err
			}
		}
		return nil
	}
	for _, do := range drv.DerivationOutputs() {
		if err := processDerivationOutputs(do); err != nil {
			return nil, err
		}
	}
	return graph, nil
}

func (drv *Derivation) runtimeDependencies() (dependencies map[string][]DerivationOutput, err error) {
	inputDerivations, err := drv.loadInputDerivations()
	if err != nil {
		return nil, err
	}
	dependencies = map[string][]DerivationOutput{}
	outputInputMap := map[string]DerivationOutput{}
	// Map output derivations with the input that created the output
	for do, drv := range inputDerivations {
		for i, output := range drv.Outputs {
			if drv.OutputNames[i] == do.OutputName {
				outputInputMap[output.Path] = do
			}
		}
	}
	for i, out := range drv.Outputs {
		dos := []DerivationOutput{}
		for _, dependency := range out.Dependencies {
			dos = append(dos, outputInputMap[dependency])
		}
		dependencies[drv.OutputNames[i]] = dos
	}
	return dependencies, err
}

func (drv *Derivation) loadInputDerivations() (inputDerivations map[DerivationOutput]*Derivation, err error) {
	inputDerivations = make(map[DerivationOutput]*Derivation)
	for _, do := range drv.InputDerivations {
		inputDrv, err := drv.store.LoadDerivation(do.Filename)
		if err != nil {
			return nil, err
		}
		inputDerivations[do] = inputDrv
	}
	return
}

func (drv *Derivation) inputFiles() []string {
	return append([]string{drv.Filename()}, drv.SourcePaths...)
}

func (drv *Derivation) runtimeFiles(outputName string) []string {
	return []string{drv.Filename(), drv.Output(outputName).Path}
}

func (drv *Derivation) PopulateOutputsFromStore() (exists bool, err error) {
	filename := drv.Filename()
	var outputs []Output
	outputs, exists, err = drv.store.checkForBuiltDerivationOutputs(filename)
	if err != nil {
		return
	}
	if exists {
		drv.Outputs = outputs
		drv.store.derivations.Store(filename, drv)
	}
	return
}

func (drv *Derivation) replaceValueInDerivation(old, new string) (err error) {
	var dummyDrv Derivation
	if err := json.Unmarshal([]byte(strings.ReplaceAll(string(drv.JSON()), old, new)), &dummyDrv); err != nil {
		return err
	}
	drv.Args = dummyDrv.Args
	drv.Env = dummyDrv.Env
	drv.Builder = dummyDrv.Builder
	return nil
}

func (drv *Derivation) CopyWithOutputValuesReplaced() (copy *Derivation, err error) {
	s := string(drv.JSON())
	for _, match := range BuiltTemplateStringRegexp.FindAllStringSubmatch(s, -1) {
		storePath := drv.store.JoinStorePath(match[1])
		if fileutil.PathExists(storePath) {
			s = strings.ReplaceAll(s, match[0], storePath)
		}
	}
	return copy, json.Unmarshal([]byte(s), &copy)
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
	Output     string
}

func (do DerivationOutput) templateString() string {
	return fmt.Sprintf(UnbuiltDerivationOutputTemplate, do.Filename, do.OutputName)
}

type DerivationOutputs []DerivationOutput

func (dos DerivationOutputs) Len() int      { return len(dos) }
func (dos DerivationOutputs) Swap(i, j int) { dos[i], dos[j] = dos[j], dos[i] }
func (dos DerivationOutputs) Less(i, j int) bool {
	return dos[i].Filename+dos[i].OutputName < dos[j].Filename+dos[j].OutputName
}

type DerivationsMap struct {
	d    map[string]*Derivation
	lock sync.RWMutex
}

func (dm *DerivationsMap) Load(filename string) *Derivation {
	dm.lock.RLock()
	defer dm.lock.RUnlock()
	return dm.d[filename]
}

func (dm *DerivationsMap) Has(filename string) bool {
	return dm.Load(filename) != nil
}
func (dm *DerivationsMap) Store(filename string, drv *Derivation) {
	dm.lock.Lock()
	defer dm.lock.Unlock()
	dm.d[filename] = drv
}

func (dm *DerivationsMap) Range(cb func(map[string]*Derivation)) {
	dm.lock.Lock()
	cb(dm.d)
	dm.lock.Unlock()
}
