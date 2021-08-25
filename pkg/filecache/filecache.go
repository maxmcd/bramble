package filecache

import (
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/pkg/errors"
)

var ErrUninitialized = errors.New("filecache has not been initized, did you make it with NewFileCache?")

func NewFileCache(dir string) (fc FileCache, err error) {
	u, err := user.Current()
	if err != nil {
		return fc, errors.Wrap(err, "error fetching user's home directory")
	}
	fc = FileCache{dir: filepath.Join(u.HomeDir, ".cache", dir)}
	if !fileutil.DirExists(fc.dir) {
		// Make either ~/.cache or ~/.cache/{dir} if they don't exist
		return fc, os.MkdirAll(fc.dir, 0755)
	}
	return fc, nil
}

type FileCache struct {
	dir string
}

func (fc FileCache) Open(name string) (f *os.File, err error) {
	p, err := fc.Path(name)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}
func (fc FileCache) Exists(name string) (bool, error) {
	p, err := fc.Path(name)
	if err != nil {
		return false, err
	}
	return fileutil.PathExists(p), nil
}

func (fc FileCache) Write(name string, b []byte) (err error) {
	p, err := fc.Path(name)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(p, b, 0666)
}
func (fc FileCache) Path(name string) (string, error) {
	if fc.dir == "" {
		return "", ErrUninitialized
	}
	return filepath.Join(fc.dir, name), nil
}
