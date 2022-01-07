package project

import (
	"context"
	"math/rand"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"strings"

	"github.com/maxmcd/bramble/internal/assert"
	"github.com/maxmcd/bramble/internal/types"
	"go.starlark.net/repl"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func (p *Project) newRuntime(target string) *runtime {
	rt := &runtime{project: p}
	rt.allDerivations = map[string]Derivation{}
	rt.cache = map[string]*entry{}

	// Random internal key so that fetch built-ins can use the network
	rt.internalKey = rand.Int63()
	derivation, err := rt.loadNativeDerivation(starlark.NewBuiltin("_derivation", rt.derivationFunction(p.location)))
	if err != nil {
		repl.PrintError(err)
		panic(err)
	}

	assertGlobals, _ := assert.LoadAssertModule()
	rt.predeclared = starlark.StringDict{
		// FOR_SUBLOAD can't use reference to project location
		"derivation": derivation,
		"test":       starlark.NewBuiltin("test", rt.testBuiltin),
		"run":        starlark.NewBuiltin("run", rt.runBuiltin),
		"assert":     assertGlobals["assert"],
		"sys":        starlarkSys(target),
		"files": starlark.NewBuiltin("files", filesBuiltin{
			projectLocation: p.location,
		}.filesBuiltin),
	}
	return rt
}

func (rt *runtime) predeclatedWithPath(projectPath string) starlark.StringDict {
	derivation, err := rt.loadNativeDerivation(starlark.NewBuiltin("_derivation", rt.derivationFunction(projectPath)))
	if err != nil {
		repl.PrintError(err)
		panic(err)
	}
	return starlark.StringDict{
		"derivation": derivation,
		"test":       rt.predeclared["test"],
		"run":        rt.predeclared["run"],
		"assert":     rt.predeclared["assert"],
		"sys":        rt.predeclared["sys"],
		"files": starlark.NewBuiltin("files", filesBuiltin{
			projectLocation: projectPath,
		}.filesBuiltin),
	}
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
	project     *Project
	internalKey int64

	allDerivations map[string]Derivation
	tests          []Test

	cache map[string]*entry

	predeclared starlark.StringDict
}

func starlarkSys(target string) *starlarkstruct.Module {
	if target == "" {
		target = types.Platform()
	}
	return &starlarkstruct.Module{
		Name: "sys",
		Members: starlark.StringDict{
			"os":       starlark.String(stdruntime.GOOS),
			"arch":     starlark.String(stdruntime.GOARCH),
			"platform": starlark.String(types.Platform()),
			"target":   starlark.String(target),
		},
	}
}

func (p *Project) REPL() {
	rt := p.newRuntime("")
	repl.REPL(rt.newThread(context.TODO(), "repl"), rt.predeclared)
}

func (p *Project) relativePathFromConfig() string {
	relativePath, _ := filepath.Rel(p.location, p.wd)
	if relativePath == "." {
		// don't add a dot to the path
		return ""
	}
	return relativePath
}

func (rt *runtime) platform() string {
	return rt.predeclared["sys"].(*starlarkstruct.Module).Members["platform"].(starlark.String).GoString()
}

type entry struct {
	globals starlark.StringDict
	err     error
}

func (rt *runtime) starlarkExecFile(thread *starlark.Thread, filename, projectPath string) (globals starlark.StringDict, err error) {
	prog, err := rt.sourceStarlarkProgram(filename)
	if err != nil {
		return
	}
	// FOR_SUBLOAD
	g, err := prog.Init(thread, rt.predeclatedWithPath(projectPath))
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
