package project

import (
	"go.starlark.net/starlark"

	_ "embed"
)

// TODO: move this into the source tree once we can stop pretending that it's
// not part of the source tree
//go:embed derivation.py
var derivationModule string

// LoadAssertModule loads the assert module. It is concurrency-safe and
// idempotent.
func (rt *runtime) loadNativeDerivation(derivation starlark.Value) (starlark.Value, error) {
	predeclared := starlark.StringDict{
		"_derivation":  derivation,
		"internal_key": starlark.MakeInt64(rt.internalKey),
	}

	thread := new(starlark.Thread)
	globals, err := starlark.ExecFile(thread, "derivation.bramble", derivationModule, predeclared)
	if err != nil {
		return nil, err
	}
	return globals["derivation"], nil
}
