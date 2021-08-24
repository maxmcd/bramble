package frontend

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
	return derivation("", builder="{}/bin/sh".format(d), env={"PATH":"{}/bin".format(d)})
b = foo()
`,
			respContains: `{{ tmb75glr3iqxaso2gn27ytrmr4ufkv6d-.drv:out }}`},
	}
	runDerivationTest(t, tests)
}
