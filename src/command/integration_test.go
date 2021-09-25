package command

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func initIntegrationTest(t *testing.T) {
	if _, ok := os.LookupEnv("BRAMBLE_INTEGRATION_TEST"); !ok {
		t.Skip("skipping integration tests unless BRAMBLE_INTEGRATION_TEST is set")
	}
}
func TestRun(t *testing.T) {
	initIntegrationTest(t)
	runRun := func(module string, args []string) (output string, exitCode int) {
		cmd := exec.Command("bramble", append([]string{"run", module}, args...)...)
		o, _ := cmd.CombinedOutput()
		fmt.Println(string(o))
		return string(o), cmd.ProcessState.ExitCode()
	}

	type test struct {
		name   string
		module string
		args   []string

		outputContains   string
		expectedExitcode int
	}
	for _, tt := range []test{
		{
			name:           "go run",
			module:         "../../lib/go:bootstrap",
			args:           []string{"go", "run", "testdata/main.go"},
			outputContains: "hello world",
		},
		{
			name:             "go run w/ exit code",
			module:           "../../lib/go:bootstrap",
			args:             []string{"go", "run", "testdata/main.go", "-exit-code", "2"},
			outputContains:   "exit status 2",
			expectedExitcode: 1, // go run will exit w/ 1 and print the non-1 exit code
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			output, exitCode := runRun(tt.module, tt.args)
			assert.Equal(t, tt.expectedExitcode, exitCode)
			assert.Contains(t, output, tt.outputContains)
		})
	}
}
