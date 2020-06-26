package bramble

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

var (
	TempDirPrefix = "bramble-"
)

func Run() (err error) {
	client, err := NewClient()
	if err != nil {
		return err
	}
	return client.Run(os.Args[1])
}

type Client struct {
	bramblePath string
	builds      map[string]Derivation
	thread      *starlark.Thread
}

func NewClient() (*Client, error) {
	bramblePath := os.Getenv("BRAMBLE_PATH")
	if bramblePath == "" {
		return nil, errors.New("environment variable BRAMBLE_PATH must be populated")
	}
	if !filepath.IsAbs(bramblePath) {
		return nil, errors.Errorf("bramble path %s must be absolute", bramblePath)
	}
	// TODO: check that the store directory structure is accurate and mkdirs if needed
	return &Client{
		bramblePath: bramblePath,
		builds:      make(map[string]Derivation),
		thread:      &starlark.Thread{Name: "main"},
	}, nil
}

func storePath() string {
	return filepath.Join(os.Getenv("BRAMBLE_PATH"), "./store")
}

func (c *Client) Run(file string) (err error) {
	globals, err := starlark.ExecFile(c.thread, file, nil, starlark.StringDict{
		"load":       starlark.NewBuiltin("load", c.StarlarkLoad),
		"derivation": starlark.NewBuiltin("derivation", c.StarlarkDerivation),
	})
	fmt.Println(globals)
	if err != nil {
		return
	}
	for _, build := range c.builds {
		if err := build.MakeDerivation(); err != nil {
			return err
		}
		if err := build.Build(); err != nil {
			return err
		}
	}
	return
}

func (c *Client) StarlarkLoad(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (v starlark.Value, err error) {
	return
}

type Derivation struct {
	Name        string
	Outputs     []Output
	Builder     string
	Platform    string
	Args        []string
	Environment map[string]string
}



func (b Derivation) MakeDerivation() (err error) {
	jsonBytes, err := json.Marshal(b)
	if err != nil {
		return err
	}
	// https://nixos.org/nixos/nix-pills/nix-store-paths.html
	fileHash := sha256.New()
	_, _ = fileHash.Write(jsonBytes)

	namePlusContentHash := sha256.New()
	_, _ = namePlusContentHash.Write([]byte(fmt.Sprintf("%x:%s", fileHash.Sum(nil), b.Name)))
	var buf bytes.Buffer
	_, _ = base32.NewEncoder(base32.StdEncoding, &buf).Write(namePlusContentHash.Sum(nil)[:20])
	// Finally the comments tell us to compute the base-32 representation of the
	// first 160 bits (truncation) of a sha256 of the above string:

	fileLocation := filepath.Join(storePath(), strings.ToLower(buf.String())+"-"+b.Name+".drv")
	// TODO: more cache checking here
	_, doesnotExistErr := os.Stat(fileLocation)
	if doesnotExistErr != nil {
		return ioutil.WriteFile(fileLocation, jsonBytes, 0444)
	}
	return nil
}

func (b Derivation) createTempDir() (tempDir string, err error) {
	tempDir, err = ioutil.TempDir("", TempDirPrefix)
	if err != nil {
		return
	}
	// TODO: create output folders and environment variables for other outputs
	err = os.MkdirAll(filepath.Join(tempDir, "out"), os.ModePerm)
	return
}

func hashDir(location string) (hash string, err error) {
	shaHash := sha256.New()
	location = filepath.Clean(location) + "/" // use the extra / to make the paths relative

	// filepath.Walk orders files in lexical order, so this will be deterministic
	if err = filepath.Walk(location, func(path string, info os.FileInfo, err error) error {
		relativePath := strings.Replace(path, location, "", -1)
		_, _ = shaHash.Write([]byte(relativePath))
		f, err := os.Open(path)
		if err != nil {
			// we already know this file exists, likely just a symlink that points nowhere
			fmt.Println(path, err)
			return nil
		}
		_, _ = io.Copy(shaHash, f)
		f.Close()
		return nil
	}); err != nil {
		return
	}
	var buf bytes.Buffer
	_, _ = base32.NewEncoder(base32.StdEncoding, &buf).Write(shaHash.Sum(nil)[:20])
	return buf.String(), nil
}

func (b Derivation) Build() (err error) {
	tempDir, err := b.createTempDir()
	outPath := filepath.Join(tempDir, "out")
	if err != nil {
		return err
	}
	if b.Builder == "fetch_url" {
		url, ok := b.Environment["url"]
		if !ok {
			return errors.New("fetch_url requires the environment variable 'url' to be set")
		}
		var resp *http.Response
		resp, err = http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		fmt.Println("downloading")

		hash := sha256.New()
		tee := io.TeeReader(resp.Body, hash)

		var gzReader io.ReadCloser
		gzReader, err = gzip.NewReader(tee)
		// xzReader, err := xz.NewReader(tee)
		if err != nil {
			return err
		}
		defer gzReader.Close()
		if err = Untar(gzReader, outPath); err != nil {
			return
		}
	}

	hashString, err := hashDir(outPath)
	if err != nil {
		return err
	}
	return os.Rename(outPath, storePath()+"/"+hashString+"-"+b.Name)
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

func newDerivationFromKWArgs(kwargs []starlark.Tuple) (build Derivation, err error) {
	te := typeError{
		funcName: "build",
	}
	// TODO: move beyond hardcoded default
	build.Outputs = []Output{{Name: "out"}}
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

func (c *Client) StarlarkDerivation(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (v starlark.Value, err error) {
	if args.Len() > 0 {
		return nil, errors.New("builtin function build() takes no positional arguments")
	}
	build, err := newDerivationFromKWArgs(kwargs)
	if err != nil {
		return nil, err
	}
	c.builds[build.Name] = build
	// TODO: return build as value
	return starlark.None, nil
}
