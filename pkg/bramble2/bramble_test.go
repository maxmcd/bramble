package bramble

import (
	"fmt"
	"path/filepath"
	"testing"

	build "github.com/maxmcd/bramble/pkg/bramblebuild"
	project "github.com/maxmcd/bramble/pkg/brambleproject"
	ds "github.com/maxmcd/bramble/pkg/data_structures"
	"github.com/stretchr/testify/require"
)

func TestBasic(t *testing.T) {
	projectLocation, err := filepath.Abs("../brambleproject/testdata/project")
	require.NoError(t, err)
	gotOutput, err := project.ExecModule(project.ExecModuleInput{
		Command:   "build",
		Arguments: []string{":chain"},
		ProjectInput: project.ProjectInput{
			WorkingDirectory: projectLocation,
			ProjectLocation:  projectLocation,
			ModuleName:       "testproject",
		},
	})
	require.NoError(t, err)

	store, err := build.NewStore("")
	require.NoError(t, err)

	// Arrange the allDerivations so that you're starting with dependencies
	// without dependents. Crawl the grap, when you run NewDerivation above take
	// the hash and replace it in all child dependents. Then build that new
	// derivation. Then hand those derivations to a build function and build the
	// unbuilt derivations while patching up the graph again.
	//
	// Maybe think about a good generalization for building the graph and then
	// patching it up, would be great to stop implementing that everywhere.

	graph, err := gotOutput.BuildDependencyGraph()
	if err != nil {
		t.Fatal(err)
	}

	walkOpts := ds.WalkDerivationGraphOptions{}
	for _, edge := range graph.Edges() {
		walkOpts.Edges = append(walkOpts.Edges,
			ds.NewDerivationGraphEdge(
				edge.Source().(project.Dependency),
				edge.Target().(project.Dependency),
			),
		)
	}
	for _, drv := range gotOutput.AllDerivations {
		walkOpts.Derivations = append(walkOpts.Derivations, drv)
	}
	err = ds.WalkDerivationGraph(walkOpts, func(do ds.DerivationOutput, drv ds.DrvReplacable) (newHash string, err error) {
		projectDrv := drv.(project.Derivation)

		inputDerivations := []build.DerivationOutput{}
		for _, dep := range projectDrv.Dependencies() {
			inputDerivations = append(inputDerivations, build.DerivationOutput{
				Output:     dep.Hash(),
				OutputName: dep.Output(),
			})
		}
		_, buildDrv, err := store.NewDerivation2(build.NewDerivationOptions{
			Args:             projectDrv.Args(),
			Builder:          projectDrv.Builder(),
			Env:              projectDrv.Env(),
			InputDerivations: inputDerivations,
			Name:             projectDrv.Name(),
			Outputs:          projectDrv.Outputs(),
			Platform:         projectDrv.Platform(),
			Sources: build.SourceFiles{
				ProjectLocation: projectLocation,
				Location:        projectDrv.Sources().Location,
				Files:           projectDrv.Sources().Files,
			},
		})
		return buildDrv.Hash(), err
	})
	require.NoError(t, err)

	for _, drv := range gotOutput.AllDerivations {
		fmt.Println(drv.JSON())
	}

}

func TestAll(t *testing.T) {
	projectLocation, err := filepath.Abs("../..")
	require.NoError(t, err)
	gotOutput, err := project.ExecModule(project.ExecModuleInput{
		Command:   "build",
		Arguments: []string{"all:all"},
		ProjectInput: project.ProjectInput{
			WorkingDirectory: projectLocation,
			ProjectLocation:  projectLocation,
			ModuleName:       "github.com/maxmcd/bramble",
		},
	})
	require.NoError(t, err)

	store, err := build.NewStore("")
	require.NoError(t, err)

	// Arrange the allDerivations so that you're starting with dependencies
	// without dependents. Crawl the grap, when you run NewDerivation above take
	// the hash and replace it in all child dependents. Then build that new
	// derivation. Then hand those derivations to a build function and build the
	// unbuilt derivations while patching up the graph again.
	//
	// Maybe think about a good generalization for building the graph and then
	// patching it up, would be great to stop implementing that everywhere.

	graph, err := gotOutput.BuildDependencyGraph()
	if err != nil {
		t.Fatal(err)
	}

	walkOpts := ds.WalkDerivationGraphOptions{}
	for _, edge := range graph.Edges() {
		if edge.Source() == ds.FakeDAGRoot {
			walkOpts.Edges = append(walkOpts.Edges,
				ds.NewDerivationGraphFakeRoot(edge.Target().(project.Dependency)))
			continue
		}
		walkOpts.Edges = append(walkOpts.Edges,
			ds.NewDerivationGraphEdge(
				edge.Source().(project.Dependency),
				edge.Target().(project.Dependency),
			),
		)
	}
	for _, drv := range gotOutput.AllDerivations {
		walkOpts.Derivations = append(walkOpts.Derivations, drv)
	}
	err = ds.WalkDerivationGraph(walkOpts, func(do ds.DerivationOutput, drv ds.DrvReplacable) (newHash string, err error) {
		projectDrv := drv.(project.Derivation)

		inputDerivations := []build.DerivationOutput{}
		for _, dep := range projectDrv.Dependencies() {
			inputDerivations = append(inputDerivations, build.DerivationOutput{
				Output:     dep.Hash(),
				OutputName: dep.Output(),
			})
		}
		_, buildDrv, err := store.NewDerivation2(build.NewDerivationOptions{
			Args:             projectDrv.Args(),
			Builder:          projectDrv.Builder(),
			Env:              projectDrv.Env(),
			InputDerivations: inputDerivations,
			Name:             projectDrv.Name(),
			Outputs:          projectDrv.Outputs(),
			Platform:         projectDrv.Platform(),
			Sources: build.SourceFiles{
				ProjectLocation: projectLocation,
				Location:        projectDrv.Sources().Location,
				Files:           projectDrv.Sources().Files,
			},
		})
		return buildDrv.Hash(), err
	})
	require.NoError(t, err)

	for _, drv := range gotOutput.AllDerivations {
		fmt.Println(drv.JSON())
	}

}
