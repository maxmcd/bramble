package bramblescript

import "go.starlark.net/starlark"

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
