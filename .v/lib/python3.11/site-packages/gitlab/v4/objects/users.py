"""
GitLab API:
https://docs.gitlab.com/ee/api/users.html
https://docs.gitlab.com/ee/api/projects.html#list-projects-starred-by-a-user
"""
from typing import Any, cast, Dict, List, Optional, Union

import requests

from gitlab import cli
from gitlab import exceptions as exc
from gitlab import types
from gitlab.base import RESTManager, RESTObject, RESTObjectList
from gitlab.mixins import (
    CreateMixin,
    CRUDMixin,
    DeleteMixin,
    GetWithoutIdMixin,
    ListMixin,
    NoUpdateMixin,
    ObjectDeleteMixin,
    RetrieveMixin,
    SaveMixin,
    UpdateMixin,
)
from gitlab.types import ArrayAttribute, RequiredOptional

from .custom_attributes import UserCustomAttributeManager  # noqa: F401
from .events import UserEventManager  # noqa: F401
from .personal_access_tokens import UserPersonalAccessTokenManager  # noqa: F401

__all__ = [
    "CurrentUserEmail",
    "CurrentUserEmailManager",
    "CurrentUserGPGKey",
    "CurrentUserGPGKeyManager",
    "CurrentUserKey",
    "CurrentUserKeyManager",
    "CurrentUserRunner",
    "CurrentUserRunnerManager",
    "CurrentUserStatus",
    "CurrentUserStatusManager",
    "CurrentUser",
    "CurrentUserManager",
    "User",
    "UserManager",
    "ProjectUser",
    "ProjectUserManager",
    "StarredProject",
    "StarredProjectManager",
    "UserEmail",
    "UserEmailManager",
    "UserActivities",
    "UserStatus",
    "UserStatusManager",
    "UserActivitiesManager",
    "UserGPGKey",
    "UserGPGKeyManager",
    "UserKey",
    "UserKeyManager",
    "UserIdentityProviderManager",
    "UserImpersonationToken",
    "UserImpersonationTokenManager",
    "UserMembership",
    "UserMembershipManager",
    "UserProject",
    "UserProjectManager",
]


class CurrentUserEmail(ObjectDeleteMixin, RESTObject):
    _repr_attr = "email"


class CurrentUserEmailManager(RetrieveMixin, CreateMixin, DeleteMixin, RESTManager):
    _path = "/user/emails"
    _obj_cls = CurrentUserEmail
    _create_attrs = RequiredOptional(required=("email",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> CurrentUserEmail:
        return cast(CurrentUserEmail, super().get(id=id, lazy=lazy, **kwargs))


class CurrentUserGPGKey(ObjectDeleteMixin, RESTObject):
    pass


class CurrentUserGPGKeyManager(RetrieveMixin, CreateMixin, DeleteMixin, RESTManager):
    _path = "/user/gpg_keys"
    _obj_cls = CurrentUserGPGKey
    _create_attrs = RequiredOptional(required=("key",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> CurrentUserGPGKey:
        return cast(CurrentUserGPGKey, super().get(id=id, lazy=lazy, **kwargs))


class CurrentUserKey(ObjectDeleteMixin, RESTObject):
    _repr_attr = "title"


class CurrentUserKeyManager(RetrieveMixin, CreateMixin, DeleteMixin, RESTManager):
    _path = "/user/keys"
    _obj_cls = CurrentUserKey
    _create_attrs = RequiredOptional(required=("title", "key"))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> CurrentUserKey:
        return cast(CurrentUserKey, super().get(id=id, lazy=lazy, **kwargs))


class CurrentUserRunner(RESTObject):
    pass


class CurrentUserRunnerManager(CreateMixin, RESTManager):
    _path = "/user/runners"
    _obj_cls = CurrentUserRunner
    _types = {"tag_list": types.CommaSeparatedListAttribute}
    _create_attrs = RequiredOptional(
        required=("runner_type",),
        optional=(
            "group_id",
            "project_id",
            "description",
            "paused",
            "locked",
            "run_untagged",
            "tag_list",
            "access_level",
            "maximum_timeout",
            "maintenance_note",
        ),
    )


class CurrentUserStatus(SaveMixin, RESTObject):
    _id_attr = None
    _repr_attr = "message"


class CurrentUserStatusManager(GetWithoutIdMixin, UpdateMixin, RESTManager):
    _path = "/user/status"
    _obj_cls = CurrentUserStatus
    _update_attrs = RequiredOptional(optional=("emoji", "message"))

    def get(self, **kwargs: Any) -> CurrentUserStatus:
        return cast(CurrentUserStatus, super().get(**kwargs))


class CurrentUser(RESTObject):
    _id_attr = None
    _repr_attr = "username"

    emails: CurrentUserEmailManager
    gpgkeys: CurrentUserGPGKeyManager
    keys: CurrentUserKeyManager
    runners: CurrentUserRunnerManager
    status: CurrentUserStatusManager


class CurrentUserManager(GetWithoutIdMixin, RESTManager):
    _path = "/user"
    _obj_cls = CurrentUser

    def get(self, **kwargs: Any) -> CurrentUser:
        return cast(CurrentUser, super().get(**kwargs))


class User(SaveMixin, ObjectDeleteMixin, RESTObject):
    _repr_attr = "username"

    customattributes: UserCustomAttributeManager
    emails: "UserEmailManager"
    events: UserEventManager
    followers_users: "UserFollowersManager"
    following_users: "UserFollowingManager"
    gpgkeys: "UserGPGKeyManager"
    identityproviders: "UserIdentityProviderManager"
    impersonationtokens: "UserImpersonationTokenManager"
    keys: "UserKeyManager"
    memberships: "UserMembershipManager"
    personal_access_tokens: UserPersonalAccessTokenManager
    projects: "UserProjectManager"
    starred_projects: "StarredProjectManager"
    status: "UserStatusManager"

    @cli.register_custom_action("User")
    @exc.on_http_error(exc.GitlabBlockError)
    def block(self, **kwargs: Any) -> Optional[bool]:
        """Block the user.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabBlockError: If the user could not be blocked

        Returns:
            Whether the user status has been changed
        """
        path = f"/users/{self.encoded_id}/block"
        # NOTE: Undocumented behavior of the GitLab API is that it returns a
        # boolean or None
        server_data = cast(
            Optional[bool], self.manager.gitlab.http_post(path, **kwargs)
        )
        if server_data is True:
            self._attrs["state"] = "blocked"
        return server_data

    @cli.register_custom_action("User")
    @exc.on_http_error(exc.GitlabFollowError)
    def follow(self, **kwargs: Any) -> Union[Dict[str, Any], requests.Response]:
        """Follow the user.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabFollowError: If the user could not be followed

        Returns:
            The new object data (*not* a RESTObject)
        """
        path = f"/users/{self.encoded_id}/follow"
        return self.manager.gitlab.http_post(path, **kwargs)

    @cli.register_custom_action("User")
    @exc.on_http_error(exc.GitlabUnfollowError)
    def unfollow(self, **kwargs: Any) -> Union[Dict[str, Any], requests.Response]:
        """Unfollow the user.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabUnfollowError: If the user could not be followed

        Returns:
            The new object data (*not* a RESTObject)
        """
        path = f"/users/{self.encoded_id}/unfollow"
        return self.manager.gitlab.http_post(path, **kwargs)

    @cli.register_custom_action("User")
    @exc.on_http_error(exc.GitlabUnblockError)
    def unblock(self, **kwargs: Any) -> Optional[bool]:
        """Unblock the user.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabUnblockError: If the user could not be unblocked

        Returns:
            Whether the user status has been changed
        """
        path = f"/users/{self.encoded_id}/unblock"
        # NOTE: Undocumented behavior of the GitLab API is that it returns a
        # boolean or None
        server_data = cast(
            Optional[bool], self.manager.gitlab.http_post(path, **kwargs)
        )
        if server_data is True:
            self._attrs["state"] = "active"
        return server_data

    @cli.register_custom_action("User")
    @exc.on_http_error(exc.GitlabDeactivateError)
    def deactivate(self, **kwargs: Any) -> Union[Dict[str, Any], requests.Response]:
        """Deactivate the user.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabDeactivateError: If the user could not be deactivated

        Returns:
            Whether the user status has been changed
        """
        path = f"/users/{self.encoded_id}/deactivate"
        server_data = self.manager.gitlab.http_post(path, **kwargs)
        if server_data:
            self._attrs["state"] = "deactivated"
        return server_data

    @cli.register_custom_action("User")
    @exc.on_http_error(exc.GitlabActivateError)
    def activate(self, **kwargs: Any) -> Union[Dict[str, Any], requests.Response]:
        """Activate the user.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabActivateError: If the user could not be activated

        Returns:
            Whether the user status has been changed
        """
        path = f"/users/{self.encoded_id}/activate"
        server_data = self.manager.gitlab.http_post(path, **kwargs)
        if server_data:
            self._attrs["state"] = "active"
        return server_data

    @cli.register_custom_action("User")
    @exc.on_http_error(exc.GitlabUserApproveError)
    def approve(self, **kwargs: Any) -> Union[Dict[str, Any], requests.Response]:
        """Approve a user creation request.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabUserApproveError: If the user could not be activated

        Returns:
            The new object data (*not* a RESTObject)
        """
        path = f"/users/{self.encoded_id}/approve"
        return self.manager.gitlab.http_post(path, **kwargs)

    @cli.register_custom_action("User")
    @exc.on_http_error(exc.GitlabUserRejectError)
    def reject(self, **kwargs: Any) -> Union[Dict[str, Any], requests.Response]:
        """Reject a user creation request.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabUserRejectError: If the user could not be rejected

        Returns:
            The new object data (*not* a RESTObject)
        """
        path = f"/users/{self.encoded_id}/reject"
        return self.manager.gitlab.http_post(path, **kwargs)

    @cli.register_custom_action("User")
    @exc.on_http_error(exc.GitlabBanError)
    def ban(self, **kwargs: Any) -> Union[Dict[str, Any], requests.Response]:
        """Ban the user.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabBanError: If the user could not be banned

        Returns:
            Whether the user has been banned
        """
        path = f"/users/{self.encoded_id}/ban"
        server_data = self.manager.gitlab.http_post(path, **kwargs)
        if server_data:
            self._attrs["state"] = "banned"
        return server_data

    @cli.register_custom_action("User")
    @exc.on_http_error(exc.GitlabUnbanError)
    def unban(self, **kwargs: Any) -> Union[Dict[str, Any], requests.Response]:
        """Unban the user.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabUnbanError: If the user could not be unbanned

        Returns:
            Whether the user has been unbanned
        """
        path = f"/users/{self.encoded_id}/unban"
        server_data = self.manager.gitlab.http_post(path, **kwargs)
        if server_data:
            self._attrs["state"] = "active"
        return server_data


class UserManager(CRUDMixin, RESTManager):
    _path = "/users"
    _obj_cls = User

    _list_filters = (
        "active",
        "blocked",
        "username",
        "extern_uid",
        "provider",
        "external",
        "search",
        "custom_attributes",
        "status",
        "two_factor",
    )
    _create_attrs = RequiredOptional(
        optional=(
            "email",
            "username",
            "name",
            "password",
            "reset_password",
            "skype",
            "linkedin",
            "twitter",
            "projects_limit",
            "extern_uid",
            "provider",
            "bio",
            "admin",
            "can_create_group",
            "website_url",
            "skip_confirmation",
            "external",
            "organization",
            "location",
            "avatar",
            "public_email",
            "private_profile",
            "color_scheme_id",
            "theme_id",
        ),
    )
    _update_attrs = RequiredOptional(
        required=("email", "username", "name"),
        optional=(
            "password",
            "skype",
            "linkedin",
            "twitter",
            "projects_limit",
            "extern_uid",
            "provider",
            "bio",
            "admin",
            "can_create_group",
            "website_url",
            "skip_reconfirmation",
            "external",
            "organization",
            "location",
            "avatar",
            "public_email",
            "private_profile",
            "color_scheme_id",
            "theme_id",
        ),
    )
    _types = {"confirm": types.LowercaseStringAttribute, "avatar": types.ImageAttribute}

    def get(self, id: Union[str, int], lazy: bool = False, **kwargs: Any) -> User:
        return cast(User, super().get(id=id, lazy=lazy, **kwargs))


class ProjectUser(RESTObject):
    pass


class ProjectUserManager(ListMixin, RESTManager):
    _path = "/projects/{project_id}/users"
    _obj_cls = ProjectUser
    _from_parent_attrs = {"project_id": "id"}
    _list_filters = ("search", "skip_users")
    _types = {"skip_users": types.ArrayAttribute}


class UserEmail(ObjectDeleteMixin, RESTObject):
    _repr_attr = "email"


class UserEmailManager(RetrieveMixin, CreateMixin, DeleteMixin, RESTManager):
    _path = "/users/{user_id}/emails"
    _obj_cls = UserEmail
    _from_parent_attrs = {"user_id": "id"}
    _create_attrs = RequiredOptional(required=("email",))

    def get(self, id: Union[str, int], lazy: bool = False, **kwargs: Any) -> UserEmail:
        return cast(UserEmail, super().get(id=id, lazy=lazy, **kwargs))


class UserActivities(RESTObject):
    _id_attr = "username"


class UserStatus(RESTObject):
    _id_attr = None
    _repr_attr = "message"


class UserStatusManager(GetWithoutIdMixin, RESTManager):
    _path = "/users/{user_id}/status"
    _obj_cls = UserStatus
    _from_parent_attrs = {"user_id": "id"}

    def get(self, **kwargs: Any) -> UserStatus:
        return cast(UserStatus, super().get(**kwargs))


class UserActivitiesManager(ListMixin, RESTManager):
    _path = "/user/activities"
    _obj_cls = UserActivities


class UserGPGKey(ObjectDeleteMixin, RESTObject):
    pass


class UserGPGKeyManager(RetrieveMixin, CreateMixin, DeleteMixin, RESTManager):
    _path = "/users/{user_id}/gpg_keys"
    _obj_cls = UserGPGKey
    _from_parent_attrs = {"user_id": "id"}
    _create_attrs = RequiredOptional(required=("key",))

    def get(self, id: Union[str, int], lazy: bool = False, **kwargs: Any) -> UserGPGKey:
        return cast(UserGPGKey, super().get(id=id, lazy=lazy, **kwargs))


class UserKey(ObjectDeleteMixin, RESTObject):
    pass


class UserKeyManager(RetrieveMixin, CreateMixin, DeleteMixin, RESTManager):
    _path = "/users/{user_id}/keys"
    _obj_cls = UserKey
    _from_parent_attrs = {"user_id": "id"}
    _create_attrs = RequiredOptional(required=("title", "key"))

    def get(self, id: Union[str, int], lazy: bool = False, **kwargs: Any) -> UserKey:
        return cast(UserKey, super().get(id=id, lazy=lazy, **kwargs))


class UserIdentityProviderManager(DeleteMixin, RESTManager):
    """Manager for user identities.

    This manager does not actually manage objects but enables
    functionality for deletion of user identities by provider.
    """

    _path = "/users/{user_id}/identities"
    _from_parent_attrs = {"user_id": "id"}


class UserImpersonationToken(ObjectDeleteMixin, RESTObject):
    pass


class UserImpersonationTokenManager(NoUpdateMixin, RESTManager):
    _path = "/users/{user_id}/impersonation_tokens"
    _obj_cls = UserImpersonationToken
    _from_parent_attrs = {"user_id": "id"}
    _create_attrs = RequiredOptional(
        required=("name", "scopes"), optional=("expires_at",)
    )
    _list_filters = ("state",)
    _types = {"scopes": ArrayAttribute}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> UserImpersonationToken:
        return cast(UserImpersonationToken, super().get(id=id, lazy=lazy, **kwargs))


class UserMembership(RESTObject):
    _id_attr = "source_id"


class UserMembershipManager(RetrieveMixin, RESTManager):
    _path = "/users/{user_id}/memberships"
    _obj_cls = UserMembership
    _from_parent_attrs = {"user_id": "id"}
    _list_filters = ("type",)

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> UserMembership:
        return cast(UserMembership, super().get(id=id, lazy=lazy, **kwargs))


# Having this outside projects avoids circular imports due to ProjectUser
class UserProject(RESTObject):
    pass


class UserProjectManager(ListMixin, CreateMixin, RESTManager):
    _path = "/projects/user/{user_id}"
    _obj_cls = UserProject
    _from_parent_attrs = {"user_id": "id"}
    _create_attrs = RequiredOptional(
        required=("name",),
        optional=(
            "default_branch",
            "issues_enabled",
            "wall_enabled",
            "merge_requests_enabled",
            "wiki_enabled",
            "snippets_enabled",
            "squash_option",
            "public",
            "visibility",
            "description",
            "builds_enabled",
            "public_builds",
            "import_url",
            "only_allow_merge_if_build_succeeds",
        ),
    )
    _list_filters = (
        "archived",
        "visibility",
        "order_by",
        "sort",
        "search",
        "simple",
        "owned",
        "membership",
        "starred",
        "statistics",
        "with_issues_enabled",
        "with_merge_requests_enabled",
        "with_custom_attributes",
        "with_programming_language",
        "wiki_checksum_failed",
        "repository_checksum_failed",
        "min_access_level",
        "id_after",
        "id_before",
    )

    def list(self, **kwargs: Any) -> Union[RESTObjectList, List[RESTObject]]:
        """Retrieve a list of objects.

        Args:
            all: If True, return all the items, without pagination
            per_page: Number of items to retrieve per request
            page: ID of the page to return (starts with page 1)
            iterator: If set to True and no pagination option is
                defined, return a generator instead of a list
            **kwargs: Extra options to send to the server (e.g. sudo)

        Returns:
            The list of objects, or a generator if `iterator` is True

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabListError: If the server cannot perform the request
        """
        if self._parent:
            path = f"/users/{self._parent.id}/projects"
        else:
            path = f"/users/{self._from_parent_attrs['user_id']}/projects"
        return ListMixin.list(self, path=path, **kwargs)


class StarredProject(RESTObject):
    pass


class StarredProjectManager(ListMixin, RESTManager):
    _path = "/users/{user_id}/starred_projects"
    _obj_cls = StarredProject
    _from_parent_attrs = {"user_id": "id"}
    _list_filters = (
        "archived",
        "membership",
        "min_access_level",
        "order_by",
        "owned",
        "search",
        "simple",
        "sort",
        "starred",
        "statistics",
        "visibility",
        "with_custom_attributes",
        "with_issues_enabled",
        "with_merge_requests_enabled",
    )


class UserFollowersManager(ListMixin, RESTManager):
    _path = "/users/{user_id}/followers"
    _obj_cls = User
    _from_parent_attrs = {"user_id": "id"}


class UserFollowingManager(ListMixin, RESTManager):
    _path = "/users/{user_id}/following"
    _obj_cls = User
    _from_parent_attrs = {"user_id": "id"}
