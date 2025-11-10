from typing import Any, BinaryIO, cast, Dict, List, Optional, Type, TYPE_CHECKING, Union

import requests

import gitlab
from gitlab import cli
from gitlab import exceptions as exc
from gitlab import types
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import (
    CreateMixin,
    CRUDMixin,
    DeleteMixin,
    ListMixin,
    NoUpdateMixin,
    ObjectDeleteMixin,
    SaveMixin,
)
from gitlab.types import RequiredOptional

from .access_requests import GroupAccessRequestManager  # noqa: F401
from .audit_events import GroupAuditEventManager  # noqa: F401
from .badges import GroupBadgeManager  # noqa: F401
from .boards import GroupBoardManager  # noqa: F401
from .clusters import GroupClusterManager  # noqa: F401
from .container_registry import GroupRegistryRepositoryManager  # noqa: F401
from .custom_attributes import GroupCustomAttributeManager  # noqa: F401
from .deploy_tokens import GroupDeployTokenManager  # noqa: F401
from .epics import GroupEpicManager  # noqa: F401
from .export_import import GroupExportManager, GroupImportManager  # noqa: F401
from .group_access_tokens import GroupAccessTokenManager  # noqa: F401
from .hooks import GroupHookManager  # noqa: F401
from .invitations import GroupInvitationManager  # noqa: F401
from .issues import GroupIssueManager  # noqa: F401
from .iterations import GroupIterationManager  # noqa: F401
from .labels import GroupLabelManager  # noqa: F401
from .members import (  # noqa: F401
    GroupBillableMemberManager,
    GroupMemberAllManager,
    GroupMemberManager,
)
from .merge_requests import GroupMergeRequestManager  # noqa: F401
from .milestones import GroupMilestoneManager  # noqa: F401
from .notification_settings import GroupNotificationSettingsManager  # noqa: F401
from .packages import GroupPackageManager  # noqa: F401
from .projects import GroupProjectManager, SharedProjectManager  # noqa: F401
from .push_rules import GroupPushRulesManager
from .runners import GroupRunnerManager  # noqa: F401
from .statistics import GroupIssuesStatisticsManager  # noqa: F401
from .variables import GroupVariableManager  # noqa: F401
from .wikis import GroupWikiManager  # noqa: F401

__all__ = [
    "Group",
    "GroupManager",
    "GroupDescendantGroup",
    "GroupDescendantGroupManager",
    "GroupLDAPGroupLink",
    "GroupLDAPGroupLinkManager",
    "GroupSubgroup",
    "GroupSubgroupManager",
    "GroupSAMLGroupLink",
    "GroupSAMLGroupLinkManager",
]


class Group(SaveMixin, ObjectDeleteMixin, RESTObject):
    _repr_attr = "name"

    access_tokens: GroupAccessTokenManager
    accessrequests: GroupAccessRequestManager
    audit_events: GroupAuditEventManager
    badges: GroupBadgeManager
    billable_members: GroupBillableMemberManager
    boards: GroupBoardManager
    clusters: GroupClusterManager
    customattributes: GroupCustomAttributeManager
    deploytokens: GroupDeployTokenManager
    descendant_groups: "GroupDescendantGroupManager"
    epics: GroupEpicManager
    exports: GroupExportManager
    hooks: GroupHookManager
    imports: GroupImportManager
    invitations: GroupInvitationManager
    issues: GroupIssueManager
    issues_statistics: GroupIssuesStatisticsManager
    iterations: GroupIterationManager
    labels: GroupLabelManager
    ldap_group_links: "GroupLDAPGroupLinkManager"
    members: GroupMemberManager
    members_all: GroupMemberAllManager
    mergerequests: GroupMergeRequestManager
    milestones: GroupMilestoneManager
    notificationsettings: GroupNotificationSettingsManager
    packages: GroupPackageManager
    projects: GroupProjectManager
    shared_projects: SharedProjectManager
    pushrules: GroupPushRulesManager
    registry_repositories: GroupRegistryRepositoryManager
    runners: GroupRunnerManager
    subgroups: "GroupSubgroupManager"
    variables: GroupVariableManager
    wikis: GroupWikiManager
    saml_group_links: "GroupSAMLGroupLinkManager"

    @cli.register_custom_action("Group", ("project_id",))
    @exc.on_http_error(exc.GitlabTransferProjectError)
    def transfer_project(self, project_id: int, **kwargs: Any) -> None:
        """Transfer a project to this group.

        Args:
            to_project_id: ID of the project to transfer
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabTransferProjectError: If the project could not be transferred
        """
        path = f"/groups/{self.encoded_id}/projects/{project_id}"
        self.manager.gitlab.http_post(path, **kwargs)

    @cli.register_custom_action("Group", (), ("group_id",))
    @exc.on_http_error(exc.GitlabGroupTransferError)
    def transfer(self, group_id: Optional[int] = None, **kwargs: Any) -> None:
        """Transfer the group to a new parent group or make it a top-level group.

        Requires GitLab ≥14.6.

        Args:
            group_id: ID of the new parent group. When not specified,
                the group to transfer is instead turned into a top-level group.
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGroupTransferError: If the group could not be transferred
        """
        path = f"/groups/{self.encoded_id}/transfer"
        post_data = {}
        if group_id is not None:
            post_data["group_id"] = group_id
        self.manager.gitlab.http_post(path, post_data=post_data, **kwargs)

    @cli.register_custom_action("Group", ("scope", "search"))
    @exc.on_http_error(exc.GitlabSearchError)
    def search(
        self, scope: str, search: str, **kwargs: Any
    ) -> Union[gitlab.GitlabList, List[Dict[str, Any]]]:
        """Search the group resources matching the provided string.

        Args:
            scope: Scope of the search
            search: Search string
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabSearchError: If the server failed to perform the request

        Returns:
            A list of dicts describing the resources found.
        """
        data = {"scope": scope, "search": search}
        path = f"/groups/{self.encoded_id}/search"
        return self.manager.gitlab.http_list(path, query_data=data, **kwargs)

    @cli.register_custom_action("Group")
    @exc.on_http_error(exc.GitlabCreateError)
    def ldap_sync(self, **kwargs: Any) -> None:
        """Sync LDAP groups.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabCreateError: If the server cannot perform the request
        """
        path = f"/groups/{self.encoded_id}/ldap_sync"
        self.manager.gitlab.http_post(path, **kwargs)

    @cli.register_custom_action("Group", ("group_id", "group_access"), ("expires_at",))
    @exc.on_http_error(exc.GitlabCreateError)
    def share(
        self,
        group_id: int,
        group_access: int,
        expires_at: Optional[str] = None,
        **kwargs: Any,
    ) -> None:
        """Share the group with a group.

        Args:
            group_id: ID of the group.
            group_access: Access level for the group.
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabCreateError: If the server failed to perform the request

        Returns:
            Group
        """
        path = f"/groups/{self.encoded_id}/share"
        data = {
            "group_id": group_id,
            "group_access": group_access,
            "expires_at": expires_at,
        }
        server_data = self.manager.gitlab.http_post(path, post_data=data, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        self._update_attrs(server_data)

    @cli.register_custom_action("Group", ("group_id",))
    @exc.on_http_error(exc.GitlabDeleteError)
    def unshare(self, group_id: int, **kwargs: Any) -> None:
        """Delete a shared group link within a group.

        Args:
            group_id: ID of the group.
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabDeleteError: If the server failed to perform the request
        """
        path = f"/groups/{self.encoded_id}/share/{group_id}"
        self.manager.gitlab.http_delete(path, **kwargs)

    @cli.register_custom_action("Group")
    @exc.on_http_error(exc.GitlabRestoreError)
    def restore(self, **kwargs: Any) -> None:
        """Restore a  group marked for deletion..

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabRestoreError: If the server failed to perform the request
        """
        path = f"/groups/{self.encoded_id}/restore"
        self.manager.gitlab.http_post(path, **kwargs)


class GroupManager(CRUDMixin, RESTManager):
    _path = "/groups"
    _obj_cls = Group
    _list_filters = (
        "skip_groups",
        "all_available",
        "search",
        "order_by",
        "sort",
        "statistics",
        "owned",
        "with_custom_attributes",
        "min_access_level",
        "top_level_only",
    )
    _create_attrs = RequiredOptional(
        required=("name", "path"),
        optional=(
            "description",
            "membership_lock",
            "visibility",
            "share_with_group_lock",
            "require_two_factor_authentication",
            "two_factor_grace_period",
            "project_creation_level",
            "auto_devops_enabled",
            "subgroup_creation_level",
            "emails_disabled",
            "avatar",
            "mentions_disabled",
            "lfs_enabled",
            "request_access_enabled",
            "parent_id",
            "default_branch_protection",
            "shared_runners_minutes_limit",
            "extra_shared_runners_minutes_limit",
        ),
    )
    _update_attrs = RequiredOptional(
        optional=(
            "name",
            "path",
            "description",
            "membership_lock",
            "share_with_group_lock",
            "visibility",
            "require_two_factor_authentication",
            "two_factor_grace_period",
            "project_creation_level",
            "auto_devops_enabled",
            "subgroup_creation_level",
            "emails_disabled",
            "avatar",
            "mentions_disabled",
            "lfs_enabled",
            "request_access_enabled",
            "default_branch_protection",
            "file_template_project_id",
            "shared_runners_minutes_limit",
            "extra_shared_runners_minutes_limit",
            "prevent_forking_outside_group",
            "shared_runners_setting",
        ),
    )
    _types = {"avatar": types.ImageAttribute, "skip_groups": types.ArrayAttribute}

    def get(self, id: Union[str, int], lazy: bool = False, **kwargs: Any) -> Group:
        return cast(Group, super().get(id=id, lazy=lazy, **kwargs))

    @exc.on_http_error(exc.GitlabImportError)
    def import_group(
        self,
        file: BinaryIO,
        path: str,
        name: str,
        parent_id: Optional[Union[int, str]] = None,
        **kwargs: Any,
    ) -> Union[Dict[str, Any], requests.Response]:
        """Import a group from an archive file.

        Args:
            file: Data or file object containing the group
            path: The path for the new group to be imported.
            name: The name for the new group.
            parent_id: ID of a parent group that the group will
                be imported into.
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabImportError: If the server failed to perform the request

        Returns:
            A representation of the import status.
        """
        files = {"file": ("file.tar.gz", file, "application/octet-stream")}
        data: Dict[str, Any] = {"path": path, "name": name}
        if parent_id is not None:
            data["parent_id"] = parent_id

        return self.gitlab.http_post(
            "/groups/import", post_data=data, files=files, **kwargs
        )


class GroupSubgroup(RESTObject):
    pass


class GroupSubgroupManager(ListMixin, RESTManager):
    _path = "/groups/{group_id}/subgroups"
    _obj_cls: Union[Type["GroupDescendantGroup"], Type[GroupSubgroup]] = GroupSubgroup
    _from_parent_attrs = {"group_id": "id"}
    _list_filters = (
        "skip_groups",
        "all_available",
        "search",
        "order_by",
        "sort",
        "statistics",
        "owned",
        "with_custom_attributes",
        "min_access_level",
    )
    _types = {"skip_groups": types.ArrayAttribute}


class GroupDescendantGroup(RESTObject):
    pass


class GroupDescendantGroupManager(GroupSubgroupManager):
    """
    This manager inherits from GroupSubgroupManager as descendant groups
    share all attributes with subgroups, except the path and object class.
    """

    _path = "/groups/{group_id}/descendant_groups"
    _obj_cls: Type[GroupDescendantGroup] = GroupDescendantGroup


class GroupLDAPGroupLink(RESTObject):
    _repr_attr = "provider"

    def _get_link_attrs(self) -> Dict[str, str]:
        # https://docs.gitlab.com/ee/api/groups.html#add-ldap-group-link-with-cn-or-filter
        # https://docs.gitlab.com/ee/api/groups.html#delete-ldap-group-link-with-cn-or-filter
        # We can tell what attribute to use based on the data returned
        data = {"provider": self.provider}
        if self.cn:
            data["cn"] = self.cn
        else:
            data["filter"] = self.filter

        return data

    def delete(self, **kwargs: Any) -> None:
        """Delete the LDAP group link from the server.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabDeleteError: If the server cannot perform the request
        """
        if TYPE_CHECKING:
            assert isinstance(self.manager, DeleteMixin)
        self.manager.delete(
            self.encoded_id, query_data=self._get_link_attrs(), **kwargs
        )


class GroupLDAPGroupLinkManager(ListMixin, CreateMixin, DeleteMixin, RESTManager):
    _path = "/groups/{group_id}/ldap_group_links"
    _obj_cls: Type[GroupLDAPGroupLink] = GroupLDAPGroupLink
    _from_parent_attrs = {"group_id": "id"}
    _create_attrs = RequiredOptional(
        required=("provider", "group_access"), exclusive=("cn", "filter")
    )


class GroupSAMLGroupLink(ObjectDeleteMixin, RESTObject):
    _id_attr = "name"
    _repr_attr = "name"


class GroupSAMLGroupLinkManager(NoUpdateMixin, RESTManager):
    _path = "/groups/{group_id}/saml_group_links"
    _obj_cls: Type[GroupSAMLGroupLink] = GroupSAMLGroupLink
    _from_parent_attrs = {"group_id": "id"}
    _create_attrs = RequiredOptional(required=("saml_group_name", "access_level"))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> GroupSAMLGroupLink:
        return cast(GroupSAMLGroupLink, super().get(id=id, lazy=lazy, **kwargs))
