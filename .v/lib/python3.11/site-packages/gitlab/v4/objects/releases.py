from typing import Any, cast, Union

from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import CRUDMixin, ObjectDeleteMixin, SaveMixin
from gitlab.types import ArrayAttribute, RequiredOptional

__all__ = [
    "ProjectRelease",
    "ProjectReleaseManager",
    "ProjectReleaseLink",
    "ProjectReleaseLinkManager",
]


class ProjectRelease(SaveMixin, RESTObject):
    _id_attr = "tag_name"

    links: "ProjectReleaseLinkManager"


class ProjectReleaseManager(CRUDMixin, RESTManager):
    _path = "/projects/{project_id}/releases"
    _obj_cls = ProjectRelease
    _from_parent_attrs = {"project_id": "id"}
    _create_attrs = RequiredOptional(
        required=("tag_name",), optional=("name", "description", "ref", "assets")
    )
    _list_filters = (
        "order_by",
        "sort",
        "include_html_description",
    )
    _update_attrs = RequiredOptional(
        optional=("name", "description", "milestones", "released_at")
    )
    _types = {"milestones": ArrayAttribute}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectRelease:
        return cast(ProjectRelease, super().get(id=id, lazy=lazy, **kwargs))


class ProjectReleaseLink(ObjectDeleteMixin, SaveMixin, RESTObject):
    pass


class ProjectReleaseLinkManager(CRUDMixin, RESTManager):
    _path = "/projects/{project_id}/releases/{tag_name}/assets/links"
    _obj_cls = ProjectReleaseLink
    _from_parent_attrs = {"project_id": "project_id", "tag_name": "tag_name"}
    _create_attrs = RequiredOptional(
        required=("name", "url"),
        optional=("filepath", "direct_asset_path", "link_type"),
    )
    _update_attrs = RequiredOptional(
        optional=("name", "url", "filepath", "direct_asset_path", "link_type")
    )

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectReleaseLink:
        return cast(ProjectReleaseLink, super().get(id=id, lazy=lazy, **kwargs))
