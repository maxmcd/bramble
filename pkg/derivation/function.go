package derivation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
)

type Bramble interface {
	BramblePath() string
	StorePath() string

	// AfterDerivation is called when it is assumed that derivation calls are
	// complete. Derivations will usually build at this point
	AfterDerivation()
	// CallDerivation is called within every derivation call. If this is called
	// after AfterDerivation it will error
	CalledDerivation() error

	ModuleCache() map[string]string
	DerivationCallCount() int
	RunEntrypoint() (module string, function string)
	FindFunctionContext(derivationCallCount int, moduleCache map[string]string, moduleEntrypoint, calledFunction string) (thread *starlark.Thread, fn *starlark.Function, err error)
}

// Function is the function that creates derivations
type Function struct {
	bramble Bramble

	derivations map[string]*Derivation

	log *logrus.Logger

	DerivationCallCount int
}

var (
	_ starlark.Value    = new(Function)
	_ starlark.Callable = new(Function)
)

func (f *Function) Freeze()               {}
func (f *Function) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("module") }
func (f *Function) Name() string          { return f.String() }
func (f *Function) String() string        { return `<built-in function cmd>` }
func (f *Function) Truth() starlark.Bool  { return true }
func (f *Function) Type() string          { return "module" }

func init() {
	// It's easier to start giving away free coffee than it is to take away
	// free coffee
	resolve.AllowFloat = false
	resolve.AllowLambda = false
	resolve.AllowNestedDef = false
	resolve.AllowRecursion = false
	resolve.AllowSet = true
}

// NewFunction creates a new client. When initialized this function checks if the
// bramble store exists and creates it if it does not.
func NewFunction(bramble Bramble) (*Function, error) {
	// TODO: don't run on this on every command run, shouldn't be needed to
	// just print health information
	// TODO: check that the store directory structure is accurate and make
	// directories if needed
	fn := &Function{
		log:         logrus.New(),
		derivations: make(map[string]*Derivation),
		bramble:     bramble,
	}
	// c.log.SetReportCaller(true)
	fn.log.SetLevel(logrus.DebugLevel)

	return fn, nil
}

func (f *Function) joinStorePath(v ...string) string {
	return filepath.Join(append([]string{f.bramble.StorePath()}, v...)...)
}

// Load derivation will load and parse a derivation from the bramble store1
func (f *Function) LoadDerivation(filename string) (drv *Derivation, exists bool, err error) {
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

func (f *Function) buildDerivation(drv *Derivation) (err error) {
	var exists bool
	exists, err = drv.checkForExisting()
	if err != nil {
		return
	}
	if exists {
		return
	}
	if err = drv.build(); err != nil {
		return
	}
	if err = drv.writeDerivation(); err != nil {
		return
	}
	return
}

// DownloadFile downloads a file into the store. Must include an expected hash
// of the downloaded file as a hex string of a  sha256 hash
func (f *Function) DownloadFile(url string, hash string) (path string, err error) {
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
	file, err := ioutil.TempFile(filepath.Join(f.bramble.BramblePath(), "tmp"), "")
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

func (f *Function) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (v starlark.Value, err error) {
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
	f.log.Debug("Calculated derivation before build: ", drv.prettyJSON())

	f.log.Debugf("Building derivation %q", drv.Name)
	if err = f.buildDerivation(drv); err != nil {
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

func (f *Function) newDerivationFromArgs(args starlark.Tuple, kwargs []starlark.Tuple) (drv *Derivation, err error) {
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
			drv.Env[input.Name] = input.Outputs["out"].Path
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
