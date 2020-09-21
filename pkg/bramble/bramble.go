package bramble

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/hashicorp/terraform/dag"
	"github.com/hashicorp/terraform/tfdiags"
	"github.com/mholt/archiver/v3"
	"github.com/pkg/errors"

	"github.com/maxmcd/bramble/pkg/assert"
	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/maxmcd/bramble/pkg/textreplace"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
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

	moduleCache   map[string]string
	filenameCache map[string]string
	importGraph   *AcyclicGraph

	afterDerivation bool

	moduleEntrypoint string
	calledFunction   string

	derivations *DerivationsMap
}

type DerivationsMap struct {
	sync.Map
}

func (dm *DerivationsMap) Get(id string) *Derivation {
	d, ok := dm.Load(id)
	if !ok {
		return nil
	}
	return d.(*Derivation)
}
func (dm *DerivationsMap) Set(id string, drv *Derivation) {
	dm.Store(id, drv)
}

func (dm *DerivationsMap) Range(f func(filename string, drv *Derivation) bool) {
	dm.Map.Range(func(key, value interface{}) bool {
		return f(key.(string), value.(*Derivation))
	})
}

// AfterDerivation is called when a builtin function is called that can't be
// run before an derivation call. This allows us to track when
func (b *Bramble) AfterDerivation() { b.afterDerivation = true }
func (b *Bramble) CalledDerivation() error {
	if b.afterDerivation {
		return errors.New("build context is dirty, can't call derivation after cmd() or other builtins")
	}
	return nil
}

func (b *Bramble) LoadDerivation(filename string) (drv *Derivation, exists bool, err error) {
	fileLocation := b.store.joinStorePath(filename)
	_, err = os.Stat(fileLocation)
	if err != nil {
		return nil, false, nil
	}
	file, err := os.Open(fileLocation)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = file.Close() }()
	drv = &Derivation{}
	return drv, true, json.NewDecoder(file).Decode(drv)
}
func (b *Bramble) checkForExistingDerivation(filename string) (outputs []Output, exists bool, err error) {
	existingDrv, exists, err := b.LoadDerivation(filename)
	// It doesn't exist if it doesn't exist
	if !exists {
		return nil, exists, err
	}
	// It doesn't exist if it doesn't have the outputs we need
	return existingDrv.Outputs, !existingDrv.MissingOutput(), err
}

func (b *Bramble) buildDerivationIfNew(drv *Derivation) (err error) {
	filename := drv.filename()
	outputs, exists, err := b.checkForExistingDerivation(filename)
	if err != nil {
		return err
	}
	if exists {
		drv.Outputs = outputs
		b.derivations.Set(filename, drv)
		return
	}
	fmt.Println("Building derivation", filename)
	if err = b.buildDerivation(drv); err != nil {
		return
	}
	return b.writeDerivation(drv)
}

func (b *Bramble) buildDerivation(drv *Derivation) (err error) {
	buildDir, err := b.createTmpDir()
	if err != nil {
		return
	}
	if drv.BuildContextSource != "" {
		if err = copyDirectory(b.store.joinStorePath(drv.BuildContextSource), buildDir); err != nil {
			err = errors.Wrap(err, "error copying sources into build dir")
			return
		}
	}
	outputPaths := map[string]string{}
	for _, name := range drv.OutputNames {
		// TODO: use diretory within store instead so that we can rewrite self-referential paths
		if outputPaths[name], err = b.createTmpDir(); err != nil {
			return
		}
	}
	drvCopy, err := b.copyDerivationWithOutputValuesReplaced(drv)
	if err != nil {
		return
	}
	switch drv.Builder {
	case "fetch_url":
		err = b.fetchURLBuilder(drvCopy, outputPaths)
	case "function":
		err = b.functionBuilder(drvCopy, buildDir, outputPaths)
	default:
		err = b.regularBuilder(drvCopy, buildDir, outputPaths)
	}
	if err != nil {
		return
	}

	for outputName, outputPath := range outputPaths {
		matches, hashString, err := b.hashAndScanDirectory(drv, outputPath)
		if err != nil {
			return errors.Wrap(err, "error scanning directory")
		}
		folderName := hashString + "-" + drv.Name
		drv.SetOutput(outputName, Output{Path: folderName, Dependencies: matches})
		newPath := b.store.joinStorePath() + "/" + folderName
		fmt.Println("Output at ", newPath)
		// We ignore an existing build directory with the same name. It contains the same contents
		if !pathExists(newPath) {
			err = os.Rename(outputPath, newPath)
		} else {
			err = os.RemoveAll(outputPath)
		}
		if err != nil {
			return err
		}
	}
	return
}

func (b *Bramble) hashAndScanDirectory(drv *Derivation, location string) (matches []string, hashString string, err error) {
	var storeValues []string
	oldStorePath := b.store.storePath
	new := BramblePrefixOfRecord

	for _, do := range drv.InputDerivations {
		storeValues = append(storeValues,
			b.store.joinStorePath(
				b.derivations.Get(do.Filename).Output(do.OutputName).Path,
			),
		)
	}

	errChan := make(chan error)
	resultChan := make(chan map[string]struct{})
	pipeReader, pipeWriter := io.Pipe()

	go func() {
		if err := reptar.Reptar(location, pipeWriter); err != nil {
			errChan <- err
		}
		if err := pipeWriter.Close(); err != nil {
			errChan <- err
		}
	}()
	hasher := NewHasher()
	go func() {
		_, matches, err := textreplace.ReplaceStringsPrefix(
			pipeReader, hasher, storeValues, oldStorePath, new)
		if err != nil {
			errChan <- err
		}
		resultChan <- matches
	}()
	select {
	case err := <-errChan:
		return nil, "", err
	case result := <-resultChan:
		for k := range result {
			// TODO: does this still need to happen?
			matches = append(matches, strings.Replace(k, oldStorePath, "$bramble_path", 1))
		}
		return matches, hasher.String(), nil
	}
}

// setDerivationBuilder is used during instantiation to set various attributes on the
// derivation for a specific builder
func (b *Bramble) setDerivationBuilder(drv *Derivation, builder starlark.Value) (err error) {
	switch v := builder.(type) {
	case starlark.String:
		drv.Builder = v.GoString()
	case *starlark.Function:
		meta := functionBuilderMeta{
			ModuleCache: map[string]string{},
		}
		globals := v.Globals()
		var match bool
		for name, fn := range globals {
			if fn == v {
				match = true
				meta.Function = name
				break
			}
		}
		if !match {
			return errors.Errorf("function %q must be a global function to be used in a function builder", v)
		}
		meta.Module = b.filenameCache[v.Position().Filename()]

		meta.ModuleCache[meta.Module] = b.moduleCache[meta.Module]
		set, err := b.importGraph.Descendents(meta.Module)
		if err != nil {
			return err
		}
		for _, m := range set {
			moduleName := m.(string)
			meta.ModuleCache[moduleName] = b.moduleCache[moduleName]
		}

		drv.Builder = "function"

		b, _ := json.Marshal(meta)
		drv.Env["function_builder_meta"] = string(b)
		for _, p := range meta.ModuleCache {
			drv.SourcePaths = append(drv.SourcePaths, p)
		}
		// TODO: put these validation checks somehere singular?
		sort.Strings(drv.SourcePaths)
	default:
		return errors.Errorf("no builder for %q", builder.Type())
	}
	return
}

func (b *Bramble) createTmpDir() (tempDir string, err error) {
	return ioutil.TempDir(b.store.storePath, TempDirPrefix)
}
func (b *Bramble) writeDerivation(drv *Derivation) error {
	filename := drv.filename()
	fileLocation := b.store.joinStorePath(filename)

	return ioutil.WriteFile(fileLocation, drv.JSON(), 0644)
}

func (b *Bramble) fetchURLBuilder(drv *Derivation, outputPaths map[string]string) (err error) {
	if _, ok := outputPaths["out"]; len(outputPaths) > 1 || !ok {
		return errors.New("the fetchurl builtin can only have the defalt output \"out\"")
	}
	url, ok := drv.Env["url"]
	if !ok {
		return errors.New("fetch_url requires the environment variable 'url' to be set")
	}
	// derivation can provide a hash, but usually this is just in the lockfile
	hash := drv.Env["hash"]
	path, err := b.DownloadFile(url, hash)
	if err != nil {
		return err
	}
	// TODO: what if this package changes?
	if err = archiver.Unarchive(path, outputPaths["out"]); err != nil {
		return errors.Wrap(err, "error unpacking url archive")
	}
	return nil
}

func (b *Bramble) regularBuilder(drv *Derivation, buildDir string, outputPaths map[string]string) (err error) {
	// drv.function.
	builderLocation := drv.Builder

	if _, err := os.Stat(builderLocation); err != nil {
		return errors.Wrap(err, "error checking if builder location exists")
	}
	cmd := exec.Command(builderLocation, drv.Args...)
	cmd.Dir = filepath.Join(buildDir, drv.BuildContextRelativePath)
	cmd.Env = drv.env()
	for outputName, outputPath := range outputPaths {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", outputName, outputPath))
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (b *Bramble) functionBuilder(drv *Derivation, buildDir string, outputPaths map[string]string) (err error) {
	var meta functionBuilderMeta

	if err = json.Unmarshal([]byte(drv.Env["function_builder_meta"]), &meta); err != nil {
		return errors.Wrap(err, "error parsing function_builder_meta")
	}
	session, err := newSession(buildDir, drv.Env)
	if err != nil {
		return
	}
	if err = session.cd(filepath.Join(buildDir, drv.BuildContextRelativePath)); err != nil {
		return
	}
	for outputName, outputPath := range outputPaths {
		session.setEnv(outputName, outputPath)
	}
	for k, v := range session.env {
		session.env[k] = strings.ReplaceAll(v, "$bramble_path", b.store.storePath)
	}
	return b.CallInlineDerivationFunction(
		outputPaths,
		meta,
		session,
	)
}

// DownloadFile downloads a file into the store. Must include an expected hash
// of the downloaded file as a hex string of a  sha256 hash
func (b *Bramble) DownloadFile(url string, hash string) (path string, err error) {
	fmt.Printf("Downloading url %s\n", url)

	if hash != "" {
		byt, err := hex.DecodeString(hash)
		if err != nil {
			err = errors.Wrap(err, fmt.Sprintf("error decoding hash %q; is it hexadecimal?", hash))
			return "", err
		}
		storePrefixHash := bytesToBase32Hash(byt)
		matches, err := filepath.Glob(b.store.joinStorePath(storePrefixHash) + "*")
		if err != nil {
			err = errors.Wrap(err, "error searching for existing hashed content")
			return "", err
		}
		if len(matches) != 0 {
			return matches[0], nil
		}
	}

	existingHash, exists := b.lockFile.URLHashes[url]
	if exists && hash != "" && hash != existingHash {
		return "", errors.Errorf("when downloading the file %q a hash %q was provided in"+
			" code but the hash %q was in the lock file, exiting", url, hash, existingHash)
	}

	// if we don't have a hash to validate, validate the one we already have
	if hash == "" && exists {
		hash = existingHash
	}

	resp, err := http.Get(url)
	if err != nil {
		err = errors.Wrap(err, fmt.Sprintf("error making request to download %q", url))
		return
	}
	defer resp.Body.Close()
	contentHash, path, err := b.store.writeReader(resp.Body, filepath.Base(url), hash)
	if err == errHashMismatch {
		err = errors.Errorf(
			"Got incorrect hash for url %s.\nwanted %q\ngot    %q",
			url, hash, contentHash)
	} else if err != nil {
		return
	}
	return path, b.addURLHashToLockfile(url, contentHash)
}

func (b *Bramble) calculateDerivationInputSources(drv *Derivation) (err error) {
	if len(drv.sources) == 0 {
		return
	}
	tmpDir, err := b.createTmpDir()
	if err != nil {
		return
	}

	sources := drv.sources
	drv.sources = []string{}
	absDir, err := filepath.Abs(drv.location)
	if err != nil {
		return
	}
	// get absolute paths for all sources
	for i, src := range sources {
		sources[i] = filepath.Join(absDir, src)
	}
	prefix := commonFilepathPrefix(append(sources, absDir))
	relBramblefileLocation, err := filepath.Rel(prefix, absDir)
	if err != nil {
		return
	}

	if err = copyFilesByPath(prefix, sources, tmpDir); err != nil {
		return
	}
	// sometimes the location the derivation runs from is not present
	// in the structure of the copied source files. ensure that we add it
	runLocation := filepath.Join(tmpDir, relBramblefileLocation)
	if err = os.MkdirAll(runLocation, 0755); err != nil {
		return
	}

	hasher := NewHasher()
	if err = reptar.Reptar(tmpDir, hasher); err != nil {
		return
	}
	storeLocation := b.store.joinStorePath(hasher.String())
	if pathExists(storeLocation) {
		if err = os.RemoveAll(tmpDir); err != nil {
			return
		}
	} else {
		if err = os.Rename(tmpDir, storeLocation); err != nil {
			return
		}
	}
	drv.BuildContextSource = hasher.String()
	drv.BuildContextRelativePath = relBramblefileLocation
	drv.SourcePaths = append(drv.SourcePaths, hasher.String())
	sort.Strings(drv.SourcePaths)
	return
}

func (b *Bramble) stringsReplacerForOutputs(outputs DerivationOutputs) (replacer *strings.Replacer, err error) {
	// Find all the replacements we need to make, template strings need to
	// become filesystem paths
	replacements := []string{}
	for _, do := range outputs {
		d := b.derivations.Get(do.Filename)
		if d == nil {
			return nil, errors.Errorf(
				"couldn't find a derivation with the filename %q in our cache. have we built it yet?", do.Filename)
		}
		path := filepath.Join(
			b.store.storePath,
			d.Output(do.OutputName).Path,
		)
		replacements = append(replacements, do.templateString(), path)
	}
	// Replace the content using the json body and then convert it back into a
	// new derivation
	return strings.NewReplacer(replacements...), nil
}

func (b *Bramble) replaceOutputValuesInCmd(cmd *Cmd) (err error) {
	outputs := cmd.searchForDerivationOutputs()
	replacer, err := b.stringsReplacerForOutputs(outputs)
	if err != nil {
		return
	}
	cmd.Path = replacer.Replace(cmd.Path)
	for i, arg := range cmd.Args {
		cmd.Args[i] = replacer.Replace(arg)
	}
	for i, env := range cmd.Env {
		cmd.Env[i] = replacer.Replace(env)
	}
	return nil
}

func (b *Bramble) copyDerivationWithOutputValuesReplaced(drv *Derivation) (copy *Derivation, err error) {
	// Find all derivation output template strings within the derivation
	outputs := drv.searchForDerivationOutputs()

	replacer, err := b.stringsReplacerForOutputs(outputs)
	if err != nil {
		return
	}
	replacedJSON := replacer.Replace(string(drv.JSON()))
	err = json.Unmarshal([]byte(replacedJSON), &copy)
	copy.location = drv.location
	return copy, err
}

func (b *Bramble) derivationsToDerivationOutputs(drvs []*Derivation) (dos DerivationOutputs) {
	for _, drv := range drvs {
		filename := drv.filename()
		for _, name := range drv.OutputNames {
			dos = append(dos, DerivationOutput{Filename: filename, OutputName: name})
		}
	}
	return dos
}

func (b *Bramble) assembleDerivationDependencyGraph(dos DerivationOutputs) *AcyclicGraph {
	graph := NewAcyclicGraph()
	_ = graph
	root := "root"
	graph.Add(root)
	var processDO func(do DerivationOutput)
	processDO = func(do DerivationOutput) {
		drv := b.derivations.Get(do.Filename)
		if drv == nil {
			panic(do)
		}
		for _, inputDO := range drv.InputDerivations {
			graph.Add(inputDO)
			graph.Connect(dag.BasicEdge(do, inputDO))
			processDO(inputDO) // TODO, not recursive
		}
	}
	for _, do := range dos {
		graph.Add(do)
		graph.Connect(dag.BasicEdge(root, do))
		processDO(do)
	}
	return graph
}

func (b *Bramble) buildDerivationOutputs(dos DerivationOutputs) (err error) {
	graph := b.assembleDerivationDependencyGraph(dos)

	errChan := make(chan error)
	semaphore := make(chan struct{}, 2)

	if err = graph.Validate(); err != nil {
		return
	}
	go func() {
		graph.Walk(func(v dag.Vertex) tfdiags.Diagnostics {
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			// serial for now

			if root, ok := v.(string); ok && root == "root" {
				return nil
			}
			do := v.(DerivationOutput)
			drv := b.derivations.Get(do.Filename)
			if drv.Output(do.OutputName).Path != "" {
				return nil
			}
			if err := b.buildDerivationIfNew(drv); err != nil {
				errChan <- err
			}
			return nil
		})
		errChan <- nil
	}()
	return <-errChan
}

func (b *Bramble) CallInlineDerivationFunction(outputPaths map[string]string, meta functionBuilderMeta, session *session) (err error) {
	// TODO: investigate if you actually need to create a new bramble
	newBramble := &Bramble{
		// retain from parent
		config:         b.config,
		configLocation: b.configLocation,
		store:          b.store,

		// populate for this task
		moduleEntrypoint: meta.Module,
		calledFunction:   meta.Function,
		moduleCache:      meta.ModuleCache,
		filenameCache:    map[string]string{},
		importGraph:      NewAcyclicGraph(),
		session:          session,
	}
	newBramble.thread = &starlark.Thread{Load: newBramble.load}
	// this will pass the session to cmd and os
	if err = newBramble.initPredeclared(); err != nil {
		return
	}
	newBramble.derivations = b.derivations

	globals, err := newBramble.resolveModule(meta.Module)
	if err != nil {
		return
	}

	d := starlark.NewDict(len(outputPaths))
	for name, path := range outputPaths {
		_ = d.SetKey(starlark.String(name), starlark.String(path))
	}
	ctx, _ := starlarkstruct.Make(nil, nil, nil, []starlark.Tuple{{starlark.String("outputs"), d}})

	_, err = starlark.Call(
		newBramble.thread,
		globals[meta.Function].(*starlark.Function),
		starlark.Tuple{ctx}, nil,
	)
	return
}

func (b *Bramble) reset() {
	b.moduleCache = map[string]string{}
	b.filenameCache = map[string]string{}
}

func (b *Bramble) init() (err error) {
	log.SetOutput(ioutil.Discard)

	if b.configLocation != "" {
		return errors.New("can't initialize Bramble twice")
	}

	b.moduleCache = map[string]string{}
	b.filenameCache = map[string]string{}
	b.derivations = &DerivationsMap{}
	b.importGraph = NewAcyclicGraph()

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
		"os":         NewOS(b, b.session, b.cmd),
		"assert":     assertGlobals["assert"],
	}
	return
}

func (b *Bramble) load(thread *starlark.Thread, module string) (globals starlark.StringDict, err error) {
	thisFilesModuleName := b.filenameCache[thread.CallFrame(0).Pos.Filename()]
	b.importGraph.Add(thisFilesModuleName)
	b.importGraph.Add(module)
	b.importGraph.Connect(dag.BasicEdge(module, thisFilesModuleName))
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
	b.filenameCache[filename] = moduleName
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

	if err = b.buildDerivationOutputs(b.derivationsToDerivationOutputs(returnedDerivations)); err != nil {
		return
	}

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
	drvCache := map[string]*Derivation{}

	loadDerivation := func(filename string) (drv *Derivation, err error) {
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
		_ = f.Close()
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
	loadDerivation func(string) (*Derivation, error),
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

func (b *Bramble) loadDerivation(filename string) (drv *Derivation, err error) {
	f, err := os.Open(b.store.joinStorePath(filename))
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	drv = &Derivation{}
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

	// See if this path is actually the name of a module, for now we just
	// support one module.
	// TODO: search through all modules in scope for this config
	if strings.HasPrefix(path, b.config.Module.Name) {
		return path, nil
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
