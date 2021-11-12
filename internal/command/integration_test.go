package command

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maxmcd/bramble/internal/config"
	"github.com/maxmcd/bramble/internal/dependency"
	"github.com/maxmcd/bramble/internal/store"
	"github.com/maxmcd/bramble/pkg/sandbox"
	"github.com/maxmcd/bramble/pkg/test"
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
		app := cliApp(".")
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
	app := cliApp(".")
	ctx, cancel := context.WithCancel(context.Background())
	errChan := make(chan error)
	test.SetEnv(t, "BRAMBLE_PATH", t.TempDir())
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

func TestBuildAllFunction(t *testing.T) {
	initIntegrationTest(t)

	app := cliApp(".")
	if err := app.Run([]string{"bramble", "build", "github.com/maxmcd/bramble:all"}); err != nil {
		t.Fatal(err)
	}
}

func TestDep(t *testing.T) {
	initIntegrationTest(t)

	bramblePath := t.TempDir()
	test.SetEnv(t, "BRAMBLE_PATH", bramblePath)
	projectDir := t.TempDir()

	// Create bramble projects with files
	// for each one, resolve dependencies and then push to build server.
	store, err := store.NewStore(bramblePath)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(dependency.ServerHandler(
		filepath.Join(store.BramblePath, "var/dependencies"),
		newBuilder(store),
		func(url, reference string) (location string, err error) {
			return filepath.Join(projectDir, url), nil
		},
	))

	type testRun struct {
		name        string
		pkg         string
		files       map[string]interface{}
		errContains string
		install     []string
	}
	for _, tt := range []testRun{
		{"first", "first", map[string]interface{}{
			"./first/bramble.toml":    config.Config{Module: config.ConfigModule{Name: "first", Version: "0.0.1"}},
			"./first/default.bramble": "def first():\n  print('print from first')",
		}, "", nil},
		{"second syntax err", "second", map[string]interface{}{
			"./second/bramble.toml":    config.Config{Module: config.ConfigModule{Name: "second", Version: "0.0.1"}},
			"./second/default.bramble": "def first)",
		}, "second/default.bramble:1:10", nil},
		{"third with load", "third", map[string]interface{}{
			"./third/bramble.toml":    config.Config{Module: config.ConfigModule{Name: "third", Version: "0.0.1"}},
			"./third/default.bramble": "load('first')\ndef third():\n  first.first()",
		}, "", []string{"first@0.0.1"}},
		{"fourth nested", "fourth", map[string]interface{}{
			"./fourth/bramble.toml":           config.Config{Module: config.ConfigModule{Name: "fourth", Version: "0.0.1"}},
			"./fourth/default.bramble":        "load('third')\ndef fourth():\n  third.third()",
			"./fourth/nested/bramble.toml":    config.Config{Module: config.ConfigModule{Name: "fourth/nested", Version: "0.0.1"}},
			"./fourth/nested/default.bramble": "def nested():\n  print('hello nested')",
		}, "", []string{"third@0.0.1"}},
		{"fifth with nested load", "fifth", map[string]interface{}{
			"./fifth/bramble.toml":    config.Config{Module: config.ConfigModule{Name: "fifth", Version: "0.0.1"}},
			"./fifth/default.bramble": "load('fourth/nested')\ndef fifth():\n  nested.nested()",
		}, "", []string{"fourth/nested@0.0.1"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for path, file := range tt.files {
				path = filepath.Join(projectDir, path)
				_ = os.MkdirAll(filepath.Dir(path), 0755)
				switch v := file.(type) {
				case string:
					test.WriteFile(t, path, v)
				case config.Config:
					var sb strings.Builder
					v.Render(&sb)
					test.WriteFile(t, path, sb.String())
				}
			}
			test.ErrContains(t, func() error {
				{
					app := cliApp(filepath.Join(projectDir, tt.pkg))
					for _, toInstall := range tt.install {
						if err := app.Run([]string{"bramble", "add", toInstall}); err != nil {
							return err
						}
					}
					if err := app.Run([]string{"bramble", "build", "--parse-only"}); err != nil {
						return err
					}
				}
				{
					app := cliApp(".")
					if err := app.Run([]string{"bramble", "publish", "--url", server.URL, tt.pkg}); err != nil {
						return err
					}
				}
				return nil
			}(), tt.errContains)
		})
	}
}
