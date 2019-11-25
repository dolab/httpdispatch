// Copyright 2013 Julien Schmidt. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found
// in the LICENSE file.

package httpdispatch

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type nodeType uint8

const (
	static nodeType = iota // default
	root
	param
	wildcard
)

type node struct {
	typo     nodeType
	path     string
	nparams  uint8
	indices  string
	handle   Handler
	priority uint32
	children []*node
	wildcard bool
}

// increments priority of the given child and reorders if necessary
func (n *node) incrementChildPriority(pos int) int {
	n.children[pos].priority++

	priority := n.children[pos].priority

	// adjust position (move to front)
	newPos := pos
	for newPos > 0 && n.children[newPos-1].priority < priority {
		// swap node positions
		tmpN := n.children[newPos-1]
		n.children[newPos-1] = n.children[newPos]
		n.children[newPos] = tmpN

		newPos--
	}

	// build new index char string
	if newPos != pos {
		n.indices = n.indices[:newPos] + // unchanged prefix, might be empty
			n.indices[pos:pos+1] + // the index char we move
			n.indices[newPos:pos] + n.indices[pos+1:] // rest without char at 'pos'
	}

	return newPos
}

// register adds a node with the given handle to the path.
// Not concurrency-safe!
func (n *node) register(uripath string, handle Handler) {
	n.priority++

	abspath := uripath
	maxParams := countParams(uripath)

	// non-empty tree
	if len(n.path) > 0 || len(n.children) > 0 {
	walk:
		for {
			// Update nparams of the current node
			if maxParams > n.nparams {
				n.nparams = maxParams
			}

			// Find the longest common prefix.
			// This also implies that the common prefix contains no ':' or '*'
			// since the existing key can't contain those chars.
			i := 0
			max := min(len(uripath), len(n.path))
			for i < max && uripath[i] == n.path[i] {
				i++
			}

			// Split edge
			if i < len(n.path) {
				child := node{
					typo:     static,
					path:     n.path[i:],
					wildcard: n.wildcard,
					indices:  n.indices,
					children: n.children,
					handle:   n.handle,
					priority: n.priority - 1,
				}

				// Update nparams (max of all children)
				for i := range child.children {
					if child.children[i].nparams > child.nparams {
						child.nparams = child.children[i].nparams
					}
				}

				n.children = []*node{&child}
				// []byte for proper unicode char conversion, see #65
				n.indices = string([]byte{n.path[i]})
				n.path = uripath[:i]
				n.handle = nil
				n.wildcard = false
			}

			// Make new node a child of this node
			if i < len(uripath) {
				uripath = uripath[i:]

				if n.wildcard {
					n = n.children[0]
					n.priority++

					// Update nparams of the child node
					if maxParams > n.nparams {
						n.nparams = maxParams
					}
					maxParams--

					// Check if the wildcard matches
					if len(uripath) >= len(n.path) && n.path == uripath[:len(n.path)] {
						// check for longer wildcard, e.g. :name and :names
						if len(n.path) >= len(uripath) || uripath[len(n.path)] == '/' {
							continue walk
						}
					}

					panic("path segment '" + uripath +
						"' conflicts with existing wildcard '" + n.path +
						"' in path '" + abspath + "'")
				}

				c := uripath[0]

				// slash after param
				if n.typo == param && c == '/' && len(n.children) == 1 {
					n = n.children[0]
					n.priority++
					continue walk
				}

				// Check if a child with the next path byte exists
				for i := 0; i < len(n.indices); i++ {
					if c == n.indices[i] {
						i = n.incrementChildPriority(i)
						n = n.children[i]
						continue walk
					}
				}

				// Otherwise insert it
				if c != ':' && c != '*' {
					// []byte for proper unicode char conversion, see #65
					n.indices += string([]byte{c})
					child := &node{
						nparams: maxParams,
					}
					n.children = append(n.children, child)
					n.incrementChildPriority(len(n.indices) - 1)
					n = child
				}
				n.insertChild(maxParams, uripath, abspath, handle)
				return

			} else if i == len(uripath) { // Make node a (in-uripath) leaf
				if n.handle != nil {
					panic("a handle is already registered for path '" + abspath + "'")
				}

				n.handle = handle
			}
			return
		}
	} else { // Empty tree
		n.typo = root
		n.insertChild(maxParams, uripath, abspath, handle)
	}
}

func (n *node) insertChild(numParams uint8, uripath, abspath string, handle Handler) {
	var offset int // already handled bytes of the uripath

	// find prefix until first placeholder (beginning with ':'' or '*'')
	for i, max := 0, len(uripath); numParams > 0; i++ {
		c := uripath[i]
		if c != ':' && c != '*' {
			continue
		}

		// find placeholder end (either '/' or uripath end)
		end := i + 1
		for end < max && uripath[end] != '/' {
			switch uripath[end] {
			// the wildcard name must not contain ':' and '*'
			case ':', '*':
				panic("only one wildcard per path segment is allowed, has: '" +
					uripath[i:] + "' in path '" + abspath + "'")
			default:
				end++
			}
		}

		// check if this node existing children which would be
		// unreachable if we insert the wildcard here
		if len(n.children) > 0 {
			panic("wildcard route '" + uripath[i:end] +
				"' conflicts with existing children in path '" + abspath + "'")
		}

		// check if the wildcard has a name
		if end-i < 2 {
			panic("wildcard must be named with a non-empty name in path '" + abspath + "'")
		}

		if c == ':' { // param
			// split path at the beginning of the wildcard
			if i > 0 {
				n.path = uripath[offset:i]
				offset = i
			}

			child := &node{
				typo:    param,
				nparams: numParams,
			}
			n.children = []*node{child}
			n.wildcard = true

			n = child
			n.priority++
			numParams--

			// if the path doesn't end with the wildcard, then there
			// will be another non-wildcard sub path starting with '/'
			if end < max {
				n.path = uripath[offset:end]
				offset = end

				child := &node{
					nparams:  numParams,
					priority: 1,
				}
				n.children = []*node{child}

				n = child
			}

		} else { // wildcard
			if end != max || numParams > 1 {
				panic("catch-all routes are only allowed at the end of the path in path '" + abspath + "'")
			}

			if len(n.path) > 0 && n.path[len(n.path)-1] == '/' {
				panic("catch-all conflicts with existing handle for the path segment root in path '" + abspath + "'")
			}

			// currently fixed width 1 for '/'
			i--
			if uripath[i] != '/' {
				panic("no / before catch-all in path '" + abspath + "'")
			}

			n.path = uripath[offset:i]

			// first node: wildcard node with empty path
			child := &node{
				typo:     wildcard,
				nparams:  1,
				wildcard: true,
			}
			n.children = []*node{child}
			n.indices = string(uripath[i])

			n = child
			n.priority++

			// second node: node holding the variable
			child = &node{
				typo:     wildcard,
				path:     uripath[i:],
				nparams:  1,
				handle:   handle,
				priority: 1,
			}
			n.children = []*node{child}

			return
		}
	}

	// insert remaining path part and handle to the leaf
	n.path = uripath[offset:]
	n.handle = handle
}

// resolve returns the handle registered with the given path (key). The values of
// wildcards are saved to a map.
// If no handle can be found, a TSR (trailing slash redirect) recommendation is
// made if a handle exists with an extra (without the) trailing slash for the
// given path.
// It returns handle also if a TSR is true. Its useful for quick fallback strategy.
func (n *node) resolve(uripath string) (handle Handler, p Params, tsr bool) {
walk: // outer loop for walking the tree
	for {
		switch {
		case len(uripath) > len(n.path):
			if uripath[:len(n.path)] == n.path {
				uripath = uripath[len(n.path):]

				// If this node does not have a wildcard (param or wildcard)
				// child,  we can just look up the next child node and continue
				// to walk down the tree
				if !n.wildcard {
					// could we stop swift for path such as /name/
					tsr = uripath == "/" && n.handle != nil
					if tsr {
						handle = n.handle

						return
					}

					c := uripath[0]
					for i := 0; i < len(n.indices); i++ {
						if c == n.indices[i] {
							n = n.children[i]
							continue walk
						}
					}

					return
				}

				// handle wildcard child
				n = n.children[0]

				switch n.typo {
				case param:
					// save param value
					if p == nil {
						// lazy allocation
						p = make(Params, 0, n.nparams)
					}

					// find param end (either '/' or path end)
					end := strings.IndexByte(uripath, '/')
					if end == -1 {
						end = len(uripath)
					}

					i := len(p)

					p = p[:i+1] // expand slice within pre-allocated capacity
					p[i].Key = n.path[1:]
					p[i].Value = uripath[:end]

					// we need to go deeper!
					if end < len(uripath) {
						// could we stop swift for path such as /:key/value/
						tsr = uripath[end:] == "/" && n.handle != nil
						if tsr {
							handle = n.handle

							return
						}

						if len(n.children) > 0 {
							uripath = uripath[end:]

							n = n.children[0]
							continue walk
						}

						return
					}

					if n.handle != nil {
						handle = n.handle

						return
					}

					// No handle found. Check if a handle for this path + a
					// trailing slash exists for TSR recommendation
					if len(n.children) == 1 {
						n = n.children[0]

						tsr = n.path == "/" && n.handle != nil
						if tsr {
							handle = n.handle
						}
					}

					return

				case wildcard:
					// save param value
					if p == nil {
						// lazy allocation
						p = make(Params, 0, n.nparams)
					}

					i := len(p)

					p = p[:i+1] // expand slice within pre-allocated capacity
					p[i].Key = n.path[2:]
					p[i].Value = uripath[1:]

					handle = n.handle

					return

				default:
					panic("invalid node type")

				}
			}

		case uripath == n.path:
			// We should have reached the node containing the handle.
			// Check if this node has a handle registered.
			if n.handle != nil {
				handle = n.handle

				return
			}

			// redirect /name/ to /name
			if uripath == "/" && n.wildcard && n.typo != root {
				tsr = true

				return
			}

			// No handle found. Check if a handle for this path + a
			// trailing slash exists for trailing slash recommendation
			for i := 0; i < len(n.indices); i++ {
				if n.indices[i] == '/' {
					n = n.children[i]

					tsr = len(n.path) == 1 && n.handle != nil
					if tsr {
						handle = n.handle

						return
					}

					tsr = n.typo == wildcard && n.children[0].handle != nil
					if tsr {
						handle = n.children[0].handle
					}

					return
				}
			}

			return

		}

		// Nothing found. We can recommend to redirect to the same URL with an
		// extra trailing slash if a leaf exists for that path

		// redirect /route/ to /route
		tsr = uripath == "/"
		if tsr {
			return
		}

		tsr = len(n.path) == (len(uripath)+1) && n.path[len(uripath)] == '/' && uripath == n.path[:len(n.path)-1] && n.handle != nil
		if tsr {
			handle = n.handle
		}

		return
	}
}

// Makes a case-insensitive lookup of the given path and tries to find a handler.
// It can optionally also fix trailing slashes.
// It returns the case-corrected path and a bool indicating whether the lookup
// was successful.
func (n *node) findCaseInsensitivePath(uripath string, fixTrailingSlash bool) (abspath []byte, found bool) {
	return n.findCaseInsensitivePathRec(
		uripath,
		strings.ToLower(uripath),
		make([]byte, 0, len(uripath)+1), // pre-allocate enough memory for new path
		[4]byte{},                       // empty rune buffer
		fixTrailingSlash,
	)
}

// recursive case-insensitive lookup function used by n.findCaseInsensitivePath
func (n *node) findCaseInsensitivePathRec(uripath, lowerPath string, newPath []byte, rb [4]byte, fixTrailingSlash bool) ([]byte, bool) {
	lowerNodePath := strings.ToLower(n.path)

walk: // outer loop for walking the tree
	for len(lowerPath) >= len(lowerNodePath) && (len(lowerNodePath) == 0 || lowerPath[1:len(lowerNodePath)] == lowerNodePath[1:]) {
		// register common path to result
		newPath = append(newPath, n.path...)

		if uripath = uripath[len(n.path):]; len(uripath) > 0 {
			oldPath := lowerPath
			lowerPath = lowerPath[len(lowerNodePath):]

			// If this node does not have a wildcard (param or wildcard) child,
			// we can just look up the next child node and continue to walk down
			// the tree
			if !n.wildcard {
				// skip rune bytes already processed
				rb = shiftNRuneBytes(rb, len(lowerNodePath))

				if rb[0] != 0 {
					// old rune not finished
					for i := 0; i < len(n.indices); i++ {
						if n.indices[i] == rb[0] {
							// continue with child node
							n = n.children[i]

							lowerNodePath = strings.ToLower(n.path)
							continue walk
						}
					}
				} else {
					// process a new rune
					var rv rune

					// find rune start
					// runes are up to 4 byte long,
					// -4 would definitely be another rune
					var off int
					for max := min(len(lowerNodePath), 3); off < max; off++ {
						if i := len(lowerNodePath) - off; utf8.RuneStart(oldPath[i]) {
							// read rune from cached lowercase path
							rv, _ = utf8.DecodeRuneInString(oldPath[i:])
							break
						}
					}

					// calculate lowercase bytes of current rune
					utf8.EncodeRune(rb[:], rv)

					// skipp already processed bytes
					rb = shiftNRuneBytes(rb, off)

					for i := 0; i < len(n.indices); i++ {
						// lowercase matches
						if n.indices[i] == rb[0] {
							// must use a recursive approach since both the
							// uppercase byte and the lowercase byte might exist
							// as an index
							if out, found := n.children[i].findCaseInsensitivePathRec(
								uripath, lowerPath, newPath, rb, fixTrailingSlash,
							); found {
								return out, true
							}

							break
						}
					}

					// same for uppercase rune, if it differs
					if up := unicode.ToUpper(rv); up != rv {
						utf8.EncodeRune(rb[:], up)

						rb = shiftNRuneBytes(rb, off)

						for i := 0; i < len(n.indices); i++ {
							// uppercase matches
							if n.indices[i] == rb[0] {
								// continue with child node
								n = n.children[i]

								lowerNodePath = strings.ToLower(n.path)

								continue walk
							}
						}
					}
				}

				// Nothing found. We can recommend to redirect to the same URL
				// without a trailing slash if a leaf exists for that path
				return newPath, fixTrailingSlash && uripath == "/" && n.handle != nil
			}

			n = n.children[0]

			switch n.typo {
			case param:
				// find param end (either '/' or path end)
				k := 0
				for k < len(uripath) && uripath[k] != '/' {
					k++
				}

				// register param value to case insensitive path
				newPath = append(newPath, uripath[:k]...)

				// we need to go deeper!
				if k < len(uripath) {
					if len(n.children) > 0 {
						// continue with child node
						n = n.children[0]

						lowerNodePath = strings.ToLower(n.path)

						lowerPath = lowerPath[k:]
						uripath = uripath[k:]
						continue
					}

					// ... but we can't
					if fixTrailingSlash && len(uripath) == k+1 {
						return newPath, true
					}
					return newPath, false
				}

				if n.handle != nil {
					return newPath, true
				}

				if fixTrailingSlash && len(n.children) == 1 {
					// No handle found. Check if a handle for this uripath + a
					// trailing slash exists
					n = n.children[0]
					if n.path == "/" && n.handle != nil {
						return append(newPath, '/'), true
					}
				}

				return newPath, false

			case wildcard:
				return append(newPath, uripath...), true

			default:
				panic("invalid node type")
			}
		} else {
			// We should have reached the node containing the handle.
			// Check if this node has a handle registered.
			if n.handle != nil {
				return newPath, true
			}

			// No handle found.
			// Try to fix the path by adding a trailing slash
			if fixTrailingSlash {
				for i := 0; i < len(n.indices); i++ {
					if n.indices[i] == '/' {
						n = n.children[i]

						if (len(n.path) == 1 && n.handle != nil) ||
							(n.typo == wildcard && n.children[0].handle != nil) {
							return append(newPath, '/'), true
						}

						return newPath, false
					}
				}
			}

			return newPath, false
		}
	}

	// Nothing found.
	// Try to fix the path by adding / removing a trailing slash
	if fixTrailingSlash {
		if uripath == "/" {
			return newPath, true
		}

		if len(lowerPath)+1 == len(lowerNodePath) &&
			lowerNodePath[len(lowerPath)] == '/' &&
			lowerPath[1:] == lowerNodePath[1:len(lowerPath)] &&
			n.handle != nil {
			return append(newPath, n.path...), true
		}
	}

	return newPath, false
}
