package bramblescript

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.starlark.net/starlark"
)

type scriptTest struct {
	name        string
	script      string
	errContains string
	returnValue string
}

func runTest(t *testing.T, tests []scriptTest) {
	for _, tt := range tests {
		t.Run(tt.script, func(t *testing.T) {
			thread := &starlark.Thread{Name: "main"}
			globals, err := starlark.ExecFile(thread, tt.name+".bramble", tt.script, Builtins(""))
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
