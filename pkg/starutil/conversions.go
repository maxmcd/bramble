package starutil

import (
	"github.com/pkg/errors"

	"go.starlark.net/starlark"
)

func IterableToGoList(list starlark.Iterable) (out []string, err error) {
	iterator := list.Iterate()
	defer iterator.Done()
	var val starlark.Value
	for iterator.Next(&val) {
		var strValue string
		strValue, err = ValueToString(val)
		if err != nil {
			return
		}
		out = append(out, strValue)
	}
	return
}

func ListToValueList(list *starlark.List) (out []starlark.Value) {
	iterator := list.Iterate()
	defer iterator.Done()
	var val starlark.Value
	for iterator.Next(&val) {
		cpy := val
		out = append(out, cpy)
	}
	return
}

func DictToGoStringMap(dict *starlark.Dict) (out map[string]string, err error) {
	out = make(map[string]string)
	for _, key := range dict.Keys() {
		envVal, _, _ := dict.Get(key)
		keyString, err := ValueToString(key)
		if err != nil {
			return nil, err
		}
		valString, err := ValueToString(envVal)
		if err != nil {
			return nil, err
		}
		out[keyString] = valString
	}
	return
}

func ValueToString(val starlark.Value) (out string, err error) {
	if val.Type() == "derivation" {
		return val.String(), nil
	}
	switch v := val.(type) {
	case starlark.String:
		out = v.GoString()
	case starlark.Int:
		out = v.String()
	case starlark.Bool:
		if v {
			out = "true"
		} else {
			out = "false"
		}
	default:
		return "", errors.Errorf("don't know how to cast type %q into a string", v.Type())
	}
	return
}
