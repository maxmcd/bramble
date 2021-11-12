package store

import (
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

const letterBytes = "0123456789"

func init() {
	rand.Seed(time.Now().UnixNano())
}

func randStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func (s *Store) storeLengthTempFile() (f *os.File, err error) {
	dir := s.StorePath
	prefix := buildDirPrefix
	try := 0
	for {
		name := filepath.Join(dir, prefix+randStringBytes(32-len(prefix)))
		f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
		if os.IsExist(err) {
			if try++; try < 10000 {
				continue
			}
			return nil, &os.PathError{Op: "createtemp", Path: dir + string(os.PathSeparator) + name, Err: os.ErrExist}
		}
		return f, err
	}
}

// storeLengthTempDir reimplements os.MkdirTemp with a guaranteed fixed path length
// and a folder permission bit of 0755. Added this after the tmpdir path length
// changed between go 1.16 and 1.17
func (s *Store) storeLengthTempDir() (string, error) {
	dir := s.StorePath
	prefix := buildDirPrefix
	try := 0
	for {
		name := filepath.Join(dir, prefix+randStringBytes(32-len(prefix)))
		err := os.Mkdir(name, 0755)
		if err == nil {
			return name, nil
		}
		if os.IsExist(err) {
			if try++; try < 10000 {
				continue
			}
			return "", &os.PathError{Op: "mkdirtemp", Path: dir + string(os.PathSeparator) + name, Err: os.ErrExist}
		}
		if os.IsNotExist(err) {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				return "", err
			}
		}
		return "", err
	}
}
