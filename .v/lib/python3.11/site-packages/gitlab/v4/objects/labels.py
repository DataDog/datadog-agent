from typing import Any, cast, Dict, Optional, Union

from gitlab import exceptions as exc
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import (
    CreateMixin,
    DeleteMixin,
    ObjectDeleteMixin,
    PromoteMixin,
    RetrieveMixin,
    SaveMixin,
    SubscribableMixin,
    UpdateMixin,
)
from gitlab.types import RequiredOptional

__all__ = [
    "GroupLabel",
    "GroupLabelManager",
    "ProjectLabel",
    "ProjectLabelManager",
]


class GroupLabel(SubscribableMixin, SaveMixin, ObjectDeleteMixin, RESTObject):
    _id_attr = "name"
    manager: "GroupLabelManager"

    # Update without ID, but we need an ID to get from list.
    @exc.on_http_error(exc.GitlabUpdateError)
    def save(self, **kwargs: Any) -> None:
        """Saves the changes made to the object to the server.

        The object is updated to match what the server returns.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct.
            GitlabUpdateError: If the server cannot perform the request.
        """
        updated_data = self._get_updated_data()

        # call the manager
        server_data = self.manager.update(None, updated_data, **kwargs)
        self._update_attrs(server_data)


class GroupLabelManager(
    RetrieveMixin, CreateMixin, UpdateMixin, DeleteMixin, RESTManager
):
    _path = "/groups/{group_id}/labels"
    _obj_cls = GroupLabel
    _from_parent_attrs = {"group_id": "id"}
    _create_attrs = RequiredOptional(
        required=("name", "color"), optional=("description", "priority")
    )
    _update_attrs = RequiredOptional(
        required=("name",), optional=("new_name", "color", "description", "priority")
    )

    def get(self, id: Union[str, int], lazy: bool = False, **kwargs: Any) -> GroupLabel:
        return cast(GroupLabel, super().get(id=id, lazy=lazy, **kwargs))

    # Update without ID.
    # NOTE(jlvillal): Signature doesn't match UpdateMixin.update() so ignore
    # type error
    def update(  # type: ignore
        self,
        name: Optional[str],
        new_data: Optional[Dict[str, Any]] = None,
        **kwargs: Any,
    ) -> Dict[str, Any]:
        """Update a Label on the server.

        Args:
            name: The name of the label
            **kwargs: Extra options to send to the server (e.g. sudo)
        """
        new_data = new_data or {}
        if name:
            new_data["name"] = name
        return super().update(id=None, new_data=new_data, **kwargs)


class ProjectLabel(
    PromoteMixin, SubscribableMixin, SaveMixin, ObjectDeleteMixin, RESTObject
):
    _id_attr = "name"
    manager: "ProjectLabelManager"

    # Update without ID, but we need an ID to get from list.
    @exc.on_http_error(exc.GitlabUpdateError)
    def save(self, **kwargs: Any) -> None:
        """Saves the changes made to the object to the server.

        The object is updated to match what the server returns.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct.
            GitlabUpdateError: If the server cannot perform the request.
        """
        updated_data = self._get_updated_data()

        # call the manager
        server_data = self.manager.update(None, updated_data, **kwargs)
        self._update_attrs(server_data)


class ProjectLabelManager(
    RetrieveMixin, CreateMixin, UpdateMixin, DeleteMixin, RESTManager
):
    _path = "/projects/{project_id}/labels"
    _obj_cls = ProjectLabel
    _from_parent_attrs = {"project_id": "id"}
    _create_attrs = RequiredOptional(
        required=("name", "color"), optional=("description", "priority")
    )
    _update_attrs = RequiredOptional(
        required=("name",), optional=("new_name", "color", "description", "priority")
    )

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectLabel:
        return cast(ProjectLabel, super().get(id=id, lazy=lazy, **kwargs))

    # Update without ID.
    # NOTE(jlvillal): Signature doesn't match UpdateMixin.update() so ignore
    # type error
    def update(  # type: ignore
        self,
        name: Optional[str],
        new_data: Optional[Dict[str, Any]] = None,
        **kwargs: Any,
    ) -> Dict[str, Any]:
        """Update a Label on the server.

        Args:
            name: The name of the label
            **kwargs: Extra options to send to the server (e.g. sudo)
        """
        new_data = new_data or {}
        if name:
            new_data["name"] = name
        return super().update(id=None, new_data=new_data, **kwargs)
