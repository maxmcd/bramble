package bramble

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/maxmcd/bramble/pkg/textreplace"
	"github.com/mholt/archiver/v3"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
)

var (
	TempDirPrefix = "bramble-"

	// BramblePrefixOfRecord is the prefix we use when hashing the build output
	// this allows us to get a consistent hash even if we're building in a
	// different location
	BramblePrefixOfRecord = "/home/bramble/bramble/bramble_store_padding/bramb"
)

// DerivationFunction is the function that creates derivations
type DerivationFunction struct {
	bramble *Bramble

	derivations map[string]*Derivation

	log *logrus.Logger

	DerivationCallCount int
}

var (
	_ starlark.Value    = new(DerivationFunction)
	_ starlark.Callable = new(DerivationFunction)
)

func (f *DerivationFunction) Freeze()               {}
func (f *DerivationFunction) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("module") }
func (f *DerivationFunction) Name() string          { return f.String() }
func (f *DerivationFunction) String() string        { return `<built-in function derivation>` }
func (f *DerivationFunction) Truth() starlark.Bool  { return true }
func (f *DerivationFunction) Type() string          { return "module" }

func init() {
	// It's easier to start giving away free coffee than it is to take away
	// free coffee
	resolve.AllowFloat = false
	resolve.AllowLambda = false
	resolve.AllowNestedDef = false
	resolve.AllowRecursion = false
	resolve.AllowSet = true
}

// NewDerivationFunction creates a new client. When initialized this function checks if the
// bramble store exists and creates it if it does not.
func NewDerivationFunction(bramble *Bramble) (*DerivationFunction, error) {
	fn := &DerivationFunction{
		log:         logrus.New(),
		derivations: make(map[string]*Derivation),
		bramble:     bramble,
	}
	// c.log.SetReportCaller(true)
	fn.log.SetLevel(logrus.DebugLevel)

	return fn, nil
}

func (f *DerivationFunction) joinStorePath(v ...string) string {
	return f.bramble.store.joinStorePath(v...)
}

func (f *DerivationFunction) checkBytesForDerivations(b []byte) (inputDerivations InputDerivations) {
	for location, derivation := range f.derivations {
		for name, output := range derivation.Outputs {
			if bytes.Contains(b, []byte(output.Path)) {
				inputDerivations = append(inputDerivations, InputDerivation{
					Path:   location,
					Output: name,
				})
			}
		}
	}
	return
}

// Load derivation will load and parse a derivation from the bramble store1
func (f *DerivationFunction) LoadDerivation(filename string) (drv *Derivation, exists bool, err error) {
	fileLocation := f.joinStorePath(filename)
	_, err = os.Stat(fileLocation)
	if err != nil {
		return nil, false, nil
	}
	file, err := os.Open(fileLocation)
	if err != nil {
		return nil, true, err
	}
	drv = &Derivation{}
	return drv, true, json.NewDecoder(file).Decode(drv)
}

// DownloadFile downloads a file into the store. Must include an expected hash
// of the downloaded file as a hex string of a  sha256 hash
func (f *DerivationFunction) DownloadFile(url string, hash string) (path string, err error) {
	f.log.Debugf("Downloading url %s", url)

	b, err := hex.DecodeString(hash)
	if err != nil {
		err = errors.Wrap(err, fmt.Sprintf("error decoding hash %q; is it hexadecimal?", hash))
		return
	}
	storePrefixHash := bytesToBase32Hash(b)
	matches, err := filepath.Glob(f.joinStorePath(storePrefixHash) + "*")
	if err != nil {
		err = errors.Wrap(err, "error searching for existing hashed content")
		return
	}
	if len(matches) != 0 {
		return matches[0], nil
	}
	resp, err := http.Get(url)
	if err != nil {
		err = errors.Wrap(err, fmt.Sprintf("error making request to download %q", url))
		return
	}
	defer resp.Body.Close()
	path, err = f.bramble.store.writeReader(resp.Body, filepath.Base(url), hash)
	if err == errHashMismatch {
		err = errors.Errorf(
			"Got incorrect hash for url %s.\nwanted %q\ngot    %q",
			url, hash, path)
	}
	return
}

type ErrFoundBuildContext struct {
	thread *starlark.Thread
	Fn     *starlark.Function
}

func (efbc ErrFoundBuildContext) Error() string {
	return "internal err found build context error"
}

func getBuilderFunction(kwargs []starlark.Tuple) *starlark.Function {
	for _, tup := range kwargs {
		key, val := tup[0].(starlark.String), tup[1]
		if key == "builder" {
			if fn, ok := val.(*starlark.Function); ok {
				return fn
			}
		}
	}
	return nil
}

func (f *DerivationFunction) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (v starlark.Value, err error) {
	if err = f.bramble.CalledDerivation(); err != nil {
		return
	}

	// we're running inside a derivation build and we need to exit with this function and function context
	if f.bramble.DerivationCallCount() == f.DerivationCallCount {
		return nil, ErrFoundBuildContext{thread: thread, Fn: getBuilderFunction(kwargs)}
	}
	if args.Len() > 0 {
		return nil, errors.New("builtin function build() takes no positional arguments")
	}
	drv, err := f.newDerivationFromArgs(args, kwargs)
	if err != nil {
		return nil, err
	}

	// At(0) is within this function, we want the file of the caller
	drv.location = filepath.Dir(thread.CallStack().At(1).Pos.Filename())
	if err = drv.calculateInputDerivations(); err != nil {
		return nil, err
	}

	if err = drv.calculateInputSources(); err != nil {
		return
	}

	f.log.Debug("Completed derivation: ", drv.prettyJSON())

	// derivation calculation complete, hard barier
	// TODO: expand on this nonsensical comment
	// ---------------------------------------------------------------

	f.log.Debugf("Building derivation %q", drv.Name)
	if err = drv.buildIfNew(); err != nil {
		return nil, err
	}
	f.log.Debug("Completed derivation: ", drv.prettyJSON())
	// TODO: add this panic but don't include outputs in the comparison
	// if beforeBuild != drv.prettyJSON() {
	// 	panic(beforeBuild + drv.prettyJSON())
	// }
	_, filename, err := drv.computeDerivation()
	if err != nil {
		return
	}
	f.derivations[filename] = drv
	return drv, nil
}

type functionBuilderMeta struct {
	DerivationCallCount int
	ModuleCache         map[string]string
	Module              string
	Function            string
}

// setBuilder is used during instantiation to set various attributes on the
// derivation for a specific builder
func setBuilder(drv *Derivation, builder starlark.Value) (err error) {
	switch v := builder.(type) {
	case starlark.String:
		drv.Builder = v.GoString()
	case *starlark.Function:
		drv.Builder = "function"
		meta := functionBuilderMeta{
			DerivationCallCount: drv.function.bramble.DerivationCallCount(),
			ModuleCache:         drv.function.bramble.ModuleCache(),
		}
		meta.Module, meta.Function = drv.function.bramble.RunEntrypoint()

		b, _ := json.Marshal(meta)
		drv.Env["function_builder_meta"] = string(b)
		for _, p := range meta.ModuleCache {
			drv.SourcePaths = append(drv.SourcePaths, p)
		}
	default:
		return errors.Errorf("no builder for %q", builder.Type())
	}
	return
}

func (f *DerivationFunction) newDerivationFromArgs(args starlark.Tuple, kwargs []starlark.Tuple) (drv *Derivation, err error) {
	drv = &Derivation{
		Outputs:  map[string]Output{"out": {}},
		Env:      map[string]string{},
		function: f,
	}
	var (
		name        starlark.String
		builder     starlark.Value = starlark.None
		argsParam   *starlark.List
		sources     *starlark.List
		env         *starlark.Dict
		outputs     *starlark.List
		buildInputs *starlark.List
	)
	if err = starlark.UnpackArgs("derivation", args, kwargs,
		"builder", &builder,
		"name?", &name,
		"args?", &argsParam,
		"sources?", &sources,
		"env?", &env,
		"outputs?", &outputs,
		"build_inputs", &buildInputs,
	); err != nil {
		return
	}

	drv.Name = name.GoString()

	if argsParam != nil {
		if drv.Args, err = starutil.IterableToGoList(argsParam); err != nil {
			return
		}
	}
	if sources != nil {
		if drv.sources, err = starutil.IterableToGoList(sources); err != nil {
			return
		}
	}

	if env != nil {
		if drv.Env, err = starutil.DictToGoStringMap(env); err != nil {
			return
		}
	}

	if buildInputs != nil {
		for _, item := range starutil.ListToValueList(buildInputs) {
			input, ok := item.(*Derivation)
			if !ok {
				err = errors.Errorf("build_inputs takes a list of derivations, found type %q", item.Type())
				return
			}
			_, filename, err := input.computeDerivation()
			if err != nil {
				panic(err)
			}
			drv.InputDerivations = append(drv.InputDerivations, InputDerivation{
				Path:   filename,
				Output: "out", // TODO: support passing other outputs
			})
			drv.Env[input.Name] = "$bramble_path/" + input.Outputs["out"].Path
		}
	}
	if outputs != nil {
		outputsList, err := starutil.IterableToGoList(outputs)
		if err != nil {
			return nil, err
		}
		delete(drv.Outputs, "out")
		for _, o := range outputsList {
			drv.Outputs[o] = Output{}
		}
	}

	if err = setBuilder(drv, builder); err != nil {
		return
	}

	return drv, nil
}

// Derivation is the basic building block of a Bramble build
type Derivation struct {
	// Name is the name of the derivation
	Name string
	// Outputs are build outputs, a derivation can have many outputs, the
	// default output is called "out". Multiple outputs are useful when your
	// build process can produce multiple artifacts, but building them as a
	// standalone derivation would involve a complete rebuild.
	//
	// This attribute is removed when hashing the derivation.
	Outputs map[string]Output
	// Builder will either be set to a string constant to signify an internal
	// builder (like "fetch_url"), or it will be set to the path of an
	// executable in the bramble store
	Builder string
	// Platform is the platform we've built this derivation on
	Platform string
	// Args are arguments that are passed to the builder
	Args []string
	// Env are environment variables set during the build
	Env map[string]string
	// InputDerivations are derivations that are using as imports to this build, outputs
	// dependencies are tracked in the outputs
	InputDerivations InputDerivations
	// InputSource is the source directory and relative build location path for the build
	InputSource InputSource
	// SourcePaths are all paths that must exist to support this build
	SourcePaths []string

	// internal fields
	sources  []string
	function *DerivationFunction
	location string
}

type InputSource struct {
	Path             string
	RelativeLocation string
}

// DerivationOutput tracks the build outputs. Outputs are not included in the
// Derivation hash. The path tracks the output location in the bramble store
// and Dependencies tracks the bramble outputs that are runtime dependencies.
type Output struct {
	Path         string
	Dependencies []string
}

// InputDerivation is one of the derivation inputs. Path is the location of
// the derivation, output is the name of the specific output this derivation
// uses for the build
type InputDerivation struct {
	Path   string
	Output string
}

type InputDerivations []InputDerivation

func sortAndUniqueInputDerivations(ids InputDerivations) InputDerivations {
	sort.Slice(ids, func(i, j int) bool {
		id := ids[i]
		jd := ids[j]
		return id.Path+id.Output < jd.Path+id.Output
	})
	if len(ids) == 0 {
		return ids
	}
	j := 0
	for i := 1; i < len(ids); i++ {
		if ids[j] == ids[i] {
			continue
		}
		j++
		ids[j] = ids[i]
	}
	return ids[:j+1]
}

var (
	_ starlark.Value    = new(Derivation)
	_ starlark.HasAttrs = new(Derivation)
)

func (drv *Derivation) Freeze()               {}
func (drv *Derivation) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("cmd") }
func (drv *Derivation) String() string        { return fmt.Sprintf("<derivation %q>", drv.Name) }
func (drv *Derivation) Truth() starlark.Bool  { return starlark.True }
func (drv *Derivation) Type() string          { return "derivation" }

func (drv *Derivation) Attr(name string) (val starlark.Value, err error) {
	output, ok := drv.Outputs[name]
	if ok {
		return starlark.String(fmt.Sprintf("$bramble_path/%s", output.Path)), nil
	}
	return nil, nil
}

func (drv *Derivation) AttrNames() (out []string) {
	for name := range drv.Outputs {
		out = append(out, name)
	}
	return
}

func (drv *Derivation) prettyJSON() string {
	b, _ := json.MarshalIndent(drv, "", "  ")
	return string(b)
}

func (drv *Derivation) calculateInputDerivations() (err error) {
	// TODO: combine this with checking of build_inputs
	// TODO: also check derivations passed to cmd run

	fileBytes, err := json.Marshal(drv)
	if err != nil {
		return
	}

	drv.InputDerivations = sortAndUniqueInputDerivations(append(
		drv.InputDerivations,
		drv.function.checkBytesForDerivations(fileBytes)...,
	))

	return nil
}

func (drv *Derivation) computeDerivation() (fileBytes []byte, filename string, err error) {
	fileBytes, err = json.Marshal(drv)
	if err != nil {
		return
	}
	outputs := drv.Outputs
	// content is hashed without the outputs attribute
	drv.Outputs = nil
	var jsonBytesForHashing []byte
	jsonBytesForHashing, err = json.Marshal(drv)
	if err != nil {
		return
	}
	drv.Outputs = outputs
	fileName := fmt.Sprintf("%s.drv", drv.Name)
	_, filename, err = hashFile(fileName, ioutil.NopCloser(bytes.NewBuffer(jsonBytesForHashing)))
	if err != nil {
		return
	}
	return
}

func (drv *Derivation) checkForExisting() (exists bool, err error) {
	_, filename, err := drv.computeDerivation()
	if err != nil {
		return
	}
	drv.function.log.Debug("derivation " + drv.Name + " evaluates to " + filename)
	existingDrv, exists, err := drv.function.LoadDerivation(filename)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	for _, v := range existingDrv.Outputs {
		if v.Path == "" {
			return false, nil
		}
	}
	drv.Outputs = existingDrv.Outputs
	return true, nil
}

func (drv *Derivation) calculateInputSources() (err error) {
	if len(drv.sources) == 0 {
		return
	}
	tmpDir, err := drv.createBuildDir()
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
	storeLocation := drv.function.joinStorePath(hasher.String())
	if pathExists(storeLocation) {
		if err = os.RemoveAll(tmpDir); err != nil {
			return
		}
	} else {
		if err = os.Rename(tmpDir, storeLocation); err != nil {
			return
		}
	}
	drv.InputSource.Path = hasher.String()
	drv.InputSource.RelativeLocation = relBramblefileLocation
	drv.SourcePaths = append(drv.SourcePaths, hasher.String())
	return
}

func (drv *Derivation) writeDerivation() (err error) {
	fileBytes, filename, err := drv.computeDerivation()
	if err != nil {
		return
	}
	fileLocation := drv.function.joinStorePath(filename)

	if !pathExists(fileLocation) {
		return ioutil.WriteFile(fileLocation, fileBytes, 0444)
	}
	return nil
}

func (drv *Derivation) storePath() string { return drv.function.bramble.store.storePath }

func (drv *Derivation) createBuildDir() (tempDir string, err error) {
	return ioutil.TempDir("", TempDirPrefix)
}

func (drv *Derivation) computeOutPath() (outPath string, err error) {
	_, filename, err := drv.computeDerivation()

	return filepath.Join(
		drv.storePath(),
		strings.TrimSuffix(filename, ".drv"),
	), err
}

func (drv *Derivation) expand(s string) string {
	return os.Expand(s, func(i string) string {
		if i == "bramble_path" {
			return drv.storePath()
		}
		if v, ok := drv.Env[i]; ok {
			return v
		}
		return ""
	})
}

func (drv *Derivation) regularBuilder(buildDir, outPath string) (err error) {
	builderLocation := drv.expand(drv.Builder)
	if _, err := os.Stat(builderLocation); err != nil {
		return errors.Wrap(err, "error checking if builder location exists")
	}
	cmd := exec.Command(builderLocation, drv.Args...)
	cmd.Dir = filepath.Join(buildDir, drv.InputSource.RelativeLocation)
	cmd.Env = []string{}
	for k, v := range drv.Env {
		v = strings.Replace(v, "$bramble_path", drv.storePath(), -1)
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", "out", outPath))
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", "bramble_path", drv.storePath()))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (drv *Derivation) fetchURLBuilder(outPath string) (err error) {
	url, ok := drv.Env["url"]
	if !ok {
		return errors.New("fetch_url requires the environment variable 'url' to be set")
	}
	hash, ok := drv.Env["hash"]
	if !ok {
		return errors.New("fetch_url requires the environment variable 'hash' to be set")
	}
	path, err := drv.function.DownloadFile(url, hash)
	if err != nil {
		return err
	}
	// TODO: what if this package changes?
	if err = archiver.Unarchive(path, outPath); err != nil {
		return errors.Wrap(err, "error unarchiving")
	}
	return nil
}

func (drv *Derivation) functionBuilder(buildDir, outPath string) (err error) {
	var meta functionBuilderMeta

	if err = json.Unmarshal([]byte(drv.Env["function_builder_meta"]), &meta); err != nil {
		return errors.Wrap(err, "error parsing function_builder_meta")
	}
	session, err := newSession(buildDir, drv.Env)
	if err != nil {
		return
	}
	if err = session.cd(filepath.Join(buildDir, drv.InputSource.RelativeLocation)); err != nil {
		return
	}
	session.setEnv("out", outPath)
	for k, v := range session.env {
		session.env[k] = strings.Replace(v, "$bramble_path", drv.storePath(), -1)
	}
	spew.Dump(session)
	return drv.function.bramble.CallInlineDerivationFunction(
		meta, session,
	)
}

func (drv *Derivation) buildIfNew() (err error) {
	var exists bool
	exists, err = drv.checkForExisting()
	if err != nil || exists {
		return
	}
	if err = drv.build(); err != nil {
		return
	}
	return drv.writeDerivation()
}

func (drv *Derivation) build() (err error) {
	buildDir, err := drv.createBuildDir()
	if err != nil {
		return
	}

	if drv.InputSource.Path != "" {
		if err = copyDirectory(drv.function.joinStorePath(drv.InputSource.Path), buildDir); err != nil {
			return errors.Wrap(err, "error copying sources into build dir")
		}
		fmt.Println(buildDir, drv.function.joinStorePath(drv.InputSource.Path))
	}

	outPath, err := drv.computeOutPath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(outPath, 0755); err != nil {
		return
	}

	switch drv.Builder {
	case "fetch_url":
		err = drv.fetchURLBuilder(outPath)
	case "function":
		err = drv.functionBuilder(buildDir, outPath)
	default:
		err = drv.regularBuilder(buildDir, outPath)
	}
	if err != nil {
		return
	}

	matches, hashString, err := drv.hashAndScanDirectory(outPath)
	if err != nil {
		return
	}
	folderName := hashString + "-" + drv.Name
	drv.Outputs["out"] = Output{Path: folderName, Dependencies: matches}

	newPath := drv.function.joinStorePath() + "/" + folderName
	_, doesnotExistErr := os.Stat(newPath)
	drv.function.log.Debug("Output at ", newPath)
	if doesnotExistErr != nil {
		return os.Rename(outPath, newPath)
	}
	return
}

func (drv *Derivation) hashAndScanDirectory(location string) (matches []string, hashString string, err error) {
	var storeValues []string
	oldStorePath := drv.storePath()
	new := BramblePrefixOfRecord

	for _, derivation := range drv.function.derivations {
		for _, output := range derivation.Outputs {
			storeValues = append(storeValues, filepath.Join(oldStorePath, output.Path))
		}
	}
	errChan := make(chan error)
	resultChan := make(chan map[string]struct{})
	pipeReader, pipeWriter := io.Pipe()

	go func() {
		if err := reptar.Reptar(location, pipeWriter); err != nil {
			errChan <- err
		}
		if err = pipeWriter.Close(); err != nil {
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
			matches = append(matches, strings.Replace(k, drv.storePath(), "$bramble_path", 1))
		}
		return matches, hasher.String(), nil
	}
}
