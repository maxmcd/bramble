package bramblescript

import "testing"

func TestByteStream(t *testing.T) {
	tests := []scriptTest{
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
		{script: `b=cmd("echo", "hi").combined_output()`,
			returnValue: `"hi\n"`},
		{script: `b=cmd("echo", "hi").stderr()`,
			returnValue: `""`},
		{script: `b=cmd("echo", "hi").stderr`,
			returnValue: `<attribute 'stderr' of 'cmd'>`},
		{script: `b=cmd("echo", "hi").stdout`,
			returnValue: `<attribute 'stdout' of 'cmd'>`},
		{script: `b=cmd("echo", "hi").combined_output`,
			returnValue: `<attribute 'combined_output' of 'cmd'>`},
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
	for line in c.combined_output:
		if "cmd_test" in line:
			return line
b = run()`,
			returnValue: `"cmd_test.go"`},
	}
	runTest(t, tests)
}
