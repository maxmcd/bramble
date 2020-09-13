package bramble

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/hashicorp/terraform/dag"
	"github.com/pkg/errors"

	"github.com/maxmcd/bramble/pkg/assert"
	"github.com/maxmcd/bramble/pkg/starutil"
	"go.starlark.net/starlark"
)

type Bramble struct {
	thread      *starlark.Thread
	predeclared starlark.StringDict

	config         Config
	configLocation string
	lockFile       LockFile
	lockFileLock   sync.Mutex

	derivationFn *DerivationFunction
	cmd          *CmdFunction
	session      *session

	store Store

	moduleCache         map[string]string
	afterDerivation     bool
	derivationCallCount int

	moduleEntrypoint string
	calledFunction   string
}

// AfterDerivation is called when a builtin function is called that can't be
// run before an derivation call. This allows us to track when
func (b *Bramble) AfterDerivation() { b.afterDerivation = true }
func (b *Bramble) CalledDerivation() error {
	b.derivationCallCount++
	if b.afterDerivation {
		return errors.New("build context is dirty, can't call derivation after cmd() or other builtins")
	}
	return nil
}

func (b *Bramble) buildDerivations(drvs []*Derivation) (err error) {
	graph := NewAcyclicGraph()
	_ = graph
	root := "root"
	graph.Add(root)
	var processDO func(do DerivationOutput)
	processDO = func(do DerivationOutput) {
		drv, ok := b.derivationFn.derivations[do.Filename]
		if !ok {
			panic(do)
		}
		for _, inputDO := range drv.InputDerivations {
			graph.Add(inputDO)
			graph.Connect(dag.BasicEdge(do, inputDO))
			processDO(inputDO) // TODO, not recursive
		}
	}
	fmt.Println(drvs)
	for _, drv := range drvs {
		filename := drv.filename()
		for _, do := range drv.InputDerivations {
			graph.Add(do)
		}
		for _, name := range drv.OutputNames {
			vertex := DerivationOutput{Filename: filename, OutputName: name}
			graph.Add(vertex)
			graph.Connect(dag.BasicEdge(root, vertex))

			// All inputs are inputs of all outputs
			for _, do := range drv.InputDerivations {
				graph.Connect(dag.BasicEdge(vertex, do))
				processDO(do)
			}
		}
	}

	fmt.Println(string(graph.Dot(nil)))
	return nil
	// TODO!
}

func (b *Bramble) CallInlineDerivationFunction(meta functionBuilderMeta, session *session) (err error) {
	newBramble := &Bramble{
		// retain from parent
		config:         b.config,
		configLocation: b.configLocation,
		store:          b.store,

		// populate for this task
		moduleEntrypoint:    meta.Module,
		calledFunction:      meta.Function,
		moduleCache:         meta.ModuleCache,
		derivationCallCount: 0,
		session:             session,
	}
	newBramble.thread = &starlark.Thread{Load: newBramble.load}
	// this will pass the session to cmd and os
	if err = newBramble.initPredeclared(); err != nil {
		return
	}
	newBramble.derivationFn.DerivationCallCount = meta.DerivationCallCount

	globals, err := newBramble.resolveModule(meta.Module)
	if err != nil {
		return
	}

	_, intentionalError := starlark.Call(
		newBramble.thread,
		globals[meta.Function].(*starlark.Function),
		nil, nil,
	)
	fn := intentionalError.(*starlark.EvalError).Unwrap().(ErrFoundBuildContext).Fn
	_, err = starlark.Call(newBramble.thread, fn, nil, nil)
	return
}

func (b *Bramble) reset() {
	b.moduleCache = map[string]string{}
	b.derivationCallCount = 0
}

func (b *Bramble) init() (err error) {
	if b.configLocation != "" {
		return errors.New("can't initialize Bramble twice")
	}

	b.moduleCache = map[string]string{}

	if b.store, err = NewStore(); err != nil {
		return
	}

	// ensures we have a bramble.toml in the current or parent dir
	if b.config, b.lockFile, b.configLocation, err = findConfig(); err != nil {
		return
	}

	b.thread = &starlark.Thread{
		Name: "main",
		Load: b.load,
	}
	if b.session, err = newSession("", nil); err != nil {
		return
	}

	return b.initPredeclared()
}

func (b *Bramble) initPredeclared() (err error) {
	if b.derivationFn != nil {
		return errors.New("can't init predeclared twice")
	}
	// creates the derivation function and checks we have a valid bramble path and store
	b.derivationFn, err = NewDerivationFunction(b)
	if err != nil {
		return
	}

	assertGlobals, err := assert.LoadAssertModule()
	if err != nil {
		return
	}

	b.cmd = NewCmdFunction(b.session, b)

	b.predeclared = starlark.StringDict{
		"derivation": b.derivationFn,
		"cmd":        b.cmd,
		"os":         NewOS(b, b.session),
		"assert":     assertGlobals["assert"],
	}
	return
}

func (b *Bramble) load(thread *starlark.Thread, module string) (globals starlark.StringDict, err error) {
	globals, err = b.resolveModule(module)
	return
}

var extension string = ".bramble"

func isTestFile(name string) bool {
	if !strings.HasSuffix(name, extension) {
		return false
	}
	nameWithoutExtension := name[:len(name)-len(extension)]
	return (strings.HasPrefix(nameWithoutExtension, "test_") ||
		strings.HasSuffix(nameWithoutExtension, "_test"))
}

func findTestFiles(path string) (testFiles []string, err error) {
	if fileExists(path) {
		return []string{path}, nil
	}
	if fileExists(path + extension) {
		return []string{path + extension}, nil
	}
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return
	}
	for _, file := range files {
		name := file.Name()
		if filepath.Ext(name) != extension {
			continue
		}
		if !isTestFile(name) {
			continue
		}
		testFiles = append(testFiles, filepath.Join(path, name))
	}

	return
}

// runErrorReporter reports errors during a run. These errors are just passed up the thread
type runErrorReporter struct{}

func (e runErrorReporter) Error(err error) {}
func (e runErrorReporter) FailNow() bool   { return true }

// testErrorReporter reports errors during an individual test
type testErrorReporter struct {
	errors []error
}

func (e *testErrorReporter) Error(err error) {
	e.errors = append(e.errors, err)
}
func (e *testErrorReporter) FailNow() bool { return false }

func (b *Bramble) compilePath(path string) (prog *starlark.Program, err error) {
	compiledProgram, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "error opening moduleCache storeLocation")
	}
	return starlark.CompiledProgram(compiledProgram)
}

func (b *Bramble) sourceProgram(moduleName, filename string) (prog *starlark.Program, err error) {
	storeLocation, ok := b.moduleCache[moduleName]
	if ok {
		// we have a cached binary location in the cache map, so we just use that
		return b.compilePath(b.store.joinStorePath(storeLocation))
	}

	// hash the file input
	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	hasher := NewHasher()
	if _, err = io.Copy(hasher, f); err != nil {
		return nil, err
	}
	inputHash := hasher.String()

	inputHashStoreLocation := b.store.joinBramblePath("var", "star-cache", inputHash)
	storeLocation, ok = validSymlinkExists(inputHashStoreLocation)
	if ok {
		// if we have the hashed input on the filesystem cache and it points to a valid path
		// in the store, use that store path and add the cached location to the map
		b.moduleCache[moduleName] = storeLocation
		return b.compilePath(storeLocation)
	}

	// if we're this far we don't have a cache of the program, process it directly
	if _, err = f.Seek(0, 0); err != nil {
		return
	}
	_, prog, err = starlark.SourceProgram(filename, f, b.predeclared.Has)
	if err != nil {
		return
	}

	var buf bytes.Buffer
	if err = prog.Write(&buf); err != nil {
		return nil, err
	}
	_, path, err := b.store.writeReader(&buf, filepath.Base(filename), "")
	if err != nil {
		return
	}
	b.moduleCache[moduleName] = filepath.Base(path)
	_ = os.Remove(inputHashStoreLocation)
	return prog, os.Symlink(path, inputHashStoreLocation)
}

func (b *Bramble) ExecFile(moduleName, filename string) (globals starlark.StringDict, err error) {
	prog, err := b.sourceProgram(moduleName, filename)
	if err != nil {
		return
	}
	g, err := prog.Init(b.thread, b.predeclared)
	g.Freeze()
	return g, err
}

func (b *Bramble) test(args []string) (err error) {
	failFast := true
	if err = b.init(); err != nil {
		return
	}
	location := "."
	if len(args) > 0 {
		location = args[0]
	}
	testFiles, err := findTestFiles(location)
	if err != nil {
		return errors.Wrap(err, "error finding test files")
	}
	for _, filename := range testFiles {
		// TODO: need to calculate module name
		b.reset()
		globals, err := b.ExecFile("", filename)
		if err != nil {
			return err
		}
		for name, fn := range globals {
			if !strings.HasPrefix(name, "test_") {
				continue
			}
			starFn, ok := fn.(*starlark.Function)
			if !ok {
				continue
			}
			fmt.Printf("running test %q\n", name)
			errors := testErrorReporter{}
			assert.SetReporter(b.thread, &errors)
			_, err = starlark.Call(b.thread, starFn, nil, nil)
			if len(errors.errors) > 0 {
				fmt.Printf("\nGot %d errors while running %q in %q:\n", len(errors.errors), name, filename)
				for _, err := range errors.errors {
					fmt.Print(starutil.AnnotateError(err))
				}
				if failFast {
					return errQuiet
				}
			}
			if err != nil {
				return err
			}
		}
	}
	return
}

func (b *Bramble) run(args []string) (err error) {
	if len(args) == 0 {
		err = errHelp
		return
	}
	set := flag.NewFlagSet("bramble", flag.ExitOnError)
	var all bool
	// TODO
	set.BoolVar(&all, "all", false, "run all public functions recursively")
	if err = set.Parse(args); err != nil {
		return
	}

	if err = b.init(); err != nil {
		return
	}
	assert.SetReporter(b.thread, runErrorReporter{})

	module, fn, err := b.argsToImport(args)
	if err != nil {
		return
	}
	globals, err := b.resolveModule(module)
	if err != nil {
		return
	}
	b.calledFunction = fn
	b.moduleEntrypoint = module
	toCall, ok := globals[fn]
	if !ok {
		return errors.Errorf("global function %q not found", fn)
	}

	values, err := starlark.Call(&starlark.Thread{}, toCall, nil, nil)
	if err != nil {
		return errors.Wrap(err, "error running")
	}
	returnedDerivations := valuesToDerivations(values)

	b.buildDerivations(returnedDerivations)

	return b.writeConfigMetadata(returnedDerivations)
}

func (b *Bramble) gc(args []string) (err error) {
	if err = b.init(); err != nil {
		return
	}
	drvQueue, err := b.collectDerivationsToPreserve()
	if err != nil {
		return
	}
	pathsToKeep := map[string]struct{}{}

	// TODO: maybe this will get too big?
	drvCache := map[string]Derivation{}

	loadDerivation := func(filename string) (drv Derivation, err error) {
		if drv, ok := drvCache[filename]; ok {
			return drv, nil
		}
		drv, err = b.loadDerivation(filename)
		if err == nil {
			drvCache[filename] = drv
		}
		return drv, err
	}
	var do DerivationOutput
	var runtimeDep bool

	defer fmt.Println(pathsToKeep)

	processedDerivations := map[DerivationOutput]bool{}
	for {
		if len(drvQueue) == 0 {
			break
		}
		// pop one off to process
		for do, runtimeDep = range drvQueue {
			break
		}
		delete(drvQueue, do)
		pathsToKeep[do.Filename] = struct{}{}
		toAdd, err := b.findDerivationsInputDerivationsToKeep(
			loadDerivation,
			pathsToKeep,
			do, runtimeDep)
		if err != nil {
			return err
		}
		for toAddID, toAddRuntimeDep := range toAdd {
			if thisIsRuntimeDep, ok := processedDerivations[toAddID]; !ok {
				// if we don't have it, add it
				drvQueue[toAddID] = toAddRuntimeDep
			} else if !thisIsRuntimeDep && toAddRuntimeDep {
				// if we do have it, but it's not a runtime dep, and this is, add it
				drvQueue[toAddID] = toAddRuntimeDep
			}
			// otherwise don't add it
		}
	}

	// delete everything in the store that's not in the map
	files, err := ioutil.ReadDir(b.store.storePath)
	if err != nil {
		return
	}
	for _, file := range files {
		if _, ok := pathsToKeep[file.Name()]; !ok {
			fmt.Println("deleting", file.Name())
			if err = os.RemoveAll(b.store.joinStorePath(file.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *Bramble) collectDerivationsToPreserve() (drvQueue map[DerivationOutput]bool, err error) {
	registryFolder := b.store.joinBramblePath("var", "config-registry")
	files, err := ioutil.ReadDir(registryFolder)
	if err != nil {
		return
	}

	drvQueue = map[DerivationOutput]bool{}
	for _, f := range files {
		var drvMap derivationMap
		registryLoc := filepath.Join(registryFolder, f.Name())
		f, err := os.Open(registryLoc)
		if err != nil {
			return nil, err
		}
		if _, err := toml.DecodeReader(f, &drvMap); err != nil {
			return nil, err
		}
		fmt.Println("assembling derivations for", drvMap.Location)
		// delete the config if we can't find the project any more
		tomlLoc := filepath.Join(drvMap.Location, "bramble.toml")
		if !pathExists(tomlLoc) {
			fmt.Printf("deleting cache for %q, it no longer exists\n", tomlLoc)
			if err := os.Remove(registryLoc); err != nil {
				return nil, err
			}
			continue
		}
		for _, list := range drvMap.Derivations {
			// TODO: check that these global entrypoints actually still exist
			for _, item := range list {
				parts := strings.Split(item, ":")
				drvQueue[DerivationOutput{
					Filename:   parts[0],
					OutputName: parts[1],
				}] = true
			}
		}
	}
	return
}

func (b *Bramble) findDerivationsInputDerivationsToKeep(
	loadDerivation func(string) (Derivation, error),
	pathsToKeep map[string]struct{},
	do DerivationOutput, runtimeDep bool) (
	addToQueue map[DerivationOutput]bool, err error) {
	addToQueue = map[DerivationOutput]bool{}

	drv, err := loadDerivation(do.Filename)
	if err != nil {
		return
	}

	// keep all source paths for all derivations
	for _, p := range drv.SourcePaths {
		pathsToKeep[p] = struct{}{}
	}

	dependencyOutputs := map[string]bool{}
	if runtimeDep {
		for _, dep := range drv.Output(do.OutputName).Dependencies {
			filename := strings.ReplaceAll(dep, "$bramble_path/", "")
			// keep outputs for all runtime dependencies
			pathsToKeep[filename] = struct{}{}
			dependencyOutputs[filename] = false
		}
	}

	for _, inputDO := range drv.InputDerivations {
		idDrv, err := loadDerivation(inputDO.Filename)
		if err != nil {
			return nil, err
		}
		outPath := idDrv.Output(inputDO.OutputName).Path
		// found this derivation in an output, add it as a runtime dep
		if _, ok := dependencyOutputs[outPath]; ok {
			addToQueue[inputDO] = true
			dependencyOutputs[outPath] = true
		} else {
			addToQueue[inputDO] = false
		}
		// keep all derivations
		pathsToKeep[inputDO.Filename] = struct{}{}
	}
	for path, found := range dependencyOutputs {
		if !found {
			return nil, errors.Errorf(
				"derivation %s has output %s which was not "+
					"found as an output of any of its input derivations.",
				do.Filename, path)
		}
	}

	return nil, nil
}

func (b *Bramble) loadDerivation(filename string) (drv Derivation, err error) {
	f, err := os.Open(b.store.joinStorePath(filename))
	if err != nil {
		return
	}
	return drv, json.NewDecoder(f).Decode(&drv)
}

func (b *Bramble) derivationBuild(args []string) error {
	return nil
}

func (b *Bramble) resolveModule(module string) (globals starlark.StringDict, err error) {
	if !strings.HasPrefix(module, b.config.Module.Name) {
		// TODO: support other modules
		err = errors.Errorf("can't find module %s", module)
		return
	}

	path := module[len(b.config.Module.Name):]
	path = filepath.Join(b.configLocation, path)

	directoryWithNameExists := pathExists(path)

	var directoryHasDefaultDotBramble bool
	if directoryWithNameExists {
		directoryHasDefaultDotBramble = fileExists(path + "/default.bramble")
	}

	fileWithNameExists := fileExists(path + extension)

	switch {
	case directoryWithNameExists && directoryHasDefaultDotBramble:
		path += "/default.bramble"
	case fileWithNameExists:
		path += extension
	default:
		err = errModuleDoesNotExist(module)
		return
	}

	return b.ExecFile(module, path)
}

func valuesToDerivations(values starlark.Value) (derivations []*Derivation) {
	switch v := values.(type) {
	case *Derivation:
		return []*Derivation{v}
	case *starlark.List:
		for _, v := range starutil.ListToValueList(v) {
			derivations = append(derivations, valuesToDerivations(v)...)
		}
	case starlark.Tuple:
		for _, v := range v {
			derivations = append(derivations, valuesToDerivations(v)...)
		}
	}
	return
}

func (b *Bramble) moduleFromPath(path string) (module string, err error) {
	module = (b.config.Module.Name + "/" + b.relativePathFromConfig())
	if path == "" {
		return
	}

	// if the relative path is nothing, we've already added the slash above
	if !strings.HasSuffix(module, "/") {
		module += "/"
	}

	// support things like bar/main.bramble:foo
	if strings.HasSuffix(path, extension) && fileExists(path) {
		return module + path[:len(path)-len(extension)], nil
	}

	fullName := path + extension
	if !fileExists(fullName) {
		if !fileExists(path + "/default.bramble") {
			return "", errors.Errorf("can't find module at %q", path)
		}
	}
	// we found it, return
	module += filepath.Join(path)
	return
}

func (b *Bramble) relativePathFromConfig() string {
	wd, _ := os.Getwd()
	relativePath, _ := filepath.Rel(b.configLocation, wd)
	if relativePath == "." {
		// don't add a dot to the path
		return ""
	}
	return relativePath
}

func (b *Bramble) argsToImport(args []string) (module, function string, err error) {
	if len(args) == 0 {
		return "", "", errRequiredFunctionArgument
	}

	firstArgument := args[0]
	if !strings.Contains(firstArgument, ":") {
		function = firstArgument
		module = b.config.Module.Name
		if loc := b.relativePathFromConfig(); loc != "" {
			module += ("/" + loc)
		}
	} else {
		parts := strings.Split(firstArgument, ":")
		if len(parts) != 2 {
			return "", "", errors.New("function name has too many colons")
		}
		var path string
		path, function = parts[0], parts[1]
		module, err = b.moduleFromPath(path)
	}

	return
}
