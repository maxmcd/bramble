package project

import (
	"fmt"
	"testing"
)

func TestDerivationCreation(t *testing.T) {
	tofn := func(v string) string {
		return fmt.Sprintf(`
def foo():
	%s
foo()
		`, v)
	}

	tests := []scriptTest{
		{script: `derivation("hi", "hi")`,
			errContains: "not within a function"},
		{script: tofn(`derivation("hi","hi", outputs=["hi", "ho"])`)},
		{script: tofn(`derivation("hi","hi", outputs=[{}])`), errContains: "cast type"},
		{script: tofn(`derivation("hi","hi", network=True)`), errContains: "use the network"},
		{script: tofn(`derivation("","hi")`), errContains: "must have a name"},
		{script: tofn(`derivation("hi","hi", outputs=[])`), errContains: "at least 1 value"},
		{script: tofn("derivation()"), errContains: "missing"},
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
