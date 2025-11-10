from typing import Any, cast, Union

from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import CRUDMixin, ListMixin, ObjectDeleteMixin, SaveMixin
from gitlab.types import RequiredOptional

__all__ = [
    "PagesDomain",
    "PagesDomainManager",
    "ProjectPagesDomain",
    "ProjectPagesDomainManager",
]


class PagesDomain(RESTObject):
    _id_attr = "domain"


class PagesDomainManager(ListMixin, RESTManager):
    _path = "/pages/domains"
    _obj_cls = PagesDomain


class ProjectPagesDomain(SaveMixin, ObjectDeleteMixin, RESTObject):
    _id_attr = "domain"


class ProjectPagesDomainManager(CRUDMixin, RESTManager):
    _path = "/projects/{project_id}/pages/domains"
    _obj_cls = ProjectPagesDomain
    _from_parent_attrs = {"project_id": "id"}
    _create_attrs = RequiredOptional(
        required=("domain",), optional=("certificate", "key")
    )
    _update_attrs = RequiredOptional(optional=("certificate", "key"))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectPagesDomain:
        return cast(ProjectPagesDomain, super().get(id=id, lazy=lazy, **kwargs))
