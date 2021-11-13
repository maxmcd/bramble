package project

import (
	"context"
	"flag"
	"fmt"
	"sync"

	"github.com/maxmcd/bramble/internal/logger"
	ds "github.com/maxmcd/bramble/internal/types"
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
	Target       string
}

type ExecModuleOutput struct {
	Output         map[string]Derivation
	AllDerivations map[string]Derivation
	Tests          map[string][]Test
	Run            []Run
	// Modules is a map of all modules run, the names of their called functions
	// and the hashes of the derivations that they output
	Modules map[string]map[string][]string
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

	rt := newRuntime(p.wd, p.location, p.config.Package.Name, input.Target, p.fetchExternalModule)

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

	toCall := map[string]starlark.Value{}
	if fn != "" {
		f, ok := globals[fn]
		if !ok {
			return output, errors.Errorf("function %q not found in %q, available functions are %q",
				fn, module, globals)
		}
		toCall[fn] = f
	} else {
		toCall = globals
	}

	output.AllDerivations = map[string]Derivation{}
	output.Output = map[string]Derivation{}
	tests := []Test{}
	output.Modules = map[string]map[string][]string{module: {}}
	for fn, callable := range toCall {
		starlarkFunc, ok := callable.(*starlark.Function)
		if !ok || (starlarkFunc.NumParams()+starlarkFunc.NumKwonlyParams() > 0) {
			// TODO: make sure this prints a useful error message if a function
			// has been explicitly called and we're silently skipping it
			continue
		}

		// Call the function, calling all applicable derivations
		logger.Debug("Calling function ", fn)
		values, err := starlarkCall(ctx, rt.newThread(ctx, "Calling "+fn), callable, nil, nil)
		if err != nil {
			return output, errors.Wrap(err, "error running")
		}

		// Add calls to run() to the output
		if run, ok := values.(Run); ok {
			output.Run = append(output.Run, run)
			output.Output[run.Derivation.hash()] = run.Derivation
		}

		// The function must return a single derivation or a list of derivations, or
		// a tuple of derivations. We turn them into an array.
		for _, d := range valuesToDerivations(values) {
			output.Output[d.hash()] = d
		}

		// If we're including tests, add them to the output
		if input.IncludeTests {
			for _, test := range rt.tests {
				output.Output[test.Derivation.hash()] = test.Derivation
			}
			tests = append(tests, rt.tests...)
		}

		// Add output hashes to module function output info
		for _, drv := range output.Output {
			output.Modules[module][fn] = append(output.Modules[module][fn], drv.hash())
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

func (rt *runtime) load(thread *starlark.Thread, module string) (globals starlark.StringDict, err error) {
	return rt.execModule(thread.Local("ctx").(context.Context), module)
}

func (rt *runtime) execModule(ctx context.Context, module string) (globals starlark.StringDict, err error) {
	var span trace.Span
	ctx, span = tracer.Start(ctx, "project.rt.execModule "+module)
	defer span.End()
	if rt.predeclared == nil {
		return nil, errors.New("thread is not initialized")
	}

	e, ok := rt.cache[module]
	// If we've loaded the module already, return the cached values
	if e != nil {
		return e.globals, e.err
	}

	// If e == nil and we have a cache value then we've tried to import a module
	// while we're still loading it.
	if ok {
		return nil, fmt.Errorf("cycle in load graph")
	}

	// Add a placeholder to indicate "load in progress".
	rt.cache[module] = nil

	path, err := rt.moduleToPath(module)
	if err != nil {
		return nil, err
	}
	// Load and initialize the module in a new thread.
	globals, err = rt.starlarkExecFile(rt.newThread(ctx, "module "+module), path)
	rt.cache[module] = &entry{globals: globals, err: err}
	return globals, err
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
