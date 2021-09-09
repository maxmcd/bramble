package brambleproject

import (
	"flag"
	"fmt"
	"sync"

	ds "github.com/maxmcd/bramble/pkg/data_structures"
	"github.com/maxmcd/bramble/pkg/logger"
	"github.com/maxmcd/dag"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

type ExecModuleInput struct {
	Command   string
	Arguments []string
}

type ExecModuleOutput struct {
	Output         []Derivation
	AllDerivations map[string]Derivation
}

func (project *Project) ExecModule(input ExecModuleInput) (output ExecModuleOutput, err error) {
	cmd, args := input.Command, input.Arguments
	if len(args) == 0 {
		logger.Printfln(`"bramble %s" requires 1 argument`, cmd)
		err = flag.ErrHelp
		return
	}

	rt := &runtime{
		workingDirectory: project.wd,
		projectLocation:  project.location,
		moduleName:       project.config.Module.Name,
	}
	rt.init()

	module, fn, err := rt.parseModuleFuncArgument(args)
	if err != nil {
		return output, err
	}
	logger.Debug("resolving module", module)
	// parse the module and all of its imports, return available functions
	globals, err := rt.execModule(module)
	if err != nil {
		return output, err
	}

	toCall, ok := globals[fn]
	if !ok {
		return output, errors.Errorf("function %q not found in module %q", fn, module)
	}

	logger.Debug("Calling function ", fn)
	values, err := starlark.Call(rt.newThread("Calling "+fn), toCall, nil, nil)
	if err != nil {
		return output, errors.Wrap(err, "error running")
	}

	// The function must return a single derivation or a list of derivations, or
	// a tuple of derivations. We turn them into an array.
	output.Output = valuesToDerivations(values)
	output.AllDerivations = rt.allDerivationDependencies(output.Output)
	return
}

func (emo ExecModuleOutput) buildDependencyGraph() (graph *dag.AcyclicGraph, err error) {
	graph = &dag.AcyclicGraph{}
	for _, outputDrv := range emo.Output {
		subGraph := &dag.AcyclicGraph{}
		var processDepedencies func(drv Derivation, dep Dependency) error
		processDepedencies = func(drv Derivation, dep Dependency) error {
			subGraph.Add(dep)
			for _, id := range drv.Dependencies {
				inputDrv, found := emo.AllDerivations[id.Hash]
				if !found {
					return errors.Errorf("Can't find derivation with hash %s from output %s", id.Hash, id.Output)
				}
				if err != nil {
					return err
				}
				subGraph.Add(id)
				subGraph.Connect(dag.BasicEdge(dep, id))
				if err := processDepedencies(inputDrv, id); err != nil {
					return err
				}
			}
			return nil
		}
		// If there are multiple build outputs we'll need to create a fake root and
		// connect all of the build outputs to our fake root.
		outputs := outputDrv.outputsAsDependencies()
		if len(outputs) > 1 {
			subGraph.Add(ds.FakeDAGRoot)
			for _, o := range outputs {
				subGraph.Connect(dag.BasicEdge(ds.FakeDAGRoot, o))
			}
		}
		for _, dep := range outputs {
			if err = processDepedencies(outputDrv, dep); err != nil {
				return nil, err
			}
		}
		ds.MergeGraphs(graph, subGraph)
	}
	return graph, nil
}

type BuildOutput struct {
	Dep        Dependency
	OutputPath string
}

func (emo ExecModuleOutput) WalkAndPatch(maxParallel int, fn func(dep Dependency, drv Derivation) (buildOutputs []BuildOutput, err error)) error {
	graph, err := emo.buildDependencyGraph()
	if err != nil {
		return err
	}

	// ds.PrintDot(graph)

	drvMap := newDrvReplaceableMap()
	for _, drv := range emo.AllDerivations {
		drvMap.add(drv)
	}
	semaphore := make(chan struct{}, maxParallel)
	errs := graph.Walk(func(v dag.Vertex) error {
		if v == ds.FakeDAGRoot {
			return nil
		}
		// Limit parallism
		if maxParallel != 0 {
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
		}
		dep := v.(Dependency)
		oldHash := dep.Hash
		drv, found := drvMap.lockDrv(oldHash)
		defer drvMap.unlockDrv(oldHash)
		if !found {
			return errors.Errorf("derivation not found in DerivationGraph with hash %q", oldHash)
		}
		buildOutputs, err := fn(dep, drv)
		if err != nil {
			return err
		}
		// Now find all immediate dependents of this output and patch them to
		// contain the new template value.
		for _, edge := range graph.EdgesTo(v) {
			if edge.Source() == ds.FakeDAGRoot {
				continue
			}
			dep := edge.Source().(Dependency)
			edgeDOHash := dep.Hash
			edgeDrv, found := drvMap.lockDrv(edgeDOHash)
			if !found {
				return errors.Errorf("derivation not found in DerivationGraph with hash %q", oldHash)
			}
			drvMap.update(edgeDOHash, edgeDrv.patchDepedencyReferences(buildOutputs))
			drvMap.unlockDrv(edgeDOHash)
		}
		return nil
	})
	if len(errs) != 0 {
		return errors.New(fmt.Sprint(errs))
	}
	return nil
}

func (rt *runtime) allDerivationDependencies(in []Derivation) map[string]Derivation {
	staging := map[string]Derivation{}
	queue := make(chan string, len(rt.allDerivations))
	for _, drv := range in {
		queue <- drv.hash()
	}
	for {
		select {
		case hash := <-queue:
			drv := rt.allDerivations[hash]
			staging[hash] = drv
			for _, dep := range drv.Dependencies {
				queue <- dep.Hash
			}
		default:
			// Nothing left in the queue
			return staging
		}
	}
}

// drvReplaceableMap provides a map of Derivations that is guarded by a mutex.
// You can also retrieve derivations from map to work on using
// Lock(hash string). The lock must be released with Unlock(hash string) in
// order for work to be done on that derivation elsewhere.
type drvReplaceableMap struct {
	drvs  map[string]Derivation
	lock  sync.Mutex
	locks map[string]*sync.Mutex
}

func newDrvReplaceableMap() *drvReplaceableMap {
	return &drvReplaceableMap{
		drvs:  map[string]Derivation{},
		locks: map[string]*sync.Mutex{},
	}
}

func (drm *drvReplaceableMap) add(drv Derivation) {
	drm.lock.Lock()
	drm.drvs[drv.hash()] = drv
	drm.lock.Unlock()
}

func (drm *drvReplaceableMap) lockDrv(hash string) (drv Derivation, found bool) {
	// Get the mutex
	drm.lock.Lock()
	lock := drm.locks[hash]
	if lock == nil {
		lock = &sync.Mutex{}
		drm.locks[hash] = lock
	}
	drm.lock.Unlock()

	// Acquire the lock
	lock.Lock()

	// Now that we have the lock, retrieve the derivation, in case it has been
	// updated while we were waiting
	drm.lock.Lock()
	defer drm.lock.Unlock()
	drv, found = drm.drvs[hash]
	return drv, found
}

func (drm *drvReplaceableMap) update(hash string, drv Derivation) {
	drm.lock.Lock()
	drm.drvs[hash] = drv
	drm.lock.Unlock()
}

func (drm *drvReplaceableMap) unlockDrv(hash string) {
	drm.lock.Lock()
	lock := drm.locks[hash]
	drm.lock.Unlock()
	if lock == nil {
		// noop
		return
	}
	lock.Unlock()
}
