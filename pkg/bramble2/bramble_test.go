package bramble

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	build "github.com/maxmcd/bramble/pkg/bramblebuild"
	project "github.com/maxmcd/bramble/pkg/brambleproject"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

// func TestBasic(t *testing.T) {
// 	projectLocation, err := filepath.Abs("../brambleproject/testdata/project")
// 	require.NoError(t, err)
// 	gotOutput, err := project.ExecModule(project.ExecModuleInput{
// 		Command:   "build",
// 		Arguments: []string{":chain"},
// 		ProjectInput: project.ProjectInput{
// 			WorkingDirectory: projectLocation,
// 			ProjectLocation:  projectLocation,
// 			ModuleName:       "testproject",
// 		},
// 	})
// 	require.NoError(t, err)

// 	store, err := build.NewStore("")
// 	require.NoError(t, err)

// 	// Arrange the allDerivations so that you're starting with dependencies
// 	// without dependents. Crawl the grap, when you run NewDerivation above take
// 	// the hash and replace it in all child dependents. Then build that new
// 	// derivation. Then hand those derivations to a build function and build the
// 	// unbuilt derivations while patching up the graph again.
// 	//
// 	// Maybe think about a good generalization for building the graph and then
// 	// patching it up, would be great to stop implementing that everywhere.

// 	graph, err := gotOutput.BuildDependencyGraph()
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	walkOpts := ds.WalkDerivationGraphOptions{}
// 	for _, edge := range graph.Edges() {
// 		walkOpts.Edges = append(walkOpts.Edges,
// 			ds.NewDerivationGraphEdge(
// 				edge.Source().(project.Dependency),
// 				edge.Target().(project.Dependency),
// 			),
// 		)
// 	}
// 	for _, drv := range gotOutput.AllDerivations {
// 		walkOpts.Derivations = append(walkOpts.Derivations, &drv)
// 	}
// 	err = ds.WalkDerivationGraph(walkOpts, func(do ds.DerivationOutput, drv ds.DrvReplacable) (newHash string, err error) {
// 		projectDrv := drv.(*project.Derivation)

// 		inputDerivations := []build.DerivationOutput{}
// 		for _, dep := range projectDrv.Dependencies() {
// 			inputDerivations = append(inputDerivations, build.DerivationOutput{
// 				Output:     dep.Hash(),
// 				OutputName: dep.Output(),
// 			})
// 		}
// 		_, buildDrv, err := store.NewDerivation2(build.NewDerivationOptions{
// 			Args:             projectDrv.Args(),
// 			Builder:          projectDrv.Builder(),
// 			Env:              projectDrv.Env(),
// 			InputDerivations: inputDerivations,
// 			Name:             projectDrv.Name(),
// 			Outputs:          projectDrv.Outputs(),
// 			Platform:         projectDrv.Platform(),
// 			Sources: build.SourceFiles{
// 				ProjectLocation: projectLocation,
// 				Location:        projectDrv.Sources().Location,
// 				Files:           projectDrv.Sources().Files,
// 			},
// 		})
// 		return buildDrv.Hash(), err
// 	})
// 	require.NoError(t, err)

// 	for _, drv := range gotOutput.AllDerivations {
// 		fmt.Println(drv.JSON())
// 	}

// }

func TestAll(t *testing.T) {
	projectLocation, err := filepath.Abs("../..")
	require.NoError(t, err)
	fmt.Println(projectLocation)

	p, err := project.NewProject(".")
	require.NoError(t, err)

	gotOutput, err := project.ExecModule(project.ExecModuleInput{
		Command:   "build",
		Arguments: []string{"all:all"},
		// Arguments: []string{"lib:busybox"},
		ProjectInput: project.ProjectInput{
			WorkingDirectory: projectLocation,
			ProjectLocation:  projectLocation,
			ModuleName:       "github.com/maxmcd/bramble",
		},
	})
	require.NoError(t, err)

	store, err := build.NewStore("")
	require.NoError(t, err)

	builder := store.NewBuilder(false, p.URLHashes())

	derivationIDUpdates := map[project.Dependency]build.DerivationOutput{}
	allDerivations := []project.Derivation{}

	err = gotOutput.WalkAndPatch(1, func(dep project.Dependency, drv project.Derivation) (buildOutputs []project.BuildOutput, err error) {
		allDerivations = append(allDerivations, drv)
		inputDerivations := []build.DerivationOutput{}
		for _, dep := range drv.Dependencies {
			do, found := derivationIDUpdates[dep]
			if !found {
				return nil, errors.Errorf("Missing build output for dep %q but we should have it", dep)
			}
			inputDerivations = append(inputDerivations, do)
		}

		_, buildDrv, err := store.NewDerivation2(build.NewDerivationOptions{
			Args:             drv.Args,
			Builder:          drv.Builder,
			Env:              drv.Env,
			InputDerivations: inputDerivations,
			Name:             drv.Name,
			Outputs:          drv.Outputs,
			Platform:         drv.Platform,
			Sources: build.SourceFiles{
				ProjectLocation: projectLocation,
				Location:        drv.Sources.Location,
				Files:           drv.Sources.Files,
			},
		})
		fmt.Println(buildDrv.PrettyJSON())
		if _, err := builder.BuildDerivationIfNew(context.Background(), buildDrv); err != nil {
			return nil, err
		}
		fmt.Println(buildDrv.PrettyJSON())
		for i, o := range buildDrv.OutputNames {
			out := buildDrv.Outputs[i]
			derivationIDUpdates[project.Dependency{
				Hash:   dep.Hash,
				Output: o,
			}] = build.DerivationOutput{
				Filename:   buildDrv.Filename(),
				OutputName: o,
				Output:     out.Path,
			}
			buildOutputs = append(buildOutputs, project.BuildOutput{
				Dep:        project.Dependency{Hash: dep.Hash, Output: o},
				OutputPath: build.BramblePrefixOfRecord + "/" + out.Path,
			})
		}
		return
	})
	require.NoError(t, err)

	for _, drv := range allDerivations {
		fmt.Println(drv.PrettyJSON())
	}
}
