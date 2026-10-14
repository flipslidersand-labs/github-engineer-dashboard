package github

import (
	"context"
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

	rl, err := client.GetRateLimit(context.Background())
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

	_, err := client.GetRateLimit(context.Background())
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

	activity, err := client.GetUserActivity(context.Background(), "octocat")
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

	_, err := client.GetUserActivity(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	ghErr, ok := err.(*Error)
	if !ok || ghErr.StatusCode != http.StatusNotFound {
		t.Errorf("got err=%v", err)
	}
}

func TestGetPRAggregatesReviewsAndFiles(t *testing.T) {
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/repos/octocat/hello/pulls/42":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number":          42,
				"title":           "Add feature",
				"state":           "open",
				"user":            map[string]string{"login": "octocat"},
				"base":            map[string]string{"ref": "main"},
				"head":            map[string]string{"ref": "feature"},
				"additions":       10,
				"deletions":       2,
				"changed_files":   3,
				"comments":        1,
				"review_comments": 4,
				"created_at":      "2024-01-01T00:00:00Z",
			})
		case "/repos/octocat/hello/pulls/42/reviews":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"user": map[string]string{"login": "reviewer1"}, "submitted_at": "2024-01-01T05:00:00Z"},
				{"user": map[string]string{"login": "reviewer2"}, "submitted_at": "2024-01-01T10:00:00Z"},
			})
		case "/repos/octocat/hello/pulls/42/files":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"filename": "main.go", "additions": 8, "deletions": 1},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	pr, err := client.GetPR(context.Background(), "octocat", "hello", 42)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr.Number != 42 || pr.Title != "Add feature" || pr.State != "open" {
		t.Errorf("got %+v", pr)
	}
	if len(pr.Reviewers) != 2 {
		t.Errorf("Reviewers = %v, want 2 entries", pr.Reviewers)
	}
	if pr.ReviewWaitHours == nil || *pr.ReviewWaitHours != 5 {
		t.Errorf("ReviewWaitHours = %v, want 5", pr.ReviewWaitHours)
	}
	if len(pr.ChangedFilesDetail) != 1 || pr.ChangedFilesDetail[0].Filename != "main.go" {
		t.Errorf("ChangedFilesDetail = %+v", pr.ChangedFilesDetail)
	}
}

func TestGetPRMergedStateOverridesOpen(t *testing.T) {
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/repos/octocat/hello/pulls/7":
			mergedAt := "2024-02-01T00:00:00Z"
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number":     7,
				"state":      "closed",
				"merged_at":  mergedAt,
				"user":       map[string]string{"login": "octocat"},
				"base":       map[string]string{"ref": "main"},
				"head":       map[string]string{"ref": "feature"},
				"created_at": "2024-01-01T00:00:00Z",
			})
		default:
			_ = json.NewEncoder(w).Encode([]any{})
		}
	})

	pr, err := client.GetPR(context.Background(), "octocat", "hello", 7)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr.State != "merged" {
		t.Errorf("State = %q, want merged", pr.State)
	}
	if pr.MergedAt == nil || *pr.MergedAt != "2024-02-01T00:00:00Z" {
		t.Errorf("MergedAt = %v", pr.MergedAt)
	}
}

func TestGetPRPropagatesFetchError(t *testing.T) {
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/octocat/hello/pulls/99" {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Not Found"})
			return
		}
		_ = json.NewEncoder(w).Encode([]any{})
	})

	_, err := client.GetPR(context.Background(), "octocat", "hello", 99)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	ghErr, ok := err.(*Error)
	if !ok || ghErr.StatusCode != http.StatusNotFound {
		t.Errorf("got err=%v", err)
	}
}

func TestGetIssueAggregatesTimelineAndLabels(t *testing.T) {
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/repos/octocat/hello/issues/5":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number":     5,
				"title":      "Bug report",
				"state":      "open",
				"user":       map[string]string{"login": "octocat"},
				"labels":     []map[string]string{{"name": "bug"}, {"name": "p1"}},
				"assignees":  []map[string]string{{"login": "octocat"}},
				"comments":   3,
				"created_at": "2024-01-01T00:00:00Z",
			})
		case "/repos/octocat/hello/issues/5/timeline":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"event": "cross-referenced",
					"source": map[string]any{
						"issue": map[string]any{"number": 10, "pull_request": map[string]any{}},
					},
				},
				{"event": "labeled"},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	issue, err := client.GetIssue(context.Background(), "octocat", "hello", 5)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if issue.Number != 5 || issue.Title != "Bug report" || issue.Author != "octocat" {
		t.Errorf("got %+v", issue)
	}
	if len(issue.Labels) != 2 || issue.Labels[0] != "bug" {
		t.Errorf("Labels = %v", issue.Labels)
	}
	if len(issue.Assignees) != 1 || issue.Assignees[0] != "octocat" {
		t.Errorf("Assignees = %v", issue.Assignees)
	}
	if len(issue.RelatedPRs) != 1 || issue.RelatedPRs[0] != 10 {
		t.Errorf("RelatedPRs = %v, want [10]", issue.RelatedPRs)
	}
}

func TestGetIssuePropagatesFetchError(t *testing.T) {
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/octocat/hello/issues/404" {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Not Found"})
			return
		}
		_ = json.NewEncoder(w).Encode([]any{})
	})

	_, err := client.GetIssue(context.Background(), "octocat", "hello", 404)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	ghErr, ok := err.(*Error)
	if !ok || ghErr.StatusCode != http.StatusNotFound {
		t.Errorf("got err=%v", err)
	}
}

// TestAggregateRepoListSetsTruncatedAtPageCap exercises the page-cap branch
// of aggregateRepoList: when every one of reposMaxPages pages comes back full
// (reposPageSize items), the loop stops at the cap and reports Truncated=true
// instead of assuming there is no more data upstream.
func TestAggregateRepoListSetsTruncatedAtPageCap(t *testing.T) {
	fullRepo := map[string]any{"stargazers_count": 1, "forks_count": 0, "language": "Go", "fork": false}
	fullPage := make([]map[string]any, reposPageSize)
	for i := range fullPage {
		fullPage[i] = fullRepo
	}

	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Every page comes back full-sized, so the loop never sees a
		// short page to naturally stop on and must hit the page cap.
		_ = json.NewEncoder(w).Encode(fullPage)
	})

	summary, err := client.GetUserReposSummary(context.Background(), "octocat", false)
	if err != nil {
		t.Fatalf("GetUserReposSummary: %v", err)
	}
	if !summary.Truncated {
		t.Error("Truncated = false, want true when every page hits the cap")
	}
	if summary.RepoCount != reposPageSize*reposMaxPages {
		t.Errorf("RepoCount = %d, want %d", summary.RepoCount, reposPageSize*reposMaxPages)
	}
}

// TestAggregateRepoListNotTruncatedOnShortPage verifies the common case: a
// short final page ends pagination without setting Truncated.
func TestAggregateRepoListNotTruncatedOnShortPage(t *testing.T) {
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"stargazers_count": 5, "forks_count": 1, "language": "Go", "fork": false},
		})
	})

	summary, err := client.GetUserReposSummary(context.Background(), "octocat", false)
	if err != nil {
		t.Fatalf("GetUserReposSummary: %v", err)
	}
	if summary.Truncated {
		t.Error("Truncated = true, want false for a single short page")
	}
	if summary.RepoCount != 1 {
		t.Errorf("RepoCount = %d, want 1", summary.RepoCount)
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

	repo, err := client.GetRepo(context.Background(), "octocat", "hello")
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

// TestGetWithAcceptCapsResponseBodySize verifies that a response body larger
// than the requested limit is rejected with a clean error instead of being
// read in full. It calls getWithAccept directly with a small test-sized
// limit so the fixture doesn't need to allocate anything near the
// production-sized 10MB/200KB constants to exercise the cap.
func TestGetWithAcceptCapsResponseBodySize(t *testing.T) {
	const testLimit = 100 // bytes

	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write well over the limit; LimitReader should stop the client from
		// reading it all, and the client should still report a bounded
		// error rather than hang or attempt to unmarshal a partial body.
		oversized := make([]byte, testLimit*10)
		_, _ = w.Write(oversized)
	})

	body, err := client.getWithAccept(context.Background(), "/big", "application/vnd.github+json", testLimit)
	if err == nil {
		t.Fatalf("getWithAccept: expected error for oversized body, got nil (len=%d)", len(body))
	}
	if body != nil {
		t.Errorf("getWithAccept: expected nil body on cap error, got %d bytes", len(body))
	}
}

// TestGetWithAcceptAllowsBodyAtLimit verifies a body exactly at the limit is
// still accepted (only bodies exceeding the limit are rejected).
func TestGetWithAcceptAllowsBodyAtLimit(t *testing.T) {
	const testLimit = 100 // bytes

	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, testLimit))
	})

	body, err := client.getWithAccept(context.Background(), "/exact", "application/vnd.github+json", testLimit)
	if err != nil {
		t.Fatalf("getWithAccept: unexpected error for body at limit: %v", err)
	}
	if len(body) != testLimit {
		t.Errorf("getWithAccept: got %d bytes, want %d", len(body), testLimit)
	}
}
