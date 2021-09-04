package brambleproject

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.starlark.net/repl"
	"go.starlark.net/starlark"
)

func TestTestProject(t *testing.T) {
	projectLocation, err := filepath.Abs("./testdata/project")
	require.NoError(t, err)

	out, err := ExecModule(ExecModuleInput{
		Command:   "build",
		Arguments: []string{":foo"},
		ProjectInput: ProjectInput{
			WorkingDirectory: projectLocation,
			ProjectLocation:  projectLocation,
			ModuleName:       "testproject",
		},
	})
	fmt.Println(out, err)
}

func TestAllFunctions(t *testing.T) {
	rt := newTestRuntime(t)
	require.NoError(t, filepath.Walk(rt.projectLocation, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() && fi.Name() == "testdata" {
			return filepath.SkipDir
		}
		// TODO: ignore .git, ignore .gitignore?
		if !strings.HasSuffix(path, ".bramble") {
			return nil
		}
		module, err := rt.filepathToModuleName(path)
		if err != nil {
			return err
		}
		globals, err := rt.execModule(module)
		if err != nil {
			return err
		}
		for name, v := range globals {
			if fn, ok := v.(*starlark.Function); ok {
				if fn.NumParams()+fn.NumKwonlyParams() > 0 {
					continue
				}
				if _, err := starlark.Call(rt.newThread("test"), fn, nil, nil); err != nil {
					repl.PrintError(err)
					return errors.Wrapf(err, "calling %q in %s", name, path)
				}
			}
		}
		return nil
	}))
}
