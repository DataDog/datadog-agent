from typing import Any, cast, Dict, TYPE_CHECKING, Union

from gitlab import cli
from gitlab import exceptions as exc
from gitlab import types
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import CRUDMixin, ObjectDeleteMixin, SaveMixin
from gitlab.types import RequiredOptional

__all__ = [
    "Topic",
    "TopicManager",
]


class Topic(SaveMixin, ObjectDeleteMixin, RESTObject):
    pass


class TopicManager(CRUDMixin, RESTManager):
    _path = "/topics"
    _obj_cls = Topic
    _create_attrs = RequiredOptional(
        # NOTE: The `title` field was added and is required in GitLab 15.0 or
        # newer. But not present before that.
        required=("name",),
        optional=("avatar", "description", "title"),
    )
    _update_attrs = RequiredOptional(optional=("avatar", "description", "name"))
    _types = {"avatar": types.ImageAttribute}

    def get(self, id: Union[str, int], lazy: bool = False, **kwargs: Any) -> Topic:
        return cast(Topic, super().get(id=id, lazy=lazy, **kwargs))

    @cli.register_custom_action(
        "TopicManager",
        mandatory=("source_topic_id", "target_topic_id"),
    )
    @exc.on_http_error(exc.GitlabMRClosedError)
    def merge(
        self,
        source_topic_id: Union[int, str],
        target_topic_id: Union[int, str],
        **kwargs: Any,
    ) -> Dict[str, Any]:
        """Merge two topics, assigning all projects to the target topic.

        Args:
            source_topic_id: ID of source project topic
            target_topic_id: ID of target project topic
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabTopicMergeError: If the merge failed

        Returns:
            The merged topic data (*not* a RESTObject)
        """
        path = f"{self.path}/merge"
        data = {
            "source_topic_id": source_topic_id,
            "target_topic_id": target_topic_id,
        }

        server_data = self.gitlab.http_post(path, post_data=data, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        return server_data
