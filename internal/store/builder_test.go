package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maxmcd/bramble/internal/types"
	"github.com/maxmcd/bramble/pkg/test"
	"github.com/stretchr/testify/require"
)

type testLockfileWriter map[string]string

var _ types.LockfileWriter = testLockfileWriter{}

func (lfw testLockfileWriter) AddEntry(k string, v string) error {
	lfw[k] = v
	return nil
}

func (lfw testLockfileWriter) LookupEntry(k string) (v string, found bool) {
	v, found = lfw[k]
	return v, found
}

func TestFetchURLBuilder(t *testing.T) {
	type args struct {
		httpResp    []byte
		urlPath     string
		drvHash     string
		confirmHash string
	}

	gziptar := bytes.NewBuffer(nil)
	{
		gzw := gzip.NewWriter(gziptar)
		tw := tar.NewWriter(gzw)
		_ = tw.WriteHeader(&tar.Header{
			Name:     "foo.txt",
			Typeflag: tar.TypeReg,
			Size:     7,
			Mode:     int64(0755),
		})
		_, _ = tw.Write([]byte("bramble"))
		_ = tw.Close()
		_ = gzw.Close()
	}

	tests := []struct {
		name        string
		args        args
		errContains string
	}{
		{
			"hi",
			args{[]byte("hi"), "hi.txt", "", ""},
			"",
		},
		{
			"hi confirm",
			args{[]byte("hi"), "hi.txt", "", "5fpq3tqlfd3r5ncyxwapgu5m7ahehe6r"},
			"",
		},
		{
			"hi confirm drv",
			args{[]byte("hi"), "hi.txt", "5fpq3tqlfd3r5ncyxwapgu5m7ahehe6r", ""},
			"",
		},
		{
			"hi wrong hash",
			args{[]byte("hi"), "hi.txt", "wronghash", ""},
			"doesn't match",
		},
		{
			"hi bad url",
			args{nil, "", "", ""},
			"requires the environment variable",
		},
		{
			"tar.gz",
			args{gziptar.Bytes(), "hi.tar.gz", "", "vl3rnztinfimffplwkrj45vdt4ihq72e"},
			"",
		},
		{
			"tar.gz",
			args{gziptar.Bytes(), "hi.tar.gz", "vl3rnztinfimffplwkrj45vdt4ihq72e", ""},
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewStore(test.TmpDir(t))
			if err != nil {
				t.Fatal(err)
			}

			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				// will only work once
				rw.Write(tt.args.httpResp)
			}))

			lfw := testLockfileWriter{}

			env := map[string]string{"url": server.URL + "/" + tt.args.urlPath}
			if tt.args.drvHash != "" {
				env["hash"] = tt.args.drvHash
			}
			if tt.args.urlPath == "" {
				delete(env, "url")
			}

			builder := store.NewBuilder(lfw)
			_, _, err = builder.BuildDerivation(context.Background(), Derivation{
				Name:        "test",
				Builder:     "basic_fetch_url",
				OutputNames: []string{"out"},
				Env:         env,
			}, BuildDerivationOptions{})
			if err != nil {
				if tt.errContains == "" {
					t.Fatal(err)
				} else {
					require.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			fmt.Println(lfw)
			if tt.args.confirmHash != "" {
				var first string
				for _, v := range lfw {
					first = v
					break
				}
				require.Equal(t, tt.args.confirmHash, first)
			}
			// Ensure rebuild is possible
			_, _, err = builder.BuildDerivation(context.Background(), Derivation{
				Name:        "test",
				Builder:     "basic_fetch_url",
				OutputNames: []string{"out"},
				Env:         env,
			}, BuildDerivationOptions{})
			require.NoError(t, err)
		})
	}
}
