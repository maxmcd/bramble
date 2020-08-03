package bramblescript

import (
	"go.starlark.net/starlark"
)

func Builtins(dir string) starlark.StringDict {
	client := NewClient(dir)
	return starlark.StringDict{
		"cmd": starlark.NewBuiltin("cmd", client.StarlarkCmd),
	}
}
