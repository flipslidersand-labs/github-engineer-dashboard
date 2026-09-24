from app.url_parser import UrlType, parse_github_url


def test_valid_user_url():
    result = parse_github_url("https://github.com/octocat")
    assert result.type == UrlType.USER
    assert result.params["username"] == "octocat"


def test_valid_repo_url():
    result = parse_github_url("https://github.com/torvalds/linux")
    assert result.type == UrlType.REPO
    assert result.params == {"username": "torvalds", "repo": "linux"}


def test_valid_pr_url():
    result = parse_github_url("https://github.com/torvalds/linux/pull/1")
    assert result.type == UrlType.PR
    assert result.params["number"] == "1"


def test_valid_org_url():
    result = parse_github_url("https://github.com/orgs/github")
    assert result.type == UrlType.ORG
    assert result.params["org"] == "github"


def test_non_github_host_is_unknown():
    result = parse_github_url("https://example.com/foo")
    assert result.type == UrlType.UNKNOWN


# ── Issue #129: reject path-traversal-shaped / reserved identifiers ────────


def test_dotdot_username_rejected():
    result = parse_github_url("https://github.com/..")
    assert result.type == UrlType.UNKNOWN


def test_dotdot_repo_rejected():
    result = parse_github_url("https://github.com/octocat/..")
    assert result.type == UrlType.UNKNOWN


def test_dotdot_owner_in_pr_url_rejected():
    result = parse_github_url("https://github.com/../linux/pull/1")
    assert result.type == UrlType.UNKNOWN


def test_leading_hyphen_username_rejected():
    result = parse_github_url("https://github.com/-octocat")
    assert result.type == UrlType.UNKNOWN


def test_leading_dot_username_rejected():
    result = parse_github_url("https://github.com/.octocat")
    assert result.type == UrlType.UNKNOWN


def test_org_with_invalid_name_rejected():
    result = parse_github_url("https://github.com/orgs/..")
    assert result.type == UrlType.UNKNOWN


def test_repo_name_with_slash_like_traversal_rejected():
    # url-encoded or literal ".." segments never reach here as one path part
    # (urlparse already splits on "/"), but a repo segment that IS ".."
    # must still be rejected explicitly.
    result = parse_github_url("https://github.com/octocat/../pull/1")
    assert result.type == UrlType.UNKNOWN
