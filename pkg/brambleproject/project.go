package brambleproject

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/pkg/errors"
)

const BrambleExtension = ".bramble"

var (
	ErrNotInProject = errors.New("couldn't find a bramble.toml file in this directory or any parent")
)

type Project struct {
	config   Config
	location string

	wd string

	lockFile     LockFile
	lockFileLock sync.Mutex
}

type Config struct {
	Module ConfigModule `toml:"module"`
}
type ConfigModule struct {
	Name string `toml:"name"`
}

// NewProject checks for an existing bramble project in the provided working
// directory and loads its configuration details if one is found.
func NewProject(wd string) (p *Project, err error) {
	absWD, err := filepath.Abs(wd)
	if err != nil {
		return nil, errors.Wrapf(err, "can't convert relative working directory path %q to absolute path", wd)
	}
	found, location := findConfig(absWD)
	if !found {
		return nil, ErrNotInProject
	}
	p = &Project{
		location: location,
		wd:       absWD,
	}
	bDotToml := filepath.Join(location, "bramble.toml")
	f, err := os.Open(bDotToml)
	if err != nil {
		return nil, errors.Wrapf(err, "error loading %q", bDotToml)
	}
	defer f.Close()
	if _, err = toml.DecodeReader(f, &p.config); err != nil {
		return nil, errors.Wrapf(err, "error decoding %q", bDotToml)
	}
	lockFile := filepath.Join(location, "bramble.lock")
	if !fileutil.FileExists(lockFile) {
		// Don't read the lockfile if we don't have one
		return p, nil
	}
	f, err = os.Open(lockFile)
	if err != nil {
		return nil, errors.Wrapf(err, "error opening lockfile %q", lockFile)
	}
	defer f.Close()
	_, err = toml.DecodeReader(f, &p.lockFile)
	return p, errors.Wrapf(err, "error decoding lockfile %q", lockFile)
}

func findConfig(wd string) (found bool, location string) {
	if o, _ := filepath.Abs(wd); o != "" {
		wd = o
	}
	for {
		if fileutil.FileExists(filepath.Join(wd, "bramble.toml")) {
			return true, wd
		}
		if wd == filepath.Join(wd, "..") {
			return false, ""
		}
		wd = filepath.Join(wd, "..")
	}
}

type LockFile struct {
	URLHashes map[string]string
}

func (p *Project) Location() string {
	return p.location
}

func (p *Project) AddURLHashesToLockfile(mapping map[string]string) (err error) {
	p.lockFileLock.Lock()
	defer p.lockFileLock.Unlock()

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
