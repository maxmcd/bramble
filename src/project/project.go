package project

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/src/tracing"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

const BrambleExtension = ".bramble"

var (
	ErrNotInProject = errors.New("couldn't find a bramble.toml file in this directory or any parent")
	tracer          = tracing.Tracer("project")
)

type Project struct {
	config   Config
	location string

	wd string

	lockFile LockFile
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
	return p, p.readConfigs()
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
	changed   bool
	lock      sync.RWMutex
}

func (l *LockFile) AddEntry(k, v string) error {
	l.lock.Lock()
	defer l.lock.Unlock()
	oldV, found := l.URLHashes[k]
	if found && oldV != v {
		return errors.Errorf(
			"Existing lockfile entry found for %q, old hash %q does not equal new has value %q",
			k, oldV, v)
	}
	if !found {
		l.URLHashes[k] = v
		l.changed = true
	}
	return nil
}

func (l *LockFile) LookupEntry(k string) (v string, found bool) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	v, found = l.URLHashes[k]
	return v, found
}

// Interface is defined in both the project and build packages
type LockfileWriter interface {
	AddEntry(string, string) error
	LookupEntry(string) (v string, found bool)
}

func (p *Project) LockfileWriter() LockfileWriter {
	return &p.lockFile
}

func (p *Project) Location() string {
	return p.location
}

func (p *Project) Module() string {
	return p.config.Module.Name
}

func (p *Project) Version() string {
	return p.config.Module.Version
}

func (p *Project) WD() string {
	return p.wd
}

func (p *Project) ReadOnlyPaths() (out []string) {
	for _, path := range p.config.Module.ReadOnlyPaths {
		out = append(out, filepath.Join(p.location, path))
	}
	return
}
func (p *Project) HiddenPaths() (out []string) {
	for _, path := range p.config.Module.HiddenPaths {
		out = append(out, filepath.Join(p.location, path))
	}
	return
}

func (p *Project) URLHashes() map[string]string {
	return p.lockFile.URLHashes
}

func (p *Project) filepathToModuleName(path string) (module string, err error) {
	if !strings.HasSuffix(path, BrambleExtension) {
		return "", errors.Errorf("path %q is not a bramblefile", path)
	}
	if !fileutil.FileExists(path) {
		return "", errors.Wrap(os.ErrNotExist, path)
	}
	rel, err := filepath.Rel(p.location, path)
	if err != nil {
		return "", errors.Wrapf(err, "%q is not relative to the project directory %q", path, p.location)
	}
	if strings.HasSuffix(path, "default"+BrambleExtension) {
		rel = strings.TrimSuffix(rel, "default"+BrambleExtension)
	} else {
		rel = strings.TrimSuffix(rel, BrambleExtension)
	}
	rel = strings.TrimSuffix(rel, "/")
	return p.config.Module.Name + "/" + rel, nil
}

type ModuleDoc struct {
	Name      string
	Docstring string
	Functions []FunctionDoc
}

type FunctionDoc struct {
	Docstring  string
	Name       string
	Definition string
}

func (p *Project) ListModuleDoc() (modules []ModuleDoc, err error) {
	files, err := filepath.Glob("*.bramble")
	if err != nil {
		return nil, errors.Wrap(err, "error finding bramble files in the current directory")
	}
	dirs, err := filepath.Glob("*/default.bramble")
	if err != nil {
		return nil, errors.Wrap(err, "error finding default.bramble files in subdirectories")
	}
	for _, path := range append(files, dirs...) {
		m, err := p.parsedModuleDocFromPath(path)
		if err != nil {
			return nil, err
		}
		modules = append(modules, m)
	}
	return
}

func (p *Project) parsedModuleDocFromPath(path string) (m ModuleDoc, err error) {
	rt := newRuntime("", "", "") // don't need a real one, just need the list of predeclared values
	absPath, err := filepath.Abs(path)
	if err != nil {
		return m, errors.Wrap(err, "list functions path is invalid")
	}
	file, _, err := starlark.SourceProgram(absPath, nil, rt.predeclared.Has)
	if err != nil {
		return m, err
	}
	m = p.parseStarlarkFile(file)
	m.Name, err = p.filepathToModuleName(file.Path)
	if err != nil {
		return m, errors.Wrap(err, "error with module path")
	}
	return m, nil
}

func (p *Project) FindAllModules() (modules []string, err error) {
	return modules, filepath.Walk(
		p.Location(),
		func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if filepath.Base(path) == "bramble.toml" &&
				path != filepath.Join(p.Location(), "bramble.toml") {
				return filepath.SkipDir
			}
			// TODO: ignore .git, ignore .gitignore?
			if strings.HasSuffix(path, ".bramble") {
				module, err := p.filepathToModuleName(path)
				if err != nil {
					return err
				}
				modules = append(modules, module)
			}
			return nil
		},
	)
}
