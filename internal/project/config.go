package project

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/v/github.com/go4org/go4/lock"
	"github.com/pkg/errors"
)

type Config struct {
	Module ConfigModule `toml:"module"`
}
type ConfigModule struct {
	Name          string   `toml:"name"`
	Version       string   `toml:"version"`
	ReadOnlyPaths []string `toml:"read_only_paths"`
	HiddenPaths   []string `toml:"hidden_paths"`
}

func (p *Project) getConfigLock() (io.Closer, error) {
	count := 0
	for {
		done, err := lock.Lock("brambleconfig.lock")
		if count++; err != nil &&
			strings.Contains(err.Error(), "temporarily") &&
			count < 5 {
			time.Sleep(time.Millisecond * 10)
			continue
		}
		if err != nil {
			return nil, err
		}
		return done, nil
	}
}

func (p *Project) readConfigs() error {
	bDotToml := filepath.Join(p.location, "bramble.toml")
	f, err := os.Open(bDotToml)
	if err != nil {
		return errors.Wrapf(err, "error loading %q", bDotToml)
	}
	defer f.Close()
	if _, err = toml.DecodeReader(f, &p.config); err != nil {
		return errors.Wrapf(err, "error decoding %q", bDotToml)
	}
	lockFile := filepath.Join(p.location, "bramble.lock")
	if !fileutil.FileExists(lockFile) {
		// Don't read the lockfile if we don't have one
		p.lockFile.URLHashes = map[string]string{}
		return nil
	}
	f, err = os.Open(lockFile)
	if err != nil {
		return errors.Wrapf(err, "error opening lockfile %q", lockFile)
	}
	defer f.Close()
	_, err = toml.DecodeReader(f, &p.lockFile)
	return errors.Wrapf(err, "error decoding lockfile %q", lockFile)
}

func (p *Project) WriteLockfile() (err error) {
	p.lockFile.lock.Lock()
	defer p.lockFile.lock.Unlock()
	if !p.lockFile.changed {
		return nil
	}

	// Get lock on lockfile
	done, err := p.getConfigLock()
	if err != nil {
		return err
	}
	defer done.Close()

	f, err := os.OpenFile(filepath.Join(p.location, "bramble.lock"),
		os.O_RDWR|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	lf := LockFile{
		URLHashes: map[string]string{},
	}
	if _, err := toml.DecodeReader(f, &lf); err != nil {
		return err
	}
	if reflect.DeepEqual(p.lockFile.URLHashes, lf.URLHashes) {
		return nil
	}

	_ = f.Truncate(0)
	_, _ = f.Seek(0, 0)
	for url, hash := range p.lockFile.URLHashes {
		if v, ok := lf.URLHashes[url]; ok && v != hash {
			return errors.Errorf("found existing hash for %q with value %q not %q, not sure how to proceed", url, v, hash)
		}
		lf.URLHashes[url] = hash
	}

	return toml.NewEncoder(f).Encode(&lf)
}
