package bramblescript

import (
	"github.com/pkg/errors"

	"go.starlark.net/starlark"
)

func kwargsToStringDict(kwargsList []starlark.Tuple) (kwargs starlark.StringDict, err error) {
	kwargs = starlark.StringDict{}
	for _, kwarg := range kwargsList {
		keyString := kwarg.Index(0).(*starlark.String).GoString()
		if _, ok := kwargs[keyString]; ok {
			return nil, errors.New("keyword argument repeated")
		}
		kwargs[keyString] = kwarg.Index(1)
	}
	return
}

func starlarkListToListOfStrings(listValue starlark.Value) (out []string, err error) {
	list, ok := listValue.(*starlark.List)
	if !ok {
		return nil, ErrIncorrectType{is: listValue.Type(), shouldBe: "list"}
	}
	iterator := list.Iterate()
	defer iterator.Done()
	var val starlark.Value
	for iterator.Next(&val) {
		str, ok := val.(starlark.String)
		if !ok {
			return nil, ErrIncorrectType{is: val.Type(), shouldBe: "string"}
		}
		out = append(out, str.GoString())
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
