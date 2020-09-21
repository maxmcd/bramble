package bramble

import (
	"strings"
	"testing"

	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
)

type scriptTest struct {
	name         string
	script       string
	errContains  string
	respContains string
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
				fixUpScript(tt.script), starlark.StringDict{"cmd": cmd},
			)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := starlark.Call(thread, globals["test"], nil, nil)
			processExecResp(t, tt, resp, err)
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
		{script: `[getattr(cmd("echo"), x) for x in dir(cmd("echo"))]`,
			respContains: ``},
		{script: `cmd("sleep 2").kill()`},
		{script: `cmd(["ls", "-lah"])`,
			respContains: `<cmd 'ls' ['-lah']>`},
		{script: `cmd("ls -lah")`,
			respContains: `<cmd 'ls' ['-lah']>`},
		{script: `cmd("ls \"-lah\"")`,
			respContains: `<cmd 'ls' ['-lah']>`},
		{script: `cmd("ls '-lah'")`,
			respContains: `<cmd 'ls' ['-lah']>`},
		{script: `cmd("ls", "-lah")`,
			respContains: `<cmd 'ls' ['-lah']>`},
		{script: `cmd("echo 'these are words'").pipe("tr ' ' '\n'").pipe("grep these").stdout()`,
			respContains: `"these\n"`},
		{script: `cmd("ls -lah").wait().exit_code`,
			respContains: `0`},
		{script: `c = cmd("echo")
cmd("echo").wait()
c.kill()`},
	}
	runCmdTest(t, tests)
}

func TestCmdPipe(t *testing.T) {
	tests := []scriptTest{
		{script: `cmd("echo 'these are words'").pipe("tr ' ' '\n'").pipe("grep these").stdout()`,
			respContains: `"these\n"`},
	}
	runCmdTest(t, tests)
}

func TestCmdCallback(t *testing.T) {
	resolve.AllowNestedDef = true
	tests := []scriptTest{
		{script: `
def echo(*args, **kwargs):
  return cmd("echo", *args, **kwargs)

echo("hi").stdout().strip()`,
			respContains: `"hi"`},
	}
	runCmdTest(t, tests)
}

func TestCmdArgs(t *testing.T) {
	tests := []scriptTest{
		{script: `cmd("grep hi", stdin=cmd("echo hi")).output()`,
			respContains: `"hi\n"`},
		{script: `cmd("grep hi", stdin="hi").output()`,
			respContains: `"hi\n"`},
		{script: `cmd("env", clear_env=True).output()`,
			errContains: `not found`},
		{script: `cmd("env", clear_env=True, env={"foo":"bar", "baz": 1}).output()`,
			errContains: `not found`},
	}
	runCmdTest(t, tests)
}

func TestCmdIfErr(t *testing.T) {
	tests := []scriptTest{
		{script: `cmd("ls", "notathing").if_err("echo", "hi").stdout()`,
			respContains: `"hi\n"`},
		{script: `cmd("ls", "notathing").if_err("echo", "hi").stdout()`,
			respContains: `"hi\n"`},
	}
	runCmdTest(t, tests)
}

func TestCallable(t *testing.T) {
	tests := []scriptTest{
		{script: `cmd("ls").pipe`,
			respContains: `<attribute 'pipe' of 'cmd'>`},
		{script: `type(cmd("ls").if_err)`,
			respContains: `"builtin_function_or_method"`},
	}
	runCmdTest(t, tests)
}

func TestByteStream(t *testing.T) {
	tests := []scriptTest{
		{script: `cmd("ls", "notathing").stdout()`,
			errContains: `exit`},
		{script: `cmd("echo","hi").stdout()`,
			respContains: `"hi\n"`},
		{script: `list(cmd("echo","hi").stdout)`,
			respContains: `["hi"]`},
		{script: `cmd("echo","hi").stdout`,
			respContains: `<attribute 'stdout' of 'cmd'>`},
		{script: `type(cmd("echo","hi").stdout)`,
			respContains: `"bytestream"`},
		{script: `cmd("echo", "hi").output()`,
			respContains: `"hi\n"`},
		{script: `cmd("echo", "hi").stderr()`,
			respContains: `""`},
		{script: `cmd("echo", "hi").stderr`,
			respContains: `<attribute 'stderr' of 'cmd'>`},
		{script: `cmd("echo", "hi").stdout`,
			respContains: `<attribute 'stdout' of 'cmd'>`},
		{script: `cmd("echo", "hi").output`,
			respContains: `<attribute 'output' of 'cmd'>`},
		{script: `cmd("echo", 1).stdout()`,
			respContains: `"1\n"`},
		{script: `c = cmd("ls")
response = ""
for line in c.stdout:
	if "cmd_test" in line:
		response = line
response`,
			respContains: `"cmd_test.go"`},
		{script: `c = cmd("ls")
for line in c.output:
	if "cmd_test" in line:
		return line
`,
			respContains: `"cmd_test.go"`},
	}
	runCmdTest(t, tests)
}
