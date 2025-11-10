from typing import Any, cast, Union

from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import CRUDMixin, ObjectDeleteMixin, SaveMixin
from gitlab.types import RequiredOptional

__all__ = [
    "ProjectTrigger",
    "ProjectTriggerManager",
]


class ProjectTrigger(SaveMixin, ObjectDeleteMixin, RESTObject):
    pass


class ProjectTriggerManager(CRUDMixin, RESTManager):
    _path = "/projects/{project_id}/triggers"
    _obj_cls = ProjectTrigger
    _from_parent_attrs = {"project_id": "id"}
    _create_attrs = RequiredOptional(required=("description",))
    _update_attrs = RequiredOptional(required=("description",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectTrigger:
        return cast(ProjectTrigger, super().get(id=id, lazy=lazy, **kwargs))
