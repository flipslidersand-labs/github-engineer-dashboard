package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Issue #158: REQUIRE_CLIENT_TOKEN disables the GITHUB_TOKEN fallback.
func TestRequireClientToken(t *testing.T) {
	cases := []struct {
		name    string
		require bool
		header  string
		want    int
	}{
		{"fallback kept by default", false, "", http.StatusOK},
		{"required: no header -> 401 despite server token", true, "", http.StatusUnauthorized},
		{"required: header accepted", true, "abc", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, r := newTestDeps(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"resources":{"core":{"limit":1,"remaining":1,"used":0,"reset":1}}}`))
			})
			d.GithubToken = "server-token"
			d.RequireClientToken = tc.require

			req := httptest.NewRequest(http.MethodGet, "/api/rate-limit", nil)
			if tc.header != "" {
				req.Header.Set("X-GitHub-Token", tc.header)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
