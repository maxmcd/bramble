package bramble

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/alecthomas/assert"
	"github.com/maxmcd/bramble/pkg/reptar"
)

var (
	TestTmpDirPrefix = "bramble-test-"
)

func TestIntegration(t *testing.T) {
	runTests := func() {
		b := Bramble{}
		if err := b.test([]string{"../../tests"}); err != nil {
			fmt.Printf("%+v", err)
			t.Error(err)
		}
	}
	hasher := NewHasher()
	dir, err := ioutil.TempDir("", TestTmpDirPrefix)
	assert.NoError(t, err)
	hasher2 := NewHasher()
	dir2, err := ioutil.TempDir("", TestTmpDirPrefix)
	assert.NoError(t, err)
	// set a unique bramble store for these tests
	os.Setenv("BRAMBLE_PATH", dir)
	runTests()
	if err = reptar.Reptar(dir+"/store", hasher); err != nil {
		t.Error(err)
	}
	os.Setenv("BRAMBLE_PATH", dir2)
	runTests()
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

func TestRunStarlarkBuilder(t *testing.T) {
	b := Bramble{}

	dir, err := ioutil.TempDir("", TestTmpDirPrefix)
	assert.NoError(t, err)
	// set a unique bramble store for these tests
	os.Setenv("BRAMBLE_PATH", dir)
	if err := b.run([]string{"../../tests/starlark-builder:run_busybox"}); err != nil {
		t.Error(err)
	}

	_ = os.RemoveAll(dir)
}
