package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAllowOriginLoopbackOnly(t *testing.T) {
	t.Setenv("CORS_ORIGINS", "")
	if !allowOrigin("http://localhost:3000") || !allowOrigin("http://127.0.0.1:3001") {
		t.Fatal("loopback should be allowed")
	}
	if allowOrigin("https://evil.example") || allowOrigin("") {
		t.Fatal("foreign origin must be rejected")
	}
}

func TestAllowOriginExtraEnv(t *testing.T) {
	t.Setenv("CORS_ORIGINS", "https://app.example")
	if !allowOrigin("https://app.example") {
		t.Fatal("CORS_ORIGINS should allow the listed origin")
	}
}

func TestCorsReflectsAllowedOrigin(t *testing.T) {
	t.Setenv("CORS_ORIGINS", "")
	handler := cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/git/status", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("allowed origin: %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}

	req = httptest.NewRequest(http.MethodGet, "/git/status", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("rejected origin leaked: %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}
