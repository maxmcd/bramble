package project

import (
	"context"
	"math/rand"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"strings"

	"github.com/maxmcd/bramble/internal/assert"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/fmtutil"
	"github.com/pkg/errors"
	"go.starlark.net/repl"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func newRuntime(workingDirectory, projectLocation, moduleName string, externalModuleFetcher externalModuleFetcher) *runtime {
	rt := &runtime{
		workingDirectory:      workingDirectory,
		projectLocation:       projectLocation,
		moduleName:            moduleName,
		externalModuleFetcher: externalModuleFetcher,
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

	externalModuleFetcher externalModuleFetcher
}

type externalModuleFetcher func(ctx context.Context, module string) (path string, err error)

var starlarkSys = &starlarkstruct.Module{
	Name: "sys",
	Members: starlark.StringDict{
		"os":       starlark.String(stdruntime.GOOS),
		"arch":     starlark.String(stdruntime.GOARCH),
		"platform": starlark.String(stdruntime.GOOS + "-" + stdruntime.GOARCH),
	},
}

func (p *Project) REPL() {
	rt := newRuntime(p.wd, p.location, p.config.Module.Name, p.fetchExternalModule)
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

func (rt *runtime) moduleToPath(module string) (path string, err error) {
	// If it's an external module
	if !strings.HasPrefix(module, rt.moduleName) {
		if path, err = rt.externalModuleFetcher(context.Background(), module); err != nil {
			fmtutil.Printpvln(err)
			return "", err
		}
	} else {
		path = module[len(rt.moduleName):]
		path = filepath.Join(rt.projectLocation, path)
	}
	fmtutil.Printqln(module, path)

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
