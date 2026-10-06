package webmcp

import (
	"bufio"
	"bytes"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestOriginTrialDownstreamAndImplicitHeaders(t *testing.T) {
	for _, mode := range []string{"explicit", "write", "no write", "flush", "controller flush"} {
		for _, token := range []string{"absent", "", "downstream"} {
			t.Run(mode+"/"+token, func(t *testing.T) {
				handler := OriginTrial("default")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if token != "absent" {
						w.Header()["origin-trial"] = []string{token}
					}
					switch mode {
					case "explicit":
						w.WriteHeader(201)
					case "write":
						_, _ = w.Write([]byte("body"))
					case "flush":
						w.(http.Flusher).Flush()
					case "controller flush":
						if err := http.NewResponseController(w).Flush(); err != nil {
							t.Error(err)
						}
					}
				}))
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
				headers := rec.Result().Header
				if token == "absent" {
					if headers.Get(OriginTrialHeader) != "default" {
						t.Fatal("missing token")
					}
				} else {
					if _, exists := headers[OriginTrialHeader]; exists {
						t.Fatal("added duplicate case variant")
					}
					if len(headers["origin-trial"]) != 1 || headers["origin-trial"][0] != token {
						t.Fatal("downstream header replaced")
					}
				}
			})
		}
	}
}

func TestOriginTrialOACWarning(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		var logs bytes.Buffer
		handler := OriginTrialWithOptions("token", OriginTrialOptions{WarnOnOACOptOut: enabled, Logger: slog.New(slog.NewTextHandler(&logs, nil))})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Origin-Agent-Cluster", "?0")
			w.WriteHeader(200)
		}))
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
				if rec.Result().Header.Get("Origin-Agent-Cluster") != "?0" {
					t.Error("OAC rewritten")
				}
			}()
		}
		wg.Wait()
		want := 0
		if enabled {
			want = 1
		}
		if got := strings.Count(logs.String(), "level=WARN"); got != want {
			t.Fatalf("warnings %d want %d: %s", got, want, logs.String())
		}
	}
	var logs bytes.Buffer
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Header().Set("Origin-Agent-Cluster", "?0") })
	handler := OriginTrialWithOptions("", OriginTrialOptions{WarnOnOACOptOut: true, Logger: slog.New(slog.NewTextHandler(&logs, nil))})(next)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if logs.Len() != 0 || rec.Header().Get(OriginTrialHeader) != "" {
		t.Fatal("empty token not pass-through")
	}
}

func TestOriginTrialInformationalResponse(t *testing.T) {
	recorder := &statusWriter{header: http.Header{}}
	handler := OriginTrial("token")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(103)
		w.Header().Set(OriginTrialHeader, "later")
		_, _ = w.Write([]byte("ok"))
	}))
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/", nil))
	if len(recorder.statuses) != 2 || recorder.statuses[0] != 103 || recorder.statuses[1] != 200 || recorder.tokens[1] != "later" {
		t.Fatalf("informational response: %#v", recorder)
	}
}

type statusWriter struct {
	header   http.Header
	statuses []int
	tokens   []string
}

func (w *statusWriter) Header() http.Header { return w.header }
func (w *statusWriter) WriteHeader(status int) {
	w.statuses = append(w.statuses, status)
	w.tokens = append(w.tokens, w.header.Get(OriginTrialHeader))
}
func (w *statusWriter) Write(b []byte) (int, error) { return len(b), nil }

func TestOriginTrialWriterInterfaces(t *testing.T) {
	// All eight capability combinations must remain visible through middleware.
	for bits := 0; bits < 8; bits++ {
		base := &statusWriter{header: http.Header{}}
		capable := &capableWriter{statusWriter: base}
		var underlying http.ResponseWriter
		switch bits {
		case 0:
			underlying = base
		case 1:
			underlying = struct {
				http.ResponseWriter
				http.Flusher
			}{base, capable}
		case 2:
			underlying = struct {
				http.ResponseWriter
				http.Hijacker
			}{base, capable}
		case 3:
			underlying = struct {
				http.ResponseWriter
				http.Flusher
				http.Hijacker
			}{base, capable, capable}
		case 4:
			underlying = struct {
				http.ResponseWriter
				http.Pusher
			}{base, capable}
		case 5:
			underlying = struct {
				http.ResponseWriter
				http.Flusher
				http.Pusher
			}{base, capable, capable}
		case 6:
			underlying = struct {
				http.ResponseWriter
				http.Hijacker
				http.Pusher
			}{base, capable, capable}
		case 7:
			underlying = capable
		}
		handler := OriginTrial("token")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, flush := w.(http.Flusher)
			_, hijack := w.(http.Hijacker)
			_, push := w.(http.Pusher)
			if flush != (bits&1 != 0) || hijack != (bits&2 != 0) || push != (bits&4 != 0) {
				t.Errorf("interfaces changed for %d", bits)
			}
			if u, ok := w.(interface{ Unwrap() http.ResponseWriter }); !ok || u.Unwrap() != underlying {
				t.Error("ResponseController cannot unwrap")
			}
			if flush {
				w.(http.Flusher).Flush()
				if !capable.flushed || w.Header().Get(OriginTrialHeader) != "token" {
					t.Error("flush skipped middleware")
				}
			}
		}))
		handler.ServeHTTP(underlying, httptest.NewRequest("GET", "/", nil))
	}
}

type capableWriter struct {
	*statusWriter
	flushed bool
}

func (w *capableWriter) Flush() { w.flushed = true }
func (w *capableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, http.ErrNotSupported
}
func (w *capableWriter) Push(string, *http.PushOptions) error { return nil }
