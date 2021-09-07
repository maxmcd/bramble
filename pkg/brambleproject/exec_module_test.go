package brambleproject

import (
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
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
				reducedOutput.all = append(reducedOutput.all, drv.Name)
			}
			for _, drv := range gotOutput.Output {
				reducedOutput.output = append(reducedOutput.output, drv.Name)
			}
			sort.Strings(reducedOutput.all)
			sort.Strings(reducedOutput.output)

			if !reflect.DeepEqual(reducedOutput, tt.wantOutput) {
				t.Errorf("ExecModule() = %v, want %v", reducedOutput, tt.wantOutput)
			}
		})
	}
}

func TestExecModuleAndWalk(t *testing.T) {
	projectLocation, err := filepath.Abs("../..")
	require.NoError(t, err)

	gotOutput, err := ExecModule(ExecModuleInput{
		Command:   "build",
		Arguments: []string{"github.com/maxmcd/bramble/tests/simple/simple:simple"},
		ProjectInput: ProjectInput{
			WorkingDirectory: projectLocation,
			ProjectLocation:  projectLocation,
			ModuleName:       "github.com/maxmcd/bramble",
		},
	})
	require.NoError(t, err)

	allDerivations := []Derivation{}
	require.NoError(t, gotOutput.WalkAndPatch(0, func(dep Dependency, drv Derivation) (buildOutputs []BuildOutput, err error) {
		allDerivations = append(allDerivations, drv)
		for _, name := range drv.Outputs {
			buildOutputs = append(buildOutputs, BuildOutput{Dep: Dependency{
				Hash:   dep.Hash,
				Output: name,
			}, OutputPath: "/this/is/how/it/goes/now"})
		}
		return buildOutputs, nil
	}))

	for _, drv := range allDerivations {
		// All template strings should have been replaced
		require.NotContains(t, drv.prettyJSON(), "{{ ")
	}
}
