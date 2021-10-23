package command

import (
	"context"
	"log"
	"net/http"
	"os"
	"runtime"
	"testing"

	"github.com/opencontainers/runc/libcontainer"
	_ "github.com/opencontainers/runc/libcontainer/nsenter"
)

// init runs the libcontainer initialization code because of the busybox style needs
// to work around the go runtime and the issues with forking
func init() {
	if len(os.Args) < 2 || os.Args[1] != "init" {
		return
	}
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
	factory, err := libcontainer.New("")
	if err != nil {
		log.Fatalf("unable to initialize for container: %s", err)
	}
	if err := factory.StartInitialization(); err != nil {
		log.Fatal(err)
	}
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

// func TestRun(t *testing.T) {
// 	initIntegrationTest(t)
// 	runRun := func(module string, args []string) (output string, exitCode int) {
// 		cmd := exec.Command("bramble", append([]string{"run", module}, args...)...)
// 		o, _ := cmd.CombinedOutput()
// 		fmt.Println(string(o))
// 		return string(o), cmd.ProcessState.ExitCode()
// 	}

// 	type test struct {
// 		name   string
// 		module string
// 		args   []string

// 		outputContains   string
// 		expectedExitcode int
// 	}
// 	for _, tt := range []test{
// 		// Removed until it's reproducible
// 		// {
// 		// 	name:           "go run",
// 		// 	module:         "../../lib/go:bootstrap",
// 		// 	args:           []string{"go", "run", "testdata/main.go"},
// 		// 	outputContains: "hello world",
// 		// },
// 		// {
// 		// 	name:             "go run w/ exit code",
// 		// 	module:           "../../lib/go:bootstrap",
// 		// 	args:             []string{"go", "run", "testdata/main.go", "-exit-code", "2"},
// 		// 	outputContains:   "exit status 2",
// 		// 	expectedExitcode: 1, // go run will exit w/ 1 and print the non-1 exit code
// 		// },
// 	} {
// 		t.Run(tt.name, func(t *testing.T) {
// 			output, exitCode := runRun(tt.module, tt.args)
// 			assert.Equal(t, tt.expectedExitcode, exitCode)
// 			assert.Contains(t, output, tt.outputContains)
// 		})
// 	}
// }

// type lockWriter struct {
// 	lock   sync.Mutex
// 	writer io.Writer
// }

// func (lw *lockWriter) Write(p []byte) (n int, err error) {
// 	lw.lock.Lock()
// 	defer lw.lock.Unlock()
// 	return lw.writer.Write(p)
// }

func TestDep_handler(t *testing.T) {
	initIntegrationTest(t)
	app := cliApp()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := app.RunContext(ctx, []string{"bramble", "server"}); err != nil {
			t.Fatal(err)
		}
	}()
	t.Cleanup(func() { cancel() })
	for {
		resp, _ := http.Get("http://localhost:2726")
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			break
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
