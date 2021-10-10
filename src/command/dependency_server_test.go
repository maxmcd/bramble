package command

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestDep_handler(t *testing.T) {
	cmd := exec.Command("bramble", "server")
	buf := &bytes.Buffer{}
	cmd.Stdout = io.MultiWriter(os.Stdout, buf)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	for {
		if strings.Contains(buf.String(), "localhost") {
			cmd.Stdout = os.Stdout
			break
		}
		time.Sleep(time.Millisecond * 100)
	}

	t.Cleanup(func() { _ = cmd.Process.Kill() })

	if err := postJob("http://localhost:2726", "github.com/maxmcd/bramble", "dependencies"); err != nil {
		t.Fatal(err)
	}
}
