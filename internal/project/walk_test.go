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
	require.NoError(t, err)
	firstGraph, err := project.ExecModule(ctx, ExecModuleInput{
		Arguments: []string{":first_graph"},
	})

	require.NoError(t, err)
	replaceCWith, err := project.ExecModule(ctx, ExecModuleInput{
		Arguments: []string{":replace_c_with"},
	})
	require.NoError(t, err)
	expectedResult, err := project.ExecModule(ctx, ExecModuleInput{
		Arguments: []string{":expected_result"},
	})

	require.NoError(t, err)

	expectedWalker, err := expectedResult.newWalker()
	require.NoError(t, err)

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

	gotOutput, err := project.ExecModule(context.Background(), ExecModuleInput{
		Command:   "build",
		Arguments: []string{"github.com/maxmcd/bramble/all:all"},
	})
	require.NoError(t, err)

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
