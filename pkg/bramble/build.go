package bramble

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	build "github.com/maxmcd/bramble/pkg/bramblebuild"
	project "github.com/maxmcd/bramble/pkg/brambleproject"

	"github.com/pkg/errors"
)

func (b bramble) runBuildFromOutput(output project.ExecModuleOutput) (outputDerivations []build.Derivation, err error) {
	return b.runBuild(buildOptions{}, func() (project.ExecModuleOutput, error) {
		return output, nil
	})
}

type buildOptions struct {
	Check bool
}

func (b bramble) runBuildFromCLI(command string, args []string, ops buildOptions) (outputDerivations []build.Derivation, err error) {
	return b.runBuild(ops, func() (output project.ExecModuleOutput, err error) {
		if len(args) > 0 {
			// Building something specific
			return b.project.ExecModule(project.ExecModuleInput{
				Command:   command,
				Arguments: args,
			})
		}

		// Building everything in the project
		modules, err := b.findAllModulesInProject()
		if err != nil {
			return output, err
		}
		output.AllDerivations = make(map[string]project.Derivation)
		output.Output = make(map[string]project.Derivation)
		for _, module := range modules {
			o, err := b.project.ExecModule(project.ExecModuleInput{
				Command:   command,
				Arguments: []string{module},
			})
			if err != nil {
				return output, err
			}
			for k, v := range o.AllDerivations {
				output.AllDerivations[k] = v
			}
			for k, v := range o.Output {
				output.Output[k] = v
			}
		}
		return output, nil
	})
}

func (b bramble) runBuild(ops buildOptions, execModule func() (project.ExecModuleOutput, error)) (outputDerivations []build.Derivation, err error) {
	output, err := execModule()
	if err != nil {
		return nil, err
	}

	builder := b.store.NewBuilder(false, b.project.URLHashes())

	derivationIDUpdates := map[project.Dependency]build.DerivationOutput{}
	// allDerivations := []build.Derivation{}
	var derivationDataLock sync.Mutex

	err = output.WalkAndPatch(8, func(dep project.Dependency, drv project.Derivation) (buildOutputs []project.BuildOutput, err error) {
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

		source, err := b.store.StoreLocalSources(build.SourceFiles{
			ProjectLocation: b.project.Location(),
			Location:        drv.Sources.Location,
			Files:           drv.Sources.Files,
		}) // TODO: delete this if the build fails?
		if err != nil {
			return nil, errors.Wrap(err, "error moving local files to the store")
		}

		_, buildDrv, err := b.store.NewDerivation(build.NewDerivationOptions{
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
		var didBuild bool
		start := time.Now()
		if buildDrv, didBuild, err = builder.BuildDerivation(context.Background(), buildDrv, build.BuildDerivationOptions{}); err != nil {
			return nil, err
		}

		if ops.Check {
			secondBuildDrv, _, err := builder.BuildDerivation(context.Background(), buildDrv, build.BuildDerivationOptions{
				ForceBuild: true,
			})
			if err != nil {
				return nil, err
			}
			for i := 0; i < len(buildDrv.Outputs); i++ {
				a := buildDrv.Outputs[i].Path
				b := secondBuildDrv.Outputs[i].Path
				if a != b {
					return nil, errors.Errorf(
						"Derivation %s is not reproducible, output %s had output %s first and %s second",
						buildDrv.Name,
						buildDrv.OutputNames[i],
						a, b,
					)
				}
			}

		}
		ts := time.Since(start).String()
		if !didBuild && !ops.Check {
			ts = "cached"
		}
		fmt.Printf("✔ %s - %s\n", buildDrv.Name, ts)
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

	err = b.project.AddURLHashesToLockfile(builder.URLHashes)
	if err != nil {
		return outputDerivations, err
	}
	_ = b.store.WriteConfigLink(b.project.Location())
	return outputDerivations, err
}

func (b bramble) findAllModulesInProject() (modules []string, err error) {
	return modules, filepath.Walk(b.project.Location(),
		func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if filepath.Base(path) == "bramble.toml" &&
				path != filepath.Join(b.project.Location(), "bramble.toml") {
				return filepath.SkipDir
			}
			// TODO: ignore .git, ignore .gitignore?
			if strings.HasSuffix(path, ".bramble") {
				module, err := b.project.FilepathToModuleName(path)
				if err != nil {
					return err
				}
				modules = append(modules, module)
			}
			return nil
		})
}
