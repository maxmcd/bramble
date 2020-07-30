package bramblescript

import (
	"go.starlark.net/starlark"
)

var Builtins = starlark.StringDict{
	"cmd":     starlark.NewBuiltin("cmd", StarlarkCmd),
	"session": starlark.NewBuiltin("session", StarlarkSesssion),
}
