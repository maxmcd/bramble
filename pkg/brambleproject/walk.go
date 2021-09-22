package brambleproject

import (
	"fmt"
	"strings"
	"sync"

	ds "github.com/maxmcd/bramble/pkg/data_structures"
	"github.com/maxmcd/dag"
	"github.com/pkg/errors"
)

type BuildOutput struct {
	Dep        Dependency
	OutputPath string
}

type Walker struct {
	graph  *dag.AcyclicGraph
	walker *dag.Walker
	drvMap *drvReplaceableMap

	lock sync.Mutex
}

// printDot prints the graph with hashes replaced with derivation names. Only
// used for debugging unclear if this is safe to run normally
func (w *Walker) printDot(g *dag.AcyclicGraph) {
	replacements := []string{}
	for h, drv := range w.drvMap.drvs {
		replacements = append(replacements, h, drv.Name)
	}
	fmt.Println(
		strings.NewReplacer(replacements...).Replace(ds.StringDot(g)),
	)
}

// Update takes the derivation input. Merges it with the existing walk graph.
// Puts a struct in the semaphore so that it can sleep without risking a lock.
// Calls update on the walker. Waits for all derivations to be fully built, and
// then returns.
func (w *Walker) Update(dep Dependency, emo ExecModuleOutput) (err error) {
	if len(emo.Output) != 1 {
		return errors.New("Output passed to walk.Update must have a single output")
	}
	// Add all derivations to the existing drvmap
	for _, drv := range emo.AllDerivations {
		w.drvMap.add(drv)
	}

	g, err := emo.buildDependencyGraph()
	if err != nil {
		return err
	}
	w.printDot(g)
	root, err := g.Root()
	if err != nil {
		return err
	}
	w.printDot(w.graph)
	fmt.Println("root", root)
	// Replace references to the node that's being built with our new tree
	w.graph.Add(root)
	for _, edge := range w.graph.EdgesTo(dep) {
		w.graph.RemoveEdge(edge)
		w.graph.Connect(dag.BasicEdge(edge.Source(), root))
	}
	w.graph.Remove(dep)
	w.printDot(w.graph)

	merged := ds.MergeGraphs(w.graph, g)
	if err := merged.Validate(); err != nil {
		w.lock.Unlock()
		return err
	}
	w.printDot(merged)

	w.lock.Lock()
	// Merge new graph into existing graph
	w.graph = merged
	// This doesn't need a mutex to be called, but maybe good to ensure that the
	// value of w.graph doesn't change under our feet
	w.walker.Update(w.graph) // Update the graph
	w.lock.Unlock()
	return nil
}

func (emo ExecModuleOutput) newWalker() (*Walker, error) {
	graph, err := emo.buildDependencyGraph()
	if err != nil {
		return nil, err
	}
	w := &Walker{graph: graph,
		drvMap: newDrvReplaceableMap()}
	for _, drv := range emo.AllDerivations {
		w.drvMap.add(drv)
	}
	return w, nil
}

func (emo ExecModuleOutput) WalkAndPatch(maxParallel int, fn func(dep Dependency, drv Derivation) (addGraph *ExecModuleOutput, buildOutputs []BuildOutput, err error)) error {
	w, err := emo.newWalker()
	if err != nil {
		return err
	}
	semaphore := make(chan struct{}, maxParallel)
	cb := func(v dag.Vertex) error {
		if v == ds.FakeRoot {
			return nil
		}
		// Limit parallism
		if maxParallel != 0 {
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
		}
		dep := v.(Dependency)
		oldHash := dep.Hash
		drv, found := w.drvMap.lockDrv(oldHash)
		defer w.drvMap.unlockDrv(oldHash)
		if !found {
			return errors.Errorf("derivation not found in DerivationGraph with hash %q", oldHash)
		}
		addGraph, buildOutputs, err := fn(dep, drv)
		if err != nil {
			fmt.Printf("%+v\n", err)
			return err
		}
		// Now find all immediate dependents of this output and patch them to
		// contain the new template value.
		for _, edge := range w.graph.EdgesTo(v) {
			if edge.Source() == ds.FakeRoot {
				continue
			}
			dep := edge.Source().(Dependency)
			edgeDOHash := dep.Hash
			edgeDrv, found := w.drvMap.lockDrv(edgeDOHash)
			if !found {
				return errors.Errorf("derivation not found in DerivationGraph with hash %q", oldHash)
			}

			if addGraph != nil {
				// If we're adding a graph, patch all of it's edges
				var toPatch Derivation
				for _, d := range addGraph.Output {
					toPatch = d
				}
				w.drvMap.update(edgeDOHash, edgeDrv.patchDerivationReferences(oldHash, drv, toPatch))
			} else if len(buildOutputs) > 0 {
				w.drvMap.update(edgeDOHash, edgeDrv.patchDependencyReferences(buildOutputs))
			}

			w.drvMap.unlockDrv(edgeDOHash)
		}
		if addGraph != nil {
			fmt.Println("updating graph")

			return w.Update(dep, *addGraph)
		}
		return nil
	}
	w.walker = &dag.Walker{Callback: cb, Reverse: true}
	w.walker.Update(w.graph)
	if errs := w.walker.Wait(); len(errs) != 0 {
		return errors.New(fmt.Sprint(errs))
	}
	return nil
}
