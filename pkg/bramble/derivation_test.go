package bramble

import (
	"os"
	"testing"

	"github.com/alecthomas/assert"
	"go.starlark.net/starlark"
)

func TestDerivationCreation(t *testing.T) {
	tests := []scriptTest{
		{script: `derivation()`,
			errContains: "missing argument for builder"},
		{script: `d = derivation(builder="fetch_url", env={"url":1});
b=derivation(builder="{}/bin/sh".format(d), env={"PATH":"{}/bin".format(d)})
`,
			respContains: `{{ 2ejs52cs4pr7vhkowpcfeijxx4w3wsmg-.drv out }}`},
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
		assert.Contains(t, drv.prettyJSON(), tt.respContains)
		return
	}
	assert.Contains(t, b.String(), tt.respContains)
}
