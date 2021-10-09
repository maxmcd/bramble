package project

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"runtime/debug"
	"strings"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/src/assert"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"go.starlark.net/repl"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func newRuntime(workingDirectory, projectLocation, moduleName string) *runtime {
	rt := &runtime{
		workingDirectory: workingDirectory,
		projectLocation:  projectLocation,
		moduleName:       moduleName,
	}
	rt.allDerivations = map[string]Derivation{}
	rt.cache = map[string]*entry{}
	rt.internalKey = rand.Int63()
	// TODO: sys will be needed by this, what else?
	derivation, err := rt.loadNativeDerivation(starlark.NewBuiltin("_derivation", rt.derivationFunction))
	if err != nil {
		repl.PrintError(err)
		panic(err)
	}
	assertGlobals, _ := assert.LoadAssertModule()
	rt.predeclared = starlark.StringDict{
		"derivation": derivation,
		"test":       starlark.NewBuiltin("test", rt.testBuiltin),
		"run":        starlark.NewBuiltin("run", rt.runBuiltin),
		"assert":     assertGlobals["assert"],
		"sys":        starlarkSys,
		"files": starlark.NewBuiltin("files", filesBuiltin{
			projectLocation: rt.projectLocation,
		}.filesBuiltin),
	}
	return rt
}

func (rt *runtime) newThread(ctx context.Context, name string) *starlark.Thread {
	thread := &starlark.Thread{
		Name: name,
		Load: rt.load,
	}
	thread.SetLocal("ctx", ctx)
	// set the necessary error reporter so that the assert package can catch
	// errors
	assert.SetReporter(thread, runErrorReporter{})
	return thread
}

type runtime struct {
	workingDirectory string
	projectLocation  string
	moduleName       string

	internalKey int64

	allDerivations map[string]Derivation
	tests          []Test

	cache map[string]*entry

	predeclared starlark.StringDict
}

var starlarkSys = &starlarkstruct.Module{
	Name: "sys",
	Members: starlark.StringDict{
		"os":       starlark.String(stdruntime.GOOS),
		"arch":     starlark.String(stdruntime.GOARCH),
		"platform": starlark.String(stdruntime.GOOS + "-" + stdruntime.GOARCH),
	},
}

func (p *Project) REPL() {
	rt := newRuntime(p.wd, p.location, p.config.Module.Name)
	repl.REPL(rt.newThread(context.Background(), "repl"), rt.predeclared)
}

func (rt *runtime) relativePathFromConfig() string {
	relativePath, _ := filepath.Rel(rt.projectLocation, rt.workingDirectory)
	if relativePath == "." {
		// don't add a dot to the path
		return ""
	}
	return relativePath
}

type entry struct {
	globals starlark.StringDict
	err     error
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

func (rt *runtime) moduleToPath(module string) (path string, err error) {
	if !strings.HasPrefix(module, rt.moduleName) {
		// TODO: support other modules
		debug.PrintStack()
		err = errors.Errorf("We don't support other projects yet! %s", module)
		return
	}

	path = module[len(rt.moduleName):]
	path = filepath.Join(rt.projectLocation, path)

	directoryWithNameExists := fileutil.PathExists(path)

	var directoryHasDefaultDotBramble bool
	if directoryWithNameExists {
		directoryHasDefaultDotBramble = fileutil.FileExists(path + "/default.bramble")
	}

	fileWithNameExists := fileutil.FileExists(path + BrambleExtension)

	switch {
	case directoryWithNameExists && directoryHasDefaultDotBramble:
		path += "/default.bramble"
	case fileWithNameExists:
		path += BrambleExtension
	default:
		return "", errors.Errorf("Module %q not found, %q is not a directory and %q does not exist",
			module, path, path+BrambleExtension)
	}

	return path, nil
}

func (rt *runtime) starlarkExecFile(thread *starlark.Thread, filename string) (globals starlark.StringDict, err error) {
	prog, err := rt.sourceStarlarkProgram(filename)
	if err != nil {
		return
	}
	g, err := prog.Init(thread, rt.predeclared)
	for name := range g {
		// no importing or calling of underscored methods
		if strings.HasPrefix(name, "_") {
			delete(g, name)
		}
	}
	g.Freeze()
	return g, err
}

func (rt *runtime) sourceStarlarkProgram(filename string) (prog *starlark.Program, err error) {
	// hash the file input
	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	_, prog, err = starlark.SourceProgram(filename, f, rt.predeclared.Has)
	return prog, err
}
