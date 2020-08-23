package bramblecmd

import (
	"errors"
	"io/ioutil"
	"log"

	"go.starlark.net/starlark"
)

var logger *log.Logger
var (
	ErrInvalidRead = errors.New("can't read from command output more than once")
)

func init() {
	logger = log.New(ioutil.Discard, "", 0)
}

func Builtins(dir string) starlark.StringDict {
	return starlark.StringDict{
		"cmd": NewClient(),
	}
}
