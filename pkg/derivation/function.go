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

// Function is the function that creates derivations
type Function struct {
	bramblePath string
	storePath   string
	derivations map[string]*Derivation
	thread      *starlark.Thread

	log *logrus.Logger
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
func NewFunction(thread *starlark.Thread) (*Function, error) {
	// TODO: don't run on this on every command run, shouldn't be needed to
	// just print health information
	bramblePath, storePath, err := ensureBramblePath()
	if err != nil {
		return nil, err
	}
	// TODO: check that the store directory structure is accurate and make
	// directories if needed
	fn := &Function{
		log:         logrus.New(),
		bramblePath: bramblePath,
		storePath:   storePath,
		derivations: make(map[string]*Derivation),
	}
	// c.log.SetReportCaller(true)
	fn.log.SetLevel(logrus.DebugLevel)

	fn.thread = thread
	return fn, nil
}

// Run runs a file given a path. Returns the global variable values from that
// file. Run will recursively run imported files.
func (f *Function) Run(file string) (globals starlark.StringDict, err error) {
	f.log.Debug("running file ", file)
	globals, err = starlark.ExecFile(f.thread, file, nil, starlark.StringDict{
		"derivation": f,
	})
	if err != nil {
		return
	}
	return
}

func (f *Function) joinStorePath(v ...string) string {
	return filepath.Join(append([]string{f.storePath}, v...)...)
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
	file, err := ioutil.TempFile(filepath.Join(f.bramblePath, "tmp"), "")
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

func (f *Function) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (v starlark.Value, err error) {
	if args.Len() > 0 {
		return nil, errors.New("builtin function build() takes no positional arguments")
	}
	drv, err := f.newDerivationFromKWArgs(kwargs)
	if err != nil {
		return nil, &starlark.EvalError{Msg: err.Error(), CallStack: f.thread.CallStack()}
	}
	drv.location = filepath.Dir(thread.CallStack().At(1).Pos.Filename())
	if err = drv.calculateInputDerivations(); err != nil {
		return nil, &starlark.EvalError{Msg: err.Error(), CallStack: f.thread.CallStack()}
	}
	f.log.Debugf("Building derivation %q", drv.Name)
	if err = f.buildDerivation(drv); err != nil {
		return nil, &starlark.EvalError{Msg: err.Error(), CallStack: f.thread.CallStack()}
	}
	f.log.Debug("Completed derivation: ", drv.prettyJSON())
	_, filename, err := drv.computeDerivation()
	if err != nil {
		return
	}
	f.derivations[filename] = drv
	return drv, nil
}
