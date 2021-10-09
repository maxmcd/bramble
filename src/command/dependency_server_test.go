package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(Job{Module: "github.com/maxmcd/bramble", Reference: "dependencies"}); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post("https://bramble-server.fly.dev/job", "application/json", &body)
	if err != nil {
		time.Sleep(time.Second)
		t.Fatal(err)
	}
	fmt.Println(resp.StatusCode)
	io.Copy(os.Stdout, resp.Body)
}
