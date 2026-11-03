"""Application settings loaded from environment variables."""

from __future__ import annotations

import os
from dataclasses import dataclass


@dataclass(frozen=True)
class Settings:
    github_api_url: str
    github_token: str | None
    anthropic_api_key: str | None
    cache_db: str
    cache_ttl_seconds: int
    cors_origins: tuple[str, ...]

    ollama_base_url: str | None
    ollama_model: str
    go_backend_url: str | None  # e.g. http://localhost:8080
    # Issue #158: when true, X-GitHub-Token is mandatory (no GITHUB_TOKEN fallback).
    require_client_token: bool = False
    # Max /api/review calls per client IP per minute (0 disables). Paid LLM guard.
    review_rate_limit_per_min: int = 10

    @staticmethod
    def from_env() -> Settings:
        # Default to no cross-origin access: GITHUB_TOKEN fallback (below) means an
        # open CORS_ORIGINS="*" default would let any third-party site ride on the
        # operator's server-side token (see Issue #123). Operators must opt in
        # explicitly via CORS_ORIGINS.
        origins = os.environ.get("CORS_ORIGINS", "")
        return Settings(
            github_api_url=os.environ.get("GITHUB_API_URL", "https://api.github.com").rstrip("/"),
            github_token=(os.environ.get("GITHUB_TOKEN") or None),
            anthropic_api_key=(os.environ.get("ANTHROPIC_API_KEY") or None),
            ollama_base_url=(os.environ.get("OLLAMA_BASE_URL") or None),
            ollama_model=os.environ.get("OLLAMA_MODEL", "qwen2.5-coder:7b"),
            go_backend_url=(os.environ.get("GO_BACKEND_URL", "").rstrip("/") or None),
            cache_db=os.environ.get("CACHE_DB", "cache.db"),
            cache_ttl_seconds=int(os.environ.get("CACHE_TTL_SECONDS", "300")),
            require_client_token=os.environ.get("REQUIRE_CLIENT_TOKEN", "").strip().lower()
            in ("1", "true", "yes", "on"),
            review_rate_limit_per_min=int(os.environ.get("REVIEW_RATE_LIMIT_PER_MIN", "10")),
            cors_origins=tuple(o.strip() for o in origins.split(",") if o.strip()),
        )
