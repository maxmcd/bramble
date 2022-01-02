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

	"github.com/maxmcd/bramble/internal/netcache"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/httpx"
	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/pkg/errors"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func (s *Store) CacheServer() http.Handler {
	router := httpx.New()
	router.GET("/derivation/:filename", func(c httpx.Context) (err error) {
		f, err := os.Open(s.joinStorePath(c.Params.ByName("filename")))
		if err != nil {
			return httpx.ErrNotFound(err)
		}
		defer f.Close()
		_, err = io.Copy(c.ResponseWriter, f)
		return err
	})
	router.GET("/output/:hash", func(c httpx.Context) (err error) {
		output := s.joinStorePath(c.Params.ByName("hash"))
		if !fileutil.DirExists(output) {
			return httpx.ErrNotFound(errors.New("Output not found"))
		}
		if err := reptar.Archive(output, c.ResponseWriter); err != nil {
			return httpx.ErrInternalServerError(err)
		}
		return nil
	})
	router.POST("/derivation/:filename", func(c httpx.Context) (err error) {
		var drv Derivation
		if err := json.NewDecoder(c.Request.Body).Decode(&drv); err != nil {
			return httpx.ErrNotAcceptable(err)
		}
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(drv); err != nil {
			return err
		}
		filename, err := s.WriteDerivation(drv)
		if err != nil {
			return err
		}
		fmt.Fprint(c.ResponseWriter, filename)
		return nil
	})
	router.POST("/output/:hash", func(c httpx.Context) (err error) {
		hash := c.Params.ByName("hash")
		tempDir, err := os.MkdirTemp("", "")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tempDir)

		if err := reptar.Unarchive(c.Request.Body, tempDir); err != nil {
			return httpx.ErrNotAcceptable(errors.Wrap(err, "error unarchiving request body"))
		}
		if err := s.hashNormalizedBuildOutput(tempDir, hash); err != nil {
			return err
		}
		storeLocation := s.joinStorePath(hash)
		if !fileutil.PathExists(storeLocation) {
			return os.Rename(tempDir, storeLocation)
		}
		return nil
	})

	return router
}

func NewCacheClient(client netcache.Client) CacheClient {
	return CacheClient{client}
}

type CacheClient struct {
	client netcache.Client
}

func (cc CacheClient) PostDerivation(ctx context.Context, drv Derivation) (string, error) {
	filename := drv.Filename()
	writer, err := cc.client.Put(ctx, "derivation/"+filename)
	if err != nil {
		return "", errors.WithStack(err)
	}
	if _, err := writer.Write([]byte(drv.JSON())); err != nil {
		return "", errors.WithStack(err)
	}
	return filename, writer.Close()
}

func (cc CacheClient) PostOutput(ctx context.Context, hash string, body io.Reader) error {
	w, err := cc.client.Put(ctx, "output/"+hash)
	if err != nil {
		return errors.WithStack(err)
	}
	if _, err := io.Copy(w, body); err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(w.Close())
}

func (cc CacheClient) GetOutput(ctx context.Context, hash string) (body io.ReadCloser, exists bool, err error) {
	w, err := cc.client.Get(ctx, "output/"+hash)
	if err != nil {
		if _, ok := err.(netcache.ErrNotFound); ok {
			return nil, false, nil
		}
		return nil, false, errors.WithStack(err)
	}
	return w, true, nil
}

func (cc CacheClient) GetDerivation(ctx context.Context, filename string) (drv Derivation, exists bool, err error) {
	w, err := cc.client.Get(ctx, "derivation/"+filename)
	if err != nil {
		if _, ok := err.(netcache.ErrNotFound); ok {
			return Derivation{}, false, nil
		}
		return Derivation{}, false, errors.WithStack(err)
	}
	return drv, true, json.NewDecoder(w).Decode(drv)
}

type DefaultCacheClient struct {
	host   string
	client *http.Client
}

func NewDefaultCacheClient(host string) *DefaultCacheClient {
	return &DefaultCacheClient{
		host: host,
		client: &http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

func (cc *DefaultCacheClient) url(path string) string {
	return fmt.Sprintf("%s/%s",
		strings.TrimSuffix(cc.host, "/"),
		strings.TrimPrefix(path, "/"),
	)
}

func (cc *DefaultCacheClient) request(ctx context.Context, method, path, contentType string, body io.Reader, resp interface{}) (err error) {
	return httpx.Request(ctx, cc.client, method, cc.url(path), contentType, body, resp)
}

func (cc *DefaultCacheClient) ObjectWriter(ctx context.Context, path string) (io.WriteCloser, error) {
	panic("unimplemented")
}

func (cc *DefaultCacheClient) PostDerivation(ctx context.Context, drv Derivation) (filename string, err error) {
	return filename, cc.request(ctx,
		http.MethodPost,
		"/derivation",
		"application/json",
		bytes.NewBuffer(drv.JSON()),
		&filename)
}

func (cc *DefaultCacheClient) PostOutput(ctx context.Context, hash string, body io.Reader) (err error) {
	return cc.request(ctx,
		http.MethodPost,
		"/output?hash="+hash,
		"application/octet-stream",
		body,
		nil)
}

func (cc *DefaultCacheClient) GetDerivation(ctx context.Context, filename string) (drv Derivation, exists bool, err error) {
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

func (cc *DefaultCacheClient) GetOutput(ctx context.Context, hash string) (body io.ReadCloser, exists bool, err error) {
	err = cc.request(ctx, http.MethodGet, "output/"+hash, "", nil, &body)
	if err == os.ErrNotExist {
		return nil, false, nil
	}
	return body, true, nil
}
