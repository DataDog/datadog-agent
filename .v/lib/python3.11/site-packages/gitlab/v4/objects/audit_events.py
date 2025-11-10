"""
GitLab API:
https://docs.gitlab.com/ee/api/audit_events.html
"""
from typing import Any, cast, Union

from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import RetrieveMixin

__all__ = [
    "AuditEvent",
    "AuditEventManager",
    "GroupAuditEvent",
    "GroupAuditEventManager",
    "ProjectAuditEvent",
    "ProjectAuditEventManager",
    "ProjectAudit",
    "ProjectAuditManager",
]


class AuditEvent(RESTObject):
    _id_attr = "id"


class AuditEventManager(RetrieveMixin, RESTManager):
    _path = "/audit_events"
    _obj_cls = AuditEvent
    _list_filters = ("created_after", "created_before", "entity_type", "entity_id")

    def get(self, id: Union[str, int], lazy: bool = False, **kwargs: Any) -> AuditEvent:
        return cast(AuditEvent, super().get(id=id, lazy=lazy, **kwargs))


class GroupAuditEvent(RESTObject):
    _id_attr = "id"


class GroupAuditEventManager(RetrieveMixin, RESTManager):
    _path = "/groups/{group_id}/audit_events"
    _obj_cls = GroupAuditEvent
    _from_parent_attrs = {"group_id": "id"}
    _list_filters = ("created_after", "created_before")

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> GroupAuditEvent:
        return cast(GroupAuditEvent, super().get(id=id, lazy=lazy, **kwargs))


class ProjectAuditEvent(RESTObject):
    _id_attr = "id"


class ProjectAuditEventManager(RetrieveMixin, RESTManager):
    _path = "/projects/{project_id}/audit_events"
    _obj_cls = ProjectAuditEvent
    _from_parent_attrs = {"project_id": "id"}
    _list_filters = ("created_after", "created_before")

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectAuditEvent:
        return cast(ProjectAuditEvent, super().get(id=id, lazy=lazy, **kwargs))


class ProjectAudit(ProjectAuditEvent):
    pass


class ProjectAuditManager(ProjectAuditEventManager):
    pass
