package brambleproject

import (
	"fmt"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/pkg/errors"
)

type errModuleDoesNotExist string

func (err errModuleDoesNotExist) Error() string {
	// TODO: this error is confusing because we can find the module we just
	// can't find the file needed to run/import this specific module path
	return fmt.Sprintf("couldn't find module %q", string(err))
}

func (p *Project) ResolveModule(module string) (path string, err error) {
	if !strings.HasPrefix(module, p.config.Module.Name) {
		// TODO: support other modules
		debug.PrintStack()
		err = errors.Errorf("can't find module %s", module)
		return
	}

	path = module[len(p.config.Module.Name):]
	path = filepath.Join(p.Location, path)

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
		return "", errors.WithStack(errModuleDoesNotExist(module))
	}

	return path, nil
}

func (p *Project) moduleFromPath(path string) (thisModule string, err error) {
	thisModule = (p.config.Module.Name + "/" + p.relativePathFromConfig())

	// See if this path is actually the name of a module, for now we just
	// support one module.
	// TODO: search through all modules in scope for this config
	if strings.HasPrefix(path, p.config.Module.Name) {
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

func (p *Project) relativePathFromConfig() string {
	relativePath, _ := filepath.Rel(p.Location, p.wd)
	if relativePath == "." {
		// don't add a dot to the path
		return ""
	}
	return relativePath
}

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
	if !strings.HasPrefix(filename, p.Location) {
		return "", errors.New("we don't support external modules yet")
	}
	relativeWorkspacePath, err := filepath.Rel(p.Location, filename)
	if err != nil {
		return "", err
	}
	moduleName = filepath.Join("github.com/maxmcd/bramble", relativeWorkspacePath)
	moduleName = strings.TrimSuffix(moduleName, "/default"+BrambleExtension)
	moduleName = strings.TrimSuffix(moduleName, BrambleExtension)
	return
}
