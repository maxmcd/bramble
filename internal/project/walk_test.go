package project

import (
	"context"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func sortLines(in string) (out string) {
	lines := strings.Split(in, "\n")
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func TestExecModuleOutput_WalkAndPatch(t *testing.T) {
	ctx := context.Background()
	project, err := NewProject("./testdata/project")

	execModule := func(name string) ExecModuleOutput {
		module, err := project.ParseModuleFuncArgument(context.Background(), name, false)
		if err != nil {
			t.Fatal(err)
		}
		out, err := project.ExecModule(ctx, ExecModuleInput{
			Module: module,
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	firstGraph := execModule("./:first_graph")
	replaceCWith := execModule("./:replace_c_with")
	expectedResult := execModule("./:expected_result")

	expectedWalker, err := expectedResult.newWalker()

	outputWalker, err := firstGraph.walkAndPatch(1, func(dep Dependency, drv Derivation) (
		addGraph *ExecModuleOutput,
		buildOutputs []BuildOutput, err error) {
		if drv.Name == "c" {
			return &replaceCWith, nil, nil
		}
		return
	})
	require.NoError(t, err)
	require.Equal(t,
		sortLines(outputWalker.stringDot(outputWalker.graph)),
		sortLines(expectedWalker.stringDot(expectedWalker.graph)),
	)
}

func TestExecModuleAndWalk(t *testing.T) {
	project, err := NewProject(".")
	require.NoError(t, err)

	module, err := project.ParseModuleFuncArgument(context.Background(), "github.com/maxmcd/bramble/tests:all", false)
	if err != nil {
		t.Fatal(err)
	}
	gotOutput, err := project.ExecModule(context.Background(), ExecModuleInput{
		Module: module,
	})
	if err != nil {
		t.Fatal(err)
	}

	allDerivations := []Derivation{}
	allDrvLock := sync.Mutex{}
	require.NoError(t, gotOutput.WalkAndPatch(0, func(dep Dependency, drv Derivation) (addGraph *ExecModuleOutput, buildOutputs []BuildOutput, err error) {
		allDrvLock.Lock()
		allDerivations = append(allDerivations, drv)
		allDrvLock.Unlock()
		for _, name := range drv.Outputs {
			buildOutputs = append(buildOutputs, BuildOutput{Dep: Dependency{
				Hash:   dep.Hash,
				Output: name,
			}, OutputPath: "/this/is/how/it/goes/now"})
		}
		return nil, buildOutputs, nil
	}))

	for _, drv := range allDerivations {
		// All template strings should have been replaced
		require.NotContains(t, drv.prettyJSON(), "{{ ")
	}
}
