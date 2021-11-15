package project

import (
	"path/filepath"

	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

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
	rt := p.newRuntime("")
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
