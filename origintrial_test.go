package webmcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOriginTrial_AddsHeaderWhenTokenSet(t *testing.T) {
	handler := OriginTrial("test-token")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get(OriginTrialHeader); got != "test-token" {
		t.Errorf("Origin-Trial header = %q, want %q", got, "test-token")
	}
}

func TestOriginTrial_NoOpWhenTokenEmpty(t *testing.T) {
	handler := OriginTrial("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get(OriginTrialHeader); got != "" {
		t.Errorf("Origin-Trial header = %q, want empty", got)
	}
}

func TestOriginTrial_PreservesExistingHeader(t *testing.T) {
	handler := OriginTrial("new-token")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	// Simulate an earlier middleware in the chain having already set the
	// header before OriginTrial's handler runs.
	rec.Header().Set(OriginTrialHeader, "existing-token")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get(OriginTrialHeader); got != "existing-token" {
		t.Errorf("Origin-Trial header = %q, want %q", got, "existing-token")
	}
}
