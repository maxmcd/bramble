package bramble

import (
	"github.com/pkg/errors"
)

var (
	errQuiet        = errors.New("")
	ErrNotInProject = errors.New("couldn't find a bramble.toml file in this directory or any parent")
)
