package netcache

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/julienschmidt/httprouter"
	"github.com/klauspost/pgzip"
	"github.com/maxmcd/bramble/pkg/io2"
	"github.com/maxmcd/bramble/pkg/url2"
	"github.com/pkg/errors"
	"github.com/rlmcpherson/s3gof3r"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Client interface {
	Get(ctx context.Context, path string) (body io.ReadCloser, err error)
	Put(ctx context.Context, path string) (writer io.WriteCloser, err error)
}

type ErrNotFound struct{ path string }

func (e ErrNotFound) Error() string {
	return fmt.Sprintf("object %q not found in cache", e.path)
}

type ErrFailedRequest struct {
	isPut bool
	path  string
	body  string
}

func (e ErrFailedRequest) Error() string {
	template := "error while trying to read %q from cache"
	if e.isPut {
		template = "error while trying to write %q to cache"
	}
	out := fmt.Sprintf(template, e.path)
	if e.body != "" {
		out = ": " + e.body
	}
	return out
}

func errUploading(path, body string) error {
	return ErrFailedRequest{isPut: true, body: body, path: path}
}
func errFetching(path, body string) error {
	return ErrFailedRequest{isPut: false, body: body, path: path}
}

var defaultRouter = func() *httprouter.Router {
	nilHandler := func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		panic("unimplemented")
	}
	router := httprouter.New()
	router.GET("/derivation/:filename", nilHandler)
	router.GET("/output/:hash", nilHandler)
	router.POST("/derivation", nilHandler)
	router.POST("/output", nilHandler)

	router.GET("/package/versions/*name", nilHandler)
	router.GET("/package/sources/*name_version", nilHandler)
	router.GET("/package/config/*name_version", nilHandler)
	return router
}()

type S3CacheOptions struct {
	AccessKeyID     string
	SecretAccessKey string
	S3url           string
	CDNPrefix       string
	PathStyle       bool
}

func NewS3Cache(opt S3CacheOptions) (Client, error) {
	keys := s3gof3r.Keys{
		AccessKey: opt.AccessKeyID,
		SecretKey: opt.SecretAccessKey,
	}
	parsed, err := url.Parse(opt.S3url)
	if err != nil {
		return nil, errors.Wrapf(err, "error pasing S3url parameter %s", opt.S3url)
	}

	// TODO: this must be set for the entire lifetime of the client/bucket.
	// Should patch underlying lib to support explicit region. Although since
	// we're not relying on this value for now this is not really issue, the
	// value just needs to be set to something.
	os.Setenv("AWS_REGION", " ")

	s3 := s3gof3r.New(parsed.Host, keys)
	cc := &S3Cache{bucket: s3.Bucket("bramble")}
	cc.bucket.Client = &http.Client{
		// For tracing
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
	cc.Scheme = parsed.Scheme
	cc.CDNPrefix = opt.CDNPrefix
	cc.PathStyle = opt.PathStyle
	return cc, nil
}

type S3Cache struct {
	bucket *s3gof3r.Bucket

	CDNPrefix string

	Scheme    string
	PathStyle bool
}

type wrapperWriteCloser struct {
	ctx         context.Context
	path        string
	writeCloser io.WriteCloser
}

func (wc wrapperWriteCloser) Write(b []byte) (n int, err error) {
	select {
	case <-wc.ctx.Done():
		return 0, context.Canceled
	default:
	}
	n, err = wc.writeCloser.Write(b)
	if err != nil {
		return n, errUploading(wc.path, err.Error())
	}
	return n, err
}

func (wc wrapperWriteCloser) Close() (err error) {
	select {
	case <-wc.ctx.Done():
		return context.Canceled
	default:
	}
	if err := wc.writeCloser.Close(); err != nil {
		return &ErrFailedRequest{isPut: true, path: wc.path, body: err.Error()}
	}
	return nil
}

func (c *S3Cache) putWriter(ctx context.Context, path string) (putWriter io.WriteCloser, err error) {
	encodedPath := encodePath(path)
	h := http.Header{}
	h.Set("x-amz-acl", "public-read")
	putWriter, err = c.bucket.PutWriter(encodedPath, h, &s3gof3r.Config{
		Client:    http.DefaultClient,
		Scheme:    c.Scheme,
		Md5Check:  false,
		PathStyle: c.PathStyle,
	})
	if err != nil {
		return nil, errUploading(path, err.Error())
	}
	gzw := pgzip.NewWriter(putWriter)
	return wrapperWriteCloser{ctx: ctx, path: path, writeCloser: io2.WriterMultiCloser(gzw, gzw, putWriter)}, nil
}

func (c *S3Cache) Get(ctx context.Context, path string) (body io.ReadCloser, err error) {
	encodedPath := encodePath(path)
	req, err := http.NewRequest(http.MethodGet, url2.Join("https://store.bramble.run", encodedPath), nil)
	if err != nil {
		return nil, errFetching(path, err.Error())
	}
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err := responseError(false, path, resp, err); err != nil {
		return nil, err
	}
	gzr, err := pgzip.NewReader(resp.Body)
	if err != nil {
		return nil, err
	}
	return io2.ReaderMultiCloser(gzr, gzr, resp.Body), nil
}

func (c *S3Cache) Put(ctx context.Context, path string) (writer io.WriteCloser, err error) {
	return c.putWriter(ctx, path)
}

var reservedObjectNames = regexp.MustCompile("^[a-zA-Z0-9-_.~/]+$")

// https://github.com/rhnvrm/simples3/blob/ad0419ef77c905b3909459f5eaaa4cefe2232981/simples3.go#L617
func encodePath(pathName string) string {
	if reservedObjectNames.MatchString(pathName) {
		return pathName
	}
	var encodedPathname strings.Builder
	for _, s := range pathName {
		if 'A' <= s && s <= 'Z' || 'a' <= s && s <= 'z' || '0' <= s && s <= '9' { // §2.3 Unreserved characters (mark)
			encodedPathname.WriteRune(s)
			continue
		}
		switch s {
		case '-', '_', '.', '~', '/': // §2.3 Unreserved characters (mark)
			encodedPathname.WriteRune(s)
			continue
		default:
			lenR := utf8.RuneLen(s)
			if lenR < 0 {
				// if utf8 cannot convert, return the same string as is
				return pathName
			}
			u := make([]byte, lenR)
			utf8.EncodeRune(u, s)
			for _, r := range u {
				hex := hex.EncodeToString([]byte{r})
				encodedPathname.WriteString("%" + strings.ToUpper(hex))
			}
		}
	}
	return encodedPathname.String()
}

type StdCache struct {
	host   string
	client *http.Client
}

func NewStdCache(host string) Client {
	return &StdCache{host: host, client: &http.Client{
		// For tracing
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}}
}

func responseError(isPut bool, path string, resp *http.Response, err error) error {
	e := ErrFailedRequest{isPut: isPut, path: path}
	if err != nil {
		e.body = err.Error()
		return e
	}
	if resp.StatusCode == http.StatusNotFound {
		if resp.Body != nil {
			resp.Body.Close()
		}
		return ErrNotFound{path}
	}
	if resp.StatusCode != http.StatusOK {
		if resp.Body != nil {
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, resp.Body)
			resp.Body.Close()
			e.body = buf.String()
			fmt.Println(buf.String())
		}
		fmt.Println("REQUEST ERROR", e)
		return e
	}
	return nil
}

func (cs *StdCache) Get(ctx context.Context, path string) (body io.ReadCloser, err error) {
	req, err := http.NewRequest(http.MethodGet, url2.Join(cs.host, path), nil)
	if err != nil {
		return nil, errFetching(path, err.Error())
	}
	resp, err := cs.client.Do(req)
	if err := responseError(false, path, resp, err); err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (cs *StdCache) Put(ctx context.Context, path string) (writer io.WriteCloser, err error) {
	errChan := make(chan error)
	pr, pw := io.Pipe()
	go func() {
		resp, err := http.Post(url2.Join(cs.host, path), "", pr)
		if err != nil {
			_ = pr.CloseWithError(err)
		}
		errChan <- responseError(true, path, resp, err)
	}()
	return io2.WriterCloseFunc(pw, func() error {
		pw.Close()
		return <-errChan
	}), nil
}
