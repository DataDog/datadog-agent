from typing import Any, cast, TYPE_CHECKING, Union

from gitlab import cli
from gitlab import exceptions as exc
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import RetrieveMixin
from gitlab.utils import EncodedId

__all__ = [
    "Namespace",
    "NamespaceManager",
]


class Namespace(RESTObject):
    pass


class NamespaceManager(RetrieveMixin, RESTManager):
    _path = "/namespaces"
    _obj_cls = Namespace
    _list_filters = ("search",)

    def get(self, id: Union[str, int], lazy: bool = False, **kwargs: Any) -> Namespace:
        return cast(Namespace, super().get(id=id, lazy=lazy, **kwargs))

    @cli.register_custom_action("NamespaceManager", ("namespace", "parent_id"))
    @exc.on_http_error(exc.GitlabGetError)
    def exists(self, namespace: str, **kwargs: Any) -> Namespace:
        """Get existence of a namespace by path.

        Args:
            namespace: The path to the namespace.
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetError: If the server failed to perform the request

        Returns:
            Data on namespace existence returned from the server.
        """
        path = f"{self.path}/{EncodedId(namespace)}/exists"
        server_data = self.gitlab.http_get(path, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        return self._obj_cls(self, server_data)
