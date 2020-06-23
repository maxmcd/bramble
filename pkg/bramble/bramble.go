package bramble

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

func Run() (err error) {
	client := NewClient()
	return client.Run(os.Args[1])
}

type Client struct {
	builds map[string]Build
	thread *starlark.Thread
}

func NewClient() *Client {
	return &Client{
		builds: make(map[string]Build),
		thread: &starlark.Thread{Name: "main"},
	}
}

func (c *Client) Run(file string) (err error) {
	globals, err := starlark.ExecFile(c.thread, file, nil, starlark.StringDict{
		"load":  starlark.NewBuiltin("load", c.StarlarkLoad),
		"build": starlark.NewBuiltin("build", c.StarlarkBuild),
	})
	fmt.Println(globals)
	if err != nil {
		return
	}
	for _, build := range c.builds {
		if err := build.Build(); err != nil {
			return err
		}
	}
	return
}

func (c *Client) StarlarkLoad(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (v starlark.Value, err error) {
	return
}

type Build struct {
	Name        string
	Outputs     []Output
	Builder     string
	Platform    string
	Args        []string
	Environment map[string]string
}

func (b Build) Build() (err error) {
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		return err
	}
	if b.Builder == "fetch_url" {
		url, ok := b.Environment["url"]
		if !ok {
			return errors.New("fetch_url requires the environment variable 'url' to be set")
		}
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		fmt.Println("downloading")
		f, err := os.Create(filepath.Join(tempDir, filepath.Base(url)))
		if err != nil {
			return err
		}
		hash := sha256.New()
		tee := io.TeeReader(resp.Body, hash)
		if _, err := io.Copy(f, tee); err != nil {
			return err
		}
		if fmt.Sprintf("%x", hash.Sum(nil)) != b.Environment["hash"] {
			return errors.New("hash mismatch")
		}
	}
	return nil
}

type Output struct {
	Name          string
	Path          string
	HashAlgorithm string
	Hash          string
}

type typeError struct {
	funcName   string
	argument   string
	wantedType string
}

func (te typeError) Error() string {
	return fmt.Sprintf("%s() keyword argument '%s' must be of type '%s'", te.funcName, te.argument, te.wantedType)
}

func newBuildFromKWArgs(kwargs []starlark.Tuple) (build Build, err error) {
	te := typeError{
		funcName: "build",
	}
	for _, kwarg := range kwargs {
		key := kwarg.Index(0).(starlark.String).GoString()
		value := kwarg.Index(1)
		switch key {
		case "name":
			name, ok := value.(starlark.String)
			if !ok {
				te.argument = "name"
				te.wantedType = "string"
				return build, te
			}
			build.Name = name.GoString()
		case "builder":
			name, ok := value.(starlark.String)
			if !ok {
				te.argument = "builder"
				te.wantedType = "string"
				return build, te
			}
			build.Builder = name.GoString()
		case "args":
		case "environment":
			build.Environment, err = valueToStringMap(value, "build", "environment")
			if err != nil {
				return
			}
		default:
			err = errors.Errorf("build() got an unexpected keyword argument '%s'", key)
			return
		}
	}
	return build, nil
}

func valueToStringMap(val starlark.Value, function, param string) (out map[string]string, err error) {
	out = map[string]string{}
	maybeErr := errors.Errorf(
		"%s parameter '%s' expects type 'dict' instead got '%s'",
		function, param, val.String())
	if val.Type() != "dict" {
		err = maybeErr
		return
	}
	dict, ok := val.(starlark.IterableMapping)
	if !ok {
		err = maybeErr
		return
	}
	items := dict.Items()
	for _, item := range items {
		key := item.Index(0)
		value := item.Index(1)
		ks, ok := key.(starlark.String)
		if !ok {
			err = errors.Errorf("%s %s expects a dictionary of strings, but got value '%s'", function, param, key.String())
			return
		}
		vs, ok := value.(starlark.String)
		if !ok {
			err = errors.Errorf("%s %s expects a dictionary of strings, but got value '%s'", function, param, value.String())
			return
		}
		out[ks.GoString()] = vs.GoString()
	}
	return
}

func (c *Client) StarlarkBuild(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (v starlark.Value, err error) {
	if args.Len() > 0 {
		return nil, errors.New("builtin function build() takes no positional arguments")
	}
	build, err := newBuildFromKWArgs(kwargs)
	if err != nil {
		return nil, err
	}
	c.builds[build.Name] = build
	// TODO: return build as value
	return starlark.None, nil
}
