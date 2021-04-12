package bramble

import "github.com/maxmcd/dag"

func (drv *Derivation) buildDependencies() (graph *AcyclicGraph, err error) {
	graph = NewAcyclicGraph()

	var processInputDerivations func(drv *Derivation) error
	processInputDerivations = func(drv *Derivation) error {
		graph.Add(drv)
		for _, do := range drv.InputDerivations {
			inputDrv, _, err := drv.bramble.loadDerivation(do.Filename)
			graph.Connect(dag.BasicEdge(drv, inputDrv))
			if err != nil {
				return err
			}
			if err := processInputDerivations(inputDrv); err != nil {
				return err
			}
		}
		return nil
	}
	return graph, processInputDerivations(drv)
}
