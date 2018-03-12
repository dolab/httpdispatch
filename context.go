package httpdispatch

import "fmt"

var (
	ctxParam    = ctxKey{}                              // for http.Request.Context() introduced from go 1.7
	ctxParamKey = fmt.Sprintf("X-Params-%p", &ctxParam) // for go <1.7
)

type ctxKey struct{}
