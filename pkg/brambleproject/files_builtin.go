package brambleproject

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

type FilesList struct {
	Files    []string
	Location string
}

var _ starlark.Value = new(FilesList)

func (fl FilesList) Freeze()               {}
func (fl FilesList) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("file_list") }
func (fl FilesList) String() string        { return fmt.Sprint([]string(fl.Files)) }
func (fl FilesList) Type() string          { return "file_list" }
func (fl FilesList) Truth() starlark.Bool  { return true }

type filesBuiltin struct {
	projectLocation string
}

func (fb filesBuiltin) starlarkGlobListFiles(includeDirectories bool, fileDirectory string, list *starlark.List) (map[string]struct{}, error) {
	projFilesystem := os.DirFS(fb.projectLocation)
	out := map[string]struct{}{}
	for _, glob := range starutil.ListToValueList(list) {
		s, ok := glob.(starlark.String)
		if !ok {
			return nil, errors.Errorf("files argument %q is not a string", glob)
		}
		strValue := string(s)

		if filepath.IsAbs(strValue) {
			return nil, errors.Errorf("argument %q is an absolute path", strValue)
		}

		searchGlob := filepath.Join(fileDirectory, strValue)
		searchGlob, err := filepath.Rel(fb.projectLocation, searchGlob)
		if err != nil || strings.Contains(searchGlob, "..") {
			return nil, errors.Errorf("file search path %q searches outside of the project directory", strValue)
		}
		if err := doublestar.GlobWalk(
			projFilesystem,
			searchGlob,
			func(path string, d fs.DirEntry) error {
				// Skip directories if we want
				if !bool(includeDirectories) && d.IsDir() {
					return nil
				}
				out[path] = struct{}{}
				return nil
			}); err != nil {
			return nil, errors.Wrapf(err, "error with pattern %q", strValue)
		}
	}
	return out, nil
}
func (fb filesBuiltin) filesBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (out starlark.Value, err error) {
	var (
		include            *starlark.List
		exclude            *starlark.List
		includeDirectories starlark.Bool
		allowEmpty         starlark.Bool
	)

	if err = starlark.UnpackArgs("files", args, kwargs,
		"include", &include,
		"exclude?", &exclude,
		"include_directories?", &includeDirectories,
		"allow_empty?", &allowEmpty,
	); err != nil {
		return
	}

	// get location of file where files() is being called
	file := thread.CallStack().At(1).Pos.Filename()
	fileDirectory := filepath.Dir(file)

	inclSet, err := fb.starlarkGlobListFiles(bool(includeDirectories), fileDirectory, include)
	if err != nil {
		return nil, err
	}
	exclSet := map[string]struct{}{}
	if exclude != nil {
		if exclSet, err = fb.starlarkGlobListFiles(bool(includeDirectories), fileDirectory, exclude); err != nil {
			return nil, err
		}
	}

	// Only put the relative location in the derivation
	relFileDirectory, err := filepath.Rel(fb.projectLocation, fileDirectory)
	if err != nil {
		return nil, errors.Wrapf(err, "can't compute relative path between %q and %q", fb.projectLocation, fileDirectory)
	}
	fl := FilesList{
		Location: relFileDirectory,
	}
	for f := range inclSet {
		if _, match := exclSet[f]; !match {
			fl.Files = append(fl.Files, f)
		}
	}
	if len(fl.Files) == 0 && !allowEmpty {
		return nil, errors.New("files() call matched zero files")
	}

	return fl, nil
}
