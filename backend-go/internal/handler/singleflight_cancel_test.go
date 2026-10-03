package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestSingleflightLeaderCancelDoesNotFailFollowers covers Issue #152:
// fromCache's shared fetch used to run with the triggering (leader) caller's
// request context. If that caller disconnected mid-fetch, the resulting
// context.Canceled error was returned to every other request waiting on the
// same singleflight key — one client leaving broke everyone else's request
// for the same resource.
func TestSingleflightLeaderCancelDoesNotFailFollowers(t *testing.T) {
	release := make(chan struct{})
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {
		<-release // hold the upstream response until the test says so
		w.WriteHeader(http.StatusOK)
		switch req.URL.Path {
		case "/users/octocat":
			_ = json.NewEncoder(w).Encode(map[string]any{"login": "octocat", "public_repos": 1})
		case "/users/octocat/events/public", "/users/octocat/repos":
			_ = json.NewEncoder(w).Encode([]any{})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	})

	do := func(ctx context.Context) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/users/octocat/activity", nil)
		req = req.WithContext(ctx)
		req.Header.Set("X-GitHub-Token", "leader-and-follower-share-this-token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	var wg sync.WaitGroup
	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	var followerResp *httptest.ResponseRecorder

	wg.Add(1)
	go func() {
		defer wg.Done()
		do(leaderCtx) // becomes the singleflight leader; its result is discarded
	}()

	// Give the leader time to register as the singleflight leader before the
	// follower joins the same in-flight call.
	time.Sleep(20 * time.Millisecond)
	cancelLeader()

	wg.Add(1)
	go func() {
		defer wg.Done()
		followerResp = do(context.Background())
	}()

	time.Sleep(20 * time.Millisecond)
	close(release) // let the upstream call complete
	wg.Wait()

	if followerResp.Code != http.StatusOK {
		t.Fatalf("follower status = %d, want 200 (leader's cancellation must not affect it), body=%s",
			followerResp.Code, followerResp.Body.String())
	}
}

// TestConcurrentJoinedCallersDoNotRaceOnResult covers the bug the above test
// uncovered under -race: fetchGroup hands the identical *model.UserActivity
// to every caller joined on the same in-flight fetch, and each handler wrote
// `.Cached` directly onto it — a data race when they run concurrently.
func TestConcurrentJoinedCallersDoNotRaceOnResult(t *testing.T) {
	release := make(chan struct{})
	_, r := newTestDeps(t, func(w http.ResponseWriter, req *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
		switch req.URL.Path {
		case "/users/octocat":
			_ = json.NewEncoder(w).Encode(map[string]any{"login": "octocat", "public_repos": 1})
		case "/users/octocat/events/public", "/users/octocat/repos":
			_ = json.NewEncoder(w).Encode([]any{})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	})

	const n = 10
	var wg sync.WaitGroup
	results := make([]*httptest.ResponseRecorder, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/api/users/octocat/activity", nil)
			req.Header.Set("X-GitHub-Token", "shared")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			results[i] = w
		}(i)
	}
	time.Sleep(20 * time.Millisecond) // let all n join the same in-flight fetch
	close(release)
	wg.Wait()

	for i, w := range results {
		if w.Code != http.StatusOK {
			t.Errorf("request %d: status = %d, want 200, body=%s", i, w.Code, w.Body.String())
		}
	}
}
