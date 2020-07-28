package bramblescript

import "testing"

func TestIfErr(t *testing.T) {
	tests := []scriptTest{
		{script: `b=cmd("ls", "-notathing").if_err("echo", "hi").stdout()`,
			returnValue: `"hi\n"`},
		{script: `b=cmd("ls", "-notathing").if_err("echo", "hi").stdout()`,
			returnValue: `"hi\n"`},
	}
	runTest(t, tests)
}
