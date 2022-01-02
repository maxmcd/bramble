package s3test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"testing"

	"github.com/rlmcpherson/s3gof3r"
	"github.com/stretchr/testify/assert"
)

func setACL(h http.Header, acl string) http.Header {
	h.Set("x-amz-acl", acl)
	return h
}

func TestFoo(t *testing.T) {
	s := StartServer(t, "127.0.0.1:8910")
	host := s.Hostname()

	keys := s3gof3r.Keys{
		AccessKey: "a",
		SecretKey: "b",
	}
	os.Setenv("AWS_REGION", " ")
	s3 := s3gof3r.New(host, keys)
	bucket := s3.Bucket("bramble")

	w, err := bucket.PutWriter("/output/foo", setACL(http.Header{}, "public-read"), &s3gof3r.Config{
		Client:      http.DefaultClient,
		Scheme:      "http", // for this test
		Md5Check:    false,
		PathStyle:   true, // for this test
		Concurrency: 12,   // for this test
	})
	if err != nil {
		t.Fatal(err)
	}
	contents := make([]byte, 1e8)
	if _, err := rand.Read(contents); err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(w, ioutil.NopCloser(bytes.NewBuffer(contents)))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(fmt.Sprintf("http://%s/bramble/%s", s.Hostname(), "output/foo"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	assert.Equal(t, b, contents)
}
