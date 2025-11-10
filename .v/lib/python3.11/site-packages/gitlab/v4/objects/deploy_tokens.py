from typing import Any, cast, Union

from gitlab import types
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import (
    CreateMixin,
    DeleteMixin,
    ListMixin,
    ObjectDeleteMixin,
    RetrieveMixin,
)
from gitlab.types import RequiredOptional

__all__ = [
    "DeployToken",
    "DeployTokenManager",
    "GroupDeployToken",
    "GroupDeployTokenManager",
    "ProjectDeployToken",
    "ProjectDeployTokenManager",
]


class DeployToken(ObjectDeleteMixin, RESTObject):
    pass


class DeployTokenManager(ListMixin, RESTManager):
    _path = "/deploy_tokens"
    _obj_cls = DeployToken


class GroupDeployToken(ObjectDeleteMixin, RESTObject):
    pass


class GroupDeployTokenManager(RetrieveMixin, CreateMixin, DeleteMixin, RESTManager):
    _path = "/groups/{group_id}/deploy_tokens"
    _from_parent_attrs = {"group_id": "id"}
    _obj_cls = GroupDeployToken
    _create_attrs = RequiredOptional(
        required=(
            "name",
            "scopes",
        ),
        optional=(
            "expires_at",
            "username",
        ),
    )
    _list_filters = ("scopes",)
    _types = {"scopes": types.ArrayAttribute}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> GroupDeployToken:
        return cast(GroupDeployToken, super().get(id=id, lazy=lazy, **kwargs))


class ProjectDeployToken(ObjectDeleteMixin, RESTObject):
    pass


class ProjectDeployTokenManager(RetrieveMixin, CreateMixin, DeleteMixin, RESTManager):
    _path = "/projects/{project_id}/deploy_tokens"
    _from_parent_attrs = {"project_id": "id"}
    _obj_cls = ProjectDeployToken
    _create_attrs = RequiredOptional(
        required=(
            "name",
            "scopes",
        ),
        optional=(
            "expires_at",
            "username",
        ),
    )
    _list_filters = ("scopes",)
    _types = {"scopes": types.ArrayAttribute}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectDeployToken:
        return cast(ProjectDeployToken, super().get(id=id, lazy=lazy, **kwargs))
