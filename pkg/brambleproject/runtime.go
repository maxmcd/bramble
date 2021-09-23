package brambleproject

import (
	"fmt"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"runtime/debug"
	"strings"

	"github.com/maxmcd/bramble/pkg/assert"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/pkg/errors"
	"go.starlark.net/repl"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func (rt *runtime) init() {
	assertGlobals, _ := assert.LoadAssertModule()
	rt.allDerivations = map[string]Derivation{}
	rt.cache = map[string]*entry{}
	rt.predeclared = starlark.StringDict{
		"derivation": starlark.NewBuiltin("derivation", rt.derivationFunction),
		"assert":     assertGlobals["assert"],
		"sys":        starlarkSys,
		"files": starlark.NewBuiltin("files", filesBuiltin{
			projectLocation: rt.projectLocation,
		}.filesBuiltin),
	}
}

func (rt *runtime) newThread(name string) *starlark.Thread {
	thread := &starlark.Thread{
		Name: name,
		Load: rt.load,
	}
	// set the necessary error reporter so that the assert package can catch
	// errors
	assert.SetReporter(thread, runErrorReporter{})
	return thread
}

type runtime struct {
	workingDirectory string
	projectLocation  string
	moduleName       string

	allDerivations map[string]Derivation

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
	t := &runtime{
		workingDirectory: p.wd,
		projectLocation:  p.location,
		moduleName:       p.config.Module.Name,
	}
	t.init()
	repl.REPL(t.newThread("repl"), t.predeclared)
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
	return rt.execModule(module)
}

func (rt *runtime) execModule(module string) (globals starlark.StringDict, err error) {
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
	globals, err = rt.starlarkExecFile(rt.newThread("module "+module), path)
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
