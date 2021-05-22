package bramble

import (
	"encoding/json"
	"strings"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/dag"
	"github.com/pkg/errors"
)

func (drv *Derivation) BuildDependencyGraph() (graph *AcyclicGraph, err error) {
	graph = NewAcyclicGraph()
	var processInputDerivations func(drv *Derivation, do DerivationOutput) error
	processInputDerivations = func(drv *Derivation, do DerivationOutput) error {
		graph.Add(do)
		for _, id := range drv.InputDerivations {
			inputDrv, _, err := drv.bramble.loadDerivation(id.Filename)
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
		graph.Add(FakeDAGRoot)
		for _, do := range dos {
			graph.Connect(dag.BasicEdge(FakeDAGRoot, do))
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
func (drv *Derivation) RuntimeDependencyGraph() (graph *AcyclicGraph, err error) {
	graph = NewAcyclicGraph()
	noOutput := errors.New("outputs missing on derivation when searching for runtime dependencies")
	if drv.MissingOutput() {
		return nil, noOutput
	}
	var processDerivationOutputs func(do DerivationOutput) error
	processDerivationOutputs = func(do DerivationOutput) error {
		drv, _, err := drv.bramble.loadDerivation(do.Filename)
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
	inputDerivations, err := drv.inputDerivations()
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

func (drv *Derivation) inputDerivations() (inputDerivations map[DerivationOutput]*Derivation, err error) {
	inputDerivations = make(map[DerivationOutput]*Derivation)
	for _, do := range drv.InputDerivations {
		inputDrv, _, err := drv.bramble.loadDerivation(do.Filename)
		if err != nil {
			return nil, err
		}
		inputDerivations[do] = inputDrv
	}
	return
}

func (drv *Derivation) inputFiles() []string {
	return append([]string{drv.filename()}, drv.SourcePaths...)
}

func (drv *Derivation) runtimeFiles(outputName string) []string {
	return []string{drv.filename(), drv.Output(outputName).Path}
}

func (drv *Derivation) hasOutputs() bool {
	return drv.Outputs != nil
}

func (drv *Derivation) populateOutputsFromStore() (exists bool, err error) {
	filename := drv.filename()
	var outputs []Output
	outputs, exists, err = drv.bramble.checkForExistingDerivation(filename)
	if err != nil {
		return
	}
	if exists {
		drv.Outputs = outputs
		drv.bramble.derivations.Store(filename, drv)
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

func (drv *Derivation) copyWithOutputValuesReplaced() (copy *Derivation, err error) {
	s := string(drv.JSON())
	for _, match := range BuiltTemplateStringRegexp.FindAllStringSubmatch(s, -1) {
		storePath := drv.bramble.store.JoinStorePath(match[1])
		if fileutil.FileExists(storePath) {
			s = strings.ReplaceAll(s, match[0], storePath)
		}
	}
	return copy, json.Unmarshal([]byte(s), &copy)
}
