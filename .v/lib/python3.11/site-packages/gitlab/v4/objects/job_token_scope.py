from typing import Any, cast

from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import (
    GetWithoutIdMixin,
    RefreshMixin,
    SaveMixin,
    UpdateMethod,
    UpdateMixin,
)

__all__ = [
    "ProjectJobTokenScope",
    "ProjectJobTokenScopeManager",
]


class ProjectJobTokenScope(RefreshMixin, SaveMixin, RESTObject):
    _id_attr = None


class ProjectJobTokenScopeManager(GetWithoutIdMixin, UpdateMixin, RESTManager):
    _path = "/projects/{project_id}/job_token_scope"
    _obj_cls = ProjectJobTokenScope
    _from_parent_attrs = {"project_id": "id"}
    _update_method = UpdateMethod.PATCH

    def get(self, **kwargs: Any) -> ProjectJobTokenScope:
        return cast(ProjectJobTokenScope, super().get(**kwargs))
