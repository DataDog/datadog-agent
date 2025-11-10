from typing import Any, cast, Dict, Union

import requests

from gitlab import cli
from gitlab import exceptions as exc
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import (
    CreateMixin,
    DeleteMixin,
    ObjectDeleteMixin,
    RetrieveMixin,
    SaveMixin,
    UpdateMixin,
)
from gitlab.types import ArrayAttribute, RequiredOptional

__all__ = [
    "ProjectEnvironment",
    "ProjectEnvironmentManager",
    "ProjectProtectedEnvironment",
    "ProjectProtectedEnvironmentManager",
]


class ProjectEnvironment(SaveMixin, ObjectDeleteMixin, RESTObject):
    @cli.register_custom_action("ProjectEnvironment")
    @exc.on_http_error(exc.GitlabStopError)
    def stop(self, **kwargs: Any) -> Union[Dict[str, Any], requests.Response]:
        """Stop the environment.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabStopError: If the operation failed

        Returns:
           A dict of the result.
        """
        path = f"{self.manager.path}/{self.encoded_id}/stop"
        return self.manager.gitlab.http_post(path, **kwargs)


class ProjectEnvironmentManager(
    RetrieveMixin, CreateMixin, UpdateMixin, DeleteMixin, RESTManager
):
    _path = "/projects/{project_id}/environments"
    _obj_cls = ProjectEnvironment
    _from_parent_attrs = {"project_id": "id"}
    _create_attrs = RequiredOptional(required=("name",), optional=("external_url",))
    _update_attrs = RequiredOptional(optional=("name", "external_url"))
    _list_filters = ("name", "search", "states")

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectEnvironment:
        return cast(ProjectEnvironment, super().get(id=id, lazy=lazy, **kwargs))


class ProjectProtectedEnvironment(ObjectDeleteMixin, RESTObject):
    _id_attr = "name"
    _repr_attr = "name"


class ProjectProtectedEnvironmentManager(
    RetrieveMixin, CreateMixin, DeleteMixin, RESTManager
):
    _path = "/projects/{project_id}/protected_environments"
    _obj_cls = ProjectProtectedEnvironment
    _from_parent_attrs = {"project_id": "id"}
    _create_attrs = RequiredOptional(
        required=(
            "name",
            "deploy_access_levels",
        ),
        optional=("required_approval_count", "approval_rules"),
    )
    _types = {"deploy_access_levels": ArrayAttribute, "approval_rules": ArrayAttribute}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectProtectedEnvironment:
        return cast(
            ProjectProtectedEnvironment, super().get(id=id, lazy=lazy, **kwargs)
        )
