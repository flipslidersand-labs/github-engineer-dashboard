"""Parse a GitHub URL and return its type and extracted parameters."""

from __future__ import annotations

import re
from dataclasses import dataclass
from enum import Enum
from urllib.parse import urlparse

# GitHub username/org: alphanumeric, may contain single hyphens, cannot begin
# or end with a hyphen, max 39 chars. Rejects "..", "-foo", "", etc. — these
# values get string-concatenated straight into upstream API paths
# (GITHUB_API_URL, which may point at an internal GHE host), so a value like
# ".." must never reach that point unvalidated (Issue #129).
_OWNER_RE = re.compile(r"^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$")
# GitHub repo name: alphanumeric plus . _ -, 1-100 chars, but "." and ".."
# are reserved and must be rejected explicitly.
_REPO_RE = re.compile(r"^[A-Za-z0-9._-]{1,100}$")
# PR/issue numbers: strict ASCII digits only. str.isdigit() returns True for
# some Unicode characters (e.g. superscript "²", U+00B2) that int() cannot
# parse, which used to leak through here and crash with an unhandled
# ValueError at the int() call site instead of being rejected as UNKNOWN
# (Issue #160).
_NUMBER_RE = re.compile(r"[0-9]+")


def _is_valid_owner(name: str) -> bool:
    return bool(_OWNER_RE.match(name))


def _is_valid_repo(name: str) -> bool:
    return bool(_REPO_RE.match(name)) and name not in (".", "..")


class UrlType(str, Enum):
    USER = "user"
    ORG = "org"
    REPO = "repo"
    PR = "pr"
    ISSUE = "issue"
    UNKNOWN = "unknown"


@dataclass(frozen=True)
class ParsedGitHubUrl:
    type: UrlType
    params: dict[str, str]


def parse_github_url(raw: str) -> ParsedGitHubUrl:
    """Parse a GitHub URL into a typed result.

    Accepts bare paths (e.g. "github.com/user/repo") as well as full URLs.
    """
    if not raw.startswith(("http://", "https://")):
        raw = "https://" + raw

    try:
        parsed = urlparse(raw)
    except Exception:
        return ParsedGitHubUrl(type=UrlType.UNKNOWN, params={})

    if parsed.hostname not in ("github.com", "www.github.com"):
        return ParsedGitHubUrl(type=UrlType.UNKNOWN, params={})

    parts = [p for p in parsed.path.split("/") if p]

    # /orgs/{org}[/...] — organization landing page. GitHub reserves the
    # "orgs" path segment, so it can never collide with a real user/repo.
    if len(parts) >= 2 and parts[0] == "orgs":
        if not _is_valid_owner(parts[1]):
            return ParsedGitHubUrl(type=UrlType.UNKNOWN, params={})
        return ParsedGitHubUrl(type=UrlType.ORG, params={"org": parts[1]})

    if len(parts) == 1:
        if not _is_valid_owner(parts[0]):
            return ParsedGitHubUrl(type=UrlType.UNKNOWN, params={})
        return ParsedGitHubUrl(type=UrlType.USER, params={"username": parts[0]})

    if len(parts) >= 4:
        owner, repo, section, number = parts[0], parts[1], parts[2], parts[3]
        if _is_valid_owner(owner) and _is_valid_repo(repo) and section in ("pull", "issues"):
            if _NUMBER_RE.fullmatch(number):
                url_type = UrlType.PR if section == "pull" else UrlType.ISSUE
                return ParsedGitHubUrl(
                    type=url_type,
                    params={"username": owner, "repo": repo, "number": number},
                )
            # A "pull"/"issues" section with a non-ASCII-digit number (e.g.
            # the Unicode superscript "²") is not a valid PR/issue reference
            # and must not fall through to being misread as a repo URL.
            return ParsedGitHubUrl(type=UrlType.UNKNOWN, params={})

    if len(parts) >= 2:
        if not _is_valid_owner(parts[0]) or not _is_valid_repo(parts[1]):
            return ParsedGitHubUrl(type=UrlType.UNKNOWN, params={})
        return ParsedGitHubUrl(
            type=UrlType.REPO,
            params={"username": parts[0], "repo": parts[1]},
        )

    return ParsedGitHubUrl(type=UrlType.UNKNOWN, params={})
