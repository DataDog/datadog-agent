from typing import Any, cast, Dict, Optional, Union

from gitlab import exceptions as exc
from gitlab import types
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import GetWithoutIdMixin, SaveMixin, UpdateMixin
from gitlab.types import RequiredOptional

__all__ = [
    "ApplicationSettings",
    "ApplicationSettingsManager",
]


class ApplicationSettings(SaveMixin, RESTObject):
    _id_attr = None


class ApplicationSettingsManager(GetWithoutIdMixin, UpdateMixin, RESTManager):
    _path = "/application/settings"
    _obj_cls = ApplicationSettings
    _update_attrs = RequiredOptional(
        optional=(
            "id",
            "default_projects_limit",
            "signup_enabled",
            "password_authentication_enabled_for_web",
            "gravatar_enabled",
            "sign_in_text",
            "created_at",
            "updated_at",
            "home_page_url",
            "default_branch_protection",
            "restricted_visibility_levels",
            "max_attachment_size",
            "session_expire_delay",
            "default_project_visibility",
            "default_snippet_visibility",
            "default_group_visibility",
            "outbound_local_requests_whitelist",
            "disabled_oauth_sign_in_sources",
            "domain_whitelist",
            "domain_blacklist_enabled",
            "domain_blacklist",
            "domain_allowlist",
            "domain_denylist_enabled",
            "domain_denylist",
            "external_authorization_service_enabled",
            "external_authorization_service_url",
            "external_authorization_service_default_label",
            "external_authorization_service_timeout",
            "import_sources",
            "user_oauth_applications",
            "after_sign_out_path",
            "container_registry_token_expire_delay",
            "repository_storages",
            "plantuml_enabled",
            "plantuml_url",
            "terminal_max_session_time",
            "polling_interval_multiplier",
            "rsa_key_restriction",
            "dsa_key_restriction",
            "ecdsa_key_restriction",
            "ed25519_key_restriction",
            "first_day_of_week",
            "enforce_terms",
            "terms",
            "performance_bar_allowed_group_id",
            "instance_statistics_visibility_private",
            "user_show_add_ssh_key_message",
            "file_template_project_id",
            "local_markdown_version",
            "asset_proxy_enabled",
            "asset_proxy_url",
            "asset_proxy_whitelist",
            "asset_proxy_allowlist",
            "geo_node_allowed_ips",
            "allow_local_requests_from_hooks_and_services",
            "allow_local_requests_from_web_hooks_and_services",
            "allow_local_requests_from_system_hooks",
        ),
    )
    _types = {
        "asset_proxy_allowlist": types.ArrayAttribute,
        "disabled_oauth_sign_in_sources": types.ArrayAttribute,
        "domain_allowlist": types.ArrayAttribute,
        "domain_denylist": types.ArrayAttribute,
        "import_sources": types.ArrayAttribute,
        "restricted_visibility_levels": types.ArrayAttribute,
    }

    @exc.on_http_error(exc.GitlabUpdateError)
    def update(
        self,
        id: Optional[Union[str, int]] = None,
        new_data: Optional[Dict[str, Any]] = None,
        **kwargs: Any,
    ) -> Dict[str, Any]:
        """Update an object on the server.

        Args:
            id: ID of the object to update (can be None if not required)
            new_data: the update data for the object
            **kwargs: Extra options to send to the server (e.g. sudo)

        Returns:
            The new object data (*not* a RESTObject)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabUpdateError: If the server cannot perform the request
        """
        new_data = new_data or {}
        data = new_data.copy()
        if "domain_whitelist" in data and data["domain_whitelist"] is None:
            data.pop("domain_whitelist")
        return super().update(id, data, **kwargs)

    def get(self, **kwargs: Any) -> ApplicationSettings:
        return cast(ApplicationSettings, super().get(**kwargs))
