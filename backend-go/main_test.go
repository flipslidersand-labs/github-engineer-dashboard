package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newCORSTestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestCorsMiddlewareAllowsListedOrigin(t *testing.T) {
	mw := corsMiddleware("https://allowed.example.com", false)
	h := mw(newCORSTestHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/rate-limit", nil)
	req.Header.Set("Origin", "https://allowed.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://allowed.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want the allowed origin", got)
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestCorsMiddlewareRejectsUnlistedOrigin(t *testing.T) {
	mw := corsMiddleware("https://allowed.example.com", false)
	h := mw(newCORSTestHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/rate-limit", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for an unlisted origin", got)
	}
}

// TestCorsMiddlewareWildcardRefusedWithServerToken covers Issue #123: when a
// GITHUB_TOKEN fallback is configured, CORS_ORIGINS="*" must be refused
// rather than reflected, since that combination would let any third-party
// site ride on the operator's shared token.
func TestCorsMiddlewareWildcardRefusedWithServerToken(t *testing.T) {
	mw := corsMiddleware("*", true)
	h := mw(newCORSTestHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/rate-limit", nil)
	req.Header.Set("Origin", "https://random.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty: wildcard must be refused when a server token is set", got)
	}
}

// TestCorsMiddlewareWildcardAllowedWithoutServerToken verifies the wildcard
// is honoured when there is no server-side token fallback to protect.
func TestCorsMiddlewareWildcardAllowedWithoutServerToken(t *testing.T) {
	mw := corsMiddleware("*", false)
	h := mw(newCORSTestHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/rate-limit", nil)
	req.Header.Set("Origin", "https://random.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://random.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want the reflected origin", got)
	}
}

func TestCorsMiddlewareHandlesOptionsPreflight(t *testing.T) {
	mw := corsMiddleware("https://allowed.example.com", false)
	h := mw(newCORSTestHandler())

	req := httptest.NewRequest(http.MethodOptions, "/api/rate-limit", nil)
	req.Header.Set("Origin", "https://allowed.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204 for OPTIONS preflight", w.Code)
	}
}

func TestCorsMiddlewareNoOriginHeaderSetsNoCORSHeaders(t *testing.T) {
	mw := corsMiddleware("https://allowed.example.com", false)
	h := mw(newCORSTestHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/rate-limit", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty when no Origin header is sent", got)
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestEnvBool(t *testing.T) {
	for v, want := range map[string]bool{"": false, "0": false, "false": false, "1": true, "true": true, " TRUE ": true, "yes": true, "on": true} {
		t.Setenv("X_TEST_BOOL", v)
		if got := envBool("X_TEST_BOOL"); got != want {
			t.Errorf("envBool(%q) = %v, want %v", v, got, want)
		}
	}
}
