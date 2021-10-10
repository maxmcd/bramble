package command

import (
	"context"
	"fmt"
	"sync"

	build "github.com/maxmcd/bramble/internal/build"
	project "github.com/maxmcd/bramble/internal/project"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

type execModuleOptions struct {
	includeTests bool
}

func (b bramble) execModule(ctx context.Context, command string, args []string, ops execModuleOptions) (output project.ExecModuleOutput, err error) {
	var span trace.Span
	ctx, span = tracer.Start(ctx, "command.execModule "+command+" "+fmt.Sprintf("%q", args))
	defer span.End()

	if len(args) > 0 {
		// Building something specific
		return b.project.ExecModule(ctx, project.ExecModuleInput{
			Command:      command,
			Arguments:    args,
			IncludeTests: ops.includeTests,
		})
	}

	// Building everything in the project
	modules, err := b.project.FindAllModules()
	if err != nil {
		return output, err
	}
	output.AllDerivations = make(map[string]project.Derivation)
	output.Output = make(map[string]project.Derivation)
	output.Modules = make(map[string]map[string][]string)
	for _, module := range modules {
		o, err := b.project.ExecModule(ctx, project.ExecModuleInput{
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
		for m, fns := range o.Modules {
			// TODO: is it possible for different sets of functions to be
			// returned for a given module
			output.Modules[m] = fns
		}
	}
	return output, nil
}

type runBuildOptions struct {
	check        bool
	shell        bool
	includeTests bool
	callback     func(dep project.Dependency, drv project.Derivation, buildDrv build.Derivation)
}

func (b bramble) runBuild(ctx context.Context, output project.ExecModuleOutput, ops runBuildOptions) (outputDerivations []build.Derivation, err error) {
	var span trace.Span
	ctx, span = tracer.Start(ctx, "command.runBuild")
	defer span.End()
	// jobPrinter := jobprinter.New()

	// go func() { _ = jobPrinter.Start() }()
	// defer jobPrinter.Stop()

	if len(output.Output) != 1 && ops.shell {
		return nil, errors.New("Can't open a shell if the function doesn't return a single derivation")
	}
	builder := b.store.NewBuilder(false, b.project.LockfileWriter())
	derivationIDUpdates := map[project.Dependency]build.DerivationOutput{}
	var derivationDataLock sync.Mutex

	err = output.WalkAndPatch(8, func(dep project.Dependency, drv project.Derivation) (addGraph *project.ExecModuleOutput, buildOutputs []project.BuildOutput, err error) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		inputDerivations := []build.DerivationOutput{}

		// job := jobPrinter.StartJob(drv.Name)
		// defer jobPrinter.EndJob(job)
		fmt.Println("building", drv.Name, dep.Hash)
		derivationDataLock.Lock()
		// Populate the input derivation from previous builds
		for _, dep := range drv.Dependencies {
			do, found := derivationIDUpdates[dep]
			if !found {
				derivationDataLock.Unlock()
				return nil, nil, errors.Errorf("Missing build output for dep %q but we should have it", dep)
			}
			inputDerivations = append(inputDerivations, do)
		}
		derivationDataLock.Unlock()

		source, err := b.store.StoreLocalSources(ctx, build.SourceFiles{
			ProjectLocation: b.project.Location(),
			Location:        drv.Sources.Location,
			Files:           drv.Sources.Files,
		}) // TODO: delete this if the build fails?
		if err != nil {
			return nil, nil, errors.Wrap(err, "error moving local files to the store")
		}

		_, buildDrv, err := b.store.NewDerivation(build.NewDerivationOptions{
			Args:             drv.Args,
			Builder:          drv.Builder,
			Env:              drv.Env,
			InputDerivations: inputDerivations,
			Name:             drv.Name,
			Network:          drv.Network,
			Outputs:          drv.Outputs,
			Platform:         drv.Platform,
			Source:           source,
		})
		if err != nil {
			return nil, nil, err
		}
		var didBuild bool
		// start := time.Now()

		runShell := false
		if len(output.Output) == 1 && ops.shell {
			for k := range output.Output {
				// This is the output derivation being built!
				if k == dep.Hash {
					runShell = true
				}
			}
		}

		if buildDrv, didBuild, err = builder.BuildDerivation(ctx, buildDrv, build.BuildDerivationOptions{
			Shell:      runShell,
			ForceBuild: runShell,
		}); err != nil {
			return nil, nil, err
		}

		if ops.check {
			secondBuildDrv, _, err := builder.BuildDerivation(ctx, buildDrv, build.BuildDerivationOptions{
				ForceBuild: true,
			})
			if err != nil {
				return nil, nil, err
			}
			for i := 0; i < len(buildDrv.Outputs); i++ {
				a := buildDrv.Outputs[i].Path
				b := secondBuildDrv.Outputs[i].Path
				if a != b {
					return nil, nil, errors.Errorf(
						"Derivation %s is not reproducible, output %s had output %s first and %s second",
						buildDrv.Name,
						buildDrv.OutputNames[i],
						a, b,
					)
				}
			}
		}
		if ops.callback != nil {
			ops.callback(dep, drv, buildDrv)
		}
		_ = didBuild
		// ts := time.Since(start).String()
		if !didBuild && !ops.check {
			// job.ReplaceTS("(cached)")
		}
		fmt.Println("built", drv.Name, didBuild)
		// fmt.Printf("✔ %s - %s\n", buildDrv.Name, ts)
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
	if err := b.project.WriteLockfile(); err != nil {
		return nil, err
	}
	if err = b.store.WriteConfigLink(b.project.Location()); err != nil {
		return nil, err
	}
	return outputDerivations, err
}

type fullBuildOptions struct {
	check bool
}

func (b bramble) fullBuild(ctx context.Context, args []string, opts fullBuildOptions) (br buildResponse, err error) {
	br.FinalHashMapping = make(map[string]build.Derivation)
	br.Output, err = b.execModule(ctx, "builder.Build", args, execModuleOptions{})
	if err != nil {
		return
	}
	var lock sync.Mutex
	_, err = b.runBuild(ctx, br.Output, runBuildOptions{
		check: opts.check,
		callback: func(dep project.Dependency, drv project.Derivation, buildDrv build.Derivation) {
			lock.Lock()
			br.FinalHashMapping[dep.Hash] = buildDrv
			lock.Unlock()
		},
	})
	return
}

type buildResponse struct {
	Output           project.ExecModuleOutput
	FinalHashMapping map[string]build.Derivation
}

func (br buildResponse) moduleFunctionMapping() (mapping map[string]map[string][]string) {
	mapping = map[string]map[string][]string{}
	for module, functions := range br.Output.Modules {
		mapping[module] = map[string][]string{}
		for fn, derivations := range functions {
			for _, drv := range derivations {
				mapping[module][fn] = append(mapping[module][fn], br.FinalHashMapping[drv].Hash())
			}
		}
	}
	return mapping
}
