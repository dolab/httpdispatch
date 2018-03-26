package httpdispatch

var (
	ctxParamKey = ctxParam{} // for http.Request.Context() introduced from go 1.7
)

type ctxParam struct{}
