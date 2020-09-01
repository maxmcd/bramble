package bramble

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/alecthomas/assert"
)

var (
	TestTmpDirPrefix = "bramble-test-"
)

func TestIntegration(t *testing.T) {
	b := Bramble{}

	dir, err := ioutil.TempDir("", TestTmpDirPrefix)
	assert.NoError(t, err)
	// set a unique bramble store for these tests
	os.Setenv("BRAMBLE_PATH", dir)
	if err := b.test([]string{"../../tests"}); err != nil {
		fmt.Printf("%+v", err)
		t.Error(err)
	}

	_ = os.RemoveAll(dir)
}

func TestRunStarlarkBuilder(t *testing.T) {
	t.Skip()
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
