package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/maxmcd/bramble/pkg/chunkedarchive"
	"github.com/maxmcd/bramble/pkg/httpx"
	"github.com/pkg/errors"
)

type storeHashFetcher struct {
	store *Store
}

var _ chunkedarchive.HashFetcher = new(storeHashFetcher)

func (hf *storeHashFetcher) Lookup(hash string) (file io.ReadCloser, err error) {
	return os.Open(hf.store.joinStorePath(hash))
}

type OutputRequestBody struct {
	Output Output
	TOC    []chunkedarchive.TOCEntry
}

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
		f, err := os.Open(s.joinStorePath(c.Params.ByName("hash")))
		if err != nil {
			return httpx.ErrNotFound(err)
		}
		var toc []chunkedarchive.TOCEntry
		if err := json.NewDecoder(f).Decode(&toc); err != nil {
			// If the hash isn't a valid TOC then it's not an output
			return httpx.ErrNotFound(err)
		}
		_, _ = f.Seek(0, 0)
		_, err = io.Copy(c.ResponseWriter, f)
		return err
	})
	router.GET("/chunk/:hash", func(c httpx.Context) (err error) {
		f, err := os.Open(s.joinStorePath(c.Params.ByName("hash")))
		if err != nil {
			return httpx.ErrNotFound(err)
		}
		_, err = io.Copy(c.ResponseWriter, f)
		return err
	})

	router.POST("/derivation", func(c httpx.Context) (err error) {
		var drv Derivation
		if err := json.NewDecoder(c.Request.Body).Decode(&drv); err != nil {
			return httpx.ErrUnprocessableEntity(err)
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
		var req OutputRequestBody
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			return httpx.ErrUnprocessableEntity(err)
		}

		tempDir, err := os.MkdirTemp("", "")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tempDir)
		if err := chunkedarchive.Unarchive(req.TOC, &storeHashFetcher{store: s}, tempDir); err != nil {
			return err
		}
		if err := s.hashNormalizedBuildOutput(tempDir, req.Output.Path); err != nil {
			return err
		}
		f, err := os.Create(s.joinStorePath(req.Output.Path + ".output"))
		if err != nil {
			return err
		}
		if err := json.NewEncoder(f).Encode(req.TOC); err != nil {
			return err
		}
		return nil
	})
	router.POST("/chunk", func(c httpx.Context) (err error) {
		hash, err := s.WriteBlob(c.Request.Body)
		if err != nil {
			return err
		}
		loc := s.joinStorePath(hash)
		fi, err := os.Stat(loc)
		if err != nil {
			return err
		}
		if fi.Size() > 4e6 {
			_ = os.Remove(loc)
			return httpx.ErrUnprocessableEntity(errors.New("chunk size can't be larger than 4MB"))
		}

		fmt.Fprint(c.ResponseWriter, hash)
		return nil
	})

	return router
}
