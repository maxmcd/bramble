package brambleproject

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"runtime/debug"
	"strings"

	"github.com/maxmcd/bramble/pkg/assert"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/logger"
	"github.com/pkg/errors"
	"go.starlark.net/repl"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func (rt *runtime) init() {
	assertGlobals, _ := assert.LoadAssertModule()
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

func (rt *runtime) filepathToModuleName(path string) (module string, err error) {
	if !strings.HasSuffix(path, BrambleExtension) {
		return "", errors.Errorf("path %q is not a bramblefile", path)
	}
	if !fileutil.FileExists(path) {
		return "", errors.Wrap(os.ErrNotExist, path)
	}
	rel, err := filepath.Rel(rt.projectLocation, path)
	if err != nil {
		return "", errors.Wrapf(err, "%q is not relative to the project directory %q", path, rt.projectLocation)
	}
	if strings.HasSuffix(path, "default"+BrambleExtension) {
		rel = strings.TrimSuffix(rel, "default"+BrambleExtension)
	} else {
		rel = strings.TrimSuffix(rel, BrambleExtension)
	}
	rel = strings.TrimSuffix(rel, "/")
	return rt.moduleName + "/" + rel, nil
}

type runtime struct {
	workingDirectory string
	projectLocation  string
	moduleName       string

	cache map[string]*entry

	predeclared starlark.StringDict
}

var starlarkSys = &starlarkstruct.Module{
	Name: "sys",
	Members: starlark.StringDict{
		"os":   starlark.String(stdruntime.GOOS),
		"arch": starlark.String(stdruntime.GOARCH),
	},
}

type ExecModuleInput struct {
	Command   string
	Arguments []string

	ProjectInput ProjectInput
}

type ProjectInput struct {
	WorkingDirectory string
	ProjectLocation  string
	ModuleName       string
}

type ExecModuleOutput struct {
	Output         []Derivation
	AllDerivations []Derivation
}

func REPL(projectInput ProjectInput) {
	t := &runtime{
		workingDirectory: projectInput.WorkingDirectory,
		projectLocation:  projectInput.ProjectLocation,
		moduleName:       projectInput.ModuleName,
	}
	t.init()
	repl.REPL(t.newThread("repl"), t.predeclared)
}

func ExecModule(input ExecModuleInput) (output ExecModuleOutput, err error) {
	// TODO: validate input

	cmd, args := input.Command, input.Arguments
	if len(args) == 0 {
		logger.Printfln(`"bramble %s" requires 1 argument`, cmd)
		err = flag.ErrHelp
		return
	}

	rt := &runtime{
		workingDirectory: input.ProjectInput.WorkingDirectory,
		projectLocation:  input.ProjectInput.ProjectLocation,
		moduleName:       input.ProjectInput.ModuleName,
	}
	rt.init()

	module, fn, err := rt.parseModuleFuncArgument(args)
	if err != nil {
		return
	}
	logger.Debug("resolving module", module)
	// parse the module and all of its imports, return available functions
	globals, err := rt.execModule(module)
	if err != nil {
		return
	}

	toCall, ok := globals[fn]
	if !ok {
		err = errors.Errorf("function %q not found in module %q", fn, module)
		return
	}

	logger.Debug("Calling function ", fn)
	values, err := starlark.Call(rt.newThread("Calling "+fn), toCall, nil, nil)
	if err != nil {
		err = errors.Wrap(err, "error running")
		return
	}

	// The function must return a single derivation or a list of derivations, or
	// a tuple of derivations. We turn them into an array.
	derivations := valuesToDerivations(values)
	_ = derivations
	// Pull store derivations out of the project derivations
	// for _, drv := range derivations {
	// 	drvs = append(drvs, &drv.Derivation)
	// }
	// _ = drvs
	return
}

func (rt *runtime) parseModuleFuncArgument(args []string) (module, function string, err error) {
	if len(args) == 0 {
		logger.Print(`"bramble build" requires 1 argument`)
		return "", "", flag.ErrHelp
	}

	firstArgument := args[0]
	lastIndex := strings.LastIndex(firstArgument, ":")
	if lastIndex < 0 {
		logger.Print("module and function argument is not properly formatted")
		return "", "", flag.ErrHelp
	}

	path, function := firstArgument[:lastIndex], firstArgument[lastIndex+1:]
	module, err = rt.moduleFromPath(path)
	return
}

func (rt *runtime) moduleFromPath(path string) (thisModule string, err error) {
	thisModule = (rt.moduleName + "/" + rt.relativePathFromConfig())

	// See if this path is actually the name of a module, for now we just
	// support one module.
	// TODO: search through all modules in scope for this config
	if strings.HasPrefix(path, rt.moduleName) {
		return path, nil
	}

	// if the relative path is nothing, we've already added the slash above
	if !strings.HasSuffix(thisModule, "/") {
		thisModule += "/"
	}

	// support things like bar/main.bramble:foo
	if strings.HasSuffix(path, BrambleExtension) &&
		fileutil.FileExists(filepath.Join(rt.workingDirectory, path)) {
		return thisModule + path[:len(path)-len(BrambleExtension)], nil
	}

	fullName := path + BrambleExtension
	if !fileutil.FileExists(filepath.Join(rt.workingDirectory, fullName)) {
		if !fileutil.FileExists(filepath.Join(rt.workingDirectory, path+"/default.bramble")) {
			return "", errors.Errorf("%q: no such file or directory", path)
		}
	}
	// we found it, return
	thisModule += filepath.Join(path)
	return strings.TrimSuffix(thisModule, "/"), nil
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

func (rt *runtime) execTestFileContents(wd string, script string) (v starlark.Value, err error) {
	globals, err := starlark.ExecFile(rt.newThread("test"), filepath.Join(wd, "foo.bramble"), script, rt.predeclared)
	if err != nil {
		return nil, err
	}
	return starlark.Call(rt.newThread("test"), globals["test"], nil, nil)
}
