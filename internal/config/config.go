package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/maxmcd/bramble/internal/types"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/fxt"
	"github.com/maxmcd/bramble/v/github.com/go4org/go4/lock"
	"github.com/pkg/errors"
	"golang.org/x/mod/semver"
)

type Config struct {
	Package      Package `toml:"package"`
	Dependencies map[string]Dependency
}

func (cfg Config) Render(w io.Writer) {
	fmt.Fprintln(w, "[package]")
	fxt.Fprintfln(w, "name = %q", cfg.Package.Name)
	fxt.Fprintfln(w, "version = %q", cfg.Package.Version)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "[dependencies]")
	var keys []string
	for key := range cfg.Dependencies {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		dep := cfg.Dependencies[key]
		if dep.Path == "" {
			fxt.Fprintfln(w, "%q = %q", key, dep.Version)
		} else {
			fxt.Fprintfln(w, "%q = {version=%q, path=%q}", key, dep.Version, dep.Path)
		}
	}
}

// LoadValueToDependency takes the string from a `load()` statement and returns
// the matching dependency in this config, if there is one
func (cfg Config) LoadValueToDependency(val string) string {
	longest := ""
	if strings.HasPrefix(val, cfg.Package.Name) {
		// TODO: need to support subprojects that could be within the projects import path
		return ""
	}

	for dep := range cfg.Dependencies {
		if strings.HasPrefix(val, dep) && len(dep) > len(longest) {
			longest = dep
		}
	}
	return longest
}

type Dependency struct {
	Version string
	Path    string
}

func (c *Dependency) UnmarshalTOML(data interface{}) error {
	switch v := data.(type) {
	case string:
		c.Version = v
	case map[string]interface{}:
		if s, ok := v["version"].(string); ok {
			c.Version = s
		}
		if s, ok := v["path"].(string); ok {
			c.Path = s
		}
	default:
		return errors.New("unexpected data type")
	}
	return nil
}

type Package struct {
	Name          string   `toml:"name"`
	Version       string   `toml:"version"`
	ReadOnlyPaths []string `toml:"read_only_paths"`
	HiddenPaths   []string `toml:"hidden_paths"`
}

func getConfigLock(dir string) (io.Closer, error) {
	count := 0
	for {
		done, err := lock.Lock(filepath.Join(dir, "brambleconfig.lock"))
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

func ReadConfig(location string) (cfg Config, err error) {
	f, err := os.Open(location)
	if err != nil {
		return cfg, errors.Wrapf(err, "error loading %q", location)
	}
	defer f.Close()
	cfg, err = ParseConfig(f)
	return cfg, errors.Wrapf(err, "error decoding %q", location)
}

func ParseConfig(r io.Reader) (cfg Config, err error) {
	if _, err = toml.DecodeReader(r, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Package.Name == "" {
		return cfg, errors.New("Package name can't be blank")
	}
	if cfg.Package.Version == "" {
		return cfg, errors.New("Version can't be blank")
	}
	if !semver.IsValid("v" + cfg.Package.Version) {
		return cfg, errors.Errorf("Package version %q is not a valid sematic version number", cfg.Package.Version)
	}
	return cfg, nil
}

func ReadConfigs(dir string) (cfg Config, lockFile *Lockfile, err error) {
	{
		bDotToml := filepath.Join(dir, "bramble.toml")
		cfg, err = ReadConfig(bDotToml)
		if err != nil {
			return cfg, nil, err
		}
		if cfg.Dependencies == nil {
			cfg.Dependencies = map[string]Dependency{}
		}
	}
	{
		lockFileLocation := filepath.Join(dir, "bramble.lock")
		if !fileutil.FileExists(lockFileLocation) {
			// Don't read the lockfile if we don't have one
			return cfg, &Lockfile{}, err
		}
		f, err := os.Open(lockFileLocation)
		if err != nil {
			return cfg, lockFile, errors.Wrapf(err, "error opening lockfile %q", lockFileLocation)
		}
		defer f.Close()
		_, err = toml.DecodeReader(f, &lockFile)
		return cfg, lockFile, errors.Wrapf(err, "error decoding lockfile %q", lockFileLocation)
	}
}

func WriteLockfile(lockFile *Lockfile, dir string) (err error) {
	lockFile.lock.Lock()
	defer lockFile.lock.Unlock()

	// Get lock on lockfile
	done, err := getConfigLock(dir)
	if err != nil {
		return err
	}
	defer done.Close()

	f, err := os.OpenFile(filepath.Join(dir, "bramble.lock"),
		os.O_RDWR|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	lf := Lockfile{
		URLHashes: map[string]string{},
	}
	if _, err := toml.DecodeReader(f, &lf); err != nil {
		return err
	}
	_ = f.Truncate(0)
	_, _ = f.Seek(0, 0)
	if !reflect.DeepEqual(lockFile.URLHashes, lf.URLHashes) {
		for url, hash := range lockFile.URLHashes {
			if v, ok := lf.URLHashes[url]; ok && v != hash {
				return errors.Errorf("found existing hash for %q with value %q not %q, not sure how to proceed", url, v, hash)
			}
			lf.URLHashes[url] = hash
		}
	}
	lf.Dependencies = lockFile.Dependencies
	lockFile.Render(f)
	return nil
}

type Lockfile struct {
	URLHashes map[string]string
	lock      sync.RWMutex

	Dependencies map[string]Dependency
}

func (lf *Lockfile) Render(w io.Writer) {
	var keys []string
	{
		fmt.Fprintln(w, "[URLHashes]")
		for key := range lf.URLHashes {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fxt.Fprintfln(w, "%q = %q", key, lf.URLHashes[key])
		}
	}
	if len(lf.Dependencies) == 0 {
		return
	}
	fmt.Fprintln(w)
	{
		fmt.Fprintln(w, "[Dependencies]")
		var keys []string
		for key := range lf.Dependencies {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			dep := lf.Dependencies[key]
			fxt.Fprintfln(w, "%q = %q", key, dep.Version)
		}
	}
}

var _ types.LockfileWriter = new(Lockfile)

func (lf *Lockfile) AddEntry(k, v string) error {
	lf.lock.Lock()
	defer lf.lock.Unlock()
	oldV, found := lf.URLHashes[k]
	if found && oldV != v {
		return errors.Errorf(
			"Existing lockfile entry found for %q, old hash %q does not equal new has value %q",
			k, oldV, v)
	}
	if !found {
		if lf.URLHashes == nil {
			lf.URLHashes = map[string]string{}
		}
		lf.URLHashes[k] = v
	}
	return nil
}

func (lf *Lockfile) LookupEntry(k string) (v string, found bool) {
	lf.lock.RLock()
	defer lf.lock.RUnlock()
	v, found = lf.URLHashes[k]
	return v, found
}

type ConfigAndLockfile struct {
	Lockfile *Lockfile
	Config   Config
}
