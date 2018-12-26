package httpdispatch

import (
	"net/http"
)

var (
	ctxParamKey = ctxParam{} // for http.Request.Context() introduced from go 1.7
)

type ctxParam struct{}

// ContextHandle defines container of registered http.Handler with useful context,
// such as package name, controller name and action name of handle.
type ContextHandle struct {
	handler http.Handler
	useCtx  bool
}

// NewContextHandle returns *ContextHandle with handler info
func NewContextHandle(handler http.Handler, useContext bool) *ContextHandle {
	return &ContextHandle{
		handler: handler,
		useCtx:  useContext,
	}
}

// FileHandle defines static files server context
type FileHandle struct {
	*ContextHandle
}

// NewFileHandle returns *FileHandle with passed http.HandlerFunc
func NewFileHandle(fs http.FileSystem) *FileHandle {
	return &FileHandle{
		ContextHandle: NewContextHandle(http.FileServer(fs), false),
	}
}

// Handle hijacks request path with filepath by overwrite
func (fh *FileHandle) Handle(w http.ResponseWriter, r *http.Request, ps Params) {
	r.URL.Path = ps.ByName("filepath")

	fh.handler.ServeHTTP(w, r)
}
