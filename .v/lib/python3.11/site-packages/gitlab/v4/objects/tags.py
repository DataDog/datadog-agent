from typing import Any, cast, Union

from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import NoUpdateMixin, ObjectDeleteMixin
from gitlab.types import RequiredOptional

__all__ = [
    "ProjectTag",
    "ProjectTagManager",
    "ProjectProtectedTag",
    "ProjectProtectedTagManager",
]


class ProjectTag(ObjectDeleteMixin, RESTObject):
    _id_attr = "name"
    _repr_attr = "name"


class ProjectTagManager(NoUpdateMixin, RESTManager):
    _path = "/projects/{project_id}/repository/tags"
    _obj_cls = ProjectTag
    _from_parent_attrs = {"project_id": "id"}
    _create_attrs = RequiredOptional(
        required=("tag_name", "ref"), optional=("message",)
    )

    def get(self, id: Union[str, int], lazy: bool = False, **kwargs: Any) -> ProjectTag:
        return cast(ProjectTag, super().get(id=id, lazy=lazy, **kwargs))


class ProjectProtectedTag(ObjectDeleteMixin, RESTObject):
    _id_attr = "name"
    _repr_attr = "name"


class ProjectProtectedTagManager(NoUpdateMixin, RESTManager):
    _path = "/projects/{project_id}/protected_tags"
    _obj_cls = ProjectProtectedTag
    _from_parent_attrs = {"project_id": "id"}
    _create_attrs = RequiredOptional(
        required=("name",), optional=("create_access_level",)
    )

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectProtectedTag:
        return cast(ProjectProtectedTag, super().get(id=id, lazy=lazy, **kwargs))
