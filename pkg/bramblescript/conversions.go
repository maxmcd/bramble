package bramblescript

import (
	"github.com/pkg/errors"

	"go.starlark.net/starlark"
)

func starlarkListToListOfStrings(listValue starlark.Value) (out []string, err error) {
	list, ok := listValue.(*starlark.List)
	if !ok {
		return nil, ErrIncorrectType{is: listValue.Type(), shouldBe: "list"}
	}
	iterator := list.Iterate()
	defer iterator.Done()
	var val starlark.Value
	for iterator.Next(&val) {
		var strValue string
		strValue, err = valueToString(val)
		if err != nil {
			return
		}
		out = append(out, strValue)
	}
	return
}

func valueToString(val starlark.Value) (out string, err error) {
	switch v := val.(type) {
	case starlark.String:
		out = v.GoString()
	case starlark.Int:
		out = v.String()
	default:
		return "", errors.Errorf("don't know how to cast type %q into a string", v.Type())
	}
	return
}
