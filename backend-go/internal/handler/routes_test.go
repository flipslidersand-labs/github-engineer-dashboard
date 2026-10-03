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

// TestUserActivityRejectsInvalidUsername covers Issue #151: this route took
// chi.URLParam("username") straight to the GitHub API without going through
// isValidOwner, unlike parseGitHubURL's routes.
func TestUserActivityRejectsInvalidUsername(t *testing.T) {
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {})

	req := httptest.NewRequest(http.MethodGet, "/api/users/-invalid/activity", nil)
	req.Header.Set("X-GitHub-Token", "abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body=%s", w.Code, w.Body.String())
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

func TestSummaryRequiresURL(t *testing.T) {
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {})

	req := httptest.NewRequest(http.MethodGet, "/api/summary", nil)
	req.Header.Set("X-GitHub-Token", "abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
}

func TestSummaryRejectsNonUserOrgURL(t *testing.T) {
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {})

	req := httptest.NewRequest(http.MethodGet, "/api/summary?url=https://github.com/octocat/hello", nil)
	req.Header.Set("X-GitHub-Token", "abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
}

func TestSummaryUserURL(t *testing.T) {
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch req.URL.Path {
		case "/users/octocat/repos":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"stargazers_count": 3, "forks_count": 1, "language": "Go", "fork": false},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/api/summary?url=https://github.com/octocat", nil)
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
	if body["owner"] != "octocat" || body["owner_type"] != "user" {
		t.Errorf("got %+v", body)
	}
}

func TestSummaryOrgURL(t *testing.T) {
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch req.URL.Path {
		case "/orgs/acme/repos":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"stargazers_count": 2, "forks_count": 0, "language": "Python", "fork": false},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/api/summary?url=https://github.com/orgs/acme", nil)
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
	if body["owner"] != "acme" || body["owner_type"] != "org" {
		t.Errorf("got %+v", body)
	}
}

func TestSummaryCachesSecondRequest(t *testing.T) {
	calls := 0
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch req.URL.Path {
		case "/users/octocat/repos":
			calls++
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/summary?url=https://github.com/octocat", nil)
		req.Header.Set("X-GitHub-Token", "abc")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, body=%s", i, w.Code, w.Body.String())
		}
	}
	if calls != 1 {
		t.Errorf("upstream /users/octocat/repos called %d times, want 1 (second request should hit cache)", calls)
	}
}

// TestWriteGitHubErrorMaps403To429 verifies writeGitHubError remaps GitHub's
// 403 (used for both permission errors and secondary rate limiting) to 429
// so clients can distinguish rate limiting from a hard permission failure.
func TestWriteGitHubErrorMaps403To429(t *testing.T) {
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "API rate limit exceeded"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/rate-limit", nil)
	req.Header.Set("X-GitHub-Token", "abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "API rate limit exceeded" {
		t.Errorf("error = %q", body["error"])
	}
}

// TestWriteGitHubErrorPreservesOtherStatusCodes verifies non-403 upstream
// errors pass through unchanged (only 403 gets remapped to 429).
func TestWriteGitHubErrorPreservesOtherStatusCodes(t *testing.T) {
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Not Found"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/rate-limit", nil)
	req.Header.Set("X-GitHub-Token", "abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
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
