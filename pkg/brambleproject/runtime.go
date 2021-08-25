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

func (r *Runtime) resolveModule(module string) (globals starlark.StringDict, err error) {
	if _, ok := r.moduleCache[module]; ok {
		filename, exists := r.filenameCache.LoadInverse(module)
		if !exists {
			return nil, errors.Errorf("module %q returns no matching filename", module)
		}
		return r.starlarkExecFile(module, filename)
	}

	path, err := r.project.ResolveModule(module)
	if err != nil {
		return nil, err
	}

	return r.starlarkExecFile(module, path)
}

func (r *Runtime) starlarkExecFile(moduleName, filename string) (globals starlark.StringDict, err error) {
	prog, err := r.sourceStarlarkProgram(moduleName, filename)
	if err != nil {
		return
	}
	g, err := prog.Init(r.thread, r.predeclared)
	for name := range g {
		// no importing or calling of underscored methods
		if strings.HasPrefix(name, "_") {
			delete(g, name)
		}
	}
	g.Freeze()
	return g, err
}

func (r *Runtime) compileStarlarkPath(name string) (prog *starlark.Program, err error) {
	compiledProgram, err := r.filecache.Open(name)
	if err != nil {
		return nil, errors.Wrap(err, "error opening moduleCache storeLocation")
	}
	return starlark.CompiledProgram(compiledProgram)
}

func (r *Runtime) sourceStarlarkProgram(moduleName, filename string) (prog *starlark.Program, err error) {
	logger.Debugw("sourceStarlarkProgram", "moduleName", moduleName, "file", filename)
	r.filenameCache.Store(filename, moduleName)
	inputHash, ok := r.moduleCache[moduleName]
	if ok {
		// we have a cached binary location in the cache map, so we just use that
		return r.compileStarlarkPath(inputHash)
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

	if exists, _ := r.filecache.Exists(inputHash); exists {
		r.moduleCache[moduleName] = inputHash
		return r.compileStarlarkPath(inputHash)
	}

	// if we're this far we don't have a cache of the program, process it directly
	if _, err = f.Seek(0, 0); err != nil {
		return
	}
	_, prog, err = starlark.SourceProgram(filename, f, r.predeclared.Has)
	if err != nil {
		return
	}

	var buf bytes.Buffer
	if err = prog.Write(&buf); err != nil {
		return nil, err
	}
	if err := r.filecache.Write(inputHash, buf.Bytes()); err != nil {
		return nil, err
	}
	r.moduleCache[moduleName] = inputHash
	return prog, nil
}

func (r *Runtime) execTestFileContents(wd string, script string) (v starlark.Value, err error) {
	globals, err := starlark.ExecFile(r.thread, filepath.Join(wd, "foo.bramble"), script, r.predeclared)
	if err != nil {
		return nil, err
	}
	return starlark.Call(r.thread, globals["test"], nil, nil)
}

func (r *Runtime) load(thread *starlark.Thread, module string) (globals starlark.StringDict, err error) {
	return r.resolveModule(module)
}
