package cacheclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/maxmcd/bramble/internal/store"
	"github.com/maxmcd/bramble/pkg/chunkedarchive"
	"github.com/maxmcd/bramble/pkg/httpx"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type cacheClient interface {
	PostChunk(context.Context, io.Reader) (string, error)
	PostDerivation(context.Context, store.Derivation) (string, error)
	PostOutput(context.Context, store.OutputRequestBody) error
}

type Client struct {
	host   string
	client *http.Client
}

var _ cacheClient = new(Client)

func New(host string) *Client {
	return &Client{
		host: host,
		client: &http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

func (cc *Client) request(ctx context.Context, method, path, contentType string, body io.Reader, resp interface{}) (err error) {
	url := fmt.Sprintf("%s/%s",
		strings.TrimSuffix(cc.host, "/"),
		strings.TrimPrefix(path, "/"),
	)
	return httpx.Request(ctx, cc.client, method, url, contentType, body, resp)
}

func (cc *Client) PostDerivation(ctx context.Context, drv store.Derivation) (filename string, err error) {
	return filename, cc.request(ctx,
		http.MethodPost,
		"/derivation",
		"application/json",
		bytes.NewBuffer(drv.JSON()),
		&filename)
}

func (cc *Client) PostOutput(ctx context.Context, req store.OutputRequestBody) (err error) {
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

func (cc *Client) PostChunk(ctx context.Context, chunk io.Reader) (hash string, err error) {
	return hash, cc.request(ctx,
		http.MethodPost,
		"/chunk",
		"application/octet-stream",
		chunk,
		&hash)
}

func (cc *Client) GetDerivation(ctx context.Context, filename string) (drv store.Derivation, exists bool, err error) {
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

func (cc *Client) GetOutput(ctx context.Context, hash string) (output []chunkedarchive.TOCEntry, exists bool, err error) {
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

func (cc *Client) GetChunk(ctx context.Context, hash string, chunk io.Writer) (err error) {
	return cc.request(ctx,
		http.MethodGet,
		"/chunk/"+hash,
		"",
		nil,
		chunk)
}
