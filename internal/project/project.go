// Package project handles language execution, package management and configuration for bramble projects.
package project

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/maxmcd/bramble/internal/config"
	"github.com/maxmcd/bramble/internal/dependency"
	"github.com/maxmcd/bramble/internal/tracing"
	"github.com/maxmcd/bramble/internal/types"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/fxt"
	"github.com/pkg/errors"
)

const BrambleExtension = ".bramble"

var (
	ErrNotInProject = errors.New("couldn't find a bramble.toml file in this directory or any parent")
	tracer          = tracing.Tracer("project")
)

type ModuleFetcher func(context.Context, types.Package) (path string, err error)

type Project struct {
	config   config.Config
	location string

	wd string

	lockFile *config.LockFile

	dm *dependency.Manager
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

func (p *Project) AddModuleFetcher(dm *dependency.Manager) {
	p.dm = dm
}

func (p *Project) Location() string {
	return p.location
}

func (p *Project) Module() string {
	return p.config.Package.Name
}

func (p *Project) Version() string {
	return p.config.Package.Version
}

func (p *Project) Config() config.Config {
	// Copy it?
	return p.config
}

func (p *Project) WD() string {
	return p.wd
}

func (p *Project) ReadOnlyPaths() (out []string) {
	for _, path := range p.config.Package.ReadOnlyPaths {
		out = append(out, filepath.Join(p.location, path))
	}
	return
}

func (p *Project) HiddenPaths() (out []string) {
	for _, path := range p.config.Package.HiddenPaths {
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

func (p *Project) findAllBramblefiles(path string) (files []string, err error) {
	path, err = fileutil.Abs(p.wd, path)
	if err != nil {
		return nil, err
	}
	if err := fileutil.PathWithinDir(p.location, path); err != nil {
		return nil, err
	}
	return files, filepath.WalkDir(
		path,
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path != p.location && d.IsDir() && fileutil.FileExists(filepath.Join(path, "bramble.toml")) {
				return fs.SkipDir
			}

			// TODO: ignore .git, ignore .gitignore?
			if strings.HasSuffix(path, ".bramble") {
				files = append(files, path)
			}
			return nil
		},
	)
}

func (p *Project) CalculateDependencies() (err error) {
	names, err := p.scanForLoadNames()
	if err != nil {
		return errors.Wrap(err, "error scanning for load statements")
	}

	if len(names) == 0 {
		return nil
	}
	external := []string{}
	for _, name := range names {
		if v := p.config.LoadValueToDependency(name); v == "" {
			external = append(external, name)
		}
	}
	if len(external) == 0 {
		return nil
	}

	fxt.Printqln(external)

	// TODO: lots of magic to navigate here, do it sometime
	return nil
}

func (p *Project) AddDependency(ctx context.Context, v types.Package) (err error) {
	name, vs, err := p.dm.FindPackageFromModuleName(ctx, v.Name, "")
	if err != nil {
		return err
	}
	_, _ = name, vs

	existing, found := p.config.Dependencies[v.Name]
	if found {
		if v.Version != "" {
			// TODO: validate that the version is well formed
			existing.Version = v.Version
			p.config.Dependencies[v.Name] = existing
		}
	} else {
		p.config.Dependencies[v.Name] = config.Dependency{
			Version: v.Version,
		}
	}
	cfg, err := p.dm.CalculateConfigBuildlist(ctx, p.config)
	if err != nil {
		return err
	}
	cfg.Render(os.Stdout)
	return p.writeConfig(cfg)
}

func (p *Project) writeConfig(cfg config.Config) (err error) {
	f, err := os.Create(filepath.Join(p.location, "bramble.toml"))
	if err != nil {
		return err
	}
	cfg.Render(f)
	return f.Close()
}

func FindAllProjects(loc string) (paths []string, err error) {
	return paths, filepath.WalkDir(loc, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "bramble.toml" {
			paths = append(paths, filepath.Dir(path))
		}
		return nil
	})
}
