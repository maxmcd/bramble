package bramble

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkjson"
)

func TestDerivationValueReplacement(t *testing.T) {
	b, err := NewBramble(".")
	require.NoError(t, err)

	fetchURL := &Derivation{
		bramble:     b,
		OutputNames: []string{"out"},
		Builder:     "fetch_url",
		Env:         map[string]string{"url": "1"},
	}
	assert.Equal(t, "{{ tmb75glr3iqxaso2gn27ytrmr4ufkv6d-.drv:out }}", fetchURL.String())

	other := &Derivation{
		bramble:     b,
		OutputNames: []string{"out"},
		Builder:     "/bin/sh",
		Env:         map[string]string{"foo": "bar"},
	}

	building := &Derivation{
		bramble:     b,
		OutputNames: []string{"out"},
		Builder:     fetchURL.String() + "/bin/sh",
		Env:         map[string]string{"PATH": other.templateString("out") + "/bin"},
	}
	// Assemble our derivations

	// Pretend we built ancestors by filling in their outputs
	fetchURL.Outputs = []Output{{Path: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
	other.Outputs = []Output{{Path: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}

	b.storeDerivation(fetchURL)
	b.storeDerivation(other)

	building.populateUnbuiltInputDerivations()
	b.store = store.Store{StorePath: "/bramble/store"}
	buildCopy, err := b.copyDerivationWithOutputValuesReplaced(building)
	if err != nil {
		t.Fatal(err)
	}
	assert.Contains(t, buildCopy.PrettyJSON(), "/bramble/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/bin/sh")
	assert.Contains(t, buildCopy.PrettyJSON(), "/bramble/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/bin")
}

type scriptTest struct {
	name, script      string
	errContains       string
	respContains      string
	respDoesntContain string
}

func fixUpScript(script string) string {
	var sb strings.Builder
	lines := strings.Split(script, "\n")
	sb.WriteString("def test():\n")
	if len(lines) > 1 {
		sb.WriteString("\t")
		sb.WriteString(strings.Join(lines[:len(lines)-1], "\n\t"))
	}
	sb.WriteString("\n\treturn " + lines[len(lines)-1])
	return sb.String()
}

func TestDerivationCreation(t *testing.T) {
	tests := []scriptTest{
		{script: `derivation()`,
			errContains: "not within a function"},
		{script: `
def foo():
  derivation()
foo()`,
			errContains: "missing argument for name"},
		{script: `
def foo():
	d = derivation("", builder="fetch_url", env={"url":1});
	return derivation("", builder="{}/bin/sh".format(d), env={"PATH":"{}/bin".format(d)})
b = foo()
`,
			respContains: `{{ tmb75glr3iqxaso2gn27ytrmr4ufkv6d-.drv:out }}`},
	}
	runDerivationTest(t, tests)
}

func runDerivationTest(t *testing.T, tests []scriptTest) {
	var err error
	dir := fileutil.TestTmpDir(t)
	os.Setenv("BRAMBLE_PATH", dir)
	t.Cleanup(func() { os.RemoveAll(dir) })

	b, err := NewBramble(".")
	if err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	require.NoError(t, err)
	for _, tt := range tests {
		name := tt.script
		if tt.name != "" {
			name = tt.name
		}
		t.Run(name, func(t *testing.T) {
			thread := &starlark.Thread{Name: "main"}
			globals, err := starlark.ExecFile(
				thread, filepath.Join(wd, "foo.bramble"),
				tt.script, b.predeclared,
			)
			processExecResp(t, tt, globals["b"], err)
		})
	}
}

func processExecResp(t *testing.T, tt scriptTest, b starlark.Value, err error) {
	if err != nil || tt.errContains != "" {
		if err == nil {
			t.Error("error is nil")
			return
		}
		assert.Contains(t, err.Error(), tt.errContains, tt)
		if tt.errContains == "" {
			t.Error(err, tt.script)
			return
		}
	}
	if tt.respContains == "" && tt.respDoesntContain == "" {
		return
	}

	if drv, ok := b.(*Derivation); ok {
		assert.Contains(t, drv.PrettyJSON(), tt.respContains)
		if tt.respDoesntContain != "" {
			assert.NotContains(t, drv.PrettyJSON(), tt.respDoesntContain)
		}
		return
	}
	assert.Contains(t, b.String(), tt.respContains)
	if tt.respDoesntContain != "" {
		assert.NotContains(t, b.String(), tt.respDoesntContain)
	}
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
