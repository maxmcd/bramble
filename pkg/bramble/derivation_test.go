package bramble

import (
	"fmt"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkjson"
)

func TestJsonEncode(t *testing.T) {
	m := starlarkjson.Module

	fetchURL := &Derivation{
		OutputNames: []string{"out"},
		Builder:     "fetch_url",
		Env:         map[string]string{"url": "1"},
	}

	v, err := m.Members["encode"].(*starlark.Builtin).CallInternal(nil, starlark.Tuple{starlark.Tuple{starlark.String("hi"), fetchURL}}, nil)
	if err != nil {
		return
	}
	v, err = m.Members["decode"].(*starlark.Builtin).CallInternal(nil, starlark.Tuple{v}, nil)
	if err != nil {
		return
	}
	fmt.Println(v)
}

func TestDerivationCaching(t *testing.T) {
	b, err := NewBramble(".")
	if err != nil {
		t.Fatal(err)
	}
	script := fixUpScript(`derivation("", builder="hello", sources=files(["*"]))`)

	v, err := b.execTestFileContents(script)
	if err != nil {
		t.Fatal(err)
	}
	_ = v
}
