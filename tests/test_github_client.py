import httpx
import pytest

from app.github_client import GitHubClient, GitHubError


def make_client(handler) -> GitHubClient:
    transport = httpx.MockTransport(handler)
    http = httpx.Client(transport=transport)
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
