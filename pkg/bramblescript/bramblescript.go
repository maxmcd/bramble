package bramblescript

import (
	"errors"
	"log"
	"os"

	"go.starlark.net/starlark"
)

var logger *log.Logger
var (
	ErrInvalidRead = errors.New("can't read from command output more than once")
)

func init() {
	logger = log.New(os.Stdout, "", 0)
}

func Builtins(dir string) starlark.StringDict {
	client := NewClient(dir)
	return starlark.StringDict{
		"cmd": starlark.NewBuiltin("cmd", client.StarlarkCmd),
	}
}
