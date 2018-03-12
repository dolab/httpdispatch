// +build go1.7

package httpdispatch

import (
	"context"
	"net/http"
)

// Handler is an adapter which allows the usage of a http.Handler as a
// request handle.
func (r *Dispatcher) Handler(method, path string, handler http.Handler) {
	r.Handle(method, path,
		func(w http.ResponseWriter, req *http.Request, ps Params) {
			ctx := context.WithValue(req.Context(), ctxParam, ps)

			handler.ServeHTTP(w, req.WithContext(ctx))
		},
	)
}

// ContextParams pulls the URL parameters from a request context,
// or returns nil if none are present.
//
// This is only present from go 1.7.
func ContextParams(r *http.Request) Params {
	params, _ := r.Context().Value(ctxParam).(Params)

	return params
}
