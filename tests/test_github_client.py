import httpx
import pytest

from app.github_client import GitHubClient, GitHubError


def make_client(handler) -> GitHubClient:
    transport = httpx.MockTransport(handler)
    # follow_redirects=True mirrors the shared client built in app/main.py's
    # lifespan (Issue #157) so tests exercise the same redirect behavior.
    http = httpx.Client(transport=transport, follow_redirects=True)
    return GitHubClient("test-token", client=http)


def test_get_rate_limit_parses_core():
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/rate_limit"
        assert request.headers["Authorization"] == "Bearer test-token"
        return httpx.Response(
            200,
            json={
                "resources": {"core": {"limit": 5000, "remaining": 4999, "used": 1, "reset": 111}}
            },
        )

    client = make_client(handler)
    core = client.get_rate_limit()
    assert core["remaining"] == 4999
    assert core["limit"] == 5000


def test_get_user_activity_aggregates_events():
    def handler(request: httpx.Request) -> httpx.Response:
        if request.url.path == "/users/octocat":
            return httpx.Response(
                200,
                json={
                    "login": "octocat",
                    "name": "The Octocat",
                    "public_repos": 8,
                    "followers": 100,
                    "following": 5,
                },
            )
        if request.url.path == "/users/octocat/events/public":
            return httpx.Response(
                200,
                json=[
                    {"type": "PushEvent"},
                    {"type": "PushEvent"},
                    {"type": "IssuesEvent"},
                    {"type": None},
                ],
            )
        return httpx.Response(404, json={"message": "not found"})

    client = make_client(handler)
    activity = client.get_user_activity("octocat")
    assert activity["username"] == "octocat"
    assert activity["name"] == "The Octocat"
    assert activity["public_repos"] == 8
    assert activity["event_counts"]["PushEvent"] == 2
    assert activity["event_counts"]["IssuesEvent"] == 1
    assert activity["event_counts"]["Unknown"] == 1
    assert activity["total_events"] == 4


def test_error_raises_github_error():
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(404, json={"message": "Not Found"})

    client = make_client(handler)
    with pytest.raises(GitHubError) as exc:
        client.get_user_activity("ghost")
    assert exc.value.status_code == 404
    assert "Not Found" in exc.value.message


def test_close_does_not_close_injected_client():
    """An injected (shared, connection-pooled) client must survive close() —
    it's reused across requests, unlike a client GitHubClient creates itself."""
    shared = httpx.Client(transport=httpx.MockTransport(lambda r: httpx.Response(200)))
    gh = GitHubClient("token", client=shared)
    gh.close()
    assert not shared.is_closed


def test_close_closes_own_client():
    gh = GitHubClient("token")
    gh.close()
    assert gh._client.is_closed


def test_get_pr_diff_returns_full_diff_under_limit():
    body = "diff --git a/foo.py b/foo.py\n+print('hello')\n"

    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/repos/octocat/hello/pulls/1"
        assert request.headers["Accept"] == "application/vnd.github.v3.diff"
        return httpx.Response(200, text=body)

    client = make_client(handler)
    diff = client.get_pr_diff("octocat", "hello", 1)
    assert diff == body


def test_get_pr_diff_truncates_at_max_bytes():
    body = "x" * 1000

    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, text=body)

    client = make_client(handler)
    diff = client.get_pr_diff("octocat", "hello", 1, max_bytes=100)
    assert diff.startswith("x" * 100)
    assert "truncated" in diff
    assert len(diff) < len(body)


def test_get_repo_follows_redirect_on_renamed_repo():
    """A renamed/transferred repo 301s to its new location (Issue #157);
    the shared client must follow it instead of treating the redirect
    itself (often empty-bodied) as a successful < 400 response."""

    def handler(request: httpx.Request) -> httpx.Response:
        if request.url.path == "/repos/octocat/old-name":
            return httpx.Response(
                301,
                headers={"Location": "https://api.github.com/repos/octocat/new-name"},
            )
        if request.url.path == "/repos/octocat/new-name":
            return httpx.Response(
                200,
                json={
                    "owner": {"login": "octocat"},
                    "name": "new-name",
                    "full_name": "octocat/new-name",
                },
            )
        return httpx.Response(404, json={"message": "not found"})

    client = make_client(handler)
    repo = client.get_repo("octocat", "old-name")
    assert repo["name"] == "new-name"
    assert repo["full_name"] == "octocat/new-name"


def test_non_json_error_body_raises_clean_github_error():
    """An HTML error page from a proxy/edge failure (e.g. Render 502) must
    not let JSONDecodeError escape uncaught — it should raise GitHubError
    with the raw text as the message instead (Issue #157)."""

    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(502, text="<html><body>Bad Gateway</body></html>")

    client = make_client(handler)
    with pytest.raises(GitHubError) as exc:
        client.get_rate_limit()
    assert exc.value.status_code == 502
    assert "Bad Gateway" in exc.value.message


def test_non_json_error_body_in_pr_diff_raises_clean_github_error():
    """Same non-JSON-error-body guard applies to the streaming _get used by
    get_pr_diff, which has its own status-check/raise path."""

    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(502, text="<html><body>Bad Gateway</body></html>")

    client = make_client(handler)
    with pytest.raises(GitHubError) as exc:
        client.get_pr_diff("octocat", "hello", 1)
    assert exc.value.status_code == 502
    assert "Bad Gateway" in exc.value.message
