import pytest
from pydantic import ValidationError

from src.core.config import Settings

_BASE = {
    "DATABASE_URL": "postgresql+asyncpg://x:x@localhost/x",
    "SECRET_KEY": "x" * 32,
}


def _make(**overrides):
    return Settings(**{**_BASE, **overrides}, _env_file=None)


def test_rejects_invalid_samesite():
    with pytest.raises(ValidationError):
        _make(COOKIE_SAMESITE="Lax ")


def test_samesite_none_requires_secure():
    with pytest.raises(ValidationError):
        _make(COOKIE_SAMESITE="none", COOKIE_SECURE=False)


def test_samesite_none_with_secure_ok():
    assert _make(COOKIE_SAMESITE="none", COOKIE_SECURE=True).COOKIE_SAMESITE == "none"


def test_rejects_placeholder_secret_in_prod():
    with pytest.raises(ValidationError):
        _make(MODE="prod", SECRET_KEY="change-me-but-still-long-enough-xxxxx")


def test_short_secret_rejected():
    with pytest.raises(ValidationError):
        _make(SECRET_KEY="too-short")
