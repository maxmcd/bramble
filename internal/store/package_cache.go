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

	"github.com/github/s3gof3r"
	"github.com/maxmcd/bramble/pkg/fileutil"
	"github.com/maxmcd/bramble/pkg/httpx"
	"github.com/maxmcd/bramble/pkg/reptar"
	"github.com/pkg/errors"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type CacheClient interface {
	PostDerivation(context.Context, Derivation) (string, error)
	PostOutput(ctx context.Context, hash string, body io.Reader) error
	ObjectWriter(ctx context.Context, key string) (io.WriteCloser, error)
}

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

type DefaultCacheClient struct {
	host   string
	client *http.Client
}

var _ CacheClient = new(DefaultCacheClient)

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

type S3CacheClient struct {
	bucket *s3gof3r.Bucket

	Scheme    string
	PathStyle bool
}

var _ CacheClient = new(S3CacheClient)

func NewS3CacheClient(accessKeyID, secretAccessKey, hostname string) *S3CacheClient {
	keys := s3gof3r.Keys{
		AccessKey: accessKeyID,
		SecretKey: secretAccessKey,
	}
	os.Setenv("AWS_REGION", " ")
	s3 := s3gof3r.New(hostname, keys)
	cc := &S3CacheClient{bucket: s3.Bucket("bramble")}
	cc.Scheme = "https"
	return cc
}

func (cc *S3CacheClient) putWriter(path string) (w io.WriteCloser, err error) {
	h := http.Header{}
	h.Set("x-amz-acl", "public-read")
	return cc.bucket.PutWriter(path, h, &s3gof3r.Config{
		Client:    http.DefaultClient,
		Scheme:    cc.Scheme,
		Md5Check:  false,
		PathStyle: cc.PathStyle,
	})
}

func (cc *S3CacheClient) ObjectWriter(ctx context.Context, path string) (writer io.WriteCloser, err error) {
	return cc.putWriter(path)
}

func (cc *S3CacheClient) PostDerivation(ctx context.Context, drv Derivation) (string, error) {
	filename := drv.Filename()
	w, err := cc.putWriter("derivation/" + filename)
	if err != nil {
		return "", errors.Wrap(err, "derivation/"+filename)
	}
	if _, err := w.Write([]byte(drv.JSON())); err != nil {
		return "", errors.WithStack(err)
	}
	return filename, w.Close()
}

func (cc *S3CacheClient) PostOutput(ctx context.Context, hash string, body io.Reader) error {
	w, err := cc.putWriter("output/" + hash)
	if err != nil {
		return errors.WithStack(err)
	}
	if _, err := io.Copy(w, body); err != nil {
		return errors.WithStack(err)
	}
	return w.Close()
}
