package bramble

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	TestTmpDirPrefix = "bramble-test-"
)

func TestMain(m *testing.M) {
	// call flag.Parse() here if TestMain uses flags
	code := m.Run()

	// Don't delete if we error, we might want to check the folder contents
	if code != 0 {
		os.Exit(code)
		return
	}
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), TestTmpDirPrefix+"*"))
	if err != nil {
		panic(err)
	}
	_ = matches
	for _, dir := range matches {
		if err = os.RemoveAll(dir); err != nil {
			panic(err)
		}
	}

	os.Exit(code)
}

type TestFile struct {
	Header *tar.Header
	Body   []byte
}

func PanicOnErr(err error) {
	if err != nil {
		fmt.Printf("%+v", err)
		panic(err)
	}
}

func TarGZipTestFiles(files []TestFile) (b []byte, err error) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	w := tar.NewWriter(gzw)
	for _, file := range files {
		file.Header.Size = int64(len(file.Body))
		if err = w.WriteHeader(file.Header); err != nil {
			return
		}
		if file.Body != nil {
			if _, err = w.Write(file.Body); err != nil {
				return
			}
		}
	}
	if err = w.Close(); err != nil {
		return
	}
	if err = gzw.Close(); err != nil {
		return
	}
	return buf.Bytes(), nil
}

func NewTestClient(t *testing.T) *Client {
	dir, err := ioutil.TempDir("", TestTmpDirPrefix)
	assert.NoError(t, err)
	os.Setenv("BRAMBLE_PATH", dir)
	c, err := NewClient()
	assert.NoError(t, err)
	return c
}
