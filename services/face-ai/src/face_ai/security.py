from __future__ import annotations

import hmac
from typing import Annotated

from fastapi import Depends, HTTPException, Security, status
from fastapi.security import APIKeyHeader, HTTPAuthorizationCredentials, HTTPBearer

from face_ai.settings import Settings, get_settings

bearer_scheme = HTTPBearer(auto_error=False)
header_scheme = APIKeyHeader(name="X-Internal-Token", auto_error=False)


def verify_internal_token(
    settings: Annotated[Settings, Depends(get_settings)],
    bearer: Annotated[HTTPAuthorizationCredentials | None, Security(bearer_scheme)] = None,
    header: Annotated[str | None, Security(header_scheme)] = None,
) -> None:
    """
    Verify the request is authorized via an internal token.
    If FACE_AI_INTERNAL_TOKEN is set in settings, it enforces the token.
    If it is not set (e.g. default dev/test), it permits the request.
    """
    if not settings.internal_token:
        return

    provided_token: str | None = None
    if header:
        provided_token = header
    elif bearer and bearer.credentials:
        provided_token = bearer.credentials

    if not provided_token:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Missing internal token",
            headers={"WWW-Authenticate": "Bearer"},
        )

    # Use constant-time comparison to prevent timing attacks
    expected = settings.internal_token.encode("utf-8")
    actual = provided_token.encode("utf-8")

    if len(expected) != len(actual) or not hmac.compare_digest(expected, actual):
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Invalid internal token",
        )
