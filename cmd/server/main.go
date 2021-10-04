package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/julienschmidt/httprouter"
	"github.com/maxmcd/bramble/pkg/chunkedarchive"
	"github.com/maxmcd/bramble/src/build"
	"github.com/pkg/errors"
)

type Context struct {
	ResponseWriter http.ResponseWriter
	Request        *http.Request
	Params         httprouter.Params
}

func H(handler func(c Context) (err error)) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		err := handler(Context{ResponseWriter: rw, Request: r, Params: p})
		if err != nil {
			code := http.StatusInternalServerError
			if v, ok := err.(errHTTPResponse); ok {
				code = v.code
			}
			http.Error(rw, err.Error(), code)
		}
	}
}

type errHTTPResponse struct {
	err  error
	code int
}

func (err errHTTPResponse) Error() string { return err.err.Error() }
func notFound(err error) error            { return errHTTPResponse{err: err, code: http.StatusNotFound} }
func unprocessable(err error) error {
	return errHTTPResponse{err: err, code: http.StatusUnprocessableEntity}
}

// Uploads a derivation and all outputs
// Sources aren't uploaded
// Outputs are uploaded in 4mb body chunks
func main() {
	store, err := build.NewStore("")
	if err != nil {
		panic(err)
	}

	router := httprouter.New()
	router.GET("/derivation/:filename", H(func(c Context) (err error) {
		f, err := os.Open(filepath.Join(store.StorePath, c.Params.ByName("filename")))
		if err != nil {
			return notFound(err)
		}
		defer f.Close()
		_, err = io.Copy(c.ResponseWriter, f)
		return err
	}))
	router.GET("/output/:hash", H(func(c Context) (err error) {
		f, err := os.Open(filepath.Join(store.StorePath, c.Params.ByName("hash")))
		if err != nil {
			return notFound(err)
		}
		var toc []chunkedarchive.TOCEntry
		if err := json.NewDecoder(f).Decode(&toc); err != nil {
			// If the hash isn't a valid TOC then it's not an output
			return notFound(err)
		}
		_, _ = f.Seek(0, 0)
		_, err = io.Copy(c.ResponseWriter, f)
		return err
	}))
	router.GET("/chunk/:hash", H(func(c Context) (err error) {
		f, err := os.Open(filepath.Join(store.StorePath, c.Params.ByName("hash")))
		if err != nil {
			return notFound(err)
		}
		_, err = io.Copy(c.ResponseWriter, f)
		return err
	}))

	router.POST("/derivation", H(func(c Context) (err error) {
		var drv build.Derivation
		if err := json.NewDecoder(c.Request.Body).Decode(&drv); err != nil {
			return unprocessable(err)
		}
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(drv); err != nil {
			return err
		}
		filename, err := store.WriteDerivation(drv)
		if err != nil {
			return err
		}
		fmt.Fprint(c.ResponseWriter, filename)
		return nil
	}))
	router.POST("/output", H(func(c Context) (err error) {
		var toc []chunkedarchive.TOCEntry
		if err := json.NewDecoder(c.Request.Body).Decode(&toc); err != nil {
			return unprocessable(err)
		}
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(toc); err != nil {
			return err
		}
		hash, err := store.WriteBlob(&buf)
		if err != nil {
			return err
		}
		fmt.Fprint(c.ResponseWriter, hash)
		return nil
	}))
	router.POST("/chunk", H(func(c Context) (err error) {
		hash, err := store.WriteBlob(c.Request.Body)
		if err != nil {
			return err
		}
		loc := filepath.Join(store.StorePath, hash)
		fi, err := os.Stat(loc)
		if err != nil {
			return err
		}
		if fi.Size() > 4e6 {
			_ = os.Remove(loc)
			return unprocessable(errors.New("chunk size can't be larger than 4MB"))
		}

		fmt.Fprint(c.ResponseWriter, hash)
		return nil
	}))

	log.Fatal(http.ListenAndServe(":8080", router))
}
