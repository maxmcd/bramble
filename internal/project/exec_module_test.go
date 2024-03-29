package project

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExecModule(t *testing.T) {
	project, err := NewProject("./testdata/project")
	require.NoError(t, err)
	type output struct {
		output []string
		all    []string
	}
	tests := []struct {
		name       string
		arg        string
		wantOutput output
		wantErr    bool
	}{
		{
			arg: ".:chain",
			wantOutput: output{
				output: []string{"c"},
				all:    []string{"a", "b", "c"},
			},
		},
		{
			arg: "./:foo",
			wantOutput: output{
				output: []string{"name"},
				all:    []string{"example.com", "name"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modules, err := project.ArgumentsToModules(context.Background(), []string{tt.arg}, false)
			if err != nil {
				t.Fatal(err)
			}
			gotOutput, err := project.ExecModule(context.Background(), ExecModuleInput{
				Module: modules[0],
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
