from typing import Any, cast, Dict, Optional, Union

from gitlab import exceptions as exc
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import CreateMixin, CRUDMixin, ObjectDeleteMixin, SaveMixin
from gitlab.types import RequiredOptional

__all__ = [
    "GroupCluster",
    "GroupClusterManager",
    "ProjectCluster",
    "ProjectClusterManager",
]


class GroupCluster(SaveMixin, ObjectDeleteMixin, RESTObject):
    pass


class GroupClusterManager(CRUDMixin, RESTManager):
    _path = "/groups/{group_id}/clusters"
    _obj_cls = GroupCluster
    _from_parent_attrs = {"group_id": "id"}
    _create_attrs = RequiredOptional(
        required=("name", "platform_kubernetes_attributes"),
        optional=("domain", "enabled", "managed", "environment_scope"),
    )
    _update_attrs = RequiredOptional(
        optional=(
            "name",
            "domain",
            "management_project_id",
            "platform_kubernetes_attributes",
            "environment_scope",
        ),
    )

    @exc.on_http_error(exc.GitlabStopError)
    def create(
        self, data: Optional[Dict[str, Any]] = None, **kwargs: Any
    ) -> GroupCluster:
        """Create a new object.

        Args:
            data: Parameters to send to the server to create the
                         resource
            **kwargs: Extra options to send to the server (e.g. sudo or
                      'ref_name', 'stage', 'name', 'all')

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabCreateError: If the server cannot perform the request

        Returns:
            A new instance of the manage object class build with
                the data sent by the server
        """
        path = f"{self.path}/user"
        return cast(GroupCluster, CreateMixin.create(self, data, path=path, **kwargs))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> GroupCluster:
        return cast(GroupCluster, super().get(id=id, lazy=lazy, **kwargs))


class ProjectCluster(SaveMixin, ObjectDeleteMixin, RESTObject):
    pass


class ProjectClusterManager(CRUDMixin, RESTManager):
    _path = "/projects/{project_id}/clusters"
    _obj_cls = ProjectCluster
    _from_parent_attrs = {"project_id": "id"}
    _create_attrs = RequiredOptional(
        required=("name", "platform_kubernetes_attributes"),
        optional=("domain", "enabled", "managed", "environment_scope"),
    )
    _update_attrs = RequiredOptional(
        optional=(
            "name",
            "domain",
            "management_project_id",
            "platform_kubernetes_attributes",
            "environment_scope",
        ),
    )

    @exc.on_http_error(exc.GitlabStopError)
    def create(
        self, data: Optional[Dict[str, Any]] = None, **kwargs: Any
    ) -> ProjectCluster:
        """Create a new object.

        Args:
            data: Parameters to send to the server to create the
                         resource
            **kwargs: Extra options to send to the server (e.g. sudo or
                      'ref_name', 'stage', 'name', 'all')

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabCreateError: If the server cannot perform the request

        Returns:
            A new instance of the manage object class build with
                the data sent by the server
        """
        path = f"{self.path}/user"
        return cast(ProjectCluster, CreateMixin.create(self, data, path=path, **kwargs))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectCluster:
        return cast(ProjectCluster, super().get(id=id, lazy=lazy, **kwargs))
