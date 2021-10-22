// Package project handles language execution, package management and configuration for bramble projects.
package project

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/maxmcd/bramble/internal/config"
	"github.com/maxmcd/bramble/internal/dependency"
	"github.com/maxmcd/bramble/internal/tracing"
	"github.com/maxmcd/bramble/internal/types"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

const BrambleExtension = ".bramble"

var (
	ErrNotInProject = errors.New("couldn't find a bramble.toml file in this directory or any parent")
	tracer          = tracing.Tracer("project")
)

type ModuleFetcher func(context.Context, dependency.Version) (path string, err error)

type Project struct {
	config   config.Config
	location string

	wd string

	lockFile *config.LockFile

	moduleFetcher ModuleFetcher
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
	p.config, p.lockFile, err = config.ReadConfigs(location)
	return p, err
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

func (p *Project) LockfileWriter() types.LockfileWriter {
	return p.lockFile
}

func (p *Project) AddModuleFetcher(mf ModuleFetcher) {
	p.moduleFetcher = mf
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

func (p *Project) Config() config.Config {
	// Copy it?
	return p.config
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

func (p *Project) WriteLockfile() error {
	return config.WriteLockfile(p.lockFile, p.location)
}

// TODO: function that takes load() argument values and references the config and pulls down the needed version
func (p *Project) fetchExternalModule(ctx context.Context, module string) (path string, err error) {
	if strings.HasPrefix(module, p.config.Module.Name) {
		return "", errors.Errorf("%q is not an external module", module)
	}
	cd, found := p.config.Dependencies[module]
	if !found {
		return "", errors.Errorf("%q is not a dependency of this project, do you need to add it?", module)
	}
	if cd.Path != "" {
		// TODO: Does this actually work
		// TODO: cd.Path must be relative?
		return filepath.Join(p.location, cd.Path), nil
	}
	return p.moduleFetcher(ctx, dependency.Version{Module: module, Version: cd.Version})
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

func (p *Project) ListModuleDoc(wd string) (modules []ModuleDoc, err error) {
	wd, err = filepath.Abs(wd)
	if err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(wd, "*.bramble"))
	if err != nil {
		return nil, errors.Wrap(err, "error finding bramble files in the current directory")
	}
	dirs, err := filepath.Glob(filepath.Join(wd, "*/default.bramble"))
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
	rt := newRuntime("", "", "", "", nil) // don't need a real one, just need the list of predeclared values
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
