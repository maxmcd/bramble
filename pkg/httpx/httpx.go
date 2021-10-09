package httpx

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

type Context struct {
	ResponseWriter http.ResponseWriter
	Request        *http.Request
	Params         httprouter.Params
}

type Router struct {
	*httprouter.Router
	errHandler func(http.ResponseWriter, string, int)
}

type ErrHTTPResponse struct {
	err  error
	code int
}

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
