from fastapi import Response

from src.core.config import settings

ACCESS_COOKIE = "access_token"
REFRESH_COOKIE = "refresh_token"


def _set(response: Response, key: str, value: str, max_age: int) -> None:
    response.set_cookie(
        key,
        value,
        max_age=max_age,
        httponly=True,
        secure=settings.COOKIE_SECURE,
        samesite=settings.COOKIE_SAMESITE,
        domain=settings.COOKIE_DOMAIN,
        path="/",
    )


def set_auth_cookies(response: Response, access_token: str, refresh_token: str) -> None:
    _set(response, ACCESS_COOKIE, access_token, settings.ACCESS_TOKEN_EXPIRE_MINUTES * 60)
    _set(response, REFRESH_COOKIE, refresh_token, settings.REFRESH_TOKEN_EXPIRE_DAYS * 86400)


def clear_auth_cookies(response: Response) -> None:
    # Mirror the attributes used on set so the delete Set-Cookie matches.
    for key in (ACCESS_COOKIE, REFRESH_COOKIE):
        response.delete_cookie(
            key,
            path="/",
            domain=settings.COOKIE_DOMAIN,
            secure=settings.COOKIE_SECURE,
            httponly=True,
            samesite=settings.COOKIE_SAMESITE,
        )
