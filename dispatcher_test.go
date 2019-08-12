// +build go1.7

package httpdispatch

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/golib/assert"
)

type mockResponseWriter struct{}

func (m *mockResponseWriter) Header() (h http.Header) {
	return http.Header{}
}

func (m *mockResponseWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (m *mockResponseWriter) WriteString(s string) (n int, err error) {
	return len(s), nil
}

func (m *mockResponseWriter) WriteHeader(int) {}

type fakeDispatcherHandler struct {
	handeled bool
}

func (h fakeDispatcherHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handeled = true
}

func TestDispatcher(t *testing.T) {
	routed := false
	want := Params{Param{"name", "gopher"}}

	dispatcher := New()
	dispatcher.RequestContext = true

	dispatcher.HandlerFunc("GET", "/user/:name", func(w http.ResponseWriter, r *http.Request) {
		routed = true

		params := ContextParams(r)
		if !reflect.DeepEqual(params, want) {
			t.Fatalf("wrong wildcard values: want %v, got %v", want, params)
		}
	})

	r, _ := http.NewRequest("GET", "/user/gopher", nil)
	w := httptest.NewRecorder()
	dispatcher.ServeHTTP(w, r)

	if !routed {
		t.Fatal("routing failed")
	}
}

func TestDispatcherWithContext(t *testing.T) {
	var (
		routedForNamed = false
		wantedForNamed = Params{Param{"name", "gopher"}}

		routedForWildcard = false
		wantedForWildcard = Params{Param{"filename", "path/to/gopher"}}
	)

	dispatcher := New()
	dispatcher.RequestContext = true

	dispatcher.HandlerFunc("GET", "/user/:name", func(w http.ResponseWriter, r *http.Request) {
		routedForNamed = true

		params := ContextParams(r)
		if !reflect.DeepEqual(params, wantedForNamed) {
			t.Fatalf("wrong named params: want %v, got %v", wantedForNamed, params)
		}
	})
	dispatcher.HandlerFunc("GET", "/static/*filename", func(w http.ResponseWriter, r *http.Request) {
		routedForWildcard = true

		params := ContextParams(r)
		if !reflect.DeepEqual(params, wantedForWildcard) {
			t.Fatalf("wrong wildcard params: want %v, got %v", wantedForWildcard, params)
		}
	})

	r, _ := http.NewRequest("GET", "/user/gopher", nil)
	w := httptest.NewRecorder()
	dispatcher.ServeHTTP(w, r)

	if !routedForNamed {
		t.Fatal("routing failed")
	}

	r, _ = http.NewRequest("GET", "/static/path/to/gopher", nil)
	w = httptest.NewRecorder()
	dispatcher.ServeHTTP(w, r)

	if !routedForWildcard {
		t.Fatal("routing failed")
	}
}

func TestDispatcherAPI(t *testing.T) {
	dispatcher := New()

	testCases := []struct {
		method  string
		path    string
		handler http.Handler
		handled bool
	}{
		{
			http.MethodOptions,
			"/options",
			fakeDispatcherHandler{},
			true,
		},
		{
			http.MethodGet,
			"/get",
			fakeDispatcherHandler{},
			true,
		},
		{
			http.MethodPost,
			"/post",
			fakeDispatcherHandler{},
			true,
		},
		{
			http.MethodPut,
			"/put",
			fakeDispatcherHandler{},
			true,
		},
		{
			http.MethodPatch,
			"/patch",
			fakeDispatcherHandler{},
			true,
		},
		{
			http.MethodHead,
			"/head",
			fakeDispatcherHandler{},
			true,
		},
		{
			http.MethodDelete,
			"/delete",
			fakeDispatcherHandler{},
			true,
		},
	}
	for _, testCase := range testCases {
		dispatcher.Handler(testCase.method, testCase.path, testCase.handler)

		r, _ := http.NewRequest(testCase.method, testCase.path, nil)
		w := new(mockResponseWriter)

		dispatcher.ServeHTTP(w, r)
		if !testCase.handled {
			t.Fatalf("Failed to route %s %s", testCase.method, testCase.path)
		}
	}

	handlerFunc := false
	dispatcher.HandlerFunc("GET", "/handlerfunc", func(w http.ResponseWriter, r *http.Request) {
		handlerFunc = true
	})

	r, _ := http.NewRequest("GET", "/handlerfunc", nil)
	w := new(mockResponseWriter)

	dispatcher.ServeHTTP(w, r)
	if !handlerFunc {
		t.Error("Failed to route GET /handlerfunc")
	}
}

func TestDispatcherRoot(t *testing.T) {
	dispatcher := New()
	recv := catchPanic(func() {
		dispatcher.GET("noSlashRoot", nil)
	})
	if recv == nil {
		t.Fatal("registering path not beginning with '/' did not panic")
	}
}

func TestDispatcherChaining(t *testing.T) {
	dispatcher1 := New()
	dispatcher2 := New()
	dispatcher1.NotFound = dispatcher2

	fooHit := false
	dispatcher1.HandlerFunc(http.MethodPost, "/foo", func(w http.ResponseWriter, req *http.Request) {
		fooHit = true

		w.WriteHeader(http.StatusOK)
	})

	barHit := false
	dispatcher2.HandlerFunc(http.MethodPost, "/bar", func(w http.ResponseWriter, req *http.Request) {
		barHit = true

		w.WriteHeader(http.StatusOK)
	})

	r, _ := http.NewRequest(http.MethodPost, "/foo", nil)
	w := httptest.NewRecorder()
	dispatcher1.ServeHTTP(w, r)
	if !(w.Code == http.StatusOK && fooHit) {
		t.Errorf("Regular routing failed with dispatcher chaining.")
		t.FailNow()
	}

	r, _ = http.NewRequest(http.MethodPost, "/bar", nil)
	w = httptest.NewRecorder()
	dispatcher1.ServeHTTP(w, r)
	if !(w.Code == http.StatusOK && barHit) {
		t.Errorf("Chained routing failed with dispatcher chaining.")
		t.FailNow()
	}

	r, _ = http.NewRequest(http.MethodPost, "/qax", nil)
	w = httptest.NewRecorder()
	dispatcher1.ServeHTTP(w, r)
	if !(w.Code == http.StatusNotFound) {
		t.Errorf("NotFound behavior failed with dispatcher chaining.")
		t.FailNow()
	}
}

func TestDispatcherOPTIONS(t *testing.T) {
	handlerFunc := func(_ http.ResponseWriter, _ *http.Request) {}

	dispatcher := New()
	dispatcher.HandlerFunc(http.MethodPost, "/path", handlerFunc)

	// test not allowed
	// * (server)
	r, _ := http.NewRequest("OPTIONS", "*", nil)
	w := httptest.NewRecorder()
	dispatcher.ServeHTTP(w, r)
	if !(w.Code == http.StatusOK) {
		t.Errorf("OPTIONS handling failed: Code=%d, Header=%v", w.Code, w.Header())
	} else if allow := w.Header().Get("Allow"); allow != "POST, OPTIONS" {
		t.Errorf("wrong Allow header value, expected: %s, got: %s", "POST, OPTIONS", allow)
	}

	// path
	r, _ = http.NewRequest("OPTIONS", "/path", nil)
	w = httptest.NewRecorder()
	dispatcher.ServeHTTP(w, r)
	if !(w.Code == http.StatusOK) {
		t.Errorf("OPTIONS handling failed: Code=%d, Header=%v", w.Code, w.Header())
	} else if allow := w.Header().Get("Allow"); allow != "POST, OPTIONS" {
		t.Errorf("wrong Allow header value, expected: %s, got: %s", "POST, OPTIONS", allow)
	}

	// not found
	r, _ = http.NewRequest("OPTIONS", "/doesnotexist", nil)
	w = httptest.NewRecorder()
	dispatcher.ServeHTTP(w, r)
	if !(w.Code == http.StatusNotFound) {
		t.Errorf("OPTIONS handling failed: Code=%d, Header=%v", w.Code, w.Header())
	}

	// register another method
	dispatcher.HandlerFunc(http.MethodGet, "/path", handlerFunc)

	// test again
	// * (server)
	r, _ = http.NewRequest("OPTIONS", "*", nil)
	w = httptest.NewRecorder()
	dispatcher.ServeHTTP(w, r)
	if !(w.Code == http.StatusOK) {
		t.Errorf("OPTIONS handling failed: Code=%d, Header=%v", w.Code, w.Header())
	} else if allow := w.Header().Get("Allow"); allow != "POST, GET, OPTIONS" && allow != "GET, POST, OPTIONS" {
		t.Errorf("wrong Allow header value, expected: %s, got: %s", "GET, POST, OPTIONS", allow)
	}

	// path
	r, _ = http.NewRequest("OPTIONS", "/path", nil)
	w = httptest.NewRecorder()
	dispatcher.ServeHTTP(w, r)
	if !(w.Code == http.StatusOK) {
		t.Errorf("OPTIONS handling failed: Code=%d, Header=%v", w.Code, w.Header())
	} else if allow := w.Header().Get("Allow"); allow != "POST, GET, OPTIONS" && allow != "GET, POST, OPTIONS" {
		t.Errorf("wrong Allow header value, expected: %s, got: %s", "GET, POST, OPTIONS", allow)
	}

	// not found
	r, _ = http.NewRequest("OPTIONS", "/doesnotexist", nil)
	w = httptest.NewRecorder()
	dispatcher.ServeHTTP(w, r)
	if !(w.Code == http.StatusNotFound) {
		t.Errorf("OPTIONS handling failed: Code=%d, Header=%v", w.Code, w.Header())
	}

	// custom handler
	var custom bool
	dispatcher.HandlerFunc(http.MethodOptions, "/path", func(w http.ResponseWriter, r *http.Request) {
		custom = true
	})

	// test again
	// * (server)
	r, _ = http.NewRequest("OPTIONS", "*", nil)
	w = httptest.NewRecorder()
	dispatcher.ServeHTTP(w, r)
	if !(w.Code == http.StatusOK) {
		t.Errorf("OPTIONS handling failed: Code=%d, Header=%v", w.Code, w.Header())
	} else if allow := w.Header().Get("Allow"); allow != "POST, GET, OPTIONS" && allow != "GET, POST, OPTIONS" {
		t.Errorf("wrong Allow header value, expected: %s, got: %s", "GET, POST, OPTIONS", allow)
	}
	if custom {
		t.Error("custom handler called on *")
	}

	// path
	r, _ = http.NewRequest("OPTIONS", "/path", nil)
	w = httptest.NewRecorder()
	dispatcher.ServeHTTP(w, r)
	if !(w.Code == http.StatusOK) {
		t.Errorf("OPTIONS handling failed: Code=%d, Header=%v", w.Code, w.Header())
	}
	if !custom {
		t.Error("custom handler not called")
	}

	// not found
	custom = false

	r, _ = http.NewRequest("OPTIONS", "/doesnotexist", nil)
	w = httptest.NewRecorder()
	dispatcher.ServeHTTP(w, r)
	if !(w.Code == http.StatusNotFound) {
		t.Errorf("OPTIONS handling failed: Code=%d, Header=%v", w.Code, w.Header())
	}
	if custom {
		t.Error("custom handler called on *")
	}
}

func TestDispatcherNotAllowed(t *testing.T) {
	handlerFunc := func(_ http.ResponseWriter, _ *http.Request) {}

	dispatcher := New()
	dispatcher.HandlerFunc(http.MethodPost, "/path", handlerFunc)

	// test not allowed
	r, _ := http.NewRequest("GET", "/path", nil)
	w := httptest.NewRecorder()
	dispatcher.ServeHTTP(w, r)
	if !(w.Code == http.StatusMethodNotAllowed) {
		t.Errorf("NotAllowed handling failed: Code=%d, Header=%v", w.Code, w.Header())
	}

	// register another method
	dispatcher.HandlerFunc(http.MethodDelete, "/path", handlerFunc)
	dispatcher.HandlerFunc(http.MethodOptions, "/path", handlerFunc) // must be ignored

	// test again
	r, _ = http.NewRequest("GET", "/path", nil)
	w = httptest.NewRecorder()
	dispatcher.ServeHTTP(w, r)
	if !(w.Code == http.StatusMethodNotAllowed) {
		t.Errorf("NotAllowed handling failed: Code=%d, Header=%v", w.Code, w.Header())
	}

	// test custom handler
	w = httptest.NewRecorder()
	responseText := "custom method"
	dispatcher.MethodNotAllowed = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte(responseText))
	})
	dispatcher.ServeHTTP(w, r)
	if got := w.Body.String(); !(got == responseText) {
		t.Errorf("unexpected response, want: %s, got: %s", responseText, got)
	}
	if w.Code != http.StatusTeapot {
		t.Errorf("unexpected response code, want: %d, got: %d", http.StatusTeapot, w.Code)
	}
	if allow := w.Header().Get("Allow"); allow != "POST, DELETE, OPTIONS" && allow != "DELETE, POST, OPTIONS" {
		t.Errorf("unexpected Allow header value, want: %s, got: %s", "POST, DELETE, OPTIONS", allow)
	}
}

func TestDispatcherNotFound(t *testing.T) {
	handlerFunc := func(_ http.ResponseWriter, _ *http.Request) {}

	dispatcher := New()
	dispatcher.HandlerFunc(http.MethodGet, "/", handlerFunc)
	dispatcher.HandlerFunc(http.MethodGet, "/path", handlerFunc)
	dispatcher.HandlerFunc(http.MethodGet, "/dir/", handlerFunc)

	testCases := []struct {
		route  string
		code   int
		header map[string]string
	}{
		{"/path/", 301, map[string]string{
			"Location":     "/path",
			"Content-Type": "text/html; charset=utf-8",
		}}, // TSR -/
		{"/dir", 301, map[string]string{
			"Location":     "/dir/",
			"Content-Type": "text/html; charset=utf-8",
		}}, // TSR +/
		{"", 301, map[string]string{
			"Location":     "/",
			"Content-Type": "text/html; charset=utf-8",
		}}, // TSR +/
		{"/PATH", 301, map[string]string{
			"Location":     "/path",
			"Content-Type": "text/html; charset=utf-8",
		}}, // Fixed Case
		{"/DIR/", 301, map[string]string{
			"Location":     "/dir/",
			"Content-Type": "text/html; charset=utf-8",
		}}, // Fixed Case
		{"/PATH/", 301, map[string]string{
			"Location":     "/path",
			"Content-Type": "text/html; charset=utf-8",
		}}, // Fixed Case -/
		{"/DIR", 301, map[string]string{
			"Location":     "/dir/",
			"Content-Type": "text/html; charset=utf-8",
		}}, // Fixed Case +/
		{"/../path", 301, map[string]string{
			"Location":     "/path",
			"Content-Type": "text/html; charset=utf-8",
		}}, // Normalize
		{"/nope", 404, map[string]string{}}, // NotFound
	}
	for _, testCase := range testCases {
		r, _ := http.NewRequest("GET", testCase.route, nil)
		w := httptest.NewRecorder()
		dispatcher.ServeHTTP(w, r)
		if !(w.Code == testCase.code && w.Header().Get("Location") == testCase.header["Location"]) {
			t.Errorf("NotFound handling route %s failed: Code=%d, Header=%v", testCase.route, w.Code, w.Header())
		}
	}

	// Test custom not found handler
	var notFound bool
	dispatcher.NotFound = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusNotFound)
		notFound = true
	})
	r, _ := http.NewRequest(http.MethodGet, "/nope", nil)
	w := httptest.NewRecorder()
	dispatcher.ServeHTTP(w, r)
	if !(w.Code == http.StatusNotFound && notFound == true) {
		t.Errorf("Custom NotFound handler failed: Code=%d, Header=%v", w.Code, w.Header())
	}

	// Test other method than GET (want 307 instead of 301)
	dispatcher.HandlerFunc(http.MethodPatch, "/path", handlerFunc)
	r, _ = http.NewRequest(http.MethodPatch, "/path/", nil)
	w = httptest.NewRecorder()
	dispatcher.ServeHTTP(w, r)
	if !(w.Code == 307 && fmt.Sprint(w.Header()) == "map[Location:[/path]]") {
		t.Errorf("Custom NotFound handler failed: Code=%d, Header=%v", w.Code, w.Header())
	}

	// Test special case where no node for the prefix "/" exists
	dispatcher = New()
	dispatcher.HandlerFunc(http.MethodGet, "/a", handlerFunc)
	r, _ = http.NewRequest(http.MethodGet, "/", nil)
	w = httptest.NewRecorder()
	dispatcher.ServeHTTP(w, r)
	if !(w.Code == 404) {
		t.Errorf("NotFound handling route / failed: Code=%d", w.Code)
	}
}

func TestDispatcherPanicHandler(t *testing.T) {
	defer func() {
		if rcv := recover(); rcv != nil {
			t.Fatal("handling panic failed")
		}
	}()

	dispatcher := New()

	panicHandled := false
	dispatcher.PanicHandler = func(rw http.ResponseWriter, r *http.Request, p interface{}) {
		panicHandled = true
	}

	dispatcher.HandlerFunc("PUT", "/user/:name", func(_ http.ResponseWriter, _ *http.Request) {
		panic("oops!")
	})

	r, _ := http.NewRequest("PUT", "/user/gopher", nil)
	w := new(mockResponseWriter)

	dispatcher.ServeHTTP(w, r)

	if !panicHandled {
		t.Fatal("simulating failed")
	}
}

func TestDispatcherLookup(t *testing.T) {
	routed := false
	wantHandle := func(_ http.ResponseWriter, _ *http.Request) {
		routed = true
	}
	wantParams := Params{Param{"name", "gopher"}}

	dispatcher := New()

	// try empty dispatcher first
	handler, _, tsr := dispatcher.Lookup("GET", "/nope")
	if handler != nil {
		t.Fatalf("Got handle for unregistered pattern: %v", handler)
	}
	if tsr {
		t.Error("Got wrong TSR recommendation!")
	}

	// insert route and try again
	dispatcher.HandlerFunc(http.MethodGet, "/user/:name", wantHandle)

	handler, params, tsr := dispatcher.Lookup(http.MethodGet, "/user/gopher")
	if handler == nil {
		t.Fatal("Got no handle!")
	} else {
		req, _ := http.NewRequest(http.MethodGet, "/user/gopher", nil)

		handler.Handle(nil, req, nil)
		if !routed {
			t.Fatal("Routing failed!")
		}
	}

	if !reflect.DeepEqual(params, wantParams) {
		t.Fatalf("Wrong parameter values: want %v, got %v", wantParams, params)
	}

	handler, _, tsr = dispatcher.Lookup("GET", "/user/gopher/")
	if handler != nil && !tsr {
		t.Fatalf("Got handle for unregistered pattern: %v", handler)
	}
	if !tsr {
		t.Error("Got no TSR recommendation!")
	}

	handler, _, tsr = dispatcher.Lookup("GET", "/nope")
	if handler != nil {
		t.Fatalf("Got handle for unregistered pattern: %v", handler)
	}
	if tsr {
		t.Error("Got wrong TSR recommendation!")
	}
}

type mockFileSystem struct {
	opened bool
}

func (mfs *mockFileSystem) Open(name string) (http.File, error) {
	mfs.opened = true
	return nil, errors.New("this is just a mock")
}

func TestDispatcherServeFiles(t *testing.T) {
	mfs := &mockFileSystem{}
	dispatcher := New()

	// t.Run("panic without /*filepath", func(t *testing.T) {
	assert.Panics(t, func() {
		dispatcher.ServeFiles("/:filepath", mfs)
	}, "registering path not ending with '*filepath' did not panic")
	// })

	// t.Run("should work", func(t *testing.T) {
	dispatcher.ServeFiles("/*filepath", mfs)

	w := new(mockResponseWriter)
	r, _ := http.NewRequest("GET", "/favicon.ico", nil)
	dispatcher.ServeHTTP(w, r)

	if !mfs.opened {
		t.Error("serving file failed")
	}
	// })
}
