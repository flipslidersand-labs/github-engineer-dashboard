from unittest.mock import patch

import httpx
import pytest

from app.reviewer import review_diff_ollama


def _resp(**kwargs) -> httpx.Response:
    return httpx.Response(200, request=httpx.Request("POST", "http://x/api/chat"), **kwargs)


def test_review_diff_ollama_non_json_body_raises_runtime_error():
    """A non-JSON Ollama response body must surface as RuntimeError (→ 502), not crash."""
    resp = _resp(content=b"not json at all")
    with patch("app.reviewer.httpx.post", return_value=resp):
        with pytest.raises(RuntimeError, match="malformed response"):
            review_diff_ollama("diff", "http://localhost:11434", "qwen2.5-coder:7b")


def test_review_diff_ollama_missing_message_key_raises_runtime_error():
    """A well-formed-JSON but KeyError-shaped body (missing message/content) → RuntimeError."""
    resp = _resp(json={"done": True})
    with patch("app.reviewer.httpx.post", return_value=resp):
        with pytest.raises(RuntimeError, match="malformed response"):
            review_diff_ollama("diff", "http://localhost:11434", "qwen2.5-coder:7b")


def test_review_diff_ollama_missing_content_key_raises_runtime_error():
    resp = _resp(json={"message": {}})
    with patch("app.reviewer.httpx.post", return_value=resp):
        with pytest.raises(RuntimeError, match="malformed response"):
            review_diff_ollama("diff", "http://localhost:11434", "qwen2.5-coder:7b")


def test_review_diff_ollama_read_error_raises_runtime_error():
    """httpx.ReadError (not one of the previously-caught specific types) must
    also convert to RuntimeError instead of escaping uncaught."""
    with patch("app.reviewer.httpx.post", side_effect=httpx.ReadError("boom")):
        with pytest.raises(RuntimeError, match="Ollama request failed"):
            review_diff_ollama("diff", "http://localhost:11434", "qwen2.5-coder:7b")


def test_review_diff_ollama_success():
    resp = _resp(json={"message": {"content": "## Summary\nLGTM"}})
    with patch("app.reviewer.httpx.post", return_value=resp):
        result = review_diff_ollama("diff", "http://localhost:11434", "qwen2.5-coder:7b")
    assert result == "## Summary\nLGTM"
