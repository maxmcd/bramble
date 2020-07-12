package bramble

import (
	"archive/tar"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDerivation_Build(t *testing.T) {
	tarBytes, err := TarGZipTestFiles([]TestFile{
		{
			Header: &tar.Header{
				Name: "./directory/hello.txt",
				Mode: 0755,
			},
			Body: []byte("hello"),
		}, {
			Header: &tar.Header{
				Name:     "hello.txt.link",
				Typeflag: tar.TypeLink,
				Linkname: "./directory/hello.txt",
				Mode:     0755,
			},
		}, {
			Header: &tar.Header{
				Name:     "hello.txt.symlink",
				Typeflag: tar.TypeSymlink,
				Linkname: "./directory/hello.txt",
				Mode:     0755,
			},
		},
	})
	PanicOnErr(err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write(tarBytes)
	}))
	drv := &Derivation{
		client:  NewTestClient(t),
		Name:    "test",
		Builder: "fetch_url",
		Environment: map[string]string{
			"url":        server.URL + "/archive.tar.gz",
			"hash":       "de80f7cc49f96e21dcbdb7fd3e1cecb731637c312be0aa329473e3c55f29b24c",
			"decompress": "true",
		},
		Outputs: map[string]Output{},
	}
	assert.NoError(t, drv.Build())
	files, err := ioutil.ReadDir(drv.client.StorePath(drv.Outputs["out"].Path))
	assert.NoError(t, err)
	assert.Len(t, files, 3)
}
