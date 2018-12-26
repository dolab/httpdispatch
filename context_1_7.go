// +build go1.7

package httpdispatch

import (
	"context"
	"net/http"
)

// ContextParams pulls the URL parameters from a request context,
// or returns nil if none are present.
//
// This is only present from go 1.7.
func ContextParams(r *http.Request) Params {
	params, _ := r.Context().Value(ctxParamKey).(Params)

	return params
}

// Handle hijacks http.Handler with request params
func (ch *ContextHandle) Handle(w http.ResponseWriter, r *http.Request, ps Params) {
	if ch.useCtx && ps != nil {
		ctx := context.WithValue(r.Context(), ctxParamKey, ps)

		r = r.WithContext(ctx)
	}

	ch.handler.ServeHTTP(w, r)
}
