package bramble

import (
	"path/filepath"
	"testing"

	"github.com/davecgh/go-spew/spew"
	build "github.com/maxmcd/bramble/pkg/bramblebuild"
	project "github.com/maxmcd/bramble/pkg/brambleproject"
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
	a := gotOutput.AllDerivations[0]

	store, err := build.NewStore("")
	require.NoError(t, err)

	exists, drv, err := store.NewDerivation2(build.NewDerivationOptions{
		Args:    a.Args,
		Builder: a.Builder,
		Env:     a.Env,
		// InputDerivations: , TODO
		Name:     a.Name,
		Outputs:  a.Outputs,
		Platform: a.Platform,
		Sources: build.SourceFiles{
			ProjectLocation: projectLocation,
			Location:        a.Sources.Location,
			Files:           a.Sources.Files,
		},
	})
	require.NoError(t, err)
	spew.Dump(exists, drv)

	// Arrange the allDerivations so that you're starting with dependencies
	// without dependents. Crawl the grap, when you run NewDerivation above take
	// the hash and replace it in all child dependents. Then build that new
	// derivation. Then hand those derivations to a build function and build the
	// unbuilt derivations while patching up the graph again.
	//
	// Maybe think about a good generalization for building the graph and then
	// patching it up, would be great to stop implementing that everywhere.
}
