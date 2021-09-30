package project

import (
	"github.com/maxmcd/bramble/pkg/starutil"
	"go.starlark.net/starlark"
)

type Test struct {
	Derivation Derivation
	Args       []string
	Location   string
}

func (rt *runtime) testBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (out starlark.Value, err error) {
	// TODO do we need to run tests only in our project?
	var (
		test     Test
		testArgs *starlark.List
	)
	test.Location = thread.CallStack().At(1).Pos.String()
	if err = starlark.UnpackArgs("test", args, kwargs,
		"derivation", &test.Derivation,
		"testArgs", &testArgs,
	); err != nil {
		return
	}

	test.Args, err = starutil.IterableToStringSlice(testArgs)

	rt.tests = append(rt.tests, test)
	return starlark.None, err
}
