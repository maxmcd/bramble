package project

import (
	"flag"
	"path/filepath"
	"strings"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/internal/logger"
	"github.com/pkg/errors"
)

func (rt *runtime) moduleNameFromFileName(filename string) (moduleName string, err error) {
	if !filepath.IsAbs(filename) {
		filename = filepath.Join(rt.workingDirectory, filename)
	}
	filename, err = filepath.Abs(filename)
	if err != nil {
		return "", err
	}
	if !fileutil.FileExists(filename) {
		return "", errors.Errorf("bramble file %q doesn't exist", filename)
	}
	if !strings.HasPrefix(filename, rt.projectLocation) {
		return "", errors.New("we don't support external modules yet")
	}
	relativeWorkspacePath, err := filepath.Rel(rt.projectLocation, filename)
	if err != nil {
		return "", err
	}
	moduleName = filepath.Join("github.com/maxmcd/bramble", relativeWorkspacePath)
	moduleName = strings.TrimSuffix(moduleName, "/default"+BrambleExtension)
	moduleName = strings.TrimSuffix(moduleName, BrambleExtension)
	return
}

func (rt *runtime) parseModuleFuncArgument(args []string) (module, function string, err error) {
	if len(args) == 0 {
		logger.Print(`"bramble build" requires 1 argument`)
		return "", "", flag.ErrHelp
	}

	firstArgument := args[0]
	// TODO: now that we accept a build string without a : we must ensure we
	// support calls when there is a : in the path
	lastIndex := strings.LastIndex(firstArgument, ":")
	path := firstArgument
	if lastIndex >= 0 {
		// We found a ":", split the path
		path, function = firstArgument[:lastIndex], firstArgument[lastIndex+1:]
	}

	module, err = rt.moduleFromPath(path)
	return
}

func (rt *runtime) moduleFromPath(path string) (thisModule string, err error) {
	thisModule = (rt.moduleName + "/" + rt.relativePathFromConfig())

	// See if this path is actually the name of a module, for now we just
	// support one module.
	// TODO: search through all modules in scope for this config
	if strings.HasPrefix(path, rt.moduleName) {
		return path, nil
	}

	// if the relative path is nothing, we've already added the slash above
	if !strings.HasSuffix(thisModule, "/") {
		thisModule += "/"
	}

	// support things like bar/main.bramble:foo
	if strings.HasSuffix(path, BrambleExtension) &&
		fileutil.FileExists(filepath.Join(rt.workingDirectory, path)) {
		return thisModule + path[:len(path)-len(BrambleExtension)], nil
	}

	fullName := path + BrambleExtension
	if !fileutil.FileExists(filepath.Join(rt.workingDirectory, fullName)) {
		if !fileutil.FileExists(filepath.Join(rt.workingDirectory, path+"/default.bramble")) {
			return "", errors.Errorf("%q: no such file or directory", path)
		}
	}
	// we found it, return
	thisModule += filepath.Join(path)
	return strings.TrimSuffix(thisModule, "/"), nil
}
