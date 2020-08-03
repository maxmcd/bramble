package bramblescript

import (
	"testing"
)

func TestStarlarkCmd(t *testing.T) {
	tests := []scriptTest{
		{script: `
c = cmd("ls")
b = [getattr(c, x) for x in dir(c)]
		`,
			returnValue: ""},
		{script: "cmd()",
			errContains: "missing 1 required positional argument"},
		{script: "cmd([])",
			errContains: "be empty"},
		{script: `cmd("")`,
			errContains: "be empty"},
		{script: `cmd("    ")`,
			errContains: `"    "`},
		{script: `b=[getattr(cmd("echo"), x) for x in dir(cmd("echo"))]`,
			returnValue: ``},
		{script: `cmd("sleep 2").kill()`},
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
		{script: `b=cmd("echo 'these are words'").pipe("tr ' ' '\n'").pipe("grep these").stdout()`,
			returnValue: `"these\n"`},
		{script: `c = cmd("echo")
cmd("echo").wait()
c.kill()`},
	}
	runTest(t, tests)
}
