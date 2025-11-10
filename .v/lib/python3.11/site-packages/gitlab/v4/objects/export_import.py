from typing import Any, cast

from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import CreateMixin, DownloadMixin, GetWithoutIdMixin, RefreshMixin
from gitlab.types import RequiredOptional

__all__ = [
    "GroupExport",
    "GroupExportManager",
    "GroupImport",
    "GroupImportManager",
    "ProjectExport",
    "ProjectExportManager",
    "ProjectImport",
    "ProjectImportManager",
]


class GroupExport(DownloadMixin, RESTObject):
    _id_attr = None


class GroupExportManager(GetWithoutIdMixin, CreateMixin, RESTManager):
    _path = "/groups/{group_id}/export"
    _obj_cls = GroupExport
    _from_parent_attrs = {"group_id": "id"}

    def get(self, **kwargs: Any) -> GroupExport:
        return cast(GroupExport, super().get(**kwargs))


class GroupImport(RESTObject):
    _id_attr = None


class GroupImportManager(GetWithoutIdMixin, RESTManager):
    _path = "/groups/{group_id}/import"
    _obj_cls = GroupImport
    _from_parent_attrs = {"group_id": "id"}

    def get(self, **kwargs: Any) -> GroupImport:
        return cast(GroupImport, super().get(**kwargs))


class ProjectExport(DownloadMixin, RefreshMixin, RESTObject):
    _id_attr = None


class ProjectExportManager(GetWithoutIdMixin, CreateMixin, RESTManager):
    _path = "/projects/{project_id}/export"
    _obj_cls = ProjectExport
    _from_parent_attrs = {"project_id": "id"}
    _create_attrs = RequiredOptional(optional=("description",))

    def get(self, **kwargs: Any) -> ProjectExport:
        return cast(ProjectExport, super().get(**kwargs))


class ProjectImport(RefreshMixin, RESTObject):
    _id_attr = None


class ProjectImportManager(GetWithoutIdMixin, RESTManager):
    _path = "/projects/{project_id}/import"
    _obj_cls = ProjectImport
    _from_parent_attrs = {"project_id": "id"}

    def get(self, **kwargs: Any) -> ProjectImport:
        return cast(ProjectImport, super().get(**kwargs))
