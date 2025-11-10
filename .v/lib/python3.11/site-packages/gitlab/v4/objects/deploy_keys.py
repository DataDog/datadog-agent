from typing import Any, cast, Dict, Union

import requests

from gitlab import cli
from gitlab import exceptions as exc
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import CRUDMixin, ListMixin, ObjectDeleteMixin, SaveMixin
from gitlab.types import RequiredOptional

__all__ = [
    "DeployKey",
    "DeployKeyManager",
    "ProjectKey",
    "ProjectKeyManager",
]


class DeployKey(RESTObject):
    pass


class DeployKeyManager(ListMixin, RESTManager):
    _path = "/deploy_keys"
    _obj_cls = DeployKey


class ProjectKey(SaveMixin, ObjectDeleteMixin, RESTObject):
    pass


class ProjectKeyManager(CRUDMixin, RESTManager):
    _path = "/projects/{project_id}/deploy_keys"
    _obj_cls = ProjectKey
    _from_parent_attrs = {"project_id": "id"}
    _create_attrs = RequiredOptional(required=("title", "key"), optional=("can_push",))
    _update_attrs = RequiredOptional(optional=("title", "can_push"))

    @cli.register_custom_action("ProjectKeyManager", ("key_id",))
    @exc.on_http_error(exc.GitlabProjectDeployKeyError)
    def enable(
        self, key_id: int, **kwargs: Any
    ) -> Union[Dict[str, Any], requests.Response]:
        """Enable a deploy key for a project.

        Args:
            key_id: The ID of the key to enable
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabProjectDeployKeyError: If the key could not be enabled

        Returns:
            A dict of the result.
        """
        path = f"{self.path}/{key_id}/enable"
        return self.gitlab.http_post(path, **kwargs)

    def get(self, id: Union[str, int], lazy: bool = False, **kwargs: Any) -> ProjectKey:
        return cast(ProjectKey, super().get(id=id, lazy=lazy, **kwargs))
