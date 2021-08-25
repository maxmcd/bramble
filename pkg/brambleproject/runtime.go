package brambleproject

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/maxmcd/bramble/pkg/assert"
	"github.com/maxmcd/bramble/pkg/bramblebuild"
	"github.com/maxmcd/bramble/pkg/dstruct"
	"github.com/maxmcd/bramble/pkg/filecache"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/bramble/pkg/logger"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func NewRuntime(project *Project, store *bramblebuild.Store) (*Runtime, error) {
	if project == nil {
		return nil, errors.New("project can't be nil")
	}
	if store == nil {
		return nil, errors.New("store can't be nil")
	}

	cache, err := filecache.NewFileCache("bramble/starlark")
	if err != nil {
		return nil, errors.Wrap(err, "error trying to create cache directory")
	}
	rt := &Runtime{
		thread:        &starlark.Thread{},
		project:       project,
		store:         store,
		moduleCache:   map[string]string{},
		filenameCache: dstruct.NewBiStringMap(),
		filecache:     cache,
	}

	// creates the derivation function and checks we have a valid bramble path and store
	rt.derivationFn = newDerivationFunction(rt)

	assertGlobals, _ := assert.LoadAssertModule()

	// set the necessary error reporter so that the assert package can catch
	// errors
	assert.SetReporter(rt.thread, runErrorReporter{})

	rt.predeclared = starlark.StringDict{
		"derivation": rt.derivationFn,
		"assert":     assertGlobals["assert"],
		"sys":        starlarkSys,
		"files":      starlark.NewBuiltin("files", rt.filesBuiltin),
	}
	return rt, nil
}

type Runtime struct {
	project *Project
	store   *bramblebuild.Store

	derivationFn *derivationFunction

	filecache filecache.FileCache

	thread      *starlark.Thread
	predeclared starlark.StringDict

	moduleCache   map[string]string
	filenameCache *dstruct.BiStringMap
}

var starlarkSys = &starlarkstruct.Module{
	Name: "sys",
	Members: starlark.StringDict{
		"os":   starlark.String(runtime.GOOS),
		"arch": starlark.String(runtime.GOARCH),
	},
}

func (rt *Runtime) newDerivation() *Derivation {
	return &Derivation{Derivation: *rt.store.NewDerivation()}
}

func (rt *Runtime) resolveModule(module string) (globals starlark.StringDict, err error) {
	if _, ok := rt.moduleCache[module]; ok {
		filename, exists := rt.filenameCache.LoadInverse(module)
		if !exists {
			return nil, errors.Errorf("module %q returns no matching filename", module)
		}
		return rt.starlarkExecFile(module, filename)
	}

	path, err := rt.project.ResolveModule(module)
	if err != nil {
		return nil, err
	}

	return rt.starlarkExecFile(module, path)
}

func (rt *Runtime) starlarkExecFile(moduleName, filename string) (globals starlark.StringDict, err error) {
	prog, err := rt.sourceStarlarkProgram(moduleName, filename)
	if err != nil {
		return
	}
	g, err := prog.Init(rt.thread, rt.predeclared)
	for name := range g {
		// no importing or calling of underscored methods
		if strings.HasPrefix(name, "_") {
			delete(g, name)
		}
	}
	g.Freeze()
	return g, err
}

func (rt *Runtime) compileStarlarkPath(name string) (prog *starlark.Program, err error) {
	compiledProgram, err := rt.filecache.Open(name)
	if err != nil {
		return nil, errors.Wrap(err, "error opening moduleCache storeLocation")
	}
	return starlark.CompiledProgram(compiledProgram)
}

func (rt *Runtime) sourceStarlarkProgram(moduleName, filename string) (prog *starlark.Program, err error) {
	logger.Debugw("sourceStarlarkProgram", "moduleName", moduleName, "file", filename)
	rt.filenameCache.Store(filename, moduleName)
	inputHash, ok := rt.moduleCache[moduleName]
	if ok {
		// we have a cached binary location in the cache map, so we just use that
		return rt.compileStarlarkPath(inputHash)
	}

	// hash the file input
	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	hshr := hasher.NewHasher()
	if _, err = io.Copy(hshr, f); err != nil {
		return nil, err
	}
	inputHash = hshr.String()

	if exists, _ := rt.filecache.Exists(inputHash); exists {
		rt.moduleCache[moduleName] = inputHash
		return rt.compileStarlarkPath(inputHash)
	}

	// if we're this far we don't have a cache of the program, process it directly
	if _, err = f.Seek(0, 0); err != nil {
		return
	}
	_, prog, err = starlark.SourceProgram(filename, f, rt.predeclared.Has)
	if err != nil {
		return
	}

	var buf bytes.Buffer
	if err = prog.Write(&buf); err != nil {
		return nil, err
	}
	if err := rt.filecache.Write(inputHash, buf.Bytes()); err != nil {
		return nil, err
	}
	rt.moduleCache[moduleName] = inputHash
	return prog, nil
}

func (rt *Runtime) execTestFileContents(wd string, script string) (v starlark.Value, err error) {
	globals, err := starlark.ExecFile(rt.thread, filepath.Join(wd, "foo.bramble"), script, rt.predeclared)
	if err != nil {
		return nil, err
	}
	return starlark.Call(rt.thread, globals["test"], nil, nil)
}

func (rt *Runtime) load(thread *starlark.Thread, module string) (globals starlark.StringDict, err error) {
	return rt.resolveModule(module)
}
