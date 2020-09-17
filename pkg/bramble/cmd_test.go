package bramble

import (
	"testing"

	"go.starlark.net/starlark"
)

type scriptTest struct {
	name         string
	script       string
	errContains  string
	respContains string
}

func runCmdTest(t *testing.T, tests []scriptTest) {
	session, err := newSession("", nil)
	if err != nil {
		t.Fatal(err)
	}
	b := Bramble{}
	if err = b.init(); err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.script, func(t *testing.T) {
			thread := &starlark.Thread{Name: "main"}
			cmd := NewCmdFunction(session, &b)
			globals, err := starlark.ExecFile(
				thread, tt.name+".bramble",
				tt.script, starlark.StringDict{"cmd": cmd},
			)
			processExecResp(t, tt, globals, err)
		})
	}
}

func TestCmd(t *testing.T) {
	tests := []scriptTest{
		{script: `
c = cmd("ls")
b = [getattr(c, x) for x in dir(c)]
		`,
			respContains: ""},
		{script: "cmd()",
			errContains: "missing 1 required positional argument"},
		{script: "cmd([])",
			errContains: "be empty"},
		{script: `cmd("")`,
			errContains: "be empty"},
		{script: `cmd("    ")`,
			errContains: `"    "`},
		{script: `b=[getattr(cmd("echo"), x) for x in dir(cmd("echo"))]`,
			respContains: ``},
		{script: `cmd("sleep 2").kill()`},
		{script: `b=cmd(["ls", "-lah"])`,
			respContains: `<cmd 'ls' ['-lah']>`},
		{script: `b=cmd("ls -lah")`,
			respContains: `<cmd 'ls' ['-lah']>`},
		{script: `b=cmd("ls \"-lah\"")`,
			respContains: `<cmd 'ls' ['-lah']>`},
		{script: `b=cmd("ls '-lah'")`,
			respContains: `<cmd 'ls' ['-lah']>`},
		{script: `b=cmd("ls", "-lah")`,
			respContains: `<cmd 'ls' ['-lah']>`},
		{script: `b=cmd("echo 'these are words'").pipe("tr ' ' '\n'").pipe("grep these").stdout()`,
			respContains: `"these\n"`},
		{script: `b=cmd("ls -lah").wait().exit_code`,
			respContains: `0`},
		{script: `c = cmd("echo")
cmd("echo").wait()
c.kill()`},
	}
	runCmdTest(t, tests)
}

func TestCmdPipe(t *testing.T) {
	tests := []scriptTest{
		{script: `b=cmd("echo 'these are words'").pipe("tr ' ' '\n'").pipe("grep these").stdout()`,
			respContains: `"these\n"`},
	}
	runCmdTest(t, tests)
}

func TestCmdCallback(t *testing.T) {
	tests := []scriptTest{
		{script: `
def echo(*args, **kwargs):
  return cmd("echo", *args, **kwargs)

b=echo("hi").stdout().strip()
`,
			respContains: `"hi"`},
	}
	runCmdTest(t, tests)
}

func TestCmdArgs(t *testing.T) {
	tests := []scriptTest{
		{script: `b=cmd("grep hi", stdin=cmd("echo hi")).output()`,
			respContains: `"hi\n"`},
		{script: `b=cmd("grep hi", stdin="hi").output()`,
			respContains: `"hi\n"`},
		{script: `b=cmd("env", clear_env=True).output()`,
			errContains: `not found`},
		{script: `b=cmd("env", clear_env=True, env={"foo":"bar", "baz": 1}).output()`,
			errContains: `not found`},
	}
	runCmdTest(t, tests)
}

func TestCmdIfErr(t *testing.T) {
	tests := []scriptTest{
		{script: `b=cmd("ls", "notathing").if_err("echo", "hi").stdout()`,
			respContains: `"hi\n"`},
		{script: `b=cmd("ls", "notathing").if_err("echo", "hi").stdout()`,
			respContains: `"hi\n"`},
	}
	runCmdTest(t, tests)
}

func TestCallable(t *testing.T) {
	tests := []scriptTest{
		{script: `b=cmd("ls").pipe`,
			respContains: `<attribute 'pipe' of 'cmd'>`},
		{script: `b=type(cmd("ls").if_err)`,
			respContains: `"builtin_function_or_method"`},
	}
	runCmdTest(t, tests)
}

func TestByteStream(t *testing.T) {
	tests := []scriptTest{
		{script: `b=cmd("ls", "notathing").stdout()`,
			errContains: `exit`},
		{script: `b=cmd("echo","hi").stdout()`,
			respContains: `"hi\n"`},
		{script: `b=list(cmd("echo","hi").stdout)`,
			respContains: `["hi"]`},
		{script: `b=cmd("echo","hi").stdout`,
			respContains: `<attribute 'stdout' of 'cmd'>`},
		{script: `b=type(cmd("echo","hi").stdout)`,
			respContains: `"bytestream"`},
		{script: `b=cmd("echo", "hi").output()`,
			respContains: `"hi\n"`},
		{script: `b=cmd("echo", "hi").stderr()`,
			respContains: `""`},
		{script: `b=cmd("echo", "hi").stderr`,
			respContains: `<attribute 'stderr' of 'cmd'>`},
		{script: `b=cmd("echo", "hi").stdout`,
			respContains: `<attribute 'stdout' of 'cmd'>`},
		{script: `b=cmd("echo", "hi").output`,
			respContains: `<attribute 'output' of 'cmd'>`},
		{script: `b=cmd("echo", 1).stdout()`,
			respContains: `"1\n"`},
		{script: `
def run():
	c = cmd("ls")
	response = ""
	for line in c.stdout:
		if "cmd_test" in line:
			response = line
	return response
b = run()`,
			respContains: `"cmd_test.go"`},
		{script: `
def run():
	c = cmd("ls")
	for line in c.output:
		if "cmd_test" in line:
			return line
b = run()`,
			respContains: `"cmd_test.go"`},
	}
	runCmdTest(t, tests)
}
