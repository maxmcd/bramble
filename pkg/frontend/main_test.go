package frontend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

func newTestRuntime() *Runtime {
	store, err := store.NewStore("")
	if err != nil {
		panic(err)
	}
	project, err := NewProject(".")
	if err != nil {
		panic(err)
	}
	rt, err := NewRuntime(project, store)
	if err != nil {
		panic(err)
	}
	return rt
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

func runDerivationTest(t *testing.T, tests []scriptTest) {
	var err error
	dir := fileutil.TestTmpDir(t)
	os.Setenv("BRAMBLE_PATH", dir)
	t.Cleanup(func() { os.RemoveAll(dir) })

	rt := newTestRuntime()
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
				tt.script, rt.predeclared,
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
		require.Contains(t, err.Error(), tt.errContains, tt)
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
