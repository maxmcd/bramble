package brambleproject

import "testing"

func TestDerivationCreation(t *testing.T) {
	tests := []scriptTest{
		{script: `derivation()`,
			errContains: "not within a function"},
		{script: `
def foo():
  derivation()
foo()`,
			errContains: "missing argument for name"},
		{script: `
def foo():
	d = derivation("", builder="fetch_url", env={"url":1});
	return derivation("", builder="{}/bin/sh".format(d), env={"PATH":"{}/bin".format(d)}, sources=files(["*"]))
b = foo()
`,
			respContains: `brambleproject/derivation.go`},
	}
	runDerivationTest(t, tests)
}
