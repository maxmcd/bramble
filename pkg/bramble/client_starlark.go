package bramble

import (
	"fmt"

	"github.com/pkg/errors"
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

func (c *Client) newDerivationFromKWArgs(kwargs []starlark.Tuple) (drv *Derivation, err error) {
	te := typeError{
		funcName: "derivation",
	}
	drv = &Derivation{
		Outputs: map[string]Output{"out": {}},
		client:  c,
	}
	for _, kwarg := range kwargs {
		key := kwarg.Index(0).(starlark.String).GoString()
		value := kwarg.Index(1)
		switch key {
		case "name":
			name, ok := value.(starlark.String)
			if !ok {
				te.argument = "name"
				te.wantedType = "string"
				return drv, te
			}
			drv.Name = name.GoString()
		case "builder":
			name, ok := value.(starlark.String)
			if !ok {
				te.argument = "builder"
				te.wantedType = "string"
				return drv, te
			}
			drv.Builder = name.GoString()
		case "args":
			drv.Args, err = valueToStringArray(value, "derivation", "args")
			if err != nil {
				return
			}
		case "sources":
			drv.Sources, err = valueToStringArray(value, "derivation", "args")
			if err != nil {
				return
			}
		case "environment":
			drv.Environment, err = valueToStringMap(value, "derivation", "environment")
			if err != nil {
				return
			}
		default:
			err = errors.Errorf("derivation() got an unexpected keyword argument '%s'", key)
			return
		}
	}
	if err = drv.AssembleSources(c.scriptLocation.Peek()); err != nil {
		return
	}
	c.log.Debug("Assembled derivation: ", drv.PrettyJson())
	return drv, nil
}

func valueToStringArray(val starlark.Value, function, param string) (out []string, err error) {
	maybeErr := errors.Errorf(
		"%s parameter '%s' expects type 'list' instead got '%s'",
		function, param, val.String())
	if val.Type() != "list" {
		err = maybeErr
		return
	}
	list, ok := val.(*starlark.List)
	if !ok {
		err = maybeErr
		return
	}
	for i := 0; i < list.Len(); i++ {
		v, ok := list.Index(i).(starlark.String)
		if !ok {
			err = errors.Errorf("%s %s expects a list of strings, but got value %s", function, param, v.String())
			return
		}
		out = append(out, v.GoString())
	}
	return
}

func valueToStringMap(val starlark.Value, function, param string) (out map[string]string, err error) {
	out = map[string]string{}
	maybeErr := errors.Errorf(
		"%s parameter '%s' expects type 'dict' instead got '%s'",
		function, param, val.String())
	if val.Type() != "dict" {
		err = maybeErr
		return
	}
	dict, ok := val.(starlark.IterableMapping)
	if !ok {
		err = maybeErr
		return
	}
	items := dict.Items()
	for _, item := range items {
		key := item.Index(0)
		value := item.Index(1)
		ks, ok := key.(starlark.String)
		if !ok {
			err = errors.Errorf("%s %s expects a dictionary of strings, but got key '%s'", function, param, key.String())
			return
		}
		drv, ok := value.(*Derivation)
		if ok {
			out[ks.GoString()] = drv.Outputs["out"].Path
			continue
		}
		vs, ok := value.(starlark.String)
		if !ok {
			err = errors.Errorf("%s %s expects a dictionary of strings, but got value '%s'", function, param, value.String())
			return
		}
		out[ks.GoString()] = vs.GoString()
	}
	return
}

func (c *Client) StarlarkDerivation(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (v starlark.Value, err error) {
	if args.Len() > 0 {
		return nil, errors.New("builtin function build() takes no positional arguments")
	}
	drv, err := c.newDerivationFromKWArgs(kwargs)
	if err != nil {
		return nil, &starlark.EvalError{Msg: err.Error(), CallStack: c.thread.CallStack()}
	}
	c.derivations[drv.Name] = drv
	return drv, nil
}
