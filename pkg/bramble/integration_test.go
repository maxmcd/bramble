package bramble

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maxmcd/bramble/pkg/reptar"
)

var (
	TestTmpDirPrefix = "bramble-test-"
)

func tmpDir() string {
	dir, err := ioutil.TempDir("/tmp", TestTmpDirPrefix)
	if err != nil {
		panic(err)
	}
	return dir
}

func runTwiceAndCheck(t *testing.T, cb func(t *testing.T)) {
	var err error
	hasher := NewHasher()
	dir := tmpDir()
	hasher2 := NewHasher()
	dir2 := tmpDir()
	// set a unique bramble store for these tests
	os.Setenv("BRAMBLE_PATH", dir)
	cb(t)
	if err = reptar.Reptar(dir+"/store", hasher); err != nil {
		t.Error(err)
	}
	os.Setenv("BRAMBLE_PATH", dir2)
	cb(t)
	if err = reptar.Reptar(dir2+"/store", hasher2); err != nil {
		t.Error(err)
	}
	if hasher.String() != hasher2.String() {
		t.Error("content doesn't match, non deterministic", dir, dir2)
		return
	}

	_ = os.RemoveAll(dir)
	_ = os.RemoveAll(dir2)
}

func TestIntegration(t *testing.T) {
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
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return modules
}

func TestRunAlmostAllPublicFunctions(t *testing.T) {
	modules := assembleModules(t)
	runTwiceAndCheck(t, func(t *testing.T) {
		for _, module := range modules {
			b := Bramble{}
			if strings.Contains(module, "lib/std") {
				continue
			}
			t.Run(module, func(t *testing.T) {
				if err := b.run([]string{module}); err != nil {
					t.Fatal(err)
				}
			})
		}
	})
}
func TestBenchmarkFullCacheHit(t *testing.T) {
	// t.Skip("don't run benchmarks")
	bramble := Bramble{}
	if err := bramble.run([]string{"../../all:all"}); err != nil {
		t.Fatal(err)
	}
	res := testing.Benchmark(func(b *testing.B) {
		b.Run("hash10", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				bramble := Bramble{}
				if err := bramble.run([]string{"../../all:all"}); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	fmt.Println(time.Duration(time.Nanosecond * time.Duration(res.NsPerOp())))
	fmt.Printf("Time per run: %s\n", res.T)
	fmt.Println(res.Extra)
}
