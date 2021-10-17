package command

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maxmcd/bramble/internal/dependency"
	"github.com/stretchr/testify/assert"
)

func initIntegrationTest(t *testing.T) {
	if _, ok := os.LookupEnv("VSCODE_CWD"); ok {
		// Allow tests to run within vscode
		return
	}
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
		// Removed until it's reproducible
		// {
		// 	name:           "go run",
		// 	module:         "../../lib/go:bootstrap",
		// 	args:           []string{"go", "run", "testdata/main.go"},
		// 	outputContains: "hello world",
		// },
		// {
		// 	name:             "go run w/ exit code",
		// 	module:           "../../lib/go:bootstrap",
		// 	args:             []string{"go", "run", "testdata/main.go", "-exit-code", "2"},
		// 	outputContains:   "exit status 2",
		// 	expectedExitcode: 1, // go run will exit w/ 1 and print the non-1 exit code
		// },
	} {
		t.Run(tt.name, func(t *testing.T) {
			output, exitCode := runRun(tt.module, tt.args)
			assert.Equal(t, tt.expectedExitcode, exitCode)
			assert.Contains(t, output, tt.outputContains)
		})
	}
}

type lockWriter struct {
	lock   sync.Mutex
	writer io.Writer
}

func (lw *lockWriter) Write(p []byte) (n int, err error) {
	lw.lock.Lock()
	defer lw.lock.Unlock()
	return lw.writer.Write(p)
}

func TestDep_handler(t *testing.T) {
	initIntegrationTest(t)
	cmd := exec.Command("bramble", "server")
	buf := &bytes.Buffer{}
	lw := &lockWriter{writer: io.MultiWriter(os.Stdout, buf)}
	cmd.Stdout = lw
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	for {
		lw.lock.Lock()
		if strings.Contains(buf.String(), "localhost") {
			lw.writer = os.Stdout
			lw.lock.Unlock()
			break
		}
		lw.lock.Unlock()
		time.Sleep(time.Millisecond * 100)
	}

	t.Cleanup(func() { _ = cmd.Process.Kill() })

	if err := dependency.PostJob("http://localhost:2726", "github.com/maxmcd/bramble", "dependencies"); err != nil {
		t.Fatal(err)
	}
}
