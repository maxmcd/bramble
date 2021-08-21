package lang

import (
	"testing"
)

func TestBramble_filesBuiltin(t *testing.T) {
	tests := []scriptTest{
		{script: `files()`,
			errContains: "missing argument"},
		{script: `files([])`,
			errContains: "matched zero files"},
		{script: `files([], ["[]a]"])`,
			errContains: "syntax error"},
		{script: `b = files(["../../../../../*.go"])`,
			errContains: "outside of the project"},
		{script: `b = files(["/*.go"])`,
			errContains: "absolute"},
		{script: `b = files([1])`,
			errContains: "not a string"},
		{script: `b = files(["."], include_directories=True)`,
			respContains: "pkg/bramble"},
		{script: `b = files(["*.go"])`,
			respContains: "bramble.go"},
		{script: `b = files(["*.go"], ["*_test.go"])`,
			respDoesntContain: "_test.go"},
		{script: `files([], allow_empty=True)`},
		{name: "ensure no directories",
			script:      `b = files(["../*"])`,
			errContains: "zero files"},
		{name: "unless we include directories",
			script:       `b = files(["../*"], include_directories=True)`,
			respContains: "bramble"},
	}
	runDerivationTest(t, tests)
}
