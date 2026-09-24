package github

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New("test-token", srv.URL), srv
}

func TestGetRateLimitParsesCore(t *testing.T) {
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rate_limit" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resources": map[string]any{
				"core": map[string]any{"limit": 5000, "remaining": 4999, "used": 1, "reset": 111},
			},
		})
	})

	rl, err := client.GetRateLimit()
	if err != nil {
		t.Fatalf("GetRateLimit: %v", err)
	}
	if rl.Remaining != 4999 || rl.Limit != 5000 {
		t.Errorf("got %+v", rl)
	}
}

func TestGetRateLimitReturnsErrorOnNon2xx(t *testing.T) {
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "rate limit exceeded"})
	})

	_, err := client.GetRateLimit()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	ghErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if ghErr.StatusCode != http.StatusForbidden {
		t.Errorf("StatusCode = %d, want %d", ghErr.StatusCode, http.StatusForbidden)
	}
	if ghErr.Message != "rate limit exceeded" {
		t.Errorf("Message = %q", ghErr.Message)
	}
}

func TestGetUserActivityAggregatesEvents(t *testing.T) {
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/users/octocat":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"login": "octocat", "name": "The Octocat",
				"public_repos": 8, "followers": 100, "following": 5,
				"created_at": "2011-01-01T00:00:00Z",
			})
		case "/users/octocat/events/public":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"type": "PushEvent"},
				{"type": "PushEvent"},
				{"type": "IssuesEvent"},
				{"type": nil},
			})
		case "/users/octocat/repos":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	activity, err := client.GetUserActivity("octocat")
	if err != nil {
		t.Fatalf("GetUserActivity: %v", err)
	}
	if activity.Username != "octocat" || activity.PublicRepos != 8 {
		t.Errorf("got %+v", activity)
	}
	if activity.EventCounts["PushEvent"] != 2 {
		t.Errorf("PushEvent count = %d, want 2", activity.EventCounts["PushEvent"])
	}
	if activity.EventCounts["Unknown"] != 1 {
		t.Errorf("Unknown count = %d, want 1", activity.EventCounts["Unknown"])
	}
	if activity.TotalEvents != 4 {
		t.Errorf("TotalEvents = %d, want 4", activity.TotalEvents)
	}
}

func TestGetUserActivityPropagatesUserFetchError(t *testing.T) {
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/users/ghost" {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Not Found"})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]any{})
	})

	_, err := client.GetUserActivity("ghost")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	ghErr, ok := err.(*Error)
	if !ok || ghErr.StatusCode != http.StatusNotFound {
		t.Errorf("got err=%v", err)
	}
}

// tryGet's failure tolerance is exercised indirectly: GetRepo's contributor/
// language/PR/release/participation sub-fetches all use tryGet and must
// degrade to zero values rather than failing the whole request.
func TestGetRepoToleratesSubFetchFailures(t *testing.T) {
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/octocat/hello" {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"owner":             map[string]string{"login": "octocat"},
				"name":              "hello",
				"full_name":         "octocat/hello",
				"stargazers_count":  42,
				"forks_count":       3,
				"open_issues_count": 1,
				"updated_at":        "2024-01-01T00:00:00Z",
			})
			return
		}
		// Every sub-resource (contributors/languages/pulls/releases/participation)
		// fails; GetRepo must still return the base repo data.
		w.WriteHeader(http.StatusInternalServerError)
	})

	repo, err := client.GetRepo("octocat", "hello")
	if err != nil {
		t.Fatalf("GetRepo: %v", err)
	}
	if repo.Stars != 42 || repo.Name != "hello" {
		t.Errorf("got %+v", repo)
	}
	if repo.Contributors == nil || len(repo.Contributors) != 0 {
		t.Errorf("Contributors = %v, want empty slice", repo.Contributors)
	}
	if repo.OpenPRCount != 0 {
		t.Errorf("OpenPRCount = %d, want 0", repo.OpenPRCount)
	}
}
