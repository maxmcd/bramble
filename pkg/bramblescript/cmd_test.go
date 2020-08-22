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
	runTest(t, tests)
}

func TestPipe(t *testing.T) {
	tests := []scriptTest{
		{script: `b=cmd("echo 'these are words'").pipe("tr ' ' '\n'").pipe("grep these").stdout()`,
			returnValue: `"these\n"`},
	}
	runTest(t, tests)
}

func TestCallback(t *testing.T) {
	tests := []scriptTest{
		{script: `
def echo(*args, **kwargs):
  return cmd("echo", *args, **kwargs)

b=echo("hi").stdout().strip()
`,
			returnValue: `"hi"`},
		{script: `
cmd.debug()
def grep(*args, **kwargs):
  return cmd("grep", *args, **kwargs)

b=cmd("echo hi").pipe(grep, "hi").stdout().strip()
`,
			returnValue: `"hi"`},
	}
	runTest(t, tests)
}

func TestArgs(t *testing.T) {
	tests := []scriptTest{
		{script: `b=cmd("grep hi", stdin=cmd("echo hi")).output()`,
			returnValue: `"hi\n"`},
		{script: `b=cmd("grep hi", stdin="hi").output()`,
			returnValue: `"hi\n"`},
		{script: `b=cmd("env", clear_env=True).output()`,
			returnValue: `""`},
		{script: `b=cmd("env", clear_env=True, env={"foo":"bar", "baz": 1}).output()`,
			returnValue: `"foo=bar\nbaz=1\n"`},
	}
	runTest(t, tests)
}

func TestIfErr(t *testing.T) {
	tests := []scriptTest{
		{script: `b=cmd("ls", "notathing").if_err("echo", "hi").stdout()`,
			returnValue: `"hi\n"`},
		{script: `b=cmd("ls", "notathing").if_err("echo", "hi").stdout()`,
			returnValue: `"hi\n"`},
	}
	runTest(t, tests)
}

func TestCallable(t *testing.T) {
	tests := []scriptTest{
		{script: `b=cmd("ls").pipe`,
			returnValue: `<attribute 'pipe' of 'cmd'>`},
		{script: `b=type(cmd("ls").if_err)`,
			returnValue: `"builtin_function_or_method"`},
	}
	runTest(t, tests)
}
