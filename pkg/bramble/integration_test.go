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

func initIntegrationTest(t *testing.T) {
	if _, ok := os.LookupEnv("BRAMBLE_INTEGRATION_TEST"); !ok {
		t.Skip("skipping integration tests unless BRAMBLE_INTEGRATION_TEST is set")
	}
}

func runTwiceAndCheck(t *testing.T, cb func(t *testing.T)) {
	log.SetOutput(ioutil.Discard)
	var err error
	hshr := hasher.NewHasher()
	dir := fileutil.TestTmpDir(t)
	hshr2 := hasher.NewHasher()
	dir2 := fileutil.TestTmpDir(t)

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
		if fi.IsDir() && fi.Name() == "testdata" {
			return filepath.SkipDir
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

// func build(context.Backgound(), args []string) e, truerror {
// 	// ensure $GOPATH/bin is in PATH
// 	// we set this so we can use the setuid binary, maybe there is a better way
// 	path := os.Getenv("PATH")
// 	gobin := filepath.Join(os.Getenv("GOPATH"), "bin")
// 	if !strings.Contains(path, gobin) {
// 		os.Setenv("PATH", path+":"+gobin)
// 	}
// 	b, err := NewBramble(".")
// 	if err != nil {
// 		return err
// 	}
// 	_, _, err = b.Build(context.Background(), args)
// 	return err
// }

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
				if _, err := runBuildFromCLI("test", []string{module}); err != nil {
					t.Fatal(starutil.AnnotateError(err))
				}
			}) {
				t.Fatal(module, "failed")
			}
		SKIP:
		}
	})
}

// func TestIntegrationSimple(t *testing.T) {
// 	initIntegrationTest(t)
// 	runTwiceAndCheck(t, func(t *testing.T) {
// 		if err := runBrambleRun([]string{"github.com/maxmcd/bramble/tests/simple/simple:simple"}); err != nil {
// 			t.Fatal(starutil.AnnotateError(err))
// 		}
// 	})
// }

// func TestIntegrationTutorial(t *testing.T) {
// 	initIntegrationTest(t)
// 	runTwiceAndCheck(t, func(t *testing.T) {
// 		if err := runBrambleRun([]string{"github.com/maxmcd/bramble/tests/tutorial:step_1"}); err != nil {
// 			t.Fatal(starutil.AnnotateError(err))
// 		}
// 	})
// }

// func TestIntegrationNixSeed(t *testing.T) {
// 	initIntegrationTest(t)
// 	runTwiceAndCheck(t, func(t *testing.T) {
// 		if err := runBrambleRun([]string{"github.com/maxmcd/bramble/lib/nix-seed:stdenv"}); err != nil {
// 			t.Fatal(starutil.AnnotateError(err))
// 		}
// 	})
// }
