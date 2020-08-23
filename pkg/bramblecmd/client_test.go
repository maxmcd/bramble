package bramblecmd

import (
	"testing"
)

func TestCmdClient(t *testing.T) {
	tests := []scriptTest{
		{script: "cmd.cd('..');b=cmd('pwd').output().strip().endswith('pkg')",
			returnValue: "True"},
		{script: "b=cmd",
			returnValue: "<built-in function cmd>"},
		{script: "b=''.join(dir(cmd))",
			returnValue: `"cddebug"`},
		{script: "cmd.cd()",
			errContains: "missing argument"},
	}
	runTest(t, tests)
}
