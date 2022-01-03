package netcache

import (
	"fmt"
	"go/build"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/pkg/errors"
)

type minio struct {
	cmd *exec.Cmd
}

func StartMinio(t *testing.T) Client {
	minioBin := filepath.Join(build.Default.GOPATH, "bin", "minio")
	mcBin := filepath.Join(build.Default.GOPATH, "bin", "mc")
	for _, bin := range [][]string{
		[]string{minioBin, "https://dl.min.io/server/minio/release/linux-amd64/minio"},
		[]string{mcBin, "https://dl.min.io/client/mc/release/linux-amd64/mc"},
	} {
		location, url := bin[0], bin[1]
		if !fileutil.FileExists(location) {
			resp, err := http.Get(url)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatal(fmt.Errorf("unexpected response code %s %s: %d", resp.Request.Method, resp.Request.URL, resp.StatusCode))
			}
			f, err := os.Create(location)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.Copy(f, resp.Body); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(location, 0755); err != nil {
				t.Fatal(err)
			}
		}
	}
	stateDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(stateDir, "bramble"), 0755); err != nil {
		t.Fatal(err)
	}
	fmt.Println("Minio state directory:", stateDir)
	// Start server with address and path to bucket for object state
	cmd := exec.Command(minioBin, "server", "--address", ":9000", "--console-address", ":9001", stateDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}

	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "MINIO_ROOT_USER=root", "MINIO_ROOT_PASSWORD=password")
	t.Cleanup(func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	})
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	for {
		if hasExited(cmd) {
			t.Fatal("process exited unexpectedly")
		}
		_, err := http.Get("http://localhost:9000")
		if err == nil {
			// We're up!
			break
		}
		time.Sleep(time.Millisecond * 100)
	}

	runCommand(t, mcBin, "alias", "set", "local", "http://localhost:9000", "root", "password")
	runCommand(t, mcBin, "policy", "set", "download", "local/bramble")

	client, err := NewS3Cache(S3CacheOptions{
		SecretAccessKey: "password",
		AccessKeyID:     "root",
		S3url:           "http://localhost:9000",
		PathStyle:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func runCommand(t *testing.T, arguments ...string) {
	cmd := exec.Command(arguments[0], arguments[1:]...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(errors.Wrap(err, fmt.Sprint(arguments, " ", string(b))))
	}
}

func hasExited(cmd *exec.Cmd) bool {
	if cmd.ProcessState != nil {
		return cmd.ProcessState.Exited()
	}
	return false
}
