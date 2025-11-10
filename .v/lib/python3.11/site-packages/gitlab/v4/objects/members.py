from typing import Any, cast, Union

from gitlab import types
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import (
    CRUDMixin,
    DeleteMixin,
    ListMixin,
    ObjectDeleteMixin,
    RetrieveMixin,
    SaveMixin,
)
from gitlab.types import RequiredOptional

__all__ = [
    "GroupBillableMember",
    "GroupBillableMemberManager",
    "GroupBillableMemberMembership",
    "GroupBillableMemberMembershipManager",
    "GroupMember",
    "GroupMemberAll",
    "GroupMemberManager",
    "GroupMemberAllManager",
    "ProjectMember",
    "ProjectMemberAll",
    "ProjectMemberManager",
    "ProjectMemberAllManager",
]


class GroupMember(SaveMixin, ObjectDeleteMixin, RESTObject):
    _repr_attr = "username"


class GroupMemberManager(CRUDMixin, RESTManager):
    _path = "/groups/{group_id}/members"
    _obj_cls = GroupMember
    _from_parent_attrs = {"group_id": "id"}
    _create_attrs = RequiredOptional(
        required=("access_level", "user_id"),
        optional=("expires_at", "tasks_to_be_done"),
    )
    _update_attrs = RequiredOptional(
        required=("access_level",), optional=("expires_at",)
    )
    _types = {
        "user_ids": types.ArrayAttribute,
        "tasks_to_be_done": types.ArrayAttribute,
    }

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> GroupMember:
        return cast(GroupMember, super().get(id=id, lazy=lazy, **kwargs))


class GroupBillableMember(ObjectDeleteMixin, RESTObject):
    _repr_attr = "username"

    memberships: "GroupBillableMemberMembershipManager"


class GroupBillableMemberManager(ListMixin, DeleteMixin, RESTManager):
    _path = "/groups/{group_id}/billable_members"
    _obj_cls = GroupBillableMember
    _from_parent_attrs = {"group_id": "id"}
    _list_filters = ("search", "sort")


class GroupBillableMemberMembership(RESTObject):
    _id_attr = "user_id"


class GroupBillableMemberMembershipManager(ListMixin, RESTManager):
    _path = "/groups/{group_id}/billable_members/{user_id}/memberships"
    _obj_cls = GroupBillableMemberMembership
    _from_parent_attrs = {"group_id": "group_id", "user_id": "id"}


class GroupMemberAll(RESTObject):
    _repr_attr = "username"


class GroupMemberAllManager(RetrieveMixin, RESTManager):
    _path = "/groups/{group_id}/members/all"
    _obj_cls = GroupMemberAll
    _from_parent_attrs = {"group_id": "id"}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> GroupMemberAll:
        return cast(GroupMemberAll, super().get(id=id, lazy=lazy, **kwargs))


class ProjectMember(SaveMixin, ObjectDeleteMixin, RESTObject):
    _repr_attr = "username"


class ProjectMemberManager(CRUDMixin, RESTManager):
    _path = "/projects/{project_id}/members"
    _obj_cls = ProjectMember
    _from_parent_attrs = {"project_id": "id"}
    _create_attrs = RequiredOptional(
        required=("access_level", "user_id"),
        optional=("expires_at", "tasks_to_be_done"),
    )
    _update_attrs = RequiredOptional(
        required=("access_level",), optional=("expires_at",)
    )
    _types = {
        "user_ids": types.ArrayAttribute,
        "tasks_to_be_dones": types.ArrayAttribute,
    }

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectMember:
        return cast(ProjectMember, super().get(id=id, lazy=lazy, **kwargs))


class ProjectMemberAll(RESTObject):
    _repr_attr = "username"


class ProjectMemberAllManager(RetrieveMixin, RESTManager):
    _path = "/projects/{project_id}/members/all"
    _obj_cls = ProjectMemberAll
    _from_parent_attrs = {"project_id": "id"}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectMemberAll:
        return cast(ProjectMemberAll, super().get(id=id, lazy=lazy, **kwargs))
