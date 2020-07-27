package bramblescript

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.starlark.net/starlark"
)

func TestStarlarkCmd(t *testing.T) {
	tests := []struct {
		name        string
		script      string
		errContains string
		returnValue string
	}{
		{script: "cmd()",
			errContains: "missing 1 required positional argument"},
		{script: "cmd([])",
			errContains: "be empty"},
		{script: `cmd("")`,
			errContains: "be empty"},
		{script: `cmd("    ")`,
			errContains: `"    "`},
		{script: "cmd([1])",
			errContains: ErrIncorrectType{is: "int", shouldBe: "string"}.Error()},
		{script: `b=cmd(["ls", "-lah"])`,
			returnValue: `<cmd 'ls' ['-lah']>`},
		{script: `b=cmd("ls -lah")`,
			returnValue: `<cmd 'ls' ['-lah']>`},
		{script: `b=cmd("ls -lah")`,
			returnValue: `<cmd 'ls' ['-lah']>`},
		{script: `b=cmd("ls \"-lah\"")`,
			returnValue: `<cmd 'ls' ['-lah']>`},
		{script: `b=cmd("ls '-lah'")`,
			returnValue: `<cmd 'ls' ['-lah']>`},
		{script: `b=cmd("ls", "-lah")`,
			returnValue: `<cmd 'ls' ['-lah']>`},
	}
	for _, tt := range tests {
		t.Run(tt.script, func(t *testing.T) {
			thread := &starlark.Thread{Name: "main"}
			globals, err := starlark.ExecFile(thread, tt.name+".bramble", tt.script, starlark.StringDict{
				"cmd": starlark.NewBuiltin("derivation", StarlarkCmd),
			})
			if err != nil || tt.errContains != "" {
				if err == nil {
					t.Error("error is nil")
				}
				assert.Contains(t, err.Error(), tt.errContains)
				if tt.errContains == "" {
					t.Error(err, tt.script)
					return
				}
			}

			if tt.returnValue == "" {
				return
			}
			b, ok := globals["b"]
			if !ok {
				t.Errorf("%q doesn't output global value b", tt.script)
			}
			assert.Equal(t, tt.returnValue, b.String())
		})
	}
}
