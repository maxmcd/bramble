package brambleproject

import (
	"path/filepath"
	"strings"

	"github.com/maxmcd/bramble/pkg/fileutil"
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
