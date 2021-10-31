package command

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/maxmcd/bramble/pkg/sandbox"
	_ "github.com/opencontainers/runc/libcontainer/nsenter"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

// The tests will run this entrypoint so that the sandbox can pick up from this
// point when it's run within a test
func init() {
	sandbox.Entrypoint()
}

func initIntegrationTest(t *testing.T) {
	t.Helper()
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
	type check func(t *testing.T, exitCode int, err error)
	type test struct {
		name   string
		args   []string
		checks []check
	}
	errContains := func(v string) check {
		return func(t *testing.T, exitCode int, err error) {
			if err == nil {
				t.Error("run did not error as expected")
			}
			assert.Contains(t, err.Error(), v)
		}
	}
	exitCodeIs := func(v int) check {
		return func(t *testing.T, exitCode int, err error) {
			assert.Equal(t, exitCode, v)
		}
	}
	noError := func() check {
		return func(t *testing.T, exitCode int, err error) {
			assert.Equal(t, 0, exitCode)
			assert.NoError(t, err)
		}
	}
	runRun := func(t *testing.T, tt test) (exitCode int, err error) {
		t.Helper()
		app := cliApp()
		err = app.Run(append([]string{"bramble", "run"}, tt.args...))
		if err != nil {
			if er, ok := errors.Cause(err).(sandbox.ExitError); ok {
				return er.ExitCode, err
			}
			return 1, err
		}
		return 0, err
	}

	for _, tt := range []test{
		{
			name:   "simple",
			args:   []string{"../../:print_simple"},
			checks: []check{noError()},
		},
		{
			name:   "simple explicit",
			args:   []string{"../../:print_simple", "simple"},
			checks: []check{noError()},
		},
		{
			name: "sim",
			args: []string{"../../:print_simple", "sim"},
			checks: []check{
				errContains("executable file not found"),
				exitCodeIs(1),
			},
		},
		{
			name: "exit code",
			args: []string{"../../:bash", "bash", "-c", "exit 2"},
			checks: []check{
				exitCodeIs(2),
			},
		},
		{
			name: "weird exit code",
			args: []string{"../../:bash", "bash", "-c", "exit 56"},
			checks: []check{
				exitCodeIs(56),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			exitCode, err := runRun(t, tt)
			for _, check := range tt.checks {
				check(t, exitCode, err)
			}
		})
	}
}

func TestDep_handler(t *testing.T) {
	initIntegrationTest(t)
	app := cliApp()
	ctx, cancel := context.WithCancel(context.Background())
	errChan := make(chan error)
	os.Setenv("BRAMBLE_PATH", t.TempDir())
	go func() {
		if err := app.RunContext(ctx, []string{"bramble", "server"}); err != nil {
			errChan <- err
		}
	}()
	t.Cleanup(func() { cancel() })
	for {
		resp, _ := http.Get("http://localhost:2726")
		if resp != nil {
			resp.Body.Close()
		}
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			break
		}
		select {
		case err := <-errChan:
			t.Fatal(err)
		default:
		}
	}
	if err := app.RunContext(ctx, []string{"bramble", "publish", "github.com/maxmcd/busybox"}); err != nil {
		t.Fatal(err)
	}
}

func TestNative(t *testing.T) {
	initIntegrationTest(t)

	app := cliApp()
	if err := app.Run([]string{"bramble", "build", "github.com/maxmcd/bramble:all"}); err != nil {
		t.Fatal(err)
	}
}
