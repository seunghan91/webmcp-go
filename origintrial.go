package webmcp

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
)

// OriginTrialHeader is the origin trial response header name.
const OriginTrialHeader = "Origin-Trial"

// OriginTrialOptions enables diagnostics for older Origin-Trial builds.
type OriginTrialOptions struct {
	WarnOnOACOptOut bool
	Logger          *slog.Logger // nil uses slog.Default when warnings are enabled.
}

// OriginTrial fills a missing Origin-Trial header when the response is written.
// It preserves existing headers (even empty ones) including downstream values.
// An empty token is a pass-through.
func OriginTrial(token string) func(http.Handler) http.Handler {
	return OriginTrialWithOptions(token, OriginTrialOptions{})
}

// OriginTrialWithOptions adds an opt-in, once-per-handler OAC warning. It never
// rewrites Origin-Agent-Cluster or forces ?1. Empty tokens remain pass-through.
func OriginTrialWithOptions(token string, opts OriginTrialOptions) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if token == "" {
			return next
		}
		var warned sync.Once
		logger := opts.Logger
		if logger == nil {
			logger = slog.Default()
		}
		prepare := func(h http.Header) {
			present := false
			optedOut := false
			for key, values := range h {
				if strings.EqualFold(key, OriginTrialHeader) {
					present = true
				}
				if strings.EqualFold(key, "Origin-Agent-Cluster") {
					for _, value := range values {
						if value == "?0" {
							optedOut = true
						}
					}
				}
			}
			if !present {
				h.Set(OriginTrialHeader, token)
			}
			if opts.WarnOnOACOptOut && optedOut {
				warned.Do(func() {
					logger.Warn("WebMCP: Origin-Agent-Cluster: ?0 may cause SecurityError in older Origin-Trial builds; header left unchanged")
				})
			}
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			wrapped := &originTrialWriter{ResponseWriter: w, prepare: prepare}
			next.ServeHTTP(wrapped.withInterfaces(), r)
			// net/http implicitly sends 200 even when the handler writes nothing.
			if !wrapped.wroteHeader {
				prepare(w.Header())
			}
		})
	}
}

type originTrialWriter struct {
	http.ResponseWriter
	prepare     func(http.Header)
	wroteHeader bool
}

// Unwrap keeps http.ResponseController operations available through middleware.
func (w *originTrialWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *originTrialWriter) WriteHeader(status int) {
	if !w.wroteHeader {
		w.prepare(w.Header())
		// Informational responses do not commit the final response headers.
		if status >= 200 || status == http.StatusSwitchingProtocols {
			w.wroteHeader = true
		}
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *originTrialWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (w *originTrialWriter) FlushError() error {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return http.NewResponseController(w.ResponseWriter).Flush()
}

type originTrialFlusher struct{ *originTrialWriter }

func (w *originTrialFlusher) Flush() { _ = w.FlushError() }

// Preserve optional net/http interfaces without advertising capabilities that
// the original writer lacks. Flush still passes through our header preparation.
func (w *originTrialWriter) withInterfaces() http.ResponseWriter {
	_, flush := w.ResponseWriter.(http.Flusher)
	hijacker, hijack := w.ResponseWriter.(http.Hijacker)
	pusher, push := w.ResponseWriter.(http.Pusher)
	f := &originTrialFlusher{w}
	switch {
	case flush && hijack && push:
		return struct {
			*originTrialWriter
			http.Flusher
			http.Hijacker
			http.Pusher
		}{w, f, hijacker, pusher}
	case flush && hijack:
		return struct {
			*originTrialWriter
			http.Flusher
			http.Hijacker
		}{w, f, hijacker}
	case flush && push:
		return struct {
			*originTrialWriter
			http.Flusher
			http.Pusher
		}{w, f, pusher}
	case hijack && push:
		return struct {
			*originTrialWriter
			http.Hijacker
			http.Pusher
		}{w, hijacker, pusher}
	case flush:
		return f
	case hijack:
		return struct {
			*originTrialWriter
			http.Hijacker
		}{w, hijacker}
	case push:
		return struct {
			*originTrialWriter
			http.Pusher
		}{w, pusher}
	default:
		return w
	}
}
