from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import (
    AccessRequestMixin,
    CreateMixin,
    DeleteMixin,
    ListMixin,
    ObjectDeleteMixin,
)

__all__ = [
    "GroupAccessRequest",
    "GroupAccessRequestManager",
    "ProjectAccessRequest",
    "ProjectAccessRequestManager",
]


class GroupAccessRequest(AccessRequestMixin, ObjectDeleteMixin, RESTObject):
    pass


class GroupAccessRequestManager(ListMixin, CreateMixin, DeleteMixin, RESTManager):
    _path = "/groups/{group_id}/access_requests"
    _obj_cls = GroupAccessRequest
    _from_parent_attrs = {"group_id": "id"}


class ProjectAccessRequest(AccessRequestMixin, ObjectDeleteMixin, RESTObject):
    pass


class ProjectAccessRequestManager(ListMixin, CreateMixin, DeleteMixin, RESTManager):
    _path = "/projects/{project_id}/access_requests"
    _obj_cls = ProjectAccessRequest
    _from_parent_attrs = {"project_id": "id"}
