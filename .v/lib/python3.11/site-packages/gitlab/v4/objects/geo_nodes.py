from typing import Any, cast, Dict, List, TYPE_CHECKING, Union

from gitlab import cli
from gitlab import exceptions as exc
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import (
    DeleteMixin,
    ObjectDeleteMixin,
    RetrieveMixin,
    SaveMixin,
    UpdateMixin,
)
from gitlab.types import RequiredOptional

__all__ = [
    "GeoNode",
    "GeoNodeManager",
]


class GeoNode(SaveMixin, ObjectDeleteMixin, RESTObject):
    @cli.register_custom_action("GeoNode")
    @exc.on_http_error(exc.GitlabRepairError)
    def repair(self, **kwargs: Any) -> None:
        """Repair the OAuth authentication of the geo node.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabRepairError: If the server failed to perform the request
        """
        path = f"/geo_nodes/{self.encoded_id}/repair"
        server_data = self.manager.gitlab.http_post(path, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        self._update_attrs(server_data)

    @cli.register_custom_action("GeoNode")
    @exc.on_http_error(exc.GitlabGetError)
    def status(self, **kwargs: Any) -> Dict[str, Any]:
        """Get the status of the geo node.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetError: If the server failed to perform the request

        Returns:
            The status of the geo node
        """
        path = f"/geo_nodes/{self.encoded_id}/status"
        result = self.manager.gitlab.http_get(path, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(result, dict)
        return result


class GeoNodeManager(RetrieveMixin, UpdateMixin, DeleteMixin, RESTManager):
    _path = "/geo_nodes"
    _obj_cls = GeoNode
    _update_attrs = RequiredOptional(
        optional=("enabled", "url", "files_max_capacity", "repos_max_capacity"),
    )

    def get(self, id: Union[str, int], lazy: bool = False, **kwargs: Any) -> GeoNode:
        return cast(GeoNode, super().get(id=id, lazy=lazy, **kwargs))

    @cli.register_custom_action("GeoNodeManager")
    @exc.on_http_error(exc.GitlabGetError)
    def status(self, **kwargs: Any) -> List[Dict[str, Any]]:
        """Get the status of all the geo nodes.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetError: If the server failed to perform the request

        Returns:
            The status of all the geo nodes
        """
        result = self.gitlab.http_list("/geo_nodes/status", **kwargs)
        if TYPE_CHECKING:
            assert isinstance(result, list)
        return result

    @cli.register_custom_action("GeoNodeManager")
    @exc.on_http_error(exc.GitlabGetError)
    def current_failures(self, **kwargs: Any) -> List[Dict[str, Any]]:
        """Get the list of failures on the current geo node.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetError: If the server failed to perform the request

        Returns:
            The list of failures
        """
        result = self.gitlab.http_list("/geo_nodes/current/failures", **kwargs)
        if TYPE_CHECKING:
            assert isinstance(result, list)
        return result
