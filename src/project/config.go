package project

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/maxmcd/bramble/v/github.com/go4org/go4/lock"
	"github.com/pkg/errors"
)

type Config struct {
	Module ConfigModule `toml:"module"`
}
type ConfigModule struct {
	Name          string   `toml:"name"`
	ReadOnlyPaths []string `toml:"read_only_paths"`
	HiddenPaths   []string `toml:"hidden_paths"`
}

func (p *Project) AddURLHashesToLockfile(mapping map[string]string) (err error) {
	p.lockFileLock.Lock()
	defer p.lockFileLock.Unlock()
	count := 0
	for {
		done, err := lock.Lock("brambleconfig.lock")
		if count++; err != nil && strings.Contains(err.Error(), "temporarily") && count < 200 {
			continue
		}
		if err != nil {
			return err
		}
		defer done.Close()
		break
	}

	f, err := os.OpenFile(filepath.Join(p.location, "bramble.lock"),
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

	for url, hash := range mapping {
		if v, ok := lf.URLHashes[url]; ok && v != hash {
			return errors.Errorf("found existing hash for %q with value %q not %q, not sure how to proceed", url, v, hash)
		}
		lf.URLHashes[url] = hash
	}

	return toml.NewEncoder(f).Encode(&lf)
}
