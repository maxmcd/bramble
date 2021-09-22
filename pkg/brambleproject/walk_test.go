package brambleproject

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWalkGCCHello(t *testing.T) {
	project, err := NewProject("./testdata/project")
	require.NoError(t, err)
	rw, err := project.ExecModule(ExecModuleInput{
		Arguments: []string{":expanded_compile"},
	})
	require.NoError(t, err)
	w, err := rw.newWalker()
	require.NoError(t, err)
	w.printDot(w.graph)
}

func TestExecModuleOutput_WalkAndPatch(t *testing.T) {
	project, err := NewProject("./testdata/project")
	require.NoError(t, err)
	firstGraph, err := project.ExecModule(ExecModuleInput{
		Arguments: []string{":first_graph"},
	})
	require.NoError(t, err)
	replaceCWith, err := project.ExecModule(ExecModuleInput{
		Arguments: []string{":replace_c_with"},
	})
	require.NoError(t, err)
	expectedResult, err := project.ExecModule(ExecModuleInput{
		Arguments: []string{":expected_result"},
	})
	require.NoError(t, err)
	_ = replaceCWith
	_ = expectedResult

	w, err := expectedResult.newWalker()
	require.NoError(t, err)
	w.printDot(w.graph)

	require.NoError(t, firstGraph.WalkAndPatch(1, func(dep Dependency, drv Derivation) (
		addGraph *ExecModuleOutput,
		buildOutputs []BuildOutput, err error) {
		fmt.Println(dep.Hash, drv.Name)
		if drv.Name == "c" {
			return &replaceCWith, nil, nil
		}
		return
	}))
}

func TestExecModuleAndWalk(t *testing.T) {
	project, err := NewProject(".")
	require.NoError(t, err)

	gotOutput, err := project.ExecModule(ExecModuleInput{
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
