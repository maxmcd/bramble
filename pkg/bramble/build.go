package bramble

import (
	"context"
	"sync"

	build "github.com/maxmcd/bramble/pkg/bramblebuild"
	project "github.com/maxmcd/bramble/pkg/brambleproject"

	"github.com/pkg/errors"
)

func runBuildFromOutput(output project.ExecModuleOutput) (outputDerivations []build.Derivation, err error) {
	return runBuild(func(p *project.Project) (project.ExecModuleOutput, error) {
		return output, nil
	})
}

func runBuildFromCLI(command string, args []string) (outputDerivations []build.Derivation, err error) {
	return runBuild(func(p *project.Project) (output project.ExecModuleOutput, err error) {
		return p.ExecModule(project.ExecModuleInput{
			Command:   command,
			Arguments: args,
		})
	})
}

func runBuild(execModule func(*project.Project) (project.ExecModuleOutput, error)) (outputDerivations []build.Derivation, err error) {
	p, err := project.NewProject(".")
	if err != nil {
		return nil, err
	}

	output, err := execModule(p)
	if err != nil {
		return nil, err
	}

	store, err := build.NewStore("")
	if err != nil {
		return nil, err
	}
	store.RegisterGetGit(getGit)

	builder := store.NewBuilder(false, p.URLHashes())

	derivationIDUpdates := map[project.Dependency]build.DerivationOutput{}
	// allDerivations := []build.Derivation{}
	derivationDataLock := sync.Mutex{}

	err = output.WalkAndPatch(1, func(dep project.Dependency, drv project.Derivation) (buildOutputs []project.BuildOutput, err error) {
		inputDerivations := []build.DerivationOutput{}

		derivationDataLock.Lock()
		// Populate the input derivation from previous buids
		for _, dep := range drv.Dependencies {
			do, found := derivationIDUpdates[dep]
			if !found {
				derivationDataLock.Unlock()
				return nil, errors.Errorf("Missing build output for dep %q but we should have it", dep)
			}
			inputDerivations = append(inputDerivations, do)
		}
		derivationDataLock.Unlock()

		source, err := store.StoreLocalSources(build.SourceFiles{
			ProjectLocation: p.Location(),
			Location:        drv.Sources.Location,
			Files:           drv.Sources.Files,
		}) // TODO: delete this if the build fails?
		if err != nil {
			return
		}

		_, buildDrv, err := store.NewDerivation(build.NewDerivationOptions{
			Args:             drv.Args,
			Builder:          drv.Builder,
			Env:              drv.Env,
			InputDerivations: inputDerivations,
			Name:             drv.Name,
			Outputs:          drv.Outputs,
			Platform:         drv.Platform,
			Source:           source,
		})
		if err != nil {
			return nil, err
		}
		if buildDrv, _, err = builder.BuildDerivation(context.Background(), buildDrv); err != nil {
			return nil, err
		}

		derivationDataLock.Lock()
		// allDerivations = append(allDerivations, buildDrv)
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
		for hash := range output.Output {
			if hash == dep.Hash {
				outputDerivations = append(outputDerivations, buildDrv)
			}
		}
		derivationDataLock.Unlock()
		return
	})
	if err != nil {
		return nil, err
	}

	err = p.AddURLHashesToLockfile(builder.URLHashes)
	if err != nil {
		return outputDerivations, err
	}
	return outputDerivations, err
}
