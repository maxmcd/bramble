package project

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"sync"

	"github.com/maxmcd/bramble/src/logger"
	ds "github.com/maxmcd/bramble/src/types"
	"github.com/maxmcd/dag"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.starlark.net/starlark"
)

type ExecModuleInput struct {
	Command      string
	Arguments    []string
	IncludeTests bool
}

type ExecModuleOutput struct {
	Output         map[string]Derivation
	AllDerivations map[string]Derivation
	Globals        []string
	Tests          map[string][]Test
}

func (p *Project) ExecModule(ctx context.Context, input ExecModuleInput) (output ExecModuleOutput, err error) {
	var span trace.Span

	cmd, args := input.Command, input.Arguments
	ctx, span = tracer.Start(ctx, "project.ExecModule "+cmd+" "+fmt.Sprintf("%q", args))
	defer span.End()
	span.SetAttributes(attribute.String("cmd", cmd))
	span.SetAttributes(attribute.StringSlice("args", args))
	if len(args) == 0 {
		logger.Printfln(`"bramble %s" requires 1 argument`, cmd)
		err = flag.ErrHelp
		return
	}

	rt := newRuntime(p.wd, p.location, p.config.Module.Name)

	module, fn, err := rt.parseModuleFuncArgument(args)
	if err != nil {
		return output, err
	}
	logger.Debug("resolving module", module)
	// parse the module and all of its imports, return available functions
	globals, err := rt.execModule(ctx, module)
	if err != nil {
		return output, err
	}
	for fn := range globals {
		output.Globals = append(output.Globals, fn)
	}
	sort.Strings(output.Globals)

	toCall := map[string]starlark.Value{}
	if fn != "" {
		f, ok := globals[fn]
		if !ok {
			return output, errors.Errorf("function %q not found in %q, available functions are %q",
				fn, module, output.Globals)
		}
		toCall[fn] = f
	} else {
		toCall = globals
	}

	output.AllDerivations = map[string]Derivation{}
	output.Output = map[string]Derivation{}
	tests := []Test{}
	for fn, callable := range toCall {
		starlarkFunc, ok := callable.(*starlark.Function)
		if !ok || (starlarkFunc.NumParams()+starlarkFunc.NumKwonlyParams() > 0) {
			// TODO: make sure this prints a useful error message if a function has been explicitly called and we're silently skipping it
			continue
		}
		logger.Debug("Calling function ", fn)
		values, err := starlarkCall(ctx, rt.newThread(ctx, "Calling "+fn), callable, nil, nil)
		if err != nil {
			return output, errors.Wrap(err, "error running")
		}
		// The function must return a single derivation or a list of derivations, or
		// a tuple of derivations. We turn them into an array.
		for _, d := range valuesToDerivations(values) {
			output.Output[d.hash()] = d
		}
		if input.IncludeTests {
			for _, test := range rt.tests {
				output.Output[test.Derivation.hash()] = test.Derivation
			}
			tests = append(tests, rt.tests...)
		}
		// Append
		for k, v := range rt.allDerivationDependencies(output.Output) {
			output.AllDerivations[k] = v
		}
	}

	for _, test := range tests {
		hash := test.Derivation.hash()
		// Append takes care of the nil case
		output.Tests[hash] = append(output.Tests[hash], test)
	}
	return
}

func starlarkCall(ctx context.Context, thread *starlark.Thread, fn starlark.Value, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	_, span := tracer.Start(ctx, "starlark.Call "+fn.String())
	defer span.End()
	return starlark.Call(thread, fn, args, kwargs)
}

func (emo ExecModuleOutput) buildDependencyGraph() (graph *dag.AcyclicGraph, err error) {
	graph = &dag.AcyclicGraph{}
	for _, outputDrv := range emo.Output {
		subGraph := &dag.AcyclicGraph{}
		var processDependencies func(drv Derivation, dep Dependency) error
		processDependencies = func(drv Derivation, dep Dependency) error {
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
				if err := processDependencies(inputDrv, id); err != nil {
					return err
				}
			}
			return nil
		}
		// If there are multiple build outputs we'll need to create a fake root and
		// connect all of the build outputs to our fake root.
		outputs := outputDrv.outputsAsDependencies()
		if len(outputs) > 1 {
			subGraph.Add(ds.FakeRoot)
			for _, o := range outputs {
				subGraph.Connect(dag.BasicEdge(ds.FakeRoot, o))
			}
		}
		for _, dep := range outputs {
			if err = processDependencies(outputDrv, dep); err != nil {
				return nil, err
			}
		}
		ds.MergeGraphs(graph, subGraph)
	}
	return graph, nil
}

func (rt *runtime) allDerivationDependencies(in map[string]Derivation) (out map[string]Derivation) {
	out = map[string]Derivation{}
	queue := []string{}
	for _, drv := range in {
		queue = append(queue, drv.hash())
	}
	// BFS
	for len(queue) > 0 {
		// pop
		hash := queue[0]
		queue = queue[1:]
		drv, ok := rt.allDerivations[hash]
		if !ok {
			panic("not found " + hash)
		}
		out[hash] = drv
		for _, dep := range drv.Dependencies {
			queue = append(queue, dep.Hash)
		}
	}
	return
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
