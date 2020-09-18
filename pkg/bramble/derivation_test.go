package bramble

import (
	"fmt"
	"os"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkjson"
)

func TestDerivationValueReplacement(t *testing.T) {
	fetchURL := Derivation{
		OutputNames: []string{"out"},
		Builder:     "fetch_url",
		Env:         map[string]string{"url": "1"},
	}
	assert.Equal(t, "{{ tmb75glr3iqxaso2gn27ytrmr4ufkv6d-.drv out }}", fetchURL.templateString("out"))

	other := Derivation{
		OutputNames: []string{"out"},
		Builder:     "/bin/sh",
		Env:         map[string]string{"foo": "bar"},
	}

	building := Derivation{
		OutputNames: []string{"out"},
		Builder:     fetchURL.templateString("out") + "/bin/sh",
		Env:         map[string]string{"PATH": other.templateString("out") + "/bin"},
	}
	// Assemble our derivations

	// Pretend we built ancestors by filling in their outputs
	fetchURL.Outputs = []Output{{Path: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
	other.Outputs = []Output{{Path: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}

	b := Bramble{}
	b.derivations = &DerivationsMap{}
	b.derivations.Set(fetchURL.filename(), &fetchURL)
	b.derivations.Set(other.filename(), &other)
	b.store = Store{storePath: "/bramble/store"}
	buildCopy, err := b.copyDerivationWithOutputValuesReplaced(&building)
	if err != nil {
		t.Fatal(err)
	}
	assert.Contains(t, buildCopy.prettyJSON(), "/bramble/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/bin/sh")
	assert.Contains(t, buildCopy.prettyJSON(), "/bramble/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/bin")
}

func TestDerivationCreation(t *testing.T) {
	tests := []scriptTest{
		{script: `derivation()`,
			errContains: "missing argument for builder"},
		{script: `d = derivation(builder="fetch_url", env={"url":1});
b=derivation(builder="{}/bin/sh".format(d), env={"PATH":"{}/bin".format(d)})
`,
			respContains: `{{ tmb75glr3iqxaso2gn27ytrmr4ufkv6d-.drv out }}`},
	}
	runDerivationTest(t, tests)
}

func runDerivationTest(t *testing.T, tests []scriptTest) {
	var err error
	dir := tmpDir()
	os.Setenv("BRAMBLE_PATH", dir)
	defer os.RemoveAll(dir)

	b := Bramble{}
	if err = b.init(); err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.script, func(t *testing.T) {
			thread := &starlark.Thread{Name: "main"}
			globals, err := starlark.ExecFile(
				thread, tt.name+".bramble",
				tt.script, b.predeclared,
			)
			processExecResp(t, tt, globals, err)
		})
	}
}

func processExecResp(t *testing.T, tt scriptTest, globals starlark.StringDict, err error) {
	if err != nil || tt.errContains != "" {
		if err == nil {
			t.Error("error is nil")
			return
		}
		assert.Contains(t, err.Error(), tt.errContains)
		if tt.errContains == "" {
			t.Error(err, tt.script)
			return
		}
	}
	if tt.respContains == "" {
		return
	}
	b, ok := globals["b"]
	if !ok {
		t.Errorf("%q doesn't output global value b", tt.script)
		return
	}

	if drv, ok := b.(*Derivation); ok {
		fmt.Println(drv.prettyJSON())
		assert.Contains(t, drv.prettyJSON(), tt.respContains)
		return
	}

	assert.Contains(t, b.String(), tt.respContains)
}

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
	spew.Dump(v)
}
