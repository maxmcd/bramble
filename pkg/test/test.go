package test

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func SetEnv(t *testing.T, k, v string) {
	old, found := os.LookupEnv(k)
	os.Setenv(k, v)
	t.Cleanup(func() {
		if !found {
			os.Unsetenv(k)
		} else {
			os.Setenv(k, old)
		}
	})
}

// Similar to testing.T.TempDir, but doesn't use the test name in the folder
// name. Helpful when a shorter filesystem path is desired.
func TmpDir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "bramble-test-")
	if err != nil {
		t.Fatal(err)
	}
	if t != nil {
		t.Cleanup(func() { os.RemoveAll(dir) })
	}
	return dir
}

func WriteFile(t *testing.T, path, body string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	// No cleanup. Writing random files in tests in unsafe, assume they are
	// being written to a test dir that will be cleaned up, or the user should
	// be left with their boneyard.
}

func ErrContains(t *testing.T, err error, errContains string) {
	t.Helper()
	if errContains == "" && err == nil {
		return
	}
	if errContains == "" && err != nil {
		t.Error(err)
		return
	}
	if errContains != "" && err == nil {
		t.Error(fmt.Sprintf("error should have contained %q, but it was nil", errContains))
	}
	assert.Contains(t, err.Error(), errContains)
}
