package build

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
	"github.com/pkg/errors"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type cacheClient struct {
	host   string
	client *http.Client
}

func (cc *cacheClient) request(ctx context.Context, method, path, contentType string, body io.Reader, resp interface{}) (err error) {
	req, err := http.NewRequest(method,
		fmt.Sprintf("%s/%s",
			strings.TrimSuffix(cc.host, "/"),
			strings.TrimPrefix(path, "/"),
		), body)
	if err != nil {
		return err
	}
	// TODO: Move elsewhere?
	cc.client.Transport = otelhttp.NewTransport(http.DefaultTransport)
	req = req.WithContext(ctx)
	if method == http.MethodPost {
		req.Header.Add("Content-Type", contentType)
	}
	httpResp, err := cc.client.Do(req)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	var buf bytes.Buffer
	if httpResp.Body != nil {
		_, _ = io.Copy(&buf, httpResp.Body)
	}
	if httpResp.StatusCode == http.StatusNotFound {
		return os.ErrNotExist
	}
	if httpResp.StatusCode != http.StatusOK {
		return errors.Errorf("Unexpected response code %d: %s",
			httpResp.StatusCode, buf.String())
	}
	if resp == nil {
		return nil
	}
	switch v := resp.(type) {
	case *string:
		*v = buf.String()
	case io.Writer:
		_, err = io.Copy(v, httpResp.Body)
	default:
		err = json.Unmarshal(buf.Bytes(), resp)
	}
	return err
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
