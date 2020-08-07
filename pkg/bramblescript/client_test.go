package bramblescript

import (
	"testing"
)

func TestCmdClient(t *testing.T) {
	tests := []scriptTest{
		{script: "cmd.cd('..');b=cmd('pwd').combined_output().strip().endswith('pkg')",
			returnValue: "True"},
	}
	runTest(t, tests)
}
