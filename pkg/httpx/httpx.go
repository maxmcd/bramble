package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

type Context struct {
	ResponseWriter http.ResponseWriter
	Request        *http.Request
	Params         httprouter.Params
}

func (c Context) JSON(v interface{}) error {
	c.ResponseWriter.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(c.ResponseWriter).Encode(v)
}

type Router struct {
	*httprouter.Router
	errHandler func(http.ResponseWriter, string, int)
}

type ErrHTTPResponse struct {
	err  error
	code int
}

type IM map[interface{}]interface{}

func (err ErrHTTPResponse) Error() string { return err.err.Error() }
func ErrNotFound(err error) error         { return ErrHTTPResponse{err: err, code: http.StatusNotFound} }
func ErrUnprocessableEntity(err error) error {
	return ErrHTTPResponse{err: err, code: http.StatusUnprocessableEntity}
}

func (r Router) h(handler func(c Context) (err error)) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	return func(rw http.ResponseWriter, req *http.Request, p httprouter.Params) {
		err := handler(Context{ResponseWriter: rw, Request: req, Params: p})
		if err != nil {
			code := http.StatusInternalServerError
			if v, ok := err.(ErrHTTPResponse); ok {
				code = v.code
			}
			if r.errHandler != nil {
				r.errHandler(rw, err.Error(), code)
				return
			}
			http.Error(rw, err.Error(), code)
		}
	}
}

func (r Router) ErrHandler(handle func(http.ResponseWriter, string, int)) {
	r.errHandler = handle
	// Support multiple at different path prefixes?
}

func (r Router) GET(path string, handle func(Context) error)  { r.Router.GET(path, r.h(handle)) }
func (r Router) POST(path string, handle func(Context) error) { r.Router.POST(path, r.h(handle)) }
func (r Router) HEAD(path string, handle func(Context) error) { r.Router.HEAD(path, r.h(handle)) }

func New() Router {
	return Router{
		Router: httprouter.New(),
	}
}

// Request fn with mucho magic
func Request(ctx context.Context, client *http.Client, method, url, contentType string, body io.Reader, resp interface{}) (err error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}
	// TODO: Move elsewhere?
	req = req.WithContext(ctx)
	if method == http.MethodPost {
		req.Header.Add("Content-Type", contentType)
	}
	httpResp, err := client.Do(req)
	if err != nil {
		return err
	}
	if httpResp.StatusCode == http.StatusNotFound {
		httpResp.Body.Close()
		return os.ErrNotExist
	}
	if httpResp.StatusCode != http.StatusOK {
		defer httpResp.Body.Close()
		var buf bytes.Buffer
		if _, err = io.Copy(&buf, httpResp.Body); err != nil {
			return errors.Wrap(err, "error reading body")
		}
		return errors.Errorf("Unexpected response code %d: %s",
			httpResp.StatusCode, buf.String())
	}
	if resp == nil {
		httpResp.Body.Close()
		return nil
	}
	if v, ok := resp.(*io.ReadCloser); ok {
		*v = httpResp.Body
		return nil
	}
	defer httpResp.Body.Close()
	{
		var buf bytes.Buffer
		if _, err = io.Copy(&buf, httpResp.Body); err != nil {
			return errors.Wrap(err, "error reading body")
		}
		switch v := resp.(type) {
		case *string:
			*v = buf.String()
		case io.Writer:
			_, err = io.Copy(v, &buf)
		default:
			err = json.Unmarshal(buf.Bytes(), resp)
		}
	}
	return err
}
