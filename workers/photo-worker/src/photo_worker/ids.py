from __future__ import annotations

from uuid import NAMESPACE_URL, UUID, uuid5


def face_id(organization_id: str, photo_id: str, face_index: int) -> str:
    return str(uuid5(NAMESPACE_URL, f"{organization_id}:{photo_id}:{face_index}"))


def vector_point_id(collection: str, organization_id: str, photo_id: str, face_index: int) -> UUID:
    return uuid5(NAMESPACE_URL, f"{collection}:{organization_id}:{photo_id}:{face_index}")
