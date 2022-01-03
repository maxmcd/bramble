package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/maxmcd/bramble/internal/netcache"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/httpx"
	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/pkg/errors"
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
	return drv, true, json.NewDecoder(w).Decode(&drv)
}
