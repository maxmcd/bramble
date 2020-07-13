package bramble

import (
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/mholt/archiver/v3"
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
	Sources     []string `toml:"sources"`
	Destination string   `toml:"destination"`
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
				name, err := ioutil.TempDir("", TestTmpDirPrefix)
				PanicOnErr(err)
				outPath := filepath.Join(name, test.ServeFile.Destination)

				err = archiver.Archive(test.ServeFile.Sources, outPath)
				PanicOnErr(err)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					f, err := os.Open(outPath)
					PanicOnErr(err)
					_, _ = io.Copy(w, f)
				}))
				c.testURL = server.URL
			}

			if _, err := c.Run(filepath.Join(testsDir, test.BrambleFile)); err != nil {
				t.Error(err)
			}
		})
	}
}
