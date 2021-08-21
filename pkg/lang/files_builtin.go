package lang

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

type filesList struct {
	files    []string
	location string
}

var _ starlark.Value = new(filesList)

func (fl filesList) Freeze()               {}
func (fl filesList) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("file_list") }
func (fl filesList) String() string        { return fmt.Sprint([]string(fl.files)) }
func (fl filesList) Type() string          { return "file_list" }
func (fl filesList) Truth() starlark.Bool  { return true }

func (r *Runtime) starlarkGlobListFiles(includeDirectories bool, fileDirectory string, list *starlark.List) (map[string]struct{}, error) {
	projFilesystem := os.DirFS(r.project.Location)
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
		searchGlob, err := filepath.Rel(r.project.Location, searchGlob)
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
func (r *Runtime) filesBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (out starlark.Value, err error) {
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

	inclSet, err := r.starlarkGlobListFiles(bool(includeDirectories), fileDirectory, include)
	if err != nil {
		return nil, err
	}
	exclSet := map[string]struct{}{}
	if exclude != nil {
		if exclSet, err = r.starlarkGlobListFiles(bool(includeDirectories), fileDirectory, exclude); err != nil {
			return nil, err
		}
	}

	fl := filesList{
		location: fileDirectory,
	}
	for f := range inclSet {
		if _, match := exclSet[f]; !match {
			fl.files = append(fl.files, f)
		}
	}
	if len(fl.files) == 0 && !allowEmpty {
		return nil, errors.New("files() call matched zero files")
	}

	return fl, nil
}
