package project

import (
	"path/filepath"
	"strings"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/pkg/errors"
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

func (p *Project) parseModuleFuncArgument(name string) (module, function string, err error) {
	if name == "" {
		return "", "", errors.New("module name can't be blank")
	}
	// TODO: now that we accept a build string without a : we must ensure we
	// support calls when there is a : in the path
	lastIndex := strings.LastIndex(name, ":")
	path := name
	if lastIndex >= 0 {
		// We found a ":", split the path
		path, function = name[:lastIndex], name[lastIndex+1:]
	}

	module, err = p.moduleFromPath(path)
	return
}

func (p *Project) moduleFromPath(path string) (thisModule string, err error) {
	thisModule = (p.config.Package.Name + "/" + p.relativePathFromConfig())

	// See if this path is actually the name of a module, for now we just
	// support one module.
	// TODO: search through all modules in scope for this config
	if strings.HasPrefix(path, p.config.Package.Name) {
		return path, nil
	}

	// if the relative path is nothing, we've already added the slash above
	if !strings.HasSuffix(thisModule, "/") {
		thisModule += "/"
	}

	// support things like bar/main.bramble:foo
	if strings.HasSuffix(path, BrambleExtension) &&
		fileutil.FileExists(filepath.Join(p.wd, path)) {
		return thisModule + path[:len(path)-len(BrambleExtension)], nil
	}

	fullName := path + BrambleExtension
	if !fileutil.FileExists(filepath.Join(p.wd, fullName)) {
		if !fileutil.FileExists(filepath.Join(p.wd, path+"/default.bramble")) {
			return "", errors.Errorf("%q: no such file or directory", path)
		}
	}
	// we found it, return
	thisModule += filepath.Join(path)
	return strings.TrimSuffix(thisModule, "/"), nil
}
