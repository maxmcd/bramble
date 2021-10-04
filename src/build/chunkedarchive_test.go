package build

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"

	"github.com/maxmcd/bramble/pkg/chunkedarchive"
	"github.com/maxmcd/bramble/pkg/hasher"
	"github.com/stretchr/testify/require"
)

// TestChunkedArchive just tests that whatever files there are in the store will
// remain the same if they are chunk archived and then unarchived. So a bit
// arbitrary.
func TestChunkedArchive(t *testing.T) {
	store, err := NewStore("")
	require.NoError(t, err)
	files, err := os.ReadDir(store.StorePath)
	require.NoError(t, err)

	for _, dir := range files {
		if !dir.IsDir() {
			continue
		}
		t.Run(dir.Name(), func(t *testing.T) {
			mw := &memWriter{chunks: map[string][]byte{}}
			loc := store.joinStorePath(dir.Name())
			toc, err := chunkedarchive.Archive(loc, mw)
			require.NoError(t, err)
			tempDir, err := ioutil.TempDir("", "")
			require.NoError(t, err)
			err = chunkedarchive.Unarchive(toc, mw, tempDir)
			require.NoError(t, err)
			cmd := "git diff --color=never --no-index " + tempDir + " " + loc
			b, err := exec.Command("bash", "-c", cmd).CombinedOutput()
			if err != nil {
				fmt.Println(cmd)
				fmt.Println(string(b))
			}
			require.NoError(t, err)
			t.Cleanup(func() { os.RemoveAll(tempDir) })
		})
	}
}

type memWriter struct {
	chunks map[string][]byte
}

func (mw *memWriter) NewChunk(f io.ReadCloser) (func() ([]string, error), error) {
	h := hasher.NewHasher()
	b, err := io.ReadAll(io.TeeReader(f, h))
	if errClose := f.Close(); errClose != nil && err == nil {
		err = errClose
	}
	if err != nil {
		return nil, err
	}
	mw.chunks[h.String()] = b

	return func() ([]string, error) {
		return []string{h.String()}, nil
	}, nil
}

func (mw *memWriter) Lookup(hash string) (body io.ReadCloser, err error) {
	chunk, ok := mw.chunks[hash]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewBuffer(chunk)), nil
}

func TestStreamChunkedArchive(t *testing.T) {
	store, err := NewStore("")
	require.NoError(t, err)
	files, err := os.ReadDir(store.StorePath)
	require.NoError(t, err)

	for _, dir := range files {
		if !dir.IsDir() {
			continue
		}

		t.Run(dir.Name(), func(t *testing.T) {
			loc := store.joinStorePath(dir.Name())

			var tempFileLocation string
			{
				streamFile, err := os.CreateTemp("", "")
				tempFileLocation = streamFile.Name()
				require.NoError(t, err)
				err = chunkedarchive.StreamArchive(loc, streamFile)
				require.NoError(t, err)

				_ = streamFile.Close()
			}

			var tempDir string
			{
				streamFile, err := os.Open(tempFileLocation)
				require.NoError(t, err)
				fi, err := streamFile.Stat()
				require.NoError(t, err)

				tempDir, err = ioutil.TempDir("", "")
				require.NoError(t, err)
				err = chunkedarchive.StreamUnarchive(io.NewSectionReader(streamFile, 0, fi.Size()), tempDir)
				require.NoError(t, err)
			}

			cmd := "git diff --color=never --no-index " + tempDir + " " + loc
			b, err := exec.Command("bash", "-c", cmd).CombinedOutput()
			if err != nil {
				fmt.Println(cmd)
				fmt.Println(string(b))
			}
			require.NoError(t, err)
			t.Cleanup(func() {
				os.RemoveAll(tempDir)
				os.RemoveAll(tempFileLocation)
			})
		})
	}
}
