from typing import Any, cast, Union

from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import (
    CreateMixin,
    DeleteMixin,
    ObjectDeleteMixin,
    ObjectRotateMixin,
    RetrieveMixin,
    RotateMixin,
)
from gitlab.types import ArrayAttribute, RequiredOptional

__all__ = [
    "GroupAccessToken",
    "GroupAccessTokenManager",
]


class GroupAccessToken(ObjectDeleteMixin, ObjectRotateMixin, RESTObject):
    pass


class GroupAccessTokenManager(
    CreateMixin, DeleteMixin, RetrieveMixin, RotateMixin, RESTManager
):
    _path = "/groups/{group_id}/access_tokens"
    _obj_cls = GroupAccessToken
    _from_parent_attrs = {"group_id": "id"}
    _create_attrs = RequiredOptional(
        required=("name", "scopes"), optional=("access_level", "expires_at")
    )
    _types = {"scopes": ArrayAttribute}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> GroupAccessToken:
        return cast(GroupAccessToken, super().get(id=id, lazy=lazy, **kwargs))
