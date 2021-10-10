package project

import "testing"

func TestDerivationCreation(t *testing.T) {
	tests := []scriptTest{
		{script: `derivation("hi", "hi")`,
			errContains: "not within a function"},
		{script: `
def foo():
  derivation()
foo()`,
			errContains: "missing"},
		{script: `
def foo():
	d = derivation("a", builder="fetch_url", env={"url":1});
	return derivation("a", builder="{}/bin/sh".format(d), env={"PATH":"{}/bin".format(d)}, sources=files(["*"]))
b = foo()
`,
			respContains: `project/derivation.go`},
	}
	runDerivationTest(t, tests, "")
}
