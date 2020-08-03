package bramble

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

	"github.com/maxmcd/bramble/pkg/bramblescript"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.starlark.net/repl"
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
)

// Client is the bramble client.
type Client struct {
	bramblePath string
	storePath   string
	derivations map[string]*Derivation
	thread      *starlark.Thread

	test    bool
	testURL string

	log            *logrus.Logger
	scriptLocation stringStack
}

func init() {
	resolve.AllowFloat = true
	resolve.AllowLambda = true
	resolve.AllowNestedDef = true
	resolve.AllowRecursion = false
	resolve.AllowSet = true
}

// NewClient creates a new client. When initialized this function checks if the
// bramble store exists and creates it if it does not.
func NewClient() (*Client, error) {
	// TODO: don't run on this on every command run, shouldn't be needed to
	// just print health information
	bramblePath, storePath, err := ensureBramblePath()
	if err != nil {
		return nil, err
	}
	// TODO: check that the store directory structure is accurate and make directories if needed
	c := &Client{
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

func (c *Client) joinStorePath(v ...string) string {
	return filepath.Join(append([]string{c.storePath}, v...)...)
}

// Load derivation will load and parse a derivation from the bramble store1
func (c *Client) LoadDerivation(filename string) (drv *Derivation, exists bool, err error) {
	fileLocation := c.joinStorePath(filename)
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

func (c *Client) buildDerivation(drv *Derivation) (err error) {
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

func (c *Client) shellCommand(args []string) (err error) {
	panic("unimplemented")
}

func (c *Client) scriptCommand(args []string) (err error) {
	thread := &starlark.Thread{Name: ""}
	// TODO: run from location of script
	wd, err := os.Getwd()
	if err != nil {
		return
	}
	builtins := bramblescript.Builtins(wd)
	if len(args) != 0 {
		if _, err := starlark.ExecFile(thread, args[0], nil, builtins); err != nil {
			return err
		}
		return nil
	}
	repl.REPL(thread, builtins)
	return nil
}

func (c *Client) buildCommand(args []string) (err error) {
	if len(args) == 0 {
		return errors.New("the build command takes a positional argument")
	}
	_, err = c.Run(args[0])
	return
}

// Run runs a file given a path. Returns the global variable values from that
// file. Run will recursively run imported files.
func (c *Client) Run(file string) (globals starlark.StringDict, err error) {
	c.log.Debug("running file ", file)
	c.scriptLocation.Push(filepath.Dir(file))
	globals, err = starlark.ExecFile(c.thread, file, nil, starlark.StringDict{
		"derivation": starlark.NewBuiltin("derivation", c.starlarkDerivation),
	})
	if err != nil {
		return
	}
	// clear the context of this Run as it might be on an import
	c.scriptLocation.Pop()
	return
}

// DownloadFile downloads a file into the store. Must include an expected hash
// of the downloaded file as a hex string of a  sha256 hash
func (c *Client) DownloadFile(url string, hash string) (path string, err error) {
	c.log.Debugf("Downloading url %s", url)

	b, err := hex.DecodeString(hash)
	if err != nil {
		err = errors.Wrap(err, fmt.Sprintf("error decoding hash %q; is it hexadecimal?", hash))
		return
	}
	storePrefixHash := bytesToBase32Hash(b)
	matches, err := filepath.Glob(c.joinStorePath(storePrefixHash) + "*")
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
	file, err := ioutil.TempFile(filepath.Join(c.bramblePath, "tmp"), "")
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
	path = c.joinStorePath(storePrefixHash + "-" + filepath.Base(url))
	// don't overwrite err if we error here, we want to try and save this, but
	// still return the incorrect hash error
	if er := os.Rename(file.Name(), path); er != nil {
		return "", errors.Wrap(er, "error moving file into store")
	}
	return path, err
}
