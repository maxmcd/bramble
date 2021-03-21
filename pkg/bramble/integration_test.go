package bramble

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

func initIntegrationTest(t *testing.T) {
	if _, ok := os.LookupEnv("BRAMBLE_INTEGRATION_TEST"); !ok {
		t.Skip("skipping integration tests unless BRAMBLE_INTEGRATION_TEST is set")
	}
}

func runTwiceAndCheck(t *testing.T, cb func(t *testing.T)) {
	log.SetOutput(ioutil.Discard)
	var err error
	hshr := hasher.NewHasher()
	dir := tmpDir(t)
	hshr2 := hasher.NewHasher()
	dir2 := tmpDir(t)

	// set a unique bramble store for these tests
	os.Setenv("BRAMBLE_PATH", dir+"/")
	cb(t)
	if err = reptar.Reptar(dir+"/store", hshr); err != nil {
		t.Error(err)
	}
	os.Setenv("BRAMBLE_PATH", dir2)
	cb(t)
	if err = reptar.Reptar(dir2+"/store", hshr2); err != nil {
		t.Error(err)
	}
	if hshr.String() != hshr2.String() {
		t.Error("content doesn't match, non deterministic", dir, dir2)
		return
	}
}

func assembleModules(t *testing.T) []string {
	modules := []string{}
	if err := filepath.Walk("../..", func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(fi.Name(), ".bramble") {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			reader := bufio.NewReader(f)
			for {
				line, err := reader.ReadString('\n')
				if err == io.EOF {
					break
				} else if err != nil {
					return err
				}
				if !strings.HasPrefix(line, "def") {
					continue
				}
				functionName := line[4:strings.Index(line, "(")]
				if strings.HasPrefix(functionName, "_") || strings.HasPrefix(functionName, "test_") {
					continue
				}
				modules = append(modules, fmt.Sprintf("%s:%s", path, functionName))
			}
			_ = f.Close()
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return modules
}

func TestAllFunctions(t *testing.T) {
	b := Bramble{}
	if err := b.init(); err != nil {
		t.Fatal(err)
	}
	err := filepath.Walk(b.configLocation, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// TODO: ignore .git, ignore .gitignore?
		if strings.HasSuffix(path, ".bramble") {
			module, err := b.filepathToModuleName(path)
			if err != nil {
				return err
			}
			globals, err := b.resolveModule(module)
			if err != nil {
				return err
			}
			for name, v := range globals {
				if fn, ok := v.(*starlark.Function); ok {
					if fn.NumParams()+fn.NumKwonlyParams() > 0 {
						continue
					}
					fn.NumParams()
					_, err := starlark.Call(b.thread, fn, nil, nil)
					if err != nil {
						return errors.Wrapf(err, "calling %q in %s", name, path)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	urls := []string{}
	b.derivations.Range(func(filename string, drv *Derivation) bool {
		if drv.Builder == "fetch_url" {
			url, ok := drv.Env["url"]
			if ok {
				urls = append(urls, url)
			}
		}
		return true
	})
	spew.Dump(urls)
}

func runBrambleRun(args []string) error {
	// ensure $GOPATH/bin is in PATH
	// we set this so we can use the setuid binary, maybe there is a better way
	path := os.Getenv("PATH")
	gobin := filepath.Join(os.Getenv("GOPATH"), "bin")
	if !strings.Contains(path, gobin) {
		os.Setenv("PATH", path+":"+gobin)
	}
	b := Bramble{}
	return b.build(context.Background(), args)
}

func TestIntegrationRunAlmostAllPublicFunctions(t *testing.T) {
	initIntegrationTest(t)
	modules := assembleModules(t)
	toSkip := []string{
		"nix-seed/default.bramble:ldd",
		"lib/std",
		"cmd-examples",
	}
	runTwiceAndCheck(t, func(t *testing.T) {
		for _, module := range modules {
			for _, skip := range toSkip {
				if strings.Contains(module, skip) {
					goto SKIP
				}
			}
			if !t.Run(module, func(t *testing.T) {
				if err := runBrambleRun([]string{module}); err != nil {
					t.Fatal(starutil.AnnotateError(err))
				}
			}) {
				t.Fatal(module, "failed")
			}
		SKIP:
		}
	})
}

func TestIntegrationSimple(t *testing.T) {
	initIntegrationTest(t)
	runTwiceAndCheck(t, func(t *testing.T) {
		if err := runBrambleRun([]string{"github.com/maxmcd/bramble/tests/simple/simple:simple"}); err != nil {
			t.Fatal(starutil.AnnotateError(err))
		}
	})
}

func TestIntegrationNixSeed(t *testing.T) {
	initIntegrationTest(t)
	runTwiceAndCheck(t, func(t *testing.T) {
		if err := runBrambleRun([]string{"github.com/maxmcd/bramble/lib/nix-seed:stdenv"}); err != nil {
			t.Fatal(starutil.AnnotateError(err))
		}
	})
}
