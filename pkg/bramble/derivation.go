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
	"regexp"
	"sort"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/maxmcd/bramble/pkg/textreplace"
	"github.com/mholt/archiver/v3"
	"github.com/pkg/errors"
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
	bramble             *Bramble
	derivations         map[string]*Derivation
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
		derivations: make(map[string]*Derivation),
		bramble:     bramble,
	}

	return fn, nil
}

func (f *DerivationFunction) joinStorePath(v ...string) string {
	return f.bramble.store.joinStorePath(v...)
}

func (f *DerivationFunction) checkBytesForDerivations(b []byte) (inputDerivations DerivationOutputs) {
	for location, derivation := range f.derivations {
		for i, output := range derivation.Outputs {
			if bytes.Contains(b, []byte(output.Path)) {
				inputDerivations = append(inputDerivations, DerivationOutput{
					Filename:   location,
					OutputName: derivation.OutputNames[i],
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
	fmt.Printf("Downloading url %s\n", url)

	if hash != "" {
		b, err := hex.DecodeString(hash)
		if err != nil {
			err = errors.Wrap(err, fmt.Sprintf("error decoding hash %q; is it hexadecimal?", hash))
			return "", err
		}
		storePrefixHash := bytesToBase32Hash(b)
		matches, err := filepath.Glob(f.joinStorePath(storePrefixHash) + "*")
		if err != nil {
			err = errors.Wrap(err, "error searching for existing hashed content")
			return "", err
		}
		if len(matches) != 0 {
			return matches[0], nil
		}
	}

	existingHash, exists := f.bramble.lockFile.URLHashes[url]
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
	contentHash, path, err := f.bramble.store.writeReader(resp.Body, filepath.Base(url), hash)
	if err == errHashMismatch {
		err = errors.Errorf(
			"Got incorrect hash for url %s.\nwanted %q\ngot    %q",
			url, hash, contentHash)
	} else if err != nil {
		return
	}
	return path, f.bramble.addURLHashToLockfile(url, contentHash)
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

	// // At(0) is within this function, we want the file of the caller
	// drv.location = filepath.Dir(thread.CallStack().At(1).Pos.Filename())
	// if err = drv.calculateInputDerivations(); err != nil {
	// 	return nil, err
	// }

	// if err = drv.calculateInputSources(); err != nil {
	// 	return
	// }

	// // derivation calculation complete, hard barier
	// // TODO: expand on this nonsensical comment
	// // ---------------------------------------------------------------

	// fmt.Printf("Building derivation %q\n", drv.Name)
	// if err = drv.buildIfNew(); err != nil {
	// 	return nil, err
	// }
	// fmt.Println("Completed derivation:", drv.Outputs)
	// // TODO: add this panic but don't include outputs in the comparison
	// // if beforeBuild != drv.prettyJSON() {
	// // 	panic(beforeBuild + drv.prettyJSON())
	// // }
	filename := drv.filename()
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

func (f *DerivationFunction) fetchDerivationOutputPath(do DerivationOutput) string {
	return f.joinStorePath(
		f.derivations[do.Filename].
			Output(do.OutputName).Path,
	)
}

func (f *DerivationFunction) newDerivationFromArgs(args starlark.Tuple, kwargs []starlark.Tuple) (drv *Derivation, err error) {
	drv = &Derivation{
		OutputNames: []string{"out"},
		Env:         map[string]string{},
		function:    f,
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
			filename := input.filename()
			drv.InputDerivations = append(drv.InputDerivations, DerivationOutput{
				Filename:   filename,
				OutputName: "out", // TODO: support passing other outputs
			})
		}
	}
	if outputs != nil {
		outputsList, err := starutil.IterableToGoList(outputs)
		if err != nil {
			return nil, err
		}
		drv.Outputs = nil
		drv.OutputNames = outputsList
	}

	if err = setBuilder(drv, builder); err != nil {
		return
	}

	return drv, nil
}

// Derivation is the basic building block of a Bramble build
type Derivation struct {
	// fields are in alphabetical order to attempt to provide consistency to
	// hashmap key ordering

	// Args are arguments that are passed to the builder
	Args []string
	// BuildContextSource is the source directory that
	BuildContextSource       string
	BuildContextRelativePath string
	// Builder will either be set to a string constant to signify an internal
	// builder (like "fetch_url"), or it will be set to the path of an
	// executable in the bramble store
	Builder string
	// Env are environment variables set during the build
	Env map[string]string
	// InputDerivations are derivations that are using as imports to this build, outputs
	// dependencies are tracked in the outputs
	InputDerivations DerivationOutputs
	// Name is the name of the derivation
	Name string
	// Outputs are build outputs, a derivation can have many outputs, the
	// default output is called "out". Multiple outputs are useful when your
	// build process can produce multiple artifacts, but building them as a
	// standalone derivation would involve a complete rebuild.
	//
	// This attribute is removed when hashing the derivation.
	OutputNames []string
	Outputs     []Output
	// Platform is the platform we've built this derivation on
	Platform string
	// SourcePaths are all paths that must exist to support this build
	SourcePaths []string

	// internal fields
	sources  []string
	function *DerivationFunction
	location string
}

// DerivationOutput tracks the build outputs. Outputs are not included in the
// Derivation hash. The path tracks the output location in the bramble store
// and Dependencies tracks the bramble outputs that are runtime dependencies.
type Output struct {
	Path         string
	Dependencies []string
}

func (o Output) Empty() bool {
	if o.Path == "" && len(o.Dependencies) == 0 {
		return true
	}
	return false
}

// DerivationOutput is one of the derivation inputs. Path is the location of
// the derivation, output is the name of the specific output this derivation
// uses for the build
type DerivationOutput struct {
	Filename   string
	OutputName string
}

func (do DerivationOutput) templateString() string {
	return fmt.Sprintf("{{ %s %s }}", do.Filename, do.OutputName)
}

type DerivationOutputs []DerivationOutput

func sortAndUniqueInputDerivations(dos DerivationOutputs) DerivationOutputs {
	sort.Slice(dos, func(i, j int) bool {
		do := dos[i]
		jd := dos[j]
		return do.Filename+do.OutputName < jd.Filename+do.OutputName
	})
	if len(dos) == 0 {
		return dos
	}
	j := 0
	for i := 1; i < len(dos); i++ {
		if dos[j] == dos[i] {
			continue
		}
		j++
		dos[j] = dos[i]
	}
	return dos[:j+1]
}

var (
	_ starlark.Value    = new(Derivation)
	_ starlark.HasAttrs = new(Derivation)
)

func (drv *Derivation) Freeze()              {}
func (drv Derivation) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("cmd") }
func (drv Derivation) Truth() starlark.Bool  { return starlark.True }
func (drv Derivation) Type() string          { return "derivation" }

func (drv Derivation) String() string {
	return drv.templateString(drv.mainOutput())
}

func (drv Derivation) Output(name string) Output {
	for i, o := range drv.OutputNames {
		if o == name {
			if len(drv.Outputs) > i {
				return drv.Outputs[i]
			}
		}
	}
	return Output{}
}

func (drv Derivation) SetOutput(name string, o Output) {
	for i, on := range drv.OutputNames {
		if on == name {
			// grow if we need to
			for len(drv.Outputs) < i {
				drv.Outputs = append(drv.Outputs, Output{})
			}
			drv.Outputs[i] = o
			return
		}
	}
	panic("unable to set output with name: " + name)
}

func (drv *Derivation) templateString(output string) string {
	outputPath := drv.Output(output).Path
	if drv.Output(output).Path != "" {
		return outputPath
	}
	fn := drv.filename()
	return fmt.Sprintf("{{ %s %s }}", fn, output)
}

func (drv *Derivation) mainOutput() string {
	if out := drv.Output("out"); out.Path != "" || len(drv.OutputNames) == 0 {
		return "out"
	}
	return drv.OutputNames[0]
}

func (drv *Derivation) env() (env []string) {
	for k, v := range drv.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	// TODO
	// for _, id := range drv.InputDerivations {
	// 	env = append(env, fmt.Sprintf("%s=%s", id.name(), drv.function.fetchDerivationOutputPath(id)))
	// }
	return
}

func (drv *Derivation) Attr(name string) (val starlark.Value, err error) {
	out := drv.Output(name)
	if out.Empty() {
		return nil, nil
	}
	return starlark.String(
		drv.templateString(name),
	), nil
}

func (drv *Derivation) AttrNames() (out []string) {
	return drv.OutputNames
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

// TemplateStringRegexp is the regular expression that matches template strings
// in our derivations. I assume the ".*" parts won't run away too much because
// of the earlier match on "{{ [0-9a-z]{32}" but might be worth further
// investigation.
//
// TODO: should we limit the content of the derivation name? would at least
// be limited by filesystem rules. If we're not eager about warning about this
// we risk having derivation names only work on certain systems through that
// limitation alone. Maybe this is ok?
var TemplateStringRegexp *regexp.Regexp = regexp.MustCompile(`\{\{ ([0-9a-z]{32}-.*?\.drv) (.*?) \}\}`)

func (drv Derivation) SearchForDerivationOutputs() DerivationOutputs {
	out := DerivationOutputs{}
	for _, match := range TemplateStringRegexp.FindAllStringSubmatch(string(drv.JSON()), -1) {
		out = append(out, DerivationOutput{
			Filename:   match[1],
			OutputName: match[2],
		})
	}
	return sortAndUniqueInputDerivations(out)
}

func (drv Derivation) ReplaceDerivationOutputsWithOutputPaths(storePath string, derivations map[string]Derivation) (
	drvCopy Derivation, err error) {
	// Find all derivation output template strings within the derivation
	outputs := drv.SearchForDerivationOutputs()

	// Find all the replacements we need to make, template strings need to
	// become filesystem paths
	replacements := []string{}
	for _, do := range outputs {
		d, ok := derivations[do.Filename]
		if !ok {
			return drvCopy, errors.Errorf(
				"couldn't find a derivation with the filename %q in our cache. have we built it yet?", do.Filename)
		}
		path := filepath.Join(
			storePath,
			d.Output(do.OutputName).Path,
		)
		replacements = append(replacements, do.templateString(), path)
	}
	// Replace the content using the json body and then convert it back into a
	// new derivation
	replacedJSON := strings.NewReplacer(replacements...).Replace(string(drv.JSON()))
	return drvCopy, json.Unmarshal([]byte(replacedJSON), &drvCopy)
}

func (drv Derivation) JSON() []byte {
	// This seems safe to ignore since we won't be updating the type signature
	// of Derivation. Is it?
	b, _ := json.Marshal(drv)
	return b
}

func (drv *Derivation) filename() (filename string) {
	outputs := drv.Outputs
	// Content is hashed without derivation outputs.
	drv.Outputs = nil

	jsonBytesForHashing := drv.JSON()

	drv.Outputs = outputs
	fileName := fmt.Sprintf("%s.drv", drv.Name)

	// We ignore this error, the errors would result from bad writes and all reads/writes are
	// in memory. Is this safe?
	_, filename, _ = hashFile(fileName, ioutil.NopCloser(bytes.NewBuffer(jsonBytesForHashing)))
	return
}

func (drv *Derivation) checkForExisting() (exists bool, err error) {
	filename := drv.filename()
	if err != nil {
		return
	}
	fmt.Println("derivation " + drv.Name + " evaluates to " + filename)
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
	tmpDir, err := drv.createTmpDir()
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
	drv.BuildContextSource = hasher.String()
	drv.BuildContextRelativePath = relBramblefileLocation
	drv.SourcePaths = append(drv.SourcePaths, hasher.String())
	return
}

func (drv *Derivation) writeDerivation() (err error) {
	filename := drv.filename()
	fileLocation := drv.function.joinStorePath(filename)

	if !pathExists(fileLocation) {
		return ioutil.WriteFile(fileLocation, drv.JSON(), 0444)
	}
	return nil
}

func (drv *Derivation) storePath() string { return drv.function.bramble.store.storePath }

func (drv *Derivation) createTmpDir() (tempDir string, err error) {
	return ioutil.TempDir(drv.storePath(), TempDirPrefix)
}

func (drv *Derivation) regularBuilder(buildDir, outPath string) (err error) {
	spew.Dump(drv.function.derivations)
	spew.Dump(drv.Builder)
	// drv.function.
	builderLocation := "TODO"
	panic(builderLocation)
	//filepath.Join(
	// drv.function.fetchDerivationOutputPath(drv.Builder.Derivation),
	// drv.Builder.FilePath)

	if _, err := os.Stat(builderLocation); err != nil {
		return errors.Wrap(err, "error checking if builder location exists")
	}
	cmd := exec.Command(builderLocation, drv.Args...)
	cmd.Dir = filepath.Join(buildDir, drv.BuildContextRelativePath)
	cmd.Env = drv.env()
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", "out", outPath))

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (drv *Derivation) fetchURLBuilder(outPath string) (err error) {
	url, ok := drv.Env["url"]
	if !ok {
		return errors.New("fetch_url requires the environment variable 'url' to be set")
	}
	// derivation can provide a hash, but usually this is just in the lockfile
	hash := drv.Env["hash"]
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
	if err = session.cd(filepath.Join(buildDir, drv.BuildContextRelativePath)); err != nil {
		return
	}
	session.setEnv("out", outPath)
	for k, v := range session.env {
		session.env[k] = strings.Replace(v, "$bramble_path", drv.storePath(), -1)
	}
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
	buildDir, err := drv.createTmpDir()
	if err != nil {
		return
	}

	if drv.BuildContextSource != "" {
		if err = copyDirectory(drv.function.joinStorePath(drv.BuildContextSource), buildDir); err != nil {
			return errors.Wrap(err, "error copying sources into build dir")
		}
	}

	outPath, err := drv.createTmpDir()
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
	drv.SetOutput("out", Output{Path: folderName, Dependencies: matches})

	newPath := drv.function.joinStorePath() + "/" + folderName
	_, doesnotExistErr := os.Stat(newPath)
	fmt.Println("Output at ", newPath)
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
