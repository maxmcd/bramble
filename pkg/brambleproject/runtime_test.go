package brambleproject

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.starlark.net/repl"
	"go.starlark.net/starlark"
)

func TestExecModule(t *testing.T) {
	projectLocation, err := filepath.Abs("./testdata/project")
	require.NoError(t, err)

	type output struct {
		output []string
		all    []string
	}
	tests := []struct {
		name       string
		args       []string
		wantOutput output
		wantErr    bool
	}{
		{
			args: []string{":chain"},
			wantOutput: output{
				output: []string{"c"},
				all:    []string{"a", "b", "c"},
			},
		},
		{
			args: []string{":foo"},
			wantOutput: output{
				output: []string{"name"},
				all:    []string{"example.com", "name"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOutput, err := ExecModule(ExecModuleInput{
				Command:   "build",
				Arguments: tt.args,
				ProjectInput: ProjectInput{
					WorkingDirectory: projectLocation,
					ProjectLocation:  projectLocation,
					ModuleName:       "testproject",
				},
			})
			if (err != nil) != tt.wantErr {
				t.Errorf("ExecModule() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			reducedOutput := output{}
			for _, drv := range gotOutput.AllDerivations {
				reducedOutput.all = append(reducedOutput.all, drv.Name())
			}
			for _, drv := range gotOutput.Output {
				reducedOutput.output = append(reducedOutput.output, drv.Name())
			}
			sort.Strings(reducedOutput.all)
			sort.Strings(reducedOutput.output)

			if !reflect.DeepEqual(reducedOutput, tt.wantOutput) {
				t.Errorf("ExecModule() = %v, want %v", reducedOutput, tt.wantOutput)
			}
		})
	}
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
