package brambleproject

// import (
// 	"fmt"
// 	"io/ioutil"
// 	"os"
// 	"path/filepath"
// 	"strings"
// 	"sync"

// 	"github.com/maxmcd/bramble/pkg/bramblebuild"
// 	"github.com/maxmcd/bramble/pkg/dstruct"
// 	"github.com/maxmcd/bramble/pkg/fileutil"
// 	"github.com/maxmcd/bramble/pkg/logger"
// 	"github.com/maxmcd/dag"
// 	"github.com/pkg/errors"
// 	"go.starlark.net/starlark"
// )

// func (rt *Runtime) GC() (err error) {
// 	derivations, err := rt.collectDerivationsToPreserve()
// 	if err != nil {
// 		return
// 	}
// 	pathsToKeep := map[string]struct{}{}
// 	graphs := []*dstruct.AcyclicGraph{}
// 	for _, drv := range derivations {
// 		graph, err := drv.BuildDependencyGraph()
// 		if err != nil {
// 			return err
// 		}
// 		graphs = append(graphs, graph)
// 	}
// 	graph := dstruct.MergeGraphs(graphs...)
// 	var lock sync.Mutex
// 	errors := graph.Walk(func(v dag.Vertex) error {
// 		lock.Lock() // Serialize
// 		defer lock.Unlock()
// 		if v == dstruct.FakeDAGRoot {
// 			return nil
// 		}
// 		do := v.(DerivationOutput)
// 		drv, err := store.LoadDerivation(do.Filename)
// 		if drv == nil {
// 			return nil
// 		}
// 		if err != nil {
// 			return err
// 		}

// 		// Fetch outputs from disk with new filename hash
// 		_, err = drv.PopulateOutputsFromStore()
// 		if err != nil {
// 			return err
// 		}

// 		for _, f := range append(drv.inputFiles(), drv.runtimeFiles(do.OutputName)...) {
// 			pathsToKeep[f] = struct{}{}
// 		}

// 		oldTemplateName := fmt.Sprintf(UnbuiltDerivationOutputTemplate, do.Filename, do.OutputName)
// 		newTemplateName := drv.String()
// 		for _, edge := range graph.EdgesTo(v) {
// 			if edge.Source() == dstruct.FakeDAGRoot {
// 				continue
// 			}
// 			childDO := edge.Source().(DerivationOutput)
// 			childDRV := b.derivations.Load(childDO.Filename)
// 			if childDRV == nil {
// 				continue
// 			}
// 			for i, input := range childDRV.InputDerivations {
// 				// Add the output to the derivation input
// 				if input.Filename == do.Filename && input.OutputName == do.OutputName {
// 					childDRV.InputDerivations[i].Output = drv.Output(do.OutputName).Path
// 				}
// 			}
// 			if err := childDRV.replaceValueInDerivation(oldTemplateName, newTemplateName); err != nil {
// 				panic(err)
// 			}
// 		}
// 		return nil
// 	})
// 	if len(errors) != 0 {
// 		panic(errors)
// 	}

// 	// delete everything in the store that's not in the map
// 	files, err := os.ReadDir(store.StorePath)
// 	if err != nil {
// 		return
// 	}
// 	for _, file := range files {
// 		if _, ok := pathsToKeep[file.Name()]; ok {
// 			continue
// 		}
// 		logger.Print("deleting", file.Name())
// 		if err = os.RemoveAll(store.JoinStorePath(file.Name())); err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }

// // REFAC: since this covers store and projects consider moving a lot of the logic into the bramble package

// func (rt *Runtime) collectDerivationsToPreserve() (derivations []*Derivation, err error) {
// 	registryFolder := rt.store.JoinBramblePath("var", "config-registry")
// 	files, err := ioutil.ReadDir(registryFolder)
// 	if err != nil {
// 		return
// 	}

// 	for _, f := range files {
// 		registryLoc := filepath.Join(registryFolder, f.Name())
// 		pathBytes, err := ioutil.ReadFile(registryLoc)
// 		if err != nil {
// 			return nil, err
// 		}
// 		path := string(pathBytes)
// 		if !fileutil.FileExists(filepath.Join(path, "bramble.toml")) {
// 			logger.Printfln("deleting cache for %q, it no longer exists", path)
// 			_ = os.Remove(registryLoc)
// 			continue
// 		}

// 		drvs, err := findAllDerivationsInProject(path)
// 		if err != nil {
// 			// TODO: this is heavy handed, would mean any syntax error in any
// 			// project prevents a global gc, think about how to deal with this
// 			return nil, errors.Wrapf(err, "error computing derivations in %q", registryLoc)
// 		}

// 		if len(drvs) == 0 {
// 			continue
// 		}
// 		// Grab derivation cache from other projects and add to ours
// 		drvs[0].derivations.Range(func(m map[string]*Derivation) {
// 			for filename, drv := range m {
// 				// Replace bramble with our bramble
// 				drv.bramble = b
// 				b.derivations.Store(filename, drv)
// 			}
// 		})
// 		derivations = append(derivations, drvs...)
// 	}
// 	return
// }

// func findAllDerivationsInProject(loc string) (derivations []*Derivation, err error) {
// 	project, err := NewProject(loc)
// 	if err != nil {
// 		return nil, err
// 	}

// 	store, err := bramblebuild.NewStore("")
// 	if err != nil {
// 		return nil, err
// 	}
// 	rt, err := NewRuntime(project, store)
// 	if err != nil {
// 		return nil, err
// 	}
// 	if err := filepath.Walk(project.Location, func(path string, fi os.FileInfo, err error) error {
// 		if err != nil {
// 			return err
// 		}
// 		// TODO: ignore .git, ignore .gitignore?
// 		if strings.HasSuffix(path, ".bramble") {
// 			module, err := project.FilepathToModuleName(path)
// 			if err != nil {
// 				return err
// 			}
// 			globals, err := rt.resolveModule(module)
// 			if err != nil {
// 				return err
// 			}
// 			for name, v := range globals {
// 				if fn, ok := v.(*starlark.Function); ok {
// 					if fn.NumParams()+fn.NumKwonlyParams() > 0 {
// 						continue
// 					}
// 					fn.NumParams()
// 					value, err := starlark.Call(rt.thread, fn, nil, nil)
// 					if err != nil {
// 						return errors.Wrapf(err, "calling %q in %s", name, path)
// 					}
// 					derivations = append(derivations, valuesToDerivations(value)...)
// 				}
// 			}
// 		}
// 		return nil
// 	}); err != nil {
// 		return nil, err
// 	}
// 	return
// }
