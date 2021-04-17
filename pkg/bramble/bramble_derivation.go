package bramble

import (
	"github.com/maxmcd/dag"
	"github.com/pkg/errors"
)

func (drv *Derivation) buildDependencies() (graph *AcyclicGraph, err error) {
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
	for _, do := range drv.DerivationOutputs() {
		if err := processInputDerivations(drv, do); err != nil {
			return nil, err
		}
	}
	return
}

// runtimeDependencyGraph graphs the full dependency graph needed at runtime for
// all outputs. Includes all immediate dependencies and their dependencies
func (drv *Derivation) runtimeDependencyGraph() (graph *AcyclicGraph, err error) {
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
