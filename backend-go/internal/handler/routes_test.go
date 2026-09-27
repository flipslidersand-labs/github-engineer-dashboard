package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/flipslidersand/github-engineer-dashboard/backend-go/internal/cache"
)

func newTestDeps(t *testing.T, githubHandler http.HandlerFunc) (*Deps, chi.Router) {
	t.Helper()
	ghSrv := httptest.NewServer(githubHandler)
	t.Cleanup(ghSrv.Close)

	c, err := cache.New(filepath.Join(t.TempDir(), "cache.db"), 300)
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	d := &Deps{Cache: c, GithubToken: "", GithubAPIURL: ghSrv.URL}
	r := chi.NewRouter()
	Register(r, d)
	return d, r
}

func TestHealthz(t *testing.T) {
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status field = %q, want ok", body["status"])
	}
}

func TestRequireTokenRejectsMissingToken(t *testing.T) {
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {})

	req := httptest.NewRequest(http.MethodGet, "/api/rate-limit", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestRateLimitWithToken(t *testing.T) {
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resources": map[string]any{
				"core": map[string]any{"limit": 5000, "remaining": 4999, "used": 1, "reset": 111},
			},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/rate-limit", nil)
	req.Header.Set("X-GitHub-Token", "abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

// TestRateLimitForbiddenWithExhaustedHeaderBecomes429 verifies a 403 IS
// remapped to 429 when GitHub signals real rate-limit exhaustion via
// X-RateLimit-Remaining: 0.
func TestRateLimitForbiddenWithExhaustedHeaderBecomes429(t *testing.T) {
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "rate limit exceeded"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/rate-limit", nil)
	req.Header.Set("X-GitHub-Token", "abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429, body=%s", w.Code, w.Body.String())
	}
}

// TestRateLimitForbiddenWithoutRateLimitHeaderStays403 verifies a permissions
// / SAML-enforcement 403 (no rate-limit signal in the headers) is NOT
// remapped to 429.
func TestRateLimitForbiddenWithoutRateLimitHeaderStays403(t *testing.T) {
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Resource not accessible by integration"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/rate-limit", nil)
	req.Header.Set("X-GitHub-Token", "abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body=%s", w.Code, w.Body.String())
	}
}

func TestAnalyzeRequiresURL(t *testing.T) {
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {})

	req := httptest.NewRequest(http.MethodGet, "/api/analyze", nil)
	req.Header.Set("X-GitHub-Token", "abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
}

func TestAnalyzeUnsupportedURL(t *testing.T) {
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {})

	req := httptest.NewRequest(http.MethodGet, "/api/analyze?url=https://example.com/foo", nil)
	req.Header.Set("X-GitHub-Token", "abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
}

func TestAnalyzeUserURL(t *testing.T) {
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch req.URL.Path {
		case "/users/octocat":
			_ = json.NewEncoder(w).Encode(map[string]any{"login": "octocat", "public_repos": 1})
		case "/users/octocat/events/public":
			_ = json.NewEncoder(w).Encode([]any{})
		case "/users/octocat/repos":
			_ = json.NewEncoder(w).Encode([]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/api/analyze?url=https://github.com/octocat", nil)
	req.Header.Set("X-GitHub-Token", "abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["type"] != "user" {
		t.Errorf("type = %v, want user", body["type"])
	}
}

// TestAnalyzeCachesSecondRequest verifies fromCache actually short-circuits
// the upstream call on a repeat request for the same key.
func TestAnalyzeCachesSecondRequest(t *testing.T) {
	calls := 0
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch req.URL.Path {
		case "/users/octocat":
			calls++
			_ = json.NewEncoder(w).Encode(map[string]any{"login": "octocat", "public_repos": 1})
		case "/users/octocat/events/public", "/users/octocat/repos":
			_ = json.NewEncoder(w).Encode([]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/analyze?url=https://github.com/octocat", nil)
		req.Header.Set("X-GitHub-Token", "abc")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, body=%s", i, w.Code, w.Body.String())
		}
	}
	if calls != 1 {
		t.Errorf("upstream /users/octocat called %d times, want 1 (second request should hit cache)", calls)
	}
}
