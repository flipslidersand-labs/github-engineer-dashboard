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

// TestAnalyzeCacheIsScopedPerToken verifies a different token never hits
// another token's cache entry for the same URL — otherwise a token without
// real access to a private repo could read data cached by a token that does
// (Issue #130).
func TestAnalyzeCacheIsScopedPerToken(t *testing.T) {
	calls := 0
	ghSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/users/octocat":
			calls++
			_ = json.NewEncoder(w).Encode(map[string]any{"login": "octocat", "public_repos": 1})
		case "/users/octocat/events/public", "/users/octocat/repos":
			_ = json.NewEncoder(w).Encode([]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ghSrv.Close)

	c, err := cache.New(filepath.Join(t.TempDir(), "cache.db"), 300)
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	d := &Deps{Cache: c, GithubAPIURL: ghSrv.URL}
	router := chi.NewRouter()
	Register(router, d)

	get := func(token string) map[string]any {
		req := httptest.NewRequest(http.MethodGet, "/api/analyze?url=https://github.com/octocat", nil)
		req.Header.Set("X-GitHub-Token", token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return body["data"].(map[string]any)
	}

	first := get("token-a")
	if first["cached"] != false {
		t.Fatalf("first request should not be cached: %+v", first)
	}
	callsAfterFirst := calls

	second := get("token-b")
	if second["cached"] != false {
		t.Errorf("a different token must not hit token-a's cache entry, got cached=%v", second["cached"])
	}
	if calls <= callsAfterFirst {
		t.Errorf("expected a real upstream fetch for token-b, calls stayed at %d", calls)
	}

	third := get("token-a")
	if third["cached"] != true {
		t.Errorf("same token as first request should hit its own cache entry, got cached=%v", third["cached"])
	}
}
