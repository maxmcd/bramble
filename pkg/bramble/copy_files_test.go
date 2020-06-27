package bramble

import (
	"fmt"
	"io/ioutil"
	"testing"
)

func TestCopyFiles(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "bramble-test-")
	if err != nil {
		t.Error(err)
	}
	fmt.Println(tmpDir)
	CopyFiles([]string{"/home/maxm/go/src/github.com/maxmcd/bramble/simple/simple.c", "/home/maxm/go/src/github.com/maxmcd/bramble/simple/simple_builder.sh"},
		tmpDir)
}
