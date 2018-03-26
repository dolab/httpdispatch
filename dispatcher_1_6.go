// +build !go1.7

package httpdispatch

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"net/http"
)

var (
	ctxParamHeaderKey = fmt.Sprintf("X-Params-%p", &ctxParamKey) // for go <1.7
)

// ContextParams pulls the URL parameters from a request context,
// or returns nil if none are present.
//
// This is only present for go <1.7.
func ContextParams(r *http.Request) Params {
	value := r.Header.Get(ctxParamHeaderKey)
	if value == "" {
		return nil
	}

	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil
	}

	var params Params
	if err := gob.NewDecoder(bytes.NewBuffer(data)).Decode(&params); err != nil {
		return nil
	}

	return params
}

// Handler is an adapter which allows the usage of a http.Handler as a
// request handle.
func (r *Dispatcher) Handler(method, path string, handler http.Handler) {
	r.Handle(method, path,
		func(w http.ResponseWriter, req *http.Request, ps Params) {
			buf := bytes.NewBuffer(nil)
			if err := gob.NewEncoder(buf).Encode(ps); err == nil {
				req.Header.Add(ctxParamHeaderKey, base64.RawURLEncoding.EncodeToString(buf.Bytes()))
			}

			handler.ServeHTTP(w, req)
		},
	)
}
