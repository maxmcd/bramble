package bramble

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/maxmcd/bramble/pkg/reptar"
)

type Config struct {
	Tests []Test `toml:"tests"`
}
type Test struct {
	Name        string     `toml:"name"`
	BrambleFile string     `toml:"bramble_file"`
	ServeFile   *ServeFile `toml:"serve_file"`
}
type ServeFile struct {
	Source      string `toml:"source"`
	Destination string `toml:"destination"`
}

func TestIntegrationTests(t *testing.T) {
	var config Config

	testsDir, err := filepath.Abs("../../tests")
	PanicOnErr(err)

	_, err = toml.DecodeFile(filepath.Join(testsDir, "tests.toml"), &config)
	PanicOnErr(err)

	err = os.Chdir(testsDir)
	PanicOnErr(err)
	for _, test := range config.Tests {
		t.Run(test.Name, func(t *testing.T) {
			c := NewTestClient(t)
			c.test = true

			if test.ServeFile != nil {
				var buf bytes.Buffer
				err = reptar.GzipReptar(test.ServeFile.Source, &buf)
				PanicOnErr(err)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					_, _ = w.Write(buf.Bytes())
				}))
				c.testURL = server.URL
			}

			if _, err := c.Run(filepath.Join(testsDir, test.BrambleFile)); err != nil {
				t.Error(err)
			}
		})
	}
}
