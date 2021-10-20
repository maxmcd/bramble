package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

func newTestRuntime(t *testing.T, wd string) *runtime {
	if wd == "" {
		var err error
		wd, err = os.Getwd()
		require.NoError(t, err)
	}

	projectLocation, err := filepath.Abs("../../")
	require.NoError(t, err)

	rt := newRuntime(wd, projectLocation, "github.com/maxmcd/bramble", "", nil)
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

func runDerivationTest(t *testing.T, tests []scriptTest, wd string) {
	var err error
	dir := fileutil.TestTmpDir(t)
	previous := os.Getenv("BRAMBLE_PATH")
	os.Setenv("BRAMBLE_PATH", dir)
	t.Cleanup(func() { os.RemoveAll(dir); os.Setenv("BRAMBLE_PATH", previous) })

	if wd == "" {
		wd, err = os.Getwd()
		require.NoError(t, err)
	}
	rt := newTestRuntime(t, wd)

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

	if drv, ok := b.(Derivation); ok {
		assert.Contains(t, drv.prettyJSON(), tt.respContains)
		if tt.respDoesntContain != "" {
			assert.NotContains(t, drv.prettyJSON(), tt.respDoesntContain)
		}
		return
	}
	assert.Contains(t, b.String(), tt.respContains)
	if tt.respDoesntContain != "" {
		assert.NotContains(t, b.String(), tt.respDoesntContain)
	}
}
