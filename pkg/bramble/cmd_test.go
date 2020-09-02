package bramble

import (
	"testing"

	"github.com/alecthomas/assert"
	"go.starlark.net/starlark"
)

type cmdTest struct {
	name        string
	script      string
	errContains string
	returnValue string
}

func runCmdTest(t *testing.T, tests []cmdTest) {
	session, err := newSession("", nil)
	if err != nil {
		t.Error(err)
	}
	for _, tt := range tests {
		t.Run(tt.script, func(t *testing.T) {
			thread := &starlark.Thread{Name: "main"}
			cmd := NewCmdFunction(session)
			globals, err := starlark.ExecFile(
				thread, tt.name+".bramble",
				tt.script, starlark.StringDict{"cmd": cmd},
			)
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
			if tt.returnValue == "" {
				return
			}
			b, ok := globals["b"]
			if !ok {
				t.Errorf("%q doesn't output global value b", tt.script)
				return
			}
			assert.Equal(t, tt.returnValue, b.String())
		})
	}
}

func TestStarlarkCmd(t *testing.T) {
	tests := []cmdTest{
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
		{script: `b=cmd(["ls", "-lah"])`,
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
		{script: `b=cmd("ls -lah").wait().exit_code`,
			returnValue: `0`},
		{script: `c = cmd("echo")
cmd("echo").wait()
c.kill()`},
	}
	runCmdTest(t, tests)
}

func TestPipe(t *testing.T) {
	tests := []cmdTest{
		{script: `b=cmd("echo 'these are words'").pipe("tr ' ' '\n'").pipe("grep these").stdout()`,
			returnValue: `"these\n"`},
	}
	runCmdTest(t, tests)
}

func TestCallback(t *testing.T) {
	tests := []cmdTest{
		{script: `
def echo(*args, **kwargs):
  return cmd("echo", *args, **kwargs)

b=echo("hi").stdout().strip()
`,
			returnValue: `"hi"`},
	}
	runCmdTest(t, tests)
}

func TestArgs(t *testing.T) {
	tests := []cmdTest{
		{script: `b=cmd("grep hi", stdin=cmd("echo hi")).output()`,
			returnValue: `"hi\n"`},
		{script: `b=cmd("grep hi", stdin="hi").output()`,
			returnValue: `"hi\n"`},
		{script: `b=cmd("env", clear_env=True).output()`,
			returnValue: `""`},
		{script: `b=cmd("env", clear_env=True, env={"foo":"bar", "baz": 1}).output()`,
			returnValue: `"foo=bar\nbaz=1\n"`},
	}
	runCmdTest(t, tests)
}

func TestIfErr(t *testing.T) {
	tests := []cmdTest{
		{script: `b=cmd("ls", "notathing").if_err("echo", "hi").stdout()`,
			returnValue: `"hi\n"`},
		{script: `b=cmd("ls", "notathing").if_err("echo", "hi").stdout()`,
			returnValue: `"hi\n"`},
	}
	runCmdTest(t, tests)
}

func TestCallable(t *testing.T) {
	tests := []cmdTest{
		{script: `b=cmd("ls").pipe`,
			returnValue: `<attribute 'pipe' of 'cmd'>`},
		{script: `b=type(cmd("ls").if_err)`,
			returnValue: `"builtin_function_or_method"`},
	}
	runCmdTest(t, tests)
}

func TestByteStream(t *testing.T) {
	tests := []cmdTest{
		{script: `b=cmd("ls", "notathing").stdout()`,
			errContains: `exit`},
		{script: `b=cmd("echo","hi").stdout()`,
			returnValue: `"hi\n"`},
		{script: `b=list(cmd("echo","hi").stdout)`,
			returnValue: `["hi"]`},
		{script: `b=cmd("echo","hi").stdout`,
			returnValue: `<attribute 'stdout' of 'cmd'>`},
		{script: `b=type(cmd("echo","hi").stdout)`,
			returnValue: `"bytestream"`},
		{script: `b=cmd("echo", "hi").output()`,
			returnValue: `"hi\n"`},
		{script: `b=cmd("echo", "hi").stderr()`,
			returnValue: `""`},
		{script: `b=cmd("echo", "hi").stderr`,
			returnValue: `<attribute 'stderr' of 'cmd'>`},
		{script: `b=cmd("echo", "hi").stdout`,
			returnValue: `<attribute 'stdout' of 'cmd'>`},
		{script: `b=cmd("echo", "hi").output`,
			returnValue: `<attribute 'output' of 'cmd'>`},
		{script: `b=cmd("echo", 1).stdout()`,
			returnValue: `"1\n"`},
		{script: `
def run():
	c = cmd("ls")
	response = ""
	for line in c.stdout:
		if "cmd_test" in line:
			response = line
	return response
b = run()`,
			returnValue: `"cmd_test.go"`},
		{script: `
def run():
	c = cmd("ls")
	for line in c.output:
		if "cmd_test" in line:
			return line
b = run()`,
			returnValue: `"cmd_test.go"`},
	}
	runCmdTest(t, tests)
}
