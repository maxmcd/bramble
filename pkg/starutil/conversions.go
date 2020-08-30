package starutil

import (
	"github.com/pkg/errors"

	"go.starlark.net/starlark"
)

func ListToGoList(list *starlark.List) (out []string, err error) {
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

func ListToListOfStrings(listValue starlark.Value) (out []string, err error) {
	list, ok := listValue.(*starlark.List)
	if !ok {
		return nil, ErrIncorrectType{is: listValue.Type(), shouldBe: "list"}
	}
	return ListToGoList(list)
}

func ValueToString(val starlark.Value) (out string, err error) {
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
