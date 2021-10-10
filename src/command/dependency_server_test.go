package command

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

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

	if err := postJob("http://localhost:2726", "github.com/maxmcd/bramble", "dependencies"); err != nil {
		t.Fatal(err)
	}
}
