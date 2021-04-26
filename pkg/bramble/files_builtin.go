package bramble

import (
	"fmt"
	"path/filepath"

	"github.com/maxmcd/bramble/pkg/starutil"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

type filesList []string

var _ starlark.Value = new(filesList)

func (fl filesList) Freeze()               {}
func (fl filesList) Hash() (uint32, error) { return 0, starutil.ErrUnhashable("file_list") }
func (fl filesList) String() string        { return fmt.Sprint(fl) }
func (fl filesList) Type() string          { return "file_list" }
func (fl filesList) Truth() starlark.Bool  { return true }

func (b *Bramble) filesBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (out starlark.Value, err error) {
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

	var (
		inclSet = map[string]struct{}{}
		exclSet = map[string]struct{}{}
	)
	for _, glob := range starutil.ListToValueList(include) {
		strValue, ok := glob.(starlark.String)
		if !ok {
			return nil, errors.Wrapf(err, "files argument %q is not a string", glob)
		}
		matches, err := filepath.Glob(strValue.GoString())
		if err != nil {
			return nil, errors.Wrapf(err, "error with pattern %q", glob)
		}
		for _, match := range matches {
			inclSet[match] = struct{}{}
		}
	}
	if exclude != nil {
		for _, glob := range starutil.ListToValueList(exclude) {
			strValue, ok := glob.(starlark.String)
			if !ok {
				return nil, errors.Wrapf(err, "files argument %q is not a string", glob)
			}
			matches, err := filepath.Glob(strValue.GoString())
			if err != nil {
				return nil, errors.Wrapf(err, "error with pattern %q", glob)
			}
			for _, match := range matches {
				exclSet[match] = struct{}{}
			}
		}
	}
	fl := filesList{}

	for f := range inclSet {
		if _, match := exclSet[f]; !match {
			fl = append(fl, f)
		}
	}

	return fl, nil
}
