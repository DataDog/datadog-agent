from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import ListMixin

__all__ = [
    "ProjectIterationManager",
    "GroupIteration",
    "GroupIterationManager",
]


class GroupIteration(RESTObject):
    _repr_attr = "title"


class GroupIterationManager(ListMixin, RESTManager):
    _path = "/groups/{group_id}/iterations"
    _obj_cls = GroupIteration
    _from_parent_attrs = {"group_id": "id"}
    _list_filters = ("state", "search", "include_ancestors")


class ProjectIterationManager(ListMixin, RESTManager):
    _path = "/projects/{project_id}/iterations"
    _obj_cls = GroupIteration
    _from_parent_attrs = {"project_id": "id"}
    _list_filters = ("state", "search", "include_ancestors")
