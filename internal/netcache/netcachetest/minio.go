package netcachetest

import (
	"fmt"
	"go/build"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/maxmcd/bramble/internal/netcache"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/pkg/errors"
)

// getFreePort asks the kernel for a free open port that is ready to use.
func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func StartMinio(t *testing.T) netcache.Client {
	minioBin := filepath.Join(build.Default.GOPATH, "bin", "minio")
	mcBin := filepath.Join(build.Default.GOPATH, "bin", "mc")
	for _, bin := range [][]string{
		{minioBin, "https://dl.min.io/server/minio/release/linux-amd64/minio"},
		{mcBin, "https://dl.min.io/client/mc/release/linux-amd64/mc"},
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

	addr, err := getFreePort()
	if err != nil {
		t.Fatal(err)
	}

	// Start server with address and path to bucket for object state
	cmd := exec.Command(minioBin, "server",
		"--address", fmt.Sprintf(":%d", addr),
		stateDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}

	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "MINIO_ROOT_USER=root", "MINIO_ROOT_PASSWORD=password")
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s3Addr := fmt.Sprint("http://localhost:", addr)
	fmt.Println("Minio running at", s3Addr)
	for {
		if hasExited(cmd) {
			t.Fatal("process exited unexpectedly")
		}
		resp, err := http.Get(s3Addr)
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		if err == nil {
			// We're up!
			break
		}
		time.Sleep(time.Millisecond * 100)
	}

	runCommand(t, mcBin, "alias", "set", "local", s3Addr, "root", "password")
	runCommand(t, mcBin, "policy", "set", "download", "local/bramble")

	client, err := netcache.NewS3Cache(netcache.S3CacheOptions{
		SecretAccessKey: "password",
		AccessKeyID:     "root",
		S3url:           s3Addr,
		PathStyle:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func runCommand(t *testing.T, arguments ...string) {
	for {
		cmd := exec.Command(arguments[0], arguments[1:]...)
		b, err := cmd.CombinedOutput()
		if strings.Contains(string(b), "Server not initialized") {
			time.Sleep(time.Millisecond * 10)
			continue
		}
		if err != nil {
			t.Fatal(errors.Wrap(err, fmt.Sprint(arguments, " ", string(b))))
		}
		return
	}
}

func hasExited(cmd *exec.Cmd) bool {
	if cmd.ProcessState != nil {
		return cmd.ProcessState.Exited()
	}
	return false
}
