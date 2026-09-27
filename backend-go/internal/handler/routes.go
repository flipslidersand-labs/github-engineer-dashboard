// Package handler registers all HTTP routes for the dashboard API.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/singleflight"

	"github.com/flipslidersand/github-engineer-dashboard/backend-go/internal/cache"
	gh "github.com/flipslidersand/github-engineer-dashboard/backend-go/internal/github"
	"github.com/flipslidersand/github-engineer-dashboard/backend-go/internal/model"
)

const version = "0.1.0"

// Deps holds shared dependencies injected into each handler.
type Deps struct {
	Cache        *cache.Cache
	GithubToken  string
	GithubAPIURL string
	// HTTPClient is shared across all requests so GitHub API calls reuse
	// pooled connections instead of a fresh TCP/TLS handshake per request.
	// Falls back to gh.New's own client if left nil (e.g. in older tests).
	HTTPClient *http.Client
}

// Register mounts all routes on r.
func Register(r chi.Router, d *Deps) {
	r.Get("/healthz", d.healthz)
	r.Get("/api/rate-limit", d.requireToken(d.rateLimit))
	r.Get("/api/users/{username}/activity", d.requireToken(d.userActivity))
	r.Get("/api/analyze", d.requireToken(d.analyze))
	r.Get("/api/summary", d.requireToken(d.summary))
}

// ── middleware ────────────────────────────────────────────────────────────────

func (d *Deps) requireToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-GitHub-Token")
		if token == "" {
			token = d.GithubToken
		}
		if token == "" {
			writeError(w, http.StatusUnauthorized,
				"GitHub token required. Provide the 'X-GitHub-Token' header.")
			return
		}
		// Store token in request context via chi's context helpers isn't needed;
		// handlers call newClient(r, d) which resolves the token.
		r.Header.Set("X-GitHub-Token", token) // normalise for newClient
		next(w, r)
	}
}

func newClient(r *http.Request, d *Deps) *gh.Client {
	token := r.Header.Get("X-GitHub-Token")
	if token == "" {
		token = d.GithubToken
	}
	if d.HTTPClient != nil {
		return gh.NewWithClient(token, d.GithubAPIURL, d.HTTPClient)
	}
	return gh.New(token, d.GithubAPIURL)
}

// ── handlers ─────────────────────────────────────────────────────────────────

func (d *Deps) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, model.Health{Status: "ok", Version: version})
}

func (d *Deps) rateLimit(w http.ResponseWriter, r *http.Request) {
	client := newClient(r, d)
	rl, err := client.GetRateLimit(r.Context())
	if err != nil {
		writeGitHubError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rl)
}

func (d *Deps) userActivity(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	client := newClient(r, d)
	v, cached, err := fromCache(d.Cache, r.Context(), client.TokenFingerprint+":activity:"+strings.ToLower(username),
		func(ctx context.Context) (*model.UserActivity, error) { return client.GetUserActivity(ctx, username) })
	if err != nil {
		writeGitHubError(w, err)
		return
	}
	v.Cached = cached
	writeJSON(w, http.StatusOK, v)
}

func (d *Deps) analyze(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		writeError(w, http.StatusUnprocessableEntity, "url query parameter required")
		return
	}

	parsed := parseGitHubURL(rawURL)
	client := newClient(r, d)

	writeAnalyze := func(typ string, v any, err error) {
		if err != nil {
			writeGitHubError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, model.AnalyzeResult{Type: typ, URL: rawURL, Data: v})
	}

	switch parsed.typ {
	case urlTypeUser:
		u := parsed.username
		v, cached, err := fromCache(d.Cache, r.Context(), client.TokenFingerprint+":activity:"+strings.ToLower(u),
			func(ctx context.Context) (*model.UserActivity, error) { return client.GetUserActivity(ctx, u) })
		if err == nil {
			v.Cached = cached
		}
		writeAnalyze("user", v, err)

	case urlTypeRepo:
		u, repo := parsed.username, parsed.repo
		key := fmt.Sprintf("%s:repo:%s/%s", client.TokenFingerprint, strings.ToLower(u), strings.ToLower(repo))
		v, cached, err := fromCache(d.Cache, r.Context(), key,
			func(ctx context.Context) (*model.RepoInfo, error) { return client.GetRepo(ctx, u, repo) })
		if err == nil {
			v.Cached = cached
		}
		writeAnalyze("repo", v, err)

	case urlTypePR:
		u, repo, num := parsed.username, parsed.repo, parsed.number
		key := fmt.Sprintf("%s:pr:%s/%s/%d", client.TokenFingerprint, strings.ToLower(u), strings.ToLower(repo), num)
		v, cached, err := fromCache(d.Cache, r.Context(), key,
			func(ctx context.Context) (*model.PRInfo, error) { return client.GetPR(ctx, u, repo, num) })
		if err == nil {
			v.Cached = cached
		}
		writeAnalyze("pr", v, err)

	case urlTypeIssue:
		u, repo, num := parsed.username, parsed.repo, parsed.number
		key := fmt.Sprintf("%s:issue:%s/%s/%d", client.TokenFingerprint, strings.ToLower(u), strings.ToLower(repo), num)
		v, cached, err := fromCache(d.Cache, r.Context(), key,
			func(ctx context.Context) (*model.IssueInfo, error) { return client.GetIssue(ctx, u, repo, num) })
		if err == nil {
			v.Cached = cached
		}
		writeAnalyze("issue", v, err)

	default:
		writeError(w, http.StatusUnprocessableEntity,
			"Unsupported URL. Provide a GitHub user, repository, PR, or issue URL.")
	}
}

func (d *Deps) summary(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		writeError(w, http.StatusUnprocessableEntity, "url query parameter required")
		return
	}
	excludeForks := r.URL.Query().Get("exclude_forks") == "true"

	parsed := parseGitHubURL(rawURL)
	client := newClient(r, d)

	var ownerKey string
	switch parsed.typ {
	case urlTypeUser:
		ownerKey = "user:" + strings.ToLower(parsed.username)
	case urlTypeOrg:
		ownerKey = "org:" + strings.ToLower(parsed.org)
	default:
		writeError(w, http.StatusUnprocessableEntity,
			"Summary requires a GitHub user or organization URL.")
		return
	}

	key := fmt.Sprintf("%s:summary:%s:forks=%d", client.TokenFingerprint, ownerKey, boolToInt(excludeForks))
	var fetchFn func(context.Context) (*model.CrossRepoSummary, error)
	if parsed.typ == urlTypeUser {
		fetchFn = func(ctx context.Context) (*model.CrossRepoSummary, error) {
			return client.GetUserReposSummary(ctx, parsed.username, excludeForks)
		}
	} else {
		fetchFn = func(ctx context.Context) (*model.CrossRepoSummary, error) {
			return client.GetOrgReposSummary(ctx, parsed.org, excludeForks)
		}
	}

	v, cached, err := fromCache(d.Cache, r.Context(), key, fetchFn)
	if err != nil {
		writeGitHubError(w, err)
		return
	}
	v.Cached = cached
	writeJSON(w, http.StatusOK, v)
}

// ── URL parser ────────────────────────────────────────────────────────────────

type urlType int

const (
	urlTypeUnknown urlType = iota
	urlTypeUser
	urlTypeOrg
	urlTypeRepo
	urlTypePR
	urlTypeIssue
)

type parsedURL struct {
	typ      urlType
	username string
	org      string
	repo     string
	number   int
}

// GitHub username/org: alphanumeric, may contain single hyphens, cannot
// begin or end with a hyphen, max 39 chars. Rejects "..", "-foo", "", etc. —
// these values get string-concatenated straight into upstream API paths
// (GithubAPIURL, which may point at an internal GHE host), so a value like
// ".." must never reach that point unvalidated (Issue #129).
var ownerRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)

// GitHub repo name: alphanumeric plus . _ -, 1-100 chars, but "." and ".."
// are reserved and must be rejected explicitly.
var repoRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)

func isValidOwner(s string) bool {
	return ownerRe.MatchString(s)
}

func isValidRepo(s string) bool {
	return repoRe.MatchString(s) && s != "." && s != ".."
}

func parseGitHubURL(raw string) parsedURL {
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return parsedURL{typ: urlTypeUnknown}
	}
	host := strings.ToLower(u.Hostname())
	if host != "github.com" && host != "www.github.com" {
		return parsedURL{typ: urlTypeUnknown}
	}

	var parts []string
	for _, p := range strings.Split(u.Path, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}

	if len(parts) >= 2 && parts[0] == "orgs" {
		if !isValidOwner(parts[1]) {
			return parsedURL{typ: urlTypeUnknown}
		}
		return parsedURL{typ: urlTypeOrg, org: parts[1]}
	}
	if len(parts) == 1 {
		if !isValidOwner(parts[0]) {
			return parsedURL{typ: urlTypeUnknown}
		}
		return parsedURL{typ: urlTypeUser, username: parts[0]}
	}
	if len(parts) >= 4 && isValidOwner(parts[0]) && isValidRepo(parts[1]) {
		n, err := strconv.Atoi(parts[3])
		if err == nil && n > 0 {
			switch parts[2] {
			case "pull":
				return parsedURL{typ: urlTypePR, username: parts[0], repo: parts[1], number: n}
			case "issues":
				return parsedURL{typ: urlTypeIssue, username: parts[0], repo: parts[1], number: n}
			}
		}
	}
	if len(parts) >= 2 {
		if !isValidOwner(parts[0]) || !isValidRepo(parts[1]) {
			return parsedURL{typ: urlTypeUnknown}
		}
		return parsedURL{typ: urlTypeRepo, username: parts[0], repo: parts[1]}
	}
	return parsedURL{typ: urlTypeUnknown}
}

// ── cache helper ──────────────────────────────────────────────────────────────

// fromCache returns cached data (wasCached=true) or calls fetch, stores result,
// and returns fresh data (wasCached=false). On fetch error returns nil, false, err.
// fetchGroup deduplicates concurrent fetches for the same cache key so that
// simultaneous requests for the same resource (e.g. two people opening the
// same PR review at once) trigger a single upstream call instead of one per
// request — important where the "fetch" is a paid LLM call (see /api/review).
var fetchGroup singleflight.Group

// partialResult is implemented by response types that can be built from an
// incomplete set of sub-fetches and therefore should not be cached as-is.
type partialResult interface {
	IsPartial() bool
}

// fetchTimeout bounds the detached context used for a shared singleflight
// fetch (see fromCache) — long enough for a real upstream call, short enough
// that a truly stuck fetch doesn't leak the goroutine forever.
const fetchTimeout = 30 * time.Second

func fromCache[T any](c *cache.Cache, ctx context.Context, key string, fetch func(context.Context) (*T, error)) (*T, bool, error) {
	var dst T
	if c.Get(key, &dst) {
		return &dst, true, nil
	}
	v, err, _ := fetchGroup.Do(key, func() (any, error) {
		// Re-check: another goroutine may have populated the cache while we
		// were waiting for our turn to run fetch().
		var dst2 T
		if c.Get(key, &dst2) {
			return &dst2, nil
		}
		// Detach from the triggering caller's context: this fetch is shared
		// via fetchGroup across every concurrent request for this key, so
		// one caller disconnecting must not cancel it (and therefore fail
		// every other waiter) — Issue #152. Still bounded by a timeout so a
		// stuck upstream can't hang this goroutine indefinitely.
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), fetchTimeout)
		defer cancel()
		result, err := fetch(fetchCtx)
		if err != nil {
			return nil, err
		}
		if p, ok := any(result).(partialResult); !ok || !p.IsPartial() {
			_ = c.Set(key, result)
		}
		return result, nil
	})
	if err != nil {
		return nil, false, err
	}
	// v may be the exact same *T shared across every caller that joined this
	// fetch via fetchGroup (singleflight hands the identical result to all
	// of them). Callers set .Cached on the pointer they get back, so return
	// each caller its own copy — otherwise concurrent joined callers race on
	// that field (Issue #152).
	result := *v.(*T)
	return &result, false, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeGitHubError(w http.ResponseWriter, err error) {
	var e *gh.Error
	if ghErr, ok := err.(*gh.Error); ok {
		e = ghErr
		status := e.StatusCode
		if status == 403 {
			status = 429
		}
		writeError(w, status, e.Message)
		return
	}
	writeError(w, http.StatusBadGateway, err.Error())
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
