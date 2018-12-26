// +build go1.7

package httpdispatch

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golib/assert"
)

var (
	fakeContextHandler *_fakeContextHandler
)

type _fakeContextHandler struct{}

func (_ *_fakeContextHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ps := ContextParams(r)
	if len(ps) != 1 {
		return
	}

	w.Write([]byte(ps[0].Key + "=" + ps[0].Value))
}

func Test_ContextHandle(t *testing.T) {
	it := assert.New(t)

	ch := NewContextHandle(fakeContextHandler, true)
	it.Implements((*Handler)(nil), ch)
	it.NotNil(ch.Handle)

	r, _ := http.NewRequest(http.MethodGet, "", nil)
	w := httptest.NewRecorder()
	ps := Params{
		Param{
			Key:   "key",
			Value: "value",
		},
	}

	ch.Handle(w, r, ps)

	it.Equal("key=value", w.Body.String())
}

func Test_FileHandle(t *testing.T) {
	it := assert.New(t)
	fs := http.Dir("./")

	ch := NewFileHandle(fs)
	it.Implements((*Handler)(nil), ch)
	it.NotNil(ch.Handle)

	r, _ := http.NewRequest(http.MethodGet, "", nil)
	w := httptest.NewRecorder()
	ps := Params{}

	ch.Handle(w, r, ps)

	it.Contains(w.Body.String(), `<a href="LICENSE">LICENSE</a>`)
}
