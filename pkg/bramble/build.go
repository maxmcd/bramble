package bramble

import (
	"context"

	build "github.com/maxmcd/bramble/pkg/bramblebuild"
	project "github.com/maxmcd/bramble/pkg/brambleproject"

	"github.com/pkg/errors"
)

func runBuild(command string, args []string) error {
	p, err := project.NewProject(".")
	if err != nil {
		return err
	}

	output, err := p.ExecModule(project.ExecModuleInput{
		Command:   command,
		Arguments: args,
	})
	if err != nil {
		return err
	}

	store, err := build.NewStore("")
	if err != nil {
		return err
	}

	builder := store.NewBuilder(false, p.URLHashes())

	derivationIDUpdates := map[project.Dependency]build.DerivationOutput{}
	allDerivations := []project.Derivation{}

	err = output.WalkAndPatch(1, func(dep project.Dependency, drv project.Derivation) (buildOutputs []project.BuildOutput, err error) {
		allDerivations = append(allDerivations, drv)
		inputDerivations := []build.DerivationOutput{}

		// Populate the input derivation from previous buids
		for _, dep := range drv.Dependencies {
			do, found := derivationIDUpdates[dep]
			if !found {
				return nil, errors.Errorf("Missing build output for dep %q but we should have it", dep)
			}
			inputDerivations = append(inputDerivations, do)
		}

		_, buildDrv, err := store.NewDerivation(build.NewDerivationOptions{
			Args:             drv.Args,
			Builder:          drv.Builder,
			Env:              drv.Env,
			InputDerivations: inputDerivations,
			Name:             drv.Name,
			Outputs:          drv.Outputs,
			Platform:         drv.Platform,
			Sources: build.SourceFiles{
				ProjectLocation: p.Location(),
				Location:        drv.Sources.Location,
				Files:           drv.Sources.Files,
			},
		})
		if err != nil {
			return nil, err
		}
		if buildDrv, _, err = builder.BuildDerivation(context.Background(), buildDrv); err != nil {
			return nil, err
		}
		// Store the derivation outputs in the map for reference when building
		// input derivations later. Also populate the buildOutputs
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
	if err != nil {
		return err
	}

	err = p.AddURLHashesToLockfile(builder.URLHashes)
	if err != nil {
		return err
	}
	return nil
}
