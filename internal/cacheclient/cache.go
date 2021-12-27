package cacheclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/maxmcd/bramble/internal/store"
	"github.com/maxmcd/bramble/pkg/httpx"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type cacheClient interface {
	PostDerivation(context.Context, store.Derivation) (string, error)
	PostOutput(ctx context.Context, hash string, body io.Reader) error
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

func (cc *Client) url(path string) string {
	return fmt.Sprintf("%s/%s",
		strings.TrimSuffix(cc.host, "/"),
		strings.TrimPrefix(path, "/"),
	)
}

func (cc *Client) request(ctx context.Context, method, path, contentType string, body io.Reader, resp interface{}) (err error) {
	return httpx.Request(ctx, cc.client, method, cc.url(path), contentType, body, resp)
}

func (cc *Client) PostDerivation(ctx context.Context, drv store.Derivation) (filename string, err error) {

	return filename, cc.request(ctx,
		http.MethodPost,
		"/derivation",
		"application/json",
		bytes.NewBuffer(drv.JSON()),
		&filename)
}

func (cc *Client) PostOutput(ctx context.Context, hash string, body io.Reader) (err error) {
	return cc.request(ctx,
		http.MethodPost,
		"/output?hash="+hash,
		"application/octet-stream",
		body,
		nil)
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

func (cc *Client) GetOutput(ctx context.Context, hash string) (body io.ReadCloser, exists bool, err error) {
	err = cc.request(ctx, http.MethodGet, "output/"+hash, "", nil, &body)
	if err == os.ErrNotExist {
		return nil, false, nil
	}
	return body, true, nil
}
