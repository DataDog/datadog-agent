"""
GitLab API:
https://docs.gitlab.com/ee/api/features.html
"""
from typing import Any, Optional, TYPE_CHECKING, Union

from gitlab import exceptions as exc
from gitlab import utils
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import DeleteMixin, ListMixin, ObjectDeleteMixin

__all__ = [
    "Feature",
    "FeatureManager",
]


class Feature(ObjectDeleteMixin, RESTObject):
    _id_attr = "name"


class FeatureManager(ListMixin, DeleteMixin, RESTManager):
    _path = "/features/"
    _obj_cls = Feature

    @exc.on_http_error(exc.GitlabSetError)
    def set(
        self,
        name: str,
        value: Union[bool, int],
        feature_group: Optional[str] = None,
        user: Optional[str] = None,
        group: Optional[str] = None,
        project: Optional[str] = None,
        **kwargs: Any,
    ) -> Feature:
        """Create or update the object.

        Args:
            name: The value to set for the object
            value: The value to set for the object
            feature_group: A feature group name
            user: A GitLab username
            group: A GitLab group
            project: A GitLab project in form group/project
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabSetError: If an error occurred

        Returns:
            The created/updated attribute
        """
        name = utils.EncodedId(name)
        path = f"{self.path}/{name}"
        data = {
            "value": value,
            "feature_group": feature_group,
            "user": user,
            "group": group,
            "project": project,
        }
        data = utils.remove_none_from_dict(data)
        server_data = self.gitlab.http_post(path, post_data=data, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        return self._obj_cls(self, server_data)
