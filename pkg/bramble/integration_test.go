// +build !race

package bramble

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/maxmcd/bramble/pkg/starutil"
)

func runTwiceAndCheck(t *testing.T, cb func(t *testing.T)) {
	log.SetOutput(ioutil.Discard)
	var err error
	hshr := hasher.NewHasher()
	dir := tmpDir()
	hshr2 := hasher.NewHasher()
	dir2 := tmpDir()

	// set a unique bramble store for these tests
	os.Setenv("BRAMBLE_PATH", dir+"/")
	hd, _ := os.UserHomeDir()
	dest := filepath.Join(dir, "var")
	_ = os.MkdirAll(dest, 0755)
	if err = fileutil.CP("", filepath.Join(hd, "bramble/var/linux-binary"), dest); err != nil {
		t.Fatal(err)
	}
	cb(t)
	if err = reptar.Reptar(dir+"/store", hshr); err != nil {
		t.Error(err)
	}
	os.Setenv("BRAMBLE_PATH", dir2)
	dest2 := filepath.Join(dir2, "var")
	_ = os.MkdirAll(dest2, 0755)
	if err = fileutil.CP("", filepath.Join(hd, "bramble/var/linux-binary"), dest2); err != nil {
		t.Fatal(err)
	}
	cb(t)
	if err = reptar.Reptar(dir2+"/store", hshr2); err != nil {
		t.Error(err)
	}
	if hshr.String() != hshr2.String() {
		t.Error("content doesn't match, non deterministic", dir, dir2)
		return
	}
	_ = os.RemoveAll(dir)
	_ = os.RemoveAll(dir2)
}

func TestIntegration(t *testing.T) {
	t.Skip("b.test doesn't work without fixing the docker pieces")
	runTests := func(t *testing.T) {
		b := Bramble{}
		if err := b.test([]string{"../../tests"}); err != nil {
			fmt.Printf("%+v", err)
			t.Error(err)
		}
	}
	runTwiceAndCheck(t, runTests)
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

func runBrambleRun(args []string) error {
	// ensure $GOPATH/bin is in PATH
	path := os.Getenv("PATH")
	gobin := filepath.Join(os.Getenv("GOPATH"), "bin")
	if !strings.Contains(path, gobin) {
		os.Setenv("PATH", path+":"+gobin)
	}
	b := Bramble{}
	return b.run(args)
}

func TestIntegrationRunAlmostAllPublicFunctions(t *testing.T) {
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
	runTwiceAndCheck(t, func(t *testing.T) {
		if err := runBrambleRun([]string{"github.com/maxmcd/bramble/tests/simple/simple:simple"}); err != nil {
			t.Fatal(starutil.AnnotateError(err))
		}
	})
}

func TestIntegrationNixSeed(t *testing.T) {
	runTwiceAndCheck(t, func(t *testing.T) {
		if err := runBrambleRun([]string{"github.com/maxmcd/bramble/lib/nix-seed:stdenv"}); err != nil {
			t.Fatal(starutil.AnnotateError(err))
		}
	})
}
