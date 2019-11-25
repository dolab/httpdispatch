// Copyright 2018 Spring MC. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be found
// in the LICENSE file.

// Package httpdispatch is a trie based high performance HTTP request dispatcher.
//
// A trivial example is:
//
//  package main
//
//  import (
//      "fmt"
//      "net/http"
//      "log"
//
//      "github.com/dolab/httpdispatch"
//  )
//
//  func Index(w http.ResponseWriter, r *http.Request) {
//      fmt.Fprint(w, "Welcome!\n")
//  }
//
//  func Hello(w http.ResponseWriter, r *http.Request) {
//		params := httpdispatch.ContextParams(r)
//
//      fmt.Fprintf(w, "hello, %s!\n", params.ByName("name"))
//  }
//
//  func main() {
//      router := httpdispatch.New()
//      router.GET("/", http.HandlerFunc(Index))
//      router.GET("/hello/:name", http.HandlerFunc(Hello))
//
//      log.Fatal(http.ListenAndServe(":8080", router))
//  }
//
// The router matches incoming requests by the request method and the path.
// If a handler is registered for this path and method, the router delegates the
// request to that function.
// For the methods OPTIONS, GET, POST, PUT, PATCH and DELETE shortcut functions exist to
// register handlers, for all other methods *dispatcher.Handler can be used.
//
// The registered path, against which the router matches incoming requests, can
// contain two types of parameters:
//  Syntax    Type
//  :name     named parameter
//  *name     wildcard parameter
//
// Named parameters are dynamic path segments. They match anything until the
// next '/' or the path end:
//  Path: /blog/:category/:post
//
//  Requests:
//   /blog/go/request-routers            match: category="go", post="request-routers"
//   /blog/go/request-routers/           no match, but the router would redirect to /blog/go/request-routers
//   /blog/go/                           no match
//   /blog/go/request-routers/comments   no match
//
// Wildcard parameters match anything until the path end, including the
// directory index (the '/' before the wildcard). Since they match anything
// until the end, wildcard parameters must always be the final path element.
//  Path: /files/*filepath
//
//  Requests:
//   /files/                             match: filepath="/"
//   /files/LICENSE                      match: filepath="/LICENSE"
//   /files/templates/article.html       match: filepath="/templates/article.html"
//   /files                              no match, but the router would redirect
//
// The value of parameters is saved as a slice of the Param struct, consisting
// each of a key and a value. The slice is passed to the Handle func as a third
// parameter.
// There are two ways to retrieve the value of a parameter:
//  // by the name of the parameter
//  user := ps.ByName("user") // defined by :user or *user
//
//  // by the index of the parameter. This way you can also get the name (key)
//  thirdKey   := ps[2].Key   // the name of the 3rd parameter
//  thirdValue := ps[2].Value // the value of the 3rd parameter
package httpdispatch

import (
	"net/http"
	"sync"
)

// Handler is an interface that can be registered to a route to handle HTTP
// requests. Like http.HandlerFunc, but has a third parameter for the values of
// wildcards (variables).
type Handler interface {
	Handle(http.ResponseWriter, *http.Request, Params)
}

// Dispatcher is a http.Handler which can be used to dispatch requests to different
// handler functions via configurable routes
type Dispatcher struct {
	mux   sync.Mutex
	trees map[string]*node

	// If enabled, the router tries to inject parsed params within http.Request.
	RequestContext bool

	// Enables automatic redirection if the current route can't be matched but a
	// handler for the path with (without) the trailing slash exists.
	// For example if /foo/ is requested but a route only exists for /foo, the
	// client is redirected to /foo with http status code 301 for GET requests
	// and 307 for all other request methods.
	RedirectTrailingSlash bool

	// If enabled, the router tries to fix the current request path, if no
	// handle is registered for it.
	// First superfluous path elements like ../ or // are removed.
	// Afterwards the router does a case-insensitive lookup of the cleaned path.
	// If a handle can be found for this route, the router makes a redirection
	// to the corrected path with status code 301 for GET requests and 307 for
	// all other request methods.
	// For example /FOO and /..//Foo could be redirected to /foo.
	// RedirectTrailingSlash is independent of this option.
	RedirectFixedPath bool

	// If enabled, the router checks if another method is allowed for the
	// current route, if the current request can not be routed.
	// If this is the case, the request is answered with 'Method Not Allowed'
	// and HTTP status code 405.
	// If no other Method is allowed, the request is delegated to the NotFound
	// handler.
	HandleMethodNotAllowed bool

	// If enabled, the router automatically replies to OPTIONS requests.
	// Custom OPTIONS handlers take priority over automatic replies.
	HandleMethodOPTIONS bool

	// Configurable http.Handler which is called when no matching route is
	// found. If it is not set, http.NotFound is used.
	NotFound http.Handler

	// Configurable http.Handler which is called when a request
	// cannot be routed and HandleMethodNotAllowed is true.
	// If it is not set, http.Error with http.StatusMethodNotAllowed is used.
	// The "Allow" header with allowed request methods is set before the handler
	// is called.
	MethodNotAllowed http.Handler

	// Configurable http.Handler which is called for OPTIONS request.
	// The "Allow" header with allowed request methods is set before the handler
	// is called.
	MethodOptions http.Handler

	// Function to handle panics recovered from http handlers.
	// It should be used to generate an error page and return the http error code
	// 500 (Internal Server Error).
	// The handler can be used to keep your server from crashing because of
	// unrecovered panics.
	PanicHandler func(http.ResponseWriter, *http.Request, interface{})
}

// Make sure the Dispatcher conforms with the http.Handler interface
var _ http.Handler = New()

// New returns a new initialized Dispatcher.
// Path auto-correction, including trailing slashes, is enabled by default.
func New() *Dispatcher {
	return &Dispatcher{
		RedirectTrailingSlash:  true,
		RedirectFixedPath:      true,
		HandleMethodNotAllowed: true,
		HandleMethodOPTIONS:    true,
	}
}

// OPTIONS is a shortcut for dispatcher.Handler("GET", path, http.Handler)
func (dp *Dispatcher) OPTIONS(uripath string, handler http.Handler) {
	dp.Handler(http.MethodOptions, uripath, handler)
}

// GET is a shortcut for dispatcher.Handler("GET", path, http.Handler)
func (dp *Dispatcher) GET(uripath string, handler http.Handler) {
	dp.Handler(http.MethodGet, uripath, handler)
}

// HEAD is a shortcut for dispatcher.Handler("GET", path, http.Handler)
func (dp *Dispatcher) HEAD(uripath string, handler http.Handler) {
	dp.Handler(http.MethodHead, uripath, handler)
}

// POST is a shortcut for dispatcher.Handler("GET", path, http.Handler)
func (dp *Dispatcher) POST(uripath string, handler http.Handler) {
	dp.Handler(http.MethodPost, uripath, handler)
}

// PUT is a shortcut for dispatcher.Handler("GET", path, http.Handler)
func (dp *Dispatcher) PUT(uripath string, handler http.Handler) {
	dp.Handler(http.MethodPut, uripath, handler)
}

// PATCH is a shortcut for dispatcher.Handler("GET", path, http.Handler)
func (dp *Dispatcher) PATCH(uripath string, handler http.Handler) {
	dp.Handler(http.MethodPatch, uripath, handler)
}

// DELETE is a shortcut for dispatcher.Handler("GET", path, http.Handler)
func (dp *Dispatcher) DELETE(uripath string, handler http.Handler) {
	dp.Handler(http.MethodDelete, uripath, handler)
}

// HandlerFunc is an adapter which allows the usage of an http.HandlerFunc as a
// request handler.
func (dp *Dispatcher) HandlerFunc(method, uripath string, handler http.HandlerFunc) {
	dp.Handler(method, uripath, handler)
}

// Handler is an adapter which allows the usage of a http.Handler as a
// request handle.
func (dp *Dispatcher) Handler(method, uripath string, handler http.Handler) {
	dp.Handle(method, uripath, NewContextHandle(handler, dp.RequestContext))
}

// ServeFiles serves files from the given file system root.
// The path must end with "/*filepath", files are then served from the local
// path /path/to/*filepath.
// For example if root is "/etc" and *filepath is "passwd", the local file
// "/etc/passwd" would be served.
// Internally a http.FileServer is used, therefore http.NotFound is used instead
// of the Dispatcher's NotFound handler.
// To use the operating system's file system implementation,
// use http.Dir:
//     router.ServeFiles("/static/*filepath", http.Dir("/var/www"))
func (dp *Dispatcher) ServeFiles(filename string, fs http.FileSystem) {
	if len(filename) < 10 || filename[len(filename)-10:] != "/*filepath" {
		panic(`static files server filename must end with /*filepath in "` + filename + `"`)
	}

	dp.Handle(http.MethodGet, filename, NewFileHandle(fs))
}

// Lookup allows the manual lookup of a method + path combo.
// This is e.g. useful to build a framework around the dispatcher.
// If the path was found, it returns the handler func and the captured parameter
// values.
//
// NOTE: It returns handler when the third returned value indicates a redirection to
// the same path with / without the trailing slash should be performed.
func (dp *Dispatcher) Lookup(method, uripath string) (Handler, Params, bool) {
	if root := dp.trees[method]; root != nil {
		return root.resolve(uripath)
	}

	return nil, nil, false
}

// ServeHTTP makes the router implement the http.Handler interface.
func (dp *Dispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if dp.PanicHandler != nil {
		defer dp.recovery(w, r)
	}

	uripath := r.URL.Path

	if root := dp.trees[r.Method]; root != nil {
		handler, params, tsr := root.resolve(uripath)

		// find an available handler
		if handler != nil {
			if !tsr || !dp.RedirectTrailingSlash {
				handler.Handle(w, r, params)
				return
			}

			// Permanent redirect, request with GET method
			code := http.StatusMovedPermanently
			if r.Method != http.MethodGet {
				// Temporary redirect, request with same method
				// As of Go 1.3, Go does not support status code 308.
				code = http.StatusTemporaryRedirect
			}

			if len(uripath) > 1 && uripath[len(uripath)-1] == '/' {
				r.URL.Path = uripath[:len(uripath)-1]
			} else {
				r.URL.Path = uripath + "/"
			}

			// redirect trailing slash pattern
			http.Redirect(w, r, r.URL.String(), code)
			return
		}

		// Try to fix the request uripath
		if dp.RedirectFixedPath && r.Method != http.MethodConnect && uripath != "/" {
			fixedPath, found := root.findCaseInsensitivePath(
				Normalize(uripath),
				dp.RedirectTrailingSlash,
			)
			if found {
				// Permanent redirect, request with GET method
				code := http.StatusMovedPermanently
				if r.Method != http.MethodGet {
					// Temporary redirect, request with same method
					// As of Go 1.3, Go does not support status code 308.
					code = http.StatusTemporaryRedirect
				}

				r.URL.Path = string(fixedPath)

				http.Redirect(w, r, r.URL.String(), code)
				return
			}
		}
	}

	if r.Method == http.MethodOptions {
		// Handle OPTIONS
		if dp.HandleMethodOPTIONS {
			allow := dp.allowed(uripath, r.Method)
			if len(allow) > 0 {
				w.Header().Set("Allow", allow)

				if dp.MethodOptions != nil {
					dp.MethodOptions.ServeHTTP(w, r)
				}

				return
			}
		}
	} else {
		// Handle 405
		if dp.HandleMethodNotAllowed {
			allow := dp.allowed(uripath, r.Method)
			if len(allow) > 0 {
				w.Header().Set("Allow", allow)

				if dp.MethodNotAllowed != nil {
					dp.MethodNotAllowed.ServeHTTP(w, r)
				} else {
					http.Error(w,
						http.StatusText(http.StatusMethodNotAllowed),
						http.StatusMethodNotAllowed,
					)
				}

				return
			}
		}
	}

	// Handle 404
	dp.notfound(w, r)
}

// Handle registers a new request handler with the given path and method.
// This is e.g. useful to build a framework around the dispatcher.
//
// For OPTIONS, GET, POST, PUT, PATCH and DELETE requests the respective shortcut
// funcs can be used.
//
// This func is intended for bulk loading and to allow the usage of less
// frequently used, non-standardized or custom methods (e.g. for internal
// communication with a proxy).
func (dp *Dispatcher) Handle(method, uripath string, handler Handler) {
	if uripath[0] != '/' {
		panic("path must begin with '/' in '" + uripath + "'")
	}

	dp.mux.Lock()
	defer dp.mux.Unlock()

	if dp.trees == nil {
		dp.trees = make(map[string]*node)
	}

	root := dp.trees[method]
	if root == nil {
		root = new(node)

		dp.trees[method] = root
	}

	root.register(uripath, handler)
}

func (dp *Dispatcher) allowed(uripath, origMethod string) (allow string) {
	if uripath == "*" { // server-wide
		for method := range dp.trees {
			if method == http.MethodOptions {
				continue
			}

			// register request method to list of allowed methods
			if len(allow) == 0 {
				allow = method
			} else {
				allow += ", " + method
			}
		}
	} else { // specific path
		for method := range dp.trees {
			// Skip the requested method - we already tried this one
			if method == origMethod || method == http.MethodOptions {
				continue
			}

			handler, _, _ := dp.trees[method].resolve(uripath)
			if handler != nil {
				// register request method to list of allowed methods
				if len(allow) == 0 {
					allow = method
				} else {
					allow += ", " + method
				}
			}
		}
	}

	if len(allow) > 0 {
		allow += ", OPTIONS"
	}

	return
}

func (dp *Dispatcher) notfound(w http.ResponseWriter, req *http.Request) {
	if dp.NotFound != nil {
		dp.NotFound.ServeHTTP(w, req)
	} else {
		http.NotFound(w, req)
	}
}

func (dp *Dispatcher) recovery(w http.ResponseWriter, req *http.Request) {
	if rcv := recover(); rcv != nil {
		dp.PanicHandler(w, req, rcv)
	}
}
