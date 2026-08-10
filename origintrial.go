// Package webmcp provides framework-agnostic helpers for WebMCP
// (https://github.com/webmachinelearning/webmcp) — the W3C proposal that
// lets web pages declare structured tools for AI agents
// (document.modelContext).
//
// Status: early development. The WebMCP spec is in Chrome origin trial
// (Chrome 149-156) and its API surface has already changed twice. This
// package currently ships the one piece of server-side plumbing every
// participating origin needs — the Origin-Trial response header — and
// stays deliberately small until the spec settles.
package webmcp

import "net/http"

// OriginTrialHeader is the HTTP response header name Chrome's origin trial
// framework expects: "Origin-Trial".
const OriginTrialHeader = "Origin-Trial"

// OriginTrial returns a standard net/http middleware that adds the
// Origin-Trial response header for the given token. If token is empty,
// the middleware passes requests through unmodified. If the response
// already carries an Origin-Trial header (set by an earlier middleware
// in the chain), that value is left untouched — OriginTrial only fills
// in a missing header, it never overrides one.
func OriginTrial(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if token == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if w.Header().Get(OriginTrialHeader) == "" {
				w.Header().Set(OriginTrialHeader, token)
			}
			next.ServeHTTP(w, r)
		})
	}
}
