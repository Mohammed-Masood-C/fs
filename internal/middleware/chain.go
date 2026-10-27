// Package middleware contains the HTTP middleware of the fs server.
package middleware

import "net/http"

// Middleware wraps an http.Handler with additional behaviour.
type Middleware func(http.Handler) http.Handler

// Chain wraps h with the given middleware. The first middleware is the
// outermost one, so it sees the request first and the response last.
func Chain(h http.Handler, stack ...Middleware) http.Handler {
	for i := len(stack) - 1; i >= 0; i-- {
		h = stack[i](h)
	}
	return h
}
