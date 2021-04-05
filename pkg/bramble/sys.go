package bramble

import (
	"runtime"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

var starlarkSys = &starlarkstruct.Module{
	Name: "sys",
	Members: starlark.StringDict{
		"os":   starlark.String(runtime.GOOS),
		"arch": starlark.String(runtime.GOARCH),
	},
}
