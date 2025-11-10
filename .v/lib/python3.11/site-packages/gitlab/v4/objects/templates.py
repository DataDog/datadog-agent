from typing import Any, cast, Union

from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import RetrieveMixin

__all__ = [
    "Dockerfile",
    "DockerfileManager",
    "Gitignore",
    "GitignoreManager",
    "Gitlabciyml",
    "GitlabciymlManager",
    "License",
    "LicenseManager",
]


class Dockerfile(RESTObject):
    _id_attr = "name"


class DockerfileManager(RetrieveMixin, RESTManager):
    _path = "/templates/dockerfiles"
    _obj_cls = Dockerfile

    def get(self, id: Union[str, int], lazy: bool = False, **kwargs: Any) -> Dockerfile:
        return cast(Dockerfile, super().get(id=id, lazy=lazy, **kwargs))


class Gitignore(RESTObject):
    _id_attr = "name"


class GitignoreManager(RetrieveMixin, RESTManager):
    _path = "/templates/gitignores"
    _obj_cls = Gitignore

    def get(self, id: Union[str, int], lazy: bool = False, **kwargs: Any) -> Gitignore:
        return cast(Gitignore, super().get(id=id, lazy=lazy, **kwargs))


class Gitlabciyml(RESTObject):
    _id_attr = "name"


class GitlabciymlManager(RetrieveMixin, RESTManager):
    _path = "/templates/gitlab_ci_ymls"
    _obj_cls = Gitlabciyml

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> Gitlabciyml:
        return cast(Gitlabciyml, super().get(id=id, lazy=lazy, **kwargs))


class License(RESTObject):
    _id_attr = "key"


class LicenseManager(RetrieveMixin, RESTManager):
    _path = "/templates/licenses"
    _obj_cls = License
    _list_filters = ("popular",)
    _optional_get_attrs = ("project", "fullname")

    def get(self, id: Union[str, int], lazy: bool = False, **kwargs: Any) -> License:
        return cast(License, super().get(id=id, lazy=lazy, **kwargs))
