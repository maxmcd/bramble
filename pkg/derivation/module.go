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

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
)

// Module is the derivation module
type Module struct {
	bramblePath string
	storePath   string
	derivations map[string]*Derivation
	thread      *starlark.Thread

	test    bool
	testURL string

	log            *logrus.Logger
	scriptLocation stringStack
}

//var (
//	_ starlark.Value = new(Module)
//)

func init() {
	// It's easier to start giving away free coffee than it is to take away
	// free coffee
	resolve.AllowFloat = false
	resolve.AllowLambda = false
	resolve.AllowNestedDef = false
	resolve.AllowRecursion = false
	resolve.AllowSet = true
}

// NewModule creates a new client. When initialized this function checks if the
// bramble store exists and creates it if it does not.
func NewModule() (*Module, error) {
	// TODO: don't run on this on every command run, shouldn't be needed to
	// just print health information
	bramblePath, storePath, err := ensureBramblePath()
	if err != nil {
		return nil, err
	}
	// TODO: check that the store directory structure is accurate and make
	// directories if needed
	c := &Module{
		log:         logrus.New(),
		bramblePath: bramblePath,
		storePath:   storePath,
		derivations: make(map[string]*Derivation),
	}
	// c.log.SetReportCaller(true)
	c.log.SetLevel(logrus.DebugLevel)

	c.thread = &starlark.Thread{Name: "main", Load: c.starlarkLoadFunc}
	return c, nil
}

func (m *Module) joinStorePath(v ...string) string {
	return filepath.Join(append([]string{m.storePath}, v...)...)
}

// Load derivation will load and parse a derivation from the bramble store1
func (m *Module) LoadDerivation(filename string) (drv *Derivation, exists bool, err error) {
	fileLocation := m.joinStorePath(filename)
	_, err = os.Stat(fileLocation)
	if err != nil {
		return nil, false, nil
	}
	f, err := os.Open(fileLocation)
	if err != nil {
		return nil, true, err
	}
	drv = &Derivation{}
	return drv, true, json.NewDecoder(f).Decode(drv)
}

func (m *Module) buildDerivation(drv *Derivation) (err error) {
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
func (m *Module) DownloadFile(url string, hash string) (path string, err error) {
	m.log.Debugf("Downloading url %s", url)

	b, err := hex.DecodeString(hash)
	if err != nil {
		err = errors.Wrap(err, fmt.Sprintf("error decoding hash %q; is it hexadecimal?", hash))
		return
	}
	storePrefixHash := bytesToBase32Hash(b)
	matches, err := filepath.Glob(m.joinStorePath(storePrefixHash) + "*")
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
	file, err := ioutil.TempFile(filepath.Join(m.bramblePath, "tmp"), "")
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
	path = m.joinStorePath(storePrefixHash + "-" + filepath.Base(url))
	// don't overwrite err if we error here, we want to try and save this, but
	// still return the incorrect hash error
	if er := os.Rename(file.Name(), path); er != nil {
		return "", errors.Wrap(er, "error moving file into store")
	}
	return path, err
}
