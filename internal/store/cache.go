package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/maxmcd/bramble/pkg/chunkedarchive"
	"github.com/maxmcd/bramble/pkg/httpx"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type cacheClient struct {
	host   string
	client *http.Client
}

func newCacheClient(host string) *cacheClient {
	return &cacheClient{
		host: host,
		client: &http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

func (cc *cacheClient) request(ctx context.Context, method, path, contentType string, body io.Reader, resp interface{}) (err error) {
	url := fmt.Sprintf("%s/%s",
		strings.TrimSuffix(cc.host, "/"),
		strings.TrimPrefix(path, "/"),
	)
	return httpx.Request(ctx, cc.client, method, url, contentType, body, resp)
}

func (cc *cacheClient) postDerivation(ctx context.Context, drv Derivation) (filename string, err error) {
	return filename, cc.request(ctx,
		http.MethodPost,
		"/derivation",
		"application/json",
		bytes.NewBuffer(drv.json()),
		&filename)
}

func (cc *cacheClient) postOutout(ctx context.Context, req outputRequestBody) (err error) {
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return cc.request(ctx,
		http.MethodPost,
		"/output",
		"application/json",
		bytes.NewBuffer(b),
		nil)
}

func (cc *cacheClient) postChunk(ctx context.Context, chunk io.Reader) (hash string, err error) {
	return hash, cc.request(ctx,
		http.MethodPost,
		"/chunk",
		"application/octet-stream",
		chunk,
		&hash)
}

func (cc *cacheClient) getDerivation(ctx context.Context, filename string) (drv Derivation, exists bool, err error) {
	err = cc.request(ctx,
		http.MethodGet,
		"/derivation/"+filename,
		"",
		nil,
		drv)
	if err == os.ErrNotExist {
		return drv, false, nil
	}
	return drv, err == nil, err
}

func (cc *cacheClient) getOutput(ctx context.Context, hash string) (output []chunkedarchive.TOCEntry, exists bool, err error) {
	err = cc.request(ctx,
		http.MethodGet,
		"/output/"+hash,
		"",
		nil,
		&output)
	if err == os.ErrNotExist {
		return nil, false, nil
	}
	return output, err == nil, err
}

func (cc *cacheClient) getChunk(ctx context.Context, hash string, chunk io.Writer) (err error) {
	return cc.request(ctx,
		http.MethodGet,
		"/chunk/"+hash,
		"",
		nil,
		chunk)
}
