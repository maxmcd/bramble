package lang

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/maxmcd/bramble/pkg/assert"
	"github.com/maxmcd/bramble/pkg/dstruct"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/bramble/pkg/logger"
	"github.com/maxmcd/bramble/pkg/project"
	"github.com/maxmcd/bramble/pkg/store"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func NewRuntime(project *project.Project, store *store.Store) *Runtime {
	return &Runtime{
		project:       project,
		store:         store,
		moduleCache:   map[string]string{},
		filenameCache: dstruct.NewBiStringMap(),
	}
}

type Runtime struct {
	project *project.Project
	store   *store.Store

	derivationFn *derivationFunction

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

func (b *Runtime) initPredeclared() (err error) {
	// creates the derivation function and checks we have a valid bramble path and store
	b.derivationFn = newDerivationFunction(b)

	assertGlobals, err := assert.LoadAssertModule()
	if err != nil {
		return
	}
	// set the necessary error reporter so that the assert package can catch
	// errors
	assert.SetReporter(b.thread, runErrorReporter{})

	b.predeclared = starlark.StringDict{
		"derivation": b.derivationFn,
		"assert":     assertGlobals["assert"],
		"sys":        starlarkSys,
		"files":      starlark.NewBuiltin("files", b.filesBuiltin),
	}
	return
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

func (r *Runtime) compileStarlarkPath(path string) (prog *starlark.Program, err error) {
	compiledProgram, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "error opening moduleCache storeLocation")
	}
	return starlark.CompiledProgram(compiledProgram)
}

func (r *Runtime) sourceStarlarkProgram(moduleName, filename string) (prog *starlark.Program, err error) {
	logger.Debugw("sourceStarlarkProgram", "moduleName", moduleName, "file", filename)
	r.filenameCache.Store(filename, moduleName)
	storeLocation, ok := r.moduleCache[moduleName]
	if ok {
		// we have a cached binary location in the cache map, so we just use that
		return r.compileStarlarkPath(r.store.JoinStorePath(storeLocation))
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
	inputHash := hshr.String()

	inputHashStoreLocation := r.store.JoinBramblePath("var", "star-cache", inputHash)
	storeLocation, ok = fileutil.ValidSymlinkExists(inputHashStoreLocation)
	if ok {
		// if we have the hashed input on the filesystem cache and it points to a valid path
		// in the store, use that store path and add the cached location to the map
		relStoreLocation, err := filepath.Rel(r.store.StorePath, storeLocation)
		if err != nil {
			return nil, err
		}
		r.moduleCache[moduleName] = relStoreLocation
		return r.compileStarlarkPath(relStoreLocation)
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
	_, path, err := r.store.WriteReader(&buf, filepath.Base(filename), "")
	if err != nil {
		return
	}
	r.moduleCache[moduleName] = filepath.Base(path)
	_ = os.Remove(inputHashStoreLocation)
	return prog, os.Symlink(path, inputHashStoreLocation)
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
