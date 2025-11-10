from typing import Any, Dict, TYPE_CHECKING

from gitlab import cli
from gitlab import exceptions as exc
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import DeleteMixin, ListMixin, ObjectDeleteMixin

__all__ = [
    "Todo",
    "TodoManager",
]


class Todo(ObjectDeleteMixin, RESTObject):
    @cli.register_custom_action("Todo")
    @exc.on_http_error(exc.GitlabTodoError)
    def mark_as_done(self, **kwargs: Any) -> Dict[str, Any]:
        """Mark the todo as done.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabTodoError: If the server failed to perform the request

        Returns:
            A dict with the result
        """
        path = f"{self.manager.path}/{self.encoded_id}/mark_as_done"
        server_data = self.manager.gitlab.http_post(path, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        self._update_attrs(server_data)
        return server_data


class TodoManager(ListMixin, DeleteMixin, RESTManager):
    _path = "/todos"
    _obj_cls = Todo
    _list_filters = ("action", "author_id", "project_id", "state", "type")

    @cli.register_custom_action("TodoManager")
    @exc.on_http_error(exc.GitlabTodoError)
    def mark_all_as_done(self, **kwargs: Any) -> None:
        """Mark all the todos as done.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabTodoError: If the server failed to perform the request

        Returns:
            The number of todos marked done
        """
        self.gitlab.http_post("/todos/mark_as_done", **kwargs)
