package derivation

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/maxmcd/bramble/pkg/starutil"
	"go.starlark.net/starlark"
)

type typeError struct {
	funcName   string
	argument   string
	wantedType string
}

func (te typeError) Error() string {
	return fmt.Sprintf("%s() keyword argument '%s' must be of type '%s'", te.funcName, te.argument, te.wantedType)
}

func (f *Function) newDerivationFromArgs(args starlark.Tuple, kwargs []starlark.Tuple) (drv *Derivation, err error) {
	drv = &Derivation{
		Outputs:  map[string]Output{"out": {}},
		Env:      map[string]string{},
		function: f,
	}
	var (
		name      starlark.String
		builder   starlark.Value = starlark.None
		argsParam *starlark.List
		sources   *starlark.List
		env       *starlark.Dict
		outputs   *starlark.List
	)
	spew.Dump(args, kwargs)
	if err = starlark.UnpackArgs("derivation", args, kwargs,
		"builder", &builder,
		"name?", &name,
		"args?", &argsParam,
		"sources?", &sources,
		"env?", &env,
		"outputs?", &outputs,
	); err != nil {
		return
	}

	drv.Name = name.GoString()

	if drv.Builder, err = starutil.ValueToString(builder); err != nil {
		return
	}

	if argsParam != nil {
		if drv.Args, err = starutil.ListToGoList(argsParam); err != nil {
			return
		}
	}
	if sources != nil {
		if drv.Sources, err = starutil.ListToGoList(sources); err != nil {
			return
		}
	}
	if env != nil {
		if drv.Env, err = starutil.DictToGoStringMap(env); err != nil {
			return
		}
	}
	if outputs != nil {
		outputsList, err := starutil.ListToGoList(outputs)
		if err != nil {
			return nil, err
		}
		delete(drv.Outputs, "out")
		for _, o := range outputsList {
			drv.Outputs[o] = Output{}
		}
	}

	return drv, nil
}
