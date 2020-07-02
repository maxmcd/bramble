package bramble

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.starlark.net/starlark"
)

// Client is the bramble client. $BRAMBLE_PATH must be set to an absolute path when initializing
type Client struct {
	bramblePath string
	derivations map[string]*Derivation
	thread      *starlark.Thread

	log            *logrus.Logger
	scriptLocation StringStack
}

func NewClient() (*Client, error) {
	bramblePath := os.Getenv("BRAMBLE_PATH")
	if bramblePath == "" {
		return nil, errors.New("environment variable BRAMBLE_PATH must be populated")
	}
	if !filepath.IsAbs(bramblePath) {
		return nil, errors.Errorf("bramble path %s must be absolute", bramblePath)
	}
	// TODO: check that the store directory structure is accurate and make directories if needed
	c := &Client{
		log:         logrus.New(),
		bramblePath: bramblePath,
		derivations: make(map[string]*Derivation),
	}
	// c.log.SetReportCaller(true)
	c.log.SetLevel(logrus.DebugLevel)

	c.thread = &starlark.Thread{Name: "main", Load: c.StarlarkLoadFunc}
	return c, nil
}

func (c *Client) StorePath(v ...string) string {
	return filepath.Join(append([]string{c.bramblePath, "./store"}, v...)...)
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

func (c *Client) Run(file string) (globals starlark.StringDict, err error) {
	c.log.Debug("running file ", file)
	c.scriptLocation.Push(filepath.Dir(file))
	globals, err = starlark.ExecFile(c.thread, file, nil, starlark.StringDict{
		"derivation": starlark.NewBuiltin("derivation", c.StarlarkDerivation),
	})
	if err != nil {
		return
	}
	for _, drv := range c.derivations {
		var exists bool
		exists, err = drv.CheckForExisting()
		if err != nil {
			return nil, err
		}
		if exists {
			continue
		}
		// TODO: calculate derivation and check if we already have it
		if err = drv.Build(); err != nil {
			return
		}
		if err = drv.WriteDerivation(); err != nil {
			return
		}
	}
	c.log.Debug("globals:", globals)
	// clear the context of this Run as it might be on an import
	c.scriptLocation.Pop()
	c.derivations = make(map[string]*Derivation)
	return
}

func (c *Client) StarlarkLoadFunc(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	c.log.Debug("load within '", c.scriptLocation.Peek(), "' of module ", module)
	return c.Run(filepath.Join(c.scriptLocation.Peek(), module+".bramble.py"))
}