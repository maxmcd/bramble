package build

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStore_CacheServer(t *testing.T) {
	clientBramblePath := t.TempDir()
	clientStore, err := NewStore(clientBramblePath)
	require.NoError(t, err)

	{
		cmd := exec.Command("bramble", "build", "../../lib:busybox")
		cmd.Env = append(cmd.Env, "BRAMBLE_PATH="+clientBramblePath)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout

		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
	}

	{
		serverBramblePath := t.TempDir()
		s, err := NewStore(serverBramblePath)
		require.NoError(t, err)
		server := httptest.NewServer(s.CacheServer())
		_ = server
		files, _ := filepath.Glob(clientBramblePath + "/store/*.drv")
		var drvs []Derivation
		for _, path := range files {
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			var drv Derivation
			if err := json.NewDecoder(f).Decode(&drv); err != nil {
				t.Fatal(err)
			}
			drvs = append(drvs, drv)
		}
		cc := &cacheClient{host: server.URL, client: &http.Client{}}
		if err := clientStore.UploadDerivationsToCache(drvs, cc); err != nil {
			t.Fatal(err)
		}

		fmt.Println(serverBramblePath)
		time.Sleep(time.Minute * 10)
	}
}
