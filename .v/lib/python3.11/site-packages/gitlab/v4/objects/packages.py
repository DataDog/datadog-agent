"""
GitLab API:
https://docs.gitlab.com/ee/api/packages.html
https://docs.gitlab.com/ee/user/packages/generic_packages/
"""

from pathlib import Path
from typing import (
    Any,
    BinaryIO,
    Callable,
    cast,
    Iterator,
    Optional,
    TYPE_CHECKING,
    Union,
)

import requests

from gitlab import cli
from gitlab import exceptions as exc
from gitlab import utils
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import DeleteMixin, GetMixin, ListMixin, ObjectDeleteMixin

__all__ = [
    "GenericPackage",
    "GenericPackageManager",
    "GroupPackage",
    "GroupPackageManager",
    "ProjectPackage",
    "ProjectPackageManager",
    "ProjectPackageFile",
    "ProjectPackageFileManager",
    "ProjectPackagePipeline",
    "ProjectPackagePipelineManager",
]


class GenericPackage(RESTObject):
    _id_attr = "package_name"


class GenericPackageManager(RESTManager):
    _path = "/projects/{project_id}/packages/generic"
    _obj_cls = GenericPackage
    _from_parent_attrs = {"project_id": "id"}

    @cli.register_custom_action(
        "GenericPackageManager",
        ("package_name", "package_version", "file_name", "path"),
    )
    @exc.on_http_error(exc.GitlabUploadError)
    def upload(
        self,
        package_name: str,
        package_version: str,
        file_name: str,
        path: Optional[Union[str, Path]] = None,
        select: Optional[str] = None,
        data: Optional[Union[bytes, BinaryIO]] = None,
        **kwargs: Any,
    ) -> GenericPackage:
        """Upload a file as a generic package.

        Args:
            package_name: The package name. Must follow generic package
                                name regex rules
            package_version: The package version. Must follow semantic
                                version regex rules
            file_name: The name of the file as uploaded in the registry
            path: The path to a local file to upload
            select: GitLab API accepts a value of 'package_file'

        Raises:
            GitlabConnectionError: If the server cannot be reached
            GitlabUploadError: If the file upload fails
            GitlabUploadError: If ``path`` cannot be read
            GitlabUploadError: If both ``path`` and ``data`` are passed

        Returns:
            An object storing the metadata of the uploaded package.

        https://docs.gitlab.com/ee/user/packages/generic_packages/
        """

        if path is None and data is None:
            raise exc.GitlabUploadError("No file contents or path specified")

        if path is not None and data is not None:
            raise exc.GitlabUploadError("File contents and file path specified")

        file_data: Optional[Union[bytes, BinaryIO]] = data

        if not file_data:
            if TYPE_CHECKING:
                assert path is not None

            try:
                with open(path, "rb") as f:
                    file_data = f.read()
            except OSError as e:
                raise exc.GitlabUploadError(
                    f"Failed to read package file {path}"
                ) from e

        url = f"{self._computed_path}/{package_name}/{package_version}/{file_name}"
        query_data = {} if select is None else {"select": select}
        server_data = self.gitlab.http_put(
            url, query_data=query_data, post_data=file_data, raw=True, **kwargs
        )
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)

        attrs = {
            "package_name": package_name,
            "package_version": package_version,
            "file_name": file_name,
            "path": path,
        }
        attrs.update(server_data)
        return self._obj_cls(self, attrs=attrs)

    @cli.register_custom_action(
        "GenericPackageManager",
        ("package_name", "package_version", "file_name"),
    )
    @exc.on_http_error(exc.GitlabGetError)
    def download(
        self,
        package_name: str,
        package_version: str,
        file_name: str,
        streamed: bool = False,
        action: Optional[Callable[[bytes], None]] = None,
        chunk_size: int = 1024,
        *,
        iterator: bool = False,
        **kwargs: Any,
    ) -> Optional[Union[bytes, Iterator[Any]]]:
        """Download a generic package.

        Args:
            package_name: The package name.
            package_version: The package version.
            file_name: The name of the file in the registry
            streamed: If True the data will be processed by chunks of
                `chunk_size` and each chunk is passed to `action` for
                treatment
            iterator: If True directly return the underlying response
                iterator
            action: Callable responsible of dealing with chunk of
                data
            chunk_size: Size of each chunk
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetError: If the server failed to perform the request

        Returns:
            The package content if streamed is False, None otherwise
        """
        path = f"{self._computed_path}/{package_name}/{package_version}/{file_name}"
        result = self.gitlab.http_get(path, streamed=streamed, raw=True, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(result, requests.Response)
        return utils.response_content(
            result, streamed, action, chunk_size, iterator=iterator
        )


class GroupPackage(RESTObject):
    pass


class GroupPackageManager(ListMixin, RESTManager):
    _path = "/groups/{group_id}/packages"
    _obj_cls = GroupPackage
    _from_parent_attrs = {"group_id": "id"}
    _list_filters = (
        "exclude_subgroups",
        "order_by",
        "sort",
        "package_type",
        "package_name",
    )


class ProjectPackage(ObjectDeleteMixin, RESTObject):
    package_files: "ProjectPackageFileManager"
    pipelines: "ProjectPackagePipelineManager"


class ProjectPackageManager(ListMixin, GetMixin, DeleteMixin, RESTManager):
    _path = "/projects/{project_id}/packages"
    _obj_cls = ProjectPackage
    _from_parent_attrs = {"project_id": "id"}
    _list_filters = (
        "order_by",
        "sort",
        "package_type",
        "package_name",
    )

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectPackage:
        return cast(ProjectPackage, super().get(id=id, lazy=lazy, **kwargs))


class ProjectPackageFile(ObjectDeleteMixin, RESTObject):
    pass


class ProjectPackageFileManager(DeleteMixin, ListMixin, RESTManager):
    _path = "/projects/{project_id}/packages/{package_id}/package_files"
    _obj_cls = ProjectPackageFile
    _from_parent_attrs = {"project_id": "project_id", "package_id": "id"}


class ProjectPackagePipeline(RESTObject):
    pass


class ProjectPackagePipelineManager(ListMixin, RESTManager):
    _path = "/projects/{project_id}/packages/{package_id}/pipelines"
    _obj_cls = ProjectPackagePipeline
    _from_parent_attrs = {"project_id": "project_id", "package_id": "id"}
