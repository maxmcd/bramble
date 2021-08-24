package filecache

import (
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/pkg/errors"
)

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
	return os.Open(fc.Path(name))
}
func (fc FileCache) Exists(name string) bool {
	return fileutil.PathExists(name)
}

func (fc FileCache) Write(name string, b []byte) (err error) {
	return ioutil.WriteFile(name, b, 0666)
}
func (fc FileCache) Path(name string) string {
	return filepath.Join(fc.dir, name)
}
