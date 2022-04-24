package project

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/maxmcd/bramble/internal/config"
	"github.com/maxmcd/bramble/internal/errs"
	"github.com/maxmcd/bramble/internal/types"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/url2"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

func (p *Project) moduleNameFromFileName(filename string) (moduleName string, err error) {
	if !filepath.IsAbs(filename) {
		filename = filepath.Join(p.wd, filename)
	}
	filename, err = filepath.Abs(filename)
	if err != nil {
		return "", err
	}
	if !fileutil.FileExists(filename) {
		return "", errors.Errorf("bramble file %q doesn't exist", filename)
	}
	if !strings.HasPrefix(filename, p.location) {
		return "", errors.New("we don't support external modules yet")
	}
	relativeWorkspacePath, err := filepath.Rel(p.location, filename)
	if err != nil {
		return "", err
	}
	moduleName = filepath.Join("github.com/maxmcd/bramble", relativeWorkspacePath)
	moduleName = strings.TrimSuffix(moduleName, "/default"+BrambleExtension)
	moduleName = strings.TrimSuffix(moduleName, BrambleExtension)
	return
}

func (p *Project) moduleFromPath(path string) (thisModule string, err error) {
	if !strings.HasPrefix(path, ".") {
		return "", errors.Errorf("path %q is not relative, must start with .", path)
	}
	thisModule = (p.config.Package.Name + "/" + p.relativePathFromConfig())

	// if the relative path is nothing, we've already added the slash above
	if !strings.HasSuffix(thisModule, "/") {
		thisModule += "/"
	}

	// support things like bar/main.bramble:foo
	if strings.HasSuffix(path, BrambleExtension) &&
		fileutil.FileExists(filepath.Join(p.wd, path)) {
		return filepath.Join(thisModule, path[:len(path)-len(BrambleExtension)]), nil
	}

	fullName := path + BrambleExtension
	if !fileutil.FileExists(filepath.Join(p.wd, fullName)) {
		if !fileutil.FileExists(filepath.Join(p.wd, path+"/default.bramble")) {
			return "", errors.Errorf("%q: no such file or directory", path)
		}
	}
	// we found it, return
	thisModule = filepath.Join(thisModule, path)
	return strings.TrimSuffix(thisModule, "/"), nil
}

type Module struct {
	Name     string
	Function string
}

func (p *Project) ArgumentsToModules(ctx context.Context, args []string, allowExternal bool) (modules []Module, err error) {
	for _, arg := range args {
		if idx := strings.Index(arg, "/..."); idx != -1 {
			arg = arg[:idx]
			if strings.HasPrefix(arg, p.config.Package.Name) {
				arg = "." + strings.TrimPrefix(arg, p.config.Package.Name)
			}
			ms, err := p.FindAllModules(arg)
			if err != nil {
				return nil, err
			}
			for _, m := range ms {
				modules = append(modules, Module{Name: m})
			}
		} else {
			module, err := p.ParseModuleFuncArgument(ctx, arg, allowExternal)
			if err != nil {
				return nil, err
			}
			modules = append(modules, module)
		}
	}
	return modules, nil
}

func (p *Project) scanForLoadNames() (moduleNames []string, err error) {
	modules, err := p.findAllBramblefiles(p.location)
	if err != nil {
		return nil, err
	}

	names := map[string]struct{}{}
	rt := p.newRuntime("")
	for _, module := range modules {
		_, program, err := starlark.SourceProgram(module, nil, rt.predeclared.Has)
		if err != nil {
			return nil, err
		}
		for i := 0; i < program.NumLoads(); i++ {
			name, _ := program.Load(i)
			names[name] = struct{}{}
		}
	}
	for n := range names {
		moduleNames = append(moduleNames, n)
	}
	return moduleNames, nil
}

type packageModule struct {
	projectPath string
	relPath     string
	module      string
}

// TODO: function that takes load() argument values and references the config and pulls down the needed version
func (p *Project) findOrDownloadModulePath(ctx context.Context, pkg string) (pm packageModule, err error) {
	if strings.HasPrefix(pkg, p.config.Package.Name) {
		path := pkg[len(p.config.Package.Name):]

		return packageModule{
			projectPath: p.location,
			relPath:     filepath.Clean(path),
			module:      p.config.Package.Name,
		}, nil

	}
	name, dep, found := p.doesModulePackageExist(pkg)
	if !found {
		return packageModule{}, errs.ErrModuleNotFoundInProject{Module: pkg}
	}

	relPath, err := url2.Rel(name, pkg)
	if err != nil {
		return packageModule{}, errors.Wrapf(err, "error calculating relative module path between %s and %s", name, pkg)
	}

	if dep.Path != "" {
		// TODO: Does this actually work
		// TODO: cd.Path must be relative?
		return packageModule{
			projectPath: filepath.Join(p.location, dep.Path),
			relPath:     relPath,
			module:      name,
		}, nil
	}
	projectPath, err := p.dm.PackagePathOrDownload(ctx, types.Package{Name: name, Version: dep.Version})
	if err != nil {
		return packageModule{}, err
	}
	return packageModule{
		projectPath: projectPath,
		relPath:     relPath,
		module:      name,
	}, nil
}

func (p *Project) doesModulePackageExist(pkg string) (name string, dep config.Dependency, found bool) {
	if d, ok := p.config.Dependencies[pkg]; ok {
		return pkg, d, ok
	}
	if d, ok := p.lockFile.Dependencies[pkg]; ok {
		return pkg, d, ok
	}
	var longestName string
	var longestDep config.Dependency
	// No specific match, looking for sub-path match
	for name, d := range p.config.Dependencies {
		if strings.HasPrefix(pkg, name) {
			if len(name) > len(longestName) {
				longestName = name
				longestDep = d
			}
		}
	}
	for name, d := range p.lockFile.Dependencies {
		if strings.HasPrefix(pkg, name) {
			if len(name) > len(longestName) {
				longestName = name
				longestDep = d
			}
		}
	}
	if longestName == "" {
		return "", config.Dependency{}, false
	}
	return longestName, longestDep, true
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
	return p.config.Package.Name + "/" + rel, nil
}

func (p *Project) ParseModuleFuncArgument(ctx context.Context, name string, allowExternal bool) (module Module, err error) {
	if name == "" {
		return Module{}, errors.New("module name can't be blank")
	}

	module.Name = name
	// TODO: now that we accept a build string without a : we must ensure we
	// support calls when there is a : in the path
	lastIndex := strings.LastIndex(name, ":")
	if lastIndex != -1 {
		// We found a ":", split the path
		module.Name, module.Function = name[:lastIndex], name[lastIndex+1:]
	}
	if module.Name == "" {
		return Module{}, errors.Errorf("module name can't be blank: %q", name)
	}
	if strings.HasPrefix(module.Name, ".") {
		module.Name, err = p.moduleFromPath(module.Name)
		return
	}
	pm, err := p.findOrDownloadModulePath(ctx, module.Name)
	if err == nil {
		module.Name = pm.module
		if pm.relPath != "." {
			module.Name += pm.relPath
		}
		return module, nil
	}
	if err != nil && !errors.Is(err, errs.ErrModuleNotFoundInProject{}) {
		return Module{}, err
	}
	// If the module is not found and external is allow, continue to trying
	// to fetch an external module that's not in the project
	if !allowExternal {
		return Module{}, err
	}
	name, versions, err := p.dm.FindPackageFromModuleName(ctx, module.Name, "")
	if err != nil {
		return Module{}, err
	}
	// TODO: pick newest version
	p.config.Dependencies[name] = config.Dependency{Version: versions[0]}
	if err := p.writeConfig(p.config); err != nil {
		return Module{}, errors.Wrapf(err, "error saving config with new package: %s", types.Package{Name: name, Version: versions[0]})
	}
	module.Name = name
	return module, nil

}

func (p *Project) moduleToPath(ctx context.Context, module string) (projectPath, path string, err error) {
	pm, err := p.findOrDownloadModulePath(ctx, module)
	if err != nil {
		return "", "", err
	}
	path = filepath.Join(pm.projectPath, pm.relPath)

	// TODO could make these three syscalls just a single directory file list call
	directoryWithNameExists := fileutil.PathExists(path)

	var directoryHasDefaultDotBramble bool
	if directoryWithNameExists {
		directoryHasDefaultDotBramble = fileutil.FileExists(path + "/default.bramble")
	}

	fileWithNameExists := fileutil.FileExists(path + BrambleExtension)

	switch {
	case directoryWithNameExists && directoryHasDefaultDotBramble:
		path += "/default.bramble"
	case fileWithNameExists:
		path += BrambleExtension
	default:
		return "", "", errors.Errorf("Module %q not found, %q is not a directory and %q does not exist",
			module, path, path+BrambleExtension)
	}

	return pm.projectPath, path, nil
}

func (p *Project) FindAllModules(path string) (modules []string, err error) {
	files, err := p.findAllBramblefiles(path)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		module, err := p.filepathToModuleName(file)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(module, "/") {
			module = module[:len(module)-1]
		}
		modules = append(modules, module)
	}
	return modules, nil
}
