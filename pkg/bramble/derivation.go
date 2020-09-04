package bramble

import (
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

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

	file, err := ioutil.TempFile(f.bramble.store.joinBramblePath("tmp"), "")
	if err != nil {
		err = errors.Wrap(err, "error creating a temporary file for a download")
		return
	}
	sha256Hash := sha256.New()
	tee := io.TeeReader(resp.Body, sha256Hash)
	if _, err = io.Copy(file, tee); err != nil {
		err = errors.Wrap(err, "error writing to the temporary download file")
		return
	}
	sha256HashBytes := sha256Hash.Sum(nil)
	hexStringHash := fmt.Sprintf("%x", sha256HashBytes)
	if hash != hexStringHash {
		err = errors.Errorf(
			"Got incorrect hash for url %s.\nwanted %q\ngot    %q",
			url, hash, hexStringHash)
		// make best effort to save this file, as we'll likely just download it again
		storePrefixHash = bytesToBase32Hash(sha256HashBytes)
	}
	path = f.joinStorePath(storePrefixHash + "-" + filepath.Base(url))
	// don't overwrite err if we error here, we want to try and save this, but
	// still return the incorrect hash error
	if er := os.Rename(file.Name(), path); er != nil {
		return "", errors.Wrap(er, "error moving file into store")
	}
	return path, err
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

	f.log.Debugf("Building derivation %q", drv.Name)
	if err = drv.buildIfNew(); err != nil {
		return nil, err
	}
	f.log.Debug("Completed derivation: ", drv.prettyJSON())
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
		if drv.Args, err = starutil.ListToGoList(argsParam); err != nil {
			return
		}
	}
	if sources != nil {
		if drv.Sources, err = starutil.ListToGoList(sources); err != nil {
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
		outputsList, err := starutil.ListToGoList(outputs)
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
	Name             string
	Outputs          map[string]Output
	Builder          string
	Platform         string
	Args             []string
	Env              map[string]string
	Sources          []string
	InputDerivations InputDerivations

	// internal fields
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

func (drv *Derivation) assembleSources(destination string) (runLocation string, err error) {
	if len(drv.Sources) == 0 {
		return
	}
	sources := drv.Sources
	drv.Sources = []string{}
	absDir, err := filepath.Abs(drv.location)
	if err != nil {
		return
	}
	// get absolute paths for all sources
	for i, src := range sources {
		sources[i] = filepath.Join(absDir, src)
	}
	prefix := commonFilepathPrefix(append(sources, absDir))

	if err = copyFiles(prefix, sources, destination); err != nil {
		return
	}
	relBramblefileLocation, err := filepath.Rel(prefix, absDir)
	if err != nil {
		return "", errors.Wrap(err, "error calculating relative bramblefile loc")
	}
	runLocation = filepath.Join(destination, relBramblefileLocation)
	if err = os.MkdirAll(runLocation, 0755); err != nil {
		return "", errors.Wrap(err, "error making build directory")
	}
	drv.Env["src"] = destination
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
	var runLocation string
	runLocation, err = drv.assembleSources(buildDir)
	if err != nil {
		return
	}
	builderLocation := drv.expand(drv.Builder)

	if _, err := os.Stat(builderLocation); err != nil {
		return errors.Wrap(err, "error checking if builder location exists")
	}
	cmd := exec.Command(builderLocation, drv.Args...)
	cmd.Dir = runLocation
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
	// hashed content is already there, just exit
	return
}

func (drv *Derivation) hashAndScanDirectory(location string) (matches []string, hashString string, err error) {
	var storeValues []string
	old := drv.storePath()
	new := BramblePrefixOfRecord

	for _, derivation := range drv.function.derivations {
		storeValues = append(storeValues, strings.Replace(derivation.String(), "$bramble_path", old, 1))
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
		_, matches, err := textreplace.ReplaceStringsPrefix(pipeReader, hasher, storeValues, old, new)
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

func commonFilepathPrefix(paths []string) string {
	sep := byte(os.PathSeparator)
	if len(paths) == 0 {
		return string(sep)
	}

	c := []byte(path.Clean(paths[0]))
	c = append(c, sep)

	for _, v := range paths[1:] {
		v = path.Clean(v) + string(sep)
		if len(v) < len(c) {
			c = c[:len(v)]
		}
		for i := 0; i < len(c); i++ {
			if v[i] != c[i] {
				c = c[:i]
				break
			}
		}
	}

	for i := len(c) - 1; i >= 0; i-- {
		if c[i] == sep {
			c = c[:i+1]
			break
		}
	}

	return string(c)
}

// Hasher is used to compute path hash values. Hasher implements io.Writer and
// takes a sha256 hash of the input bytes. The output string is a lowercase
// base32 representation of the first 160 bits of the hash
type Hasher struct {
	hash hash.Hash
}

func NewHasher() *Hasher {
	return &Hasher{
		hash: sha256.New(),
	}
}

func (h *Hasher) Write(b []byte) (n int, err error) {
	return h.hash.Write(b)
}

func (h *Hasher) String() string {
	return bytesToBase32Hash(h.hash.Sum(nil))
}

// bytesToBase32Hash copies nix here
// https://nixos.org/nixos/nix-pills/nix-store-paths.html
// Finally the comments tell us to compute the base32 representation of the
// first 160 bits (truncation) of a sha256 of the above string:
func bytesToBase32Hash(b []byte) string {
	var buf bytes.Buffer
	_, _ = base32.NewEncoder(base32.StdEncoding, &buf).Write(b[:20])
	return strings.ToLower(buf.String())
}

func hashFile(name string, file io.ReadCloser) (fileHash, filename string, err error) {
	defer file.Close()
	hasher := NewHasher()
	if _, err = hasher.Write([]byte(name)); err != nil {
		return
	}
	if _, err = io.Copy(hasher, file); err != nil {
		return
	}
	filename = fmt.Sprintf("%s-%s", hasher.String(), name)
	return
}

// CopyFiles takes a list of absolute paths to files and copies them into
// another directory, maintaining structure
func copyFiles(prefix string, files []string, dest string) (err error) {
	files, err = expandPathDirectories(files)
	if err != nil {
		return err
	}

	sort.Slice(files, func(i, j int) bool { return len(files[i]) < len(files[j]) })
	for _, file := range files {
		destPath := filepath.Join(dest, strings.TrimPrefix(file, prefix))
		fileInfo, err := os.Stat(file)
		if err != nil {
			return errors.Wrap(err, "error finding source file")
		}

		stat, ok := fileInfo.Sys().(*syscall.Stat_t)
		if !ok {
			return errors.Errorf("failed to get raw syscall.Stat_t data for '%s'", file)
		}

		switch fileInfo.Mode() & os.ModeType {
		case os.ModeDir:
			if err := createDirIfNotExists(destPath, 0755); err != nil {
				return err
			}
		case os.ModeSymlink:
			if err := copySymLink(file, destPath); err != nil {
				return err
			}
		default:
			if err := copyFile(file, destPath); err != nil {
				return err
			}
		}

		if err := os.Lchown(destPath, int(stat.Uid), int(stat.Gid)); err != nil {
			return err
		}

		// TODO: when does this happen???
		isSymlink := fileInfo.Mode()&os.ModeSymlink != 0
		if !isSymlink {
			if err := os.Chmod(destPath, fileInfo.Mode()); err != nil {
				return err
			}
		}
	}
	return
}

// takes a list of paths and adds all files in all subdirectories
func expandPathDirectories(files []string) (out []string, err error) {
	for _, file := range files {
		if err = filepath.Walk(file,
			func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				out = append(out, path)
				return nil
			}); err != nil {
			return
		}
	}
	return
}

func copyFile(srcFile, dstFile string) error {
	out, err := os.Create(dstFile)
	if err != nil {
		return err
	}

	defer out.Close()

	in, err := os.Open(srcFile)
	if err != nil {
		return err
	}
	defer in.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	return nil
}

func createDirIfNotExists(dir string, perm os.FileMode) error {
	if pathExists(dir) {
		return nil
	}

	if err := os.MkdirAll(dir, perm); err != nil {
		return fmt.Errorf("failed to create directory: '%s', error: '%s'", dir, err.Error())
	}

	return nil
}

func copySymLink(source, dest string) error {
	link, err := os.Readlink(source)
	if err != nil {
		return err
	}
	return os.Symlink(link, dest)
}
