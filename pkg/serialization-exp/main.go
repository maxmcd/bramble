package main

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
)

func init() {
	resolve.AllowFloat = true
	resolve.AllowLambda = true
	resolve.AllowNestedDef = true
	resolve.AllowRecursion = false
	resolve.AllowSet = true
}

func main() {
	thread := &starlark.Thread{Name: "main"}

	globals, err := starlark.ExecFile(thread, "main.star", nil, starlark.StringDict{
		"derivation": starlark.NewBuiltin("derivation", starlarkDerivation),
	})

	fmt.Println(globals, err)
}

var callCount int = 0

func starlarkDerivation(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (v starlark.Value, err error) {
	callCount++
	fmt.Println("called derivation", callCount)
	spew.Dump(args)
	args.Index(1).(*starlark.Function).CallInternal(thread, nil, nil)
	return starlark.None, nil
}
