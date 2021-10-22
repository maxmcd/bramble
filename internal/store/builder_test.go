package store

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maxmcd/bramble/internal/types"
	"github.com/maxmcd/bramble/pkg/fileutil"
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
		httpResp    io.Reader
		urlPath     string
		drvHash     string
		confirmHash string
	}
	tests := []struct {
		name        string
		args        args
		errContains string
	}{
		{"hi",
			args{bytes.NewBufferString("hi"), "hi.txt", "", ""}, ""},
		{"hi confirm",
			args{bytes.NewBufferString("hi"), "hi.txt", "", "5fpq3tqlfd3r5ncyxwapgu5m7ahehe6r"}, ""},
		{"hi confirm drv",
			args{bytes.NewBufferString("hi"), "hi.txt", "5fpq3tqlfd3r5ncyxwapgu5m7ahehe6r", ""}, ""},
		{"hi wrong hash",
			args{bytes.NewBufferString("hi"), "hi.txt", "wronghash", ""}, "doesn't match"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewStore(fileutil.TestTmpDir(t))
			if err != nil {
				t.Fatal(err)
			}

			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				// will only work once
				_, _ = io.Copy(rw, tt.args.httpResp)
			}))

			lfw := testLockfileWriter{}

			env := map[string]string{"url": server.URL + "/" + tt.args.urlPath}
			if tt.args.drvHash != "" {
				env["hash"] = tt.args.drvHash
			}

			builder := store.NewBuilder(lfw)
			_, _, err = builder.BuildDerivation(context.Background(), Derivation{
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
			}
			if tt.args.confirmHash != "" {
				var first string
				for _, v := range lfw {
					first = v
					break
				}
				require.Equal(t, tt.args.confirmHash, first)
			}
		})
	}
}
