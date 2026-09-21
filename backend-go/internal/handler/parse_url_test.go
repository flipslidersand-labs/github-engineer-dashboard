package handler

import "testing"

func TestParseGitHubURLValidUser(t *testing.T) {
	p := parseGitHubURL("https://github.com/octocat")
	if p.typ != urlTypeUser || p.username != "octocat" {
		t.Errorf("got %+v", p)
	}
}

func TestParseGitHubURLValidRepo(t *testing.T) {
	p := parseGitHubURL("https://github.com/torvalds/linux")
	if p.typ != urlTypeRepo || p.username != "torvalds" || p.repo != "linux" {
		t.Errorf("got %+v", p)
	}
}

func TestParseGitHubURLValidPR(t *testing.T) {
	p := parseGitHubURL("https://github.com/torvalds/linux/pull/1")
	if p.typ != urlTypePR || p.number != 1 {
		t.Errorf("got %+v", p)
	}
}

func TestParseGitHubURLValidOrg(t *testing.T) {
	p := parseGitHubURL("https://github.com/orgs/github")
	if p.typ != urlTypeOrg || p.org != "github" {
		t.Errorf("got %+v", p)
	}
}

func TestParseGitHubURLNonGitHubHost(t *testing.T) {
	p := parseGitHubURL("https://example.com/foo")
	if p.typ != urlTypeUnknown {
		t.Errorf("got %+v, want unknown", p)
	}
}

// ── Issue #129: reject path-traversal-shaped / reserved identifiers ───────

func TestParseGitHubURLDotDotUsernameRejected(t *testing.T) {
	p := parseGitHubURL("https://github.com/..")
	if p.typ != urlTypeUnknown {
		t.Errorf("got %+v, want unknown", p)
	}
}

func TestParseGitHubURLDotDotRepoRejected(t *testing.T) {
	p := parseGitHubURL("https://github.com/octocat/..")
	if p.typ != urlTypeUnknown {
		t.Errorf("got %+v, want unknown", p)
	}
}

func TestParseGitHubURLDotDotOwnerInPRURLRejected(t *testing.T) {
	p := parseGitHubURL("https://github.com/../linux/pull/1")
	if p.typ != urlTypeUnknown {
		t.Errorf("got %+v, want unknown", p)
	}
}

func TestParseGitHubURLLeadingHyphenUsernameRejected(t *testing.T) {
	p := parseGitHubURL("https://github.com/-octocat")
	if p.typ != urlTypeUnknown {
		t.Errorf("got %+v, want unknown", p)
	}
}

func TestParseGitHubURLOrgInvalidNameRejected(t *testing.T) {
	p := parseGitHubURL("https://github.com/orgs/..")
	if p.typ != urlTypeUnknown {
		t.Errorf("got %+v, want unknown", p)
	}
}

func TestParseGitHubURLRepoDotDotInFourPartPathRejected(t *testing.T) {
	p := parseGitHubURL("https://github.com/octocat/../pull/1")
	if p.typ != urlTypeUnknown {
		t.Errorf("got %+v, want unknown", p)
	}
}
