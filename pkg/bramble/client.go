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

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
)

// Client is the bramble client. $BRAMBLE_PATH must be set to an absolute path when initializing
type Client struct {
	bramblePath string
	storePath   string
	derivations map[string]*Derivation
	thread      *starlark.Thread

	test    bool
	testURL string

	log            *logrus.Logger
	scriptLocation StringStack
}

func NewClient() (*Client, error) {
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

	resolve.AllowFloat = true
	resolve.AllowLambda = true
	resolve.AllowNestedDef = true
	resolve.AllowRecursion = false
	resolve.AllowSet = true

	c.thread = &starlark.Thread{Name: "main", Load: c.StarlarkLoadFunc}
	return c, nil
}

func (c *Client) StorePath(v ...string) string {
	return filepath.Join(append([]string{c.storePath}, v...)...)
}

func (c *Client) LoadDerivation(filename string) (drv *Derivation, exists bool, err error) {
	fileLocation := c.StorePath(filename)
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
	exists, err = drv.CheckForExisting()
	if err != nil {
		return
	}
	if exists {
		return
	}
	// TODO: calculate derivation and check if we already have it
	if err = drv.Build(); err != nil {
		return
	}
	if err = drv.WriteDerivation(); err != nil {
		return
	}
	return
}

func (c *Client) Run(file string) (globals starlark.StringDict, err error) {
	c.log.Debug("running file ", file)
	c.scriptLocation.Push(filepath.Dir(file))
	globals, err = starlark.ExecFile(c.thread, file, nil, starlark.StringDict{
		"derivation": starlark.NewBuiltin("derivation", c.StarlarkDerivation),
	})
	if err != nil {
		return
	}
	// clear the context of this Run as it might be on an import
	c.scriptLocation.Pop()
	return
}

func (c *Client) DownloadFile(url string, hash string) (path string, err error) {
	c.log.Debugf("Downloading url %s", url)

	b, err := hex.DecodeString(hash)
	if err != nil {
		err = errors.Wrap(err, fmt.Sprintf("error decoding hash %q; is it hexadecimal?", hash))
		return
	}
	storePrefixHash := bytesToBase32Hash(b)
	matches, err := filepath.Glob(c.StorePath(storePrefixHash) + "*")
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
	path = c.StorePath(storePrefixHash + "-" + filepath.Base(url))
	// don't overwrite err if we error here, we want to try and save this, but
	// still return the incorrect hash error
	if er := os.Rename(file.Name(), path); er != nil {
		return "", errors.Wrap(er, "error moving file into store")
	}
	return path, err
}
