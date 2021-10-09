package project

import (
	"fmt"
	"path/filepath"

	"github.com/maxmcd/bramble/pkg/starutil"
	"go.starlark.net/starlark"
)

type Test struct {
	Derivation Derivation
	Args       []string
	Location   string
}

func (rt *runtime) testBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (out starlark.Value, err error) {
	// TODO do we need to run tests only in our project?
	var (
		test     Test
		testArgs *starlark.List
	)
	test.Location = thread.CallStack().At(1).Pos.String()
	if err = starlark.UnpackArgs("test", args, kwargs,
		"derivation", &test.Derivation,
		"testArgs", &testArgs,
	); err != nil {
		return
	}

	test.Args, err = starutil.IterableToStringSlice(testArgs)

	rt.tests = append(rt.tests, test)
	return starlark.None, err
}

type Run struct {
	Derivation    Derivation
	Cmd           string
	Args          []string
	Paths         []string
	ReadOnlyPaths []string
	HiddenPaths   []string
	Network       bool
	Location      string
}

var _ starlark.Value = Run{}

func (run Run) String() string {
	return fmt.Sprintf("<run %s %s %q>", run.Derivation.Name, run.Cmd, run.Args)
}
func (run Run) Type() string          { return "run" }
func (run Run) Freeze()               {}
func (run Run) Truth() starlark.Bool  { return true }
func (run Run) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("run") }

func (run Run) makePathsAbsolute() Run {
	pathHavers := [][]string{run.Paths, run.ReadOnlyPaths, run.HiddenPaths}
	for i, paths := range pathHavers {
		for j, path := range paths {
			if filepath.IsAbs(path) {
				continue
			}
			pathHavers[i][j] = filepath.Join(run.Location, path)
		}
	}
	return run
}

func (rt *runtime) runBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (out starlark.Value, err error) {
	// TODO do we need to run tests only in our project?
	var (
		run           = Run{Location: filepath.Dir(thread.CallStack().At(1).Pos.Filename())}
		runArgs       *starlark.List
		paths         *starlark.List
		readOnlyPaths *starlark.List
		hiddenPaths   *starlark.List
	)
	if err = starlark.UnpackArgs("run", args, kwargs,
		"derivation", &run.Derivation,
		"cmd", &run.Cmd,
		"args?", &runArgs,
		"paths?", &paths,
		"read_only_paths?", &readOnlyPaths,
		"hidden_paths?", &hiddenPaths,
		"network?", &run.Network,
	); err != nil {
		return
	}

	if runArgs != nil {
		if run.Args, err = starutil.IterableToStringSlice(runArgs); err != nil {
			return nil, err
		}
	}
	if paths != nil {
		if run.Paths, err = starutil.IterableToStringSlice(paths); err != nil {
			return nil, err
		}
	}
	if readOnlyPaths != nil {
		if run.ReadOnlyPaths, err = starutil.IterableToStringSlice(readOnlyPaths); err != nil {
			return nil, err
		}
	}
	if hiddenPaths != nil {
		if run.HiddenPaths, err = starutil.IterableToStringSlice(hiddenPaths); err != nil {
			return nil, err
		}
	}
	return run.makePathsAbsolute(), nil
}
