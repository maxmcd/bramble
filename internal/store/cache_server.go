package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/httpx"
	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/pkg/errors"
)

// Uploads a derivation and all outputs
// Sources aren't uploaded
// Outputs are uploaded in 4mb body chunks
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
	router.POST("/derivation", func(c httpx.Context) (err error) {
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
	router.POST("/output", func(c httpx.Context) (err error) {
		hash := c.Request.URL.Query().Get("hash")
		if hash == "" {
			return httpx.ErrNotAcceptable(errors.New("the 'hash' query parameter is required"))
		}
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
