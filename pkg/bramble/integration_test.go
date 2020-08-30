package bramble

import (
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
		t.Error(err)
	}

	_ = os.RemoveAll(dir)
}
