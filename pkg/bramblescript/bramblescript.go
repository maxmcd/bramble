package bramblescript

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
	client := NewClient(dir)
	return starlark.StringDict{
		"cmd": client,
	}
}
