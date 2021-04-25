package bramble

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/pkg/errors"
)

type Config struct {
	Module ConfigModule `toml:"module"`
}
type ConfigModule struct {
	Name string `toml:"name"`
}

func findConfig(wd string) (found bool, location string) {
	if o, _ := filepath.Abs(wd); o != "" {
		wd = o
	}
	for {
		if fileutil.FileExists(filepath.Join(wd, "bramble.toml")) {
			return true, wd
		}
		fmt.Println(wd, filepath.Join(wd, ".."))
		if wd == filepath.Join(wd, "..") {
			return false, ""
		}
		wd = filepath.Join(wd, "..")
	}
}

func (b *Bramble) loadConfig(location string) (err error) {
	b.configLocation = location
	bDotToml := filepath.Join(location, "bramble.toml")
	f, err := os.Open(bDotToml)
	if err != nil {
		return errors.Wrapf(err, "error loading %q", bDotToml)
	}
	defer f.Close()
	if _, err = toml.DecodeReader(f, &b.config); err != nil {
		return errors.Wrapf(err, "error decoding %q", bDotToml)
	}
	lockFile := filepath.Join(location, "bramble.lock")
	if !fileutil.FileExists(lockFile) {
		return
	}
	f, err = os.Open(lockFile)
	if err != nil {
		return errors.Wrapf(err, "error opening lockfile %q", lockFile)
	}
	defer f.Close()
	_, err = toml.DecodeReader(f, &b.lockFile)
	return errors.Wrapf(err, "error decoding lockfile %q", lockFile)
}

type LockFile struct {
	URLHashes map[string]string
}

func (b *Bramble) writeConfigMetadata() (err error) {
	return b.store.WriteConfigLink(b.configLocation)
}

func (b *Bramble) addURLHashToLockfile(url, hash string) (err error) {
	b.lockFileLock.Lock()
	defer b.lockFileLock.Unlock()

	f, err := os.OpenFile(filepath.Join(b.configLocation, "bramble.lock"),
		os.O_RDWR|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	lf := LockFile{
		URLHashes: map[string]string{},
	}
	if _, err = toml.DecodeReader(f, &lf); err != nil {
		return
	}
	_ = f.Truncate(0)
	_, _ = f.Seek(0, 0)
	if v, ok := lf.URLHashes[url]; ok && v != hash {
		return errors.Errorf("found existing hash for %q with value %q not %q, not sure how to proceed", url, v, hash)
	}
	lf.URLHashes[url] = hash

	return toml.NewEncoder(f).Encode(&lf)
}
