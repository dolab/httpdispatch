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
	"context"
	"net/http"
)

type ContextKey int

const (
	ContextKeyRoute ContextKey = iota
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
func (d *Dispatcher) OPTIONS(path string, handler http.Handler) {
	d.Handler(http.MethodOptions, path, handler)
}

// GET is a shortcut for dispatcher.Handler("GET", path, http.Handler)
func (d *Dispatcher) GET(path string, handler http.Handler) {
	d.Handler(http.MethodGet, path, handler)
}

// HEAD is a shortcut for dispatcher.Handler("GET", path, http.Handler)
func (d *Dispatcher) HEAD(path string, handler http.Handler) {
	d.Handler(http.MethodHead, path, handler)
}

// POST is a shortcut for dispatcher.Handler("GET", path, http.Handler)
func (d *Dispatcher) POST(path string, handler http.Handler) {
	d.Handler(http.MethodPost, path, handler)
}

// PUT is a shortcut for dispatcher.Handler("GET", path, http.Handler)
func (d *Dispatcher) PUT(path string, handler http.Handler) {
	d.Handler(http.MethodPut, path, handler)
}

// PATCH is a shortcut for dispatcher.Handler("GET", path, http.Handler)
func (d *Dispatcher) PATCH(path string, handler http.Handler) {
	d.Handler(http.MethodPatch, path, handler)
}

// DELETE is a shortcut for dispatcher.Handler("GET", path, http.Handler)
func (d *Dispatcher) DELETE(path string, handler http.Handler) {
	d.Handler(http.MethodDelete, path, handler)
}

// Handler is an adapter which allows the usage of a http.Handler as a
// request handle.
func (d *Dispatcher) Handler(method, path string, handler http.Handler) {
	d.Handle(method, path, NewContextHandle(handler, d.RequestContext))
}

// HandlerFunc is an adapter which allows the usage of an http.HandlerFunc as a
// request handler.
func (d *Dispatcher) HandlerFunc(method, path string, handler http.HandlerFunc) {
	d.Handler(method, path, handler)
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
func (d *Dispatcher) ServeFiles(path string, fs http.FileSystem) {
	if len(path) < 10 || path[len(path)-10:] != "/*filepath" {
		panic(`static files server path must end with /*filepath in "` + path + `"`)
	}

	d.Handle(http.MethodGet, path, NewFileHandle(fs))
}

// Lookup allows the manual lookup of a method + path combo.
// This is e.g. useful to build a framework around the dispatcher.
// If the path was found, it returns the handler func and the captured parameter
// values.
//
// NOTE: It returns handler when the third returned value indicates a redirection to
// the same path with / without the trailing slash should be performed.
func (d *Dispatcher) Lookup(method, path string) (Handler, Params, bool) {
	if root := d.trees[method]; root != nil {
		handler, params, tsr, _ := root.getValue(path)
		return handler, params, tsr
	}

	return nil, nil, false
}

// ServeHTTP makes the router implement the http.Handler interface.
func (d *Dispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if d.PanicHandler != nil {
		defer d.recovery(w, r)
	}

	path := r.URL.Path

	if root := d.trees[r.Method]; root != nil {
		handler, params, tsr, route := root.getValue(path)

		if handler != nil {
			if !tsr || !d.RedirectTrailingSlash {
				r = r.WithContext(context.WithValue(r.Context(), ContextKeyRoute, route))
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

			if len(path) > 1 && path[len(path)-1] == '/' {
				r.URL.Path = path[:len(path)-1]
			} else {
				r.URL.Path = path + "/"
			}

			// redirect trailing slash pattern
			http.Redirect(w, r, r.URL.String(), code)
			return
		}

		// Try to fix the request path
		if d.RedirectFixedPath && r.Method != http.MethodConnect && path != "/" {
			fixedPath, found := root.findCaseInsensitivePath(
				Normalize(path),
				d.RedirectTrailingSlash,
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
		if d.HandleMethodOPTIONS {
			allow := d.allowed(path, r.Method)
			if len(allow) > 0 {
				w.Header().Set("Allow", allow)
				return
			}
		}
	} else {
		// Handle 405
		if d.HandleMethodNotAllowed {
			allow := d.allowed(path, r.Method)
			if len(allow) > 0 {
				w.Header().Set("Allow", allow)

				if d.MethodNotAllowed != nil {
					d.MethodNotAllowed.ServeHTTP(w, r)
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
	d.notfound(w, r)
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
func (d *Dispatcher) Handle(method, path string, handler Handler) {
	if path[0] != '/' {
		panic("path must begin with '/' in '" + path + "'")
	}

	if d.trees == nil {
		d.trees = make(map[string]*node)
	}

	root := d.trees[method]
	if root == nil {
		root = new(node)

		d.trees[method] = root
	}

	root.add(path, handler)
}

func (d *Dispatcher) allowed(path, reqMethod string) (allow string) {
	if path == "*" { // server-wide
		for method := range d.trees {
			if method == http.MethodOptions {
				continue
			}

			// add request method to list of allowed methods
			if len(allow) == 0 {
				allow = method
			} else {
				allow += ", " + method
			}
		}
	} else { // specific path
		for method := range d.trees {
			// Skip the requested method - we already tried this one
			if method == reqMethod || method == http.MethodOptions {
				continue
			}

			handler, _, _, _ := d.trees[method].getValue(path)
			if handler != nil {
				// add request method to list of allowed methods
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

func (d *Dispatcher) notfound(w http.ResponseWriter, req *http.Request) {
	if d.NotFound != nil {
		d.NotFound.ServeHTTP(w, req)
	} else {
		http.NotFound(w, req)
	}
}

func (d *Dispatcher) recovery(w http.ResponseWriter, req *http.Request) {
	if rcv := recover(); rcv != nil {
		d.PanicHandler(w, req, rcv)
	}
}
