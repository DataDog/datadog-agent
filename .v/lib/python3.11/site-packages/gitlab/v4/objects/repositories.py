"""
GitLab API: https://docs.gitlab.com/ee/api/repositories.html

Currently this module only contains repository-related methods for projects.
"""
from typing import Any, Callable, Dict, Iterator, List, Optional, TYPE_CHECKING, Union

import requests

import gitlab
from gitlab import cli
from gitlab import exceptions as exc
from gitlab import types, utils

if TYPE_CHECKING:
    # When running mypy we use these as the base classes
    _RestObjectBase = gitlab.base.RESTObject
else:
    _RestObjectBase = object


class RepositoryMixin(_RestObjectBase):
    @cli.register_custom_action("Project", ("submodule", "branch", "commit_sha"))
    @exc.on_http_error(exc.GitlabUpdateError)
    def update_submodule(
        self, submodule: str, branch: str, commit_sha: str, **kwargs: Any
    ) -> Union[Dict[str, Any], requests.Response]:
        """Update a project submodule

        Args:
            submodule: Full path to the submodule
            branch: Name of the branch to commit into
            commit_sha: Full commit SHA to update the submodule to
            commit_message: Commit message. If no message is provided, a
                default one will be set (optional)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabPutError: If the submodule could not be updated
        """

        submodule = utils.EncodedId(submodule)
        path = f"/projects/{self.encoded_id}/repository/submodules/{submodule}"
        data = {"branch": branch, "commit_sha": commit_sha}
        if "commit_message" in kwargs:
            data["commit_message"] = kwargs["commit_message"]
        return self.manager.gitlab.http_put(path, post_data=data)

    @cli.register_custom_action("Project", (), ("path", "ref", "recursive"))
    @exc.on_http_error(exc.GitlabGetError)
    def repository_tree(
        self, path: str = "", ref: str = "", recursive: bool = False, **kwargs: Any
    ) -> Union[gitlab.client.GitlabList, List[Dict[str, Any]]]:
        """Return a list of files in the repository.

        Args:
            path: Path of the top folder (/ by default)
            ref: Reference to a commit or branch
            recursive: Whether to get the tree recursively
            all: If True, return all the items, without pagination
            per_page: Number of items to retrieve per request
            page: ID of the page to return (starts with page 1)
            iterator: If set to True and no pagination option is
                defined, return a generator instead of a list
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetError: If the server failed to perform the request

        Returns:
            The representation of the tree
        """
        gl_path = f"/projects/{self.encoded_id}/repository/tree"
        query_data: Dict[str, Any] = {"recursive": recursive}
        if path:
            query_data["path"] = path
        if ref:
            query_data["ref"] = ref
        return self.manager.gitlab.http_list(gl_path, query_data=query_data, **kwargs)

    @cli.register_custom_action("Project", ("sha",))
    @exc.on_http_error(exc.GitlabGetError)
    def repository_blob(
        self, sha: str, **kwargs: Any
    ) -> Union[Dict[str, Any], requests.Response]:
        """Return a file by blob SHA.

        Args:
            sha: ID of the blob
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetError: If the server failed to perform the request

        Returns:
            The blob content and metadata
        """

        path = f"/projects/{self.encoded_id}/repository/blobs/{sha}"
        return self.manager.gitlab.http_get(path, **kwargs)

    @cli.register_custom_action("Project", ("sha",))
    @exc.on_http_error(exc.GitlabGetError)
    def repository_raw_blob(
        self,
        sha: str,
        streamed: bool = False,
        action: Optional[Callable[..., Any]] = None,
        chunk_size: int = 1024,
        *,
        iterator: bool = False,
        **kwargs: Any,
    ) -> Optional[Union[bytes, Iterator[Any]]]:
        """Return the raw file contents for a blob.

        Args:
            sha: ID of the blob
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
            The blob content if streamed is False, None otherwise
        """
        path = f"/projects/{self.encoded_id}/repository/blobs/{sha}/raw"
        result = self.manager.gitlab.http_get(
            path, streamed=streamed, raw=True, **kwargs
        )
        if TYPE_CHECKING:
            assert isinstance(result, requests.Response)
        return utils.response_content(
            result, streamed, action, chunk_size, iterator=iterator
        )

    @cli.register_custom_action("Project", ("from_", "to"))
    @exc.on_http_error(exc.GitlabGetError)
    def repository_compare(
        self, from_: str, to: str, **kwargs: Any
    ) -> Union[Dict[str, Any], requests.Response]:
        """Return a diff between two branches/commits.

        Args:
            from_: Source branch/SHA
            to: Destination branch/SHA
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetError: If the server failed to perform the request

        Returns:
            The diff
        """
        path = f"/projects/{self.encoded_id}/repository/compare"
        query_data = {"from": from_, "to": to}
        return self.manager.gitlab.http_get(path, query_data=query_data, **kwargs)

    @cli.register_custom_action("Project")
    @exc.on_http_error(exc.GitlabGetError)
    def repository_contributors(
        self, **kwargs: Any
    ) -> Union[gitlab.client.GitlabList, List[Dict[str, Any]]]:
        """Return a list of contributors for the project.

        Args:
            all: If True, return all the items, without pagination
            per_page: Number of items to retrieve per request
            page: ID of the page to return (starts with page 1)
            iterator: If set to True and no pagination option is
                defined, return a generator instead of a list
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetError: If the server failed to perform the request

        Returns:
            The contributors
        """
        path = f"/projects/{self.encoded_id}/repository/contributors"
        return self.manager.gitlab.http_list(path, **kwargs)

    @cli.register_custom_action("Project", (), ("sha", "format"))
    @exc.on_http_error(exc.GitlabListError)
    def repository_archive(
        self,
        sha: Optional[str] = None,
        streamed: bool = False,
        action: Optional[Callable[..., Any]] = None,
        chunk_size: int = 1024,
        format: Optional[str] = None,
        path: Optional[str] = None,
        *,
        iterator: bool = False,
        **kwargs: Any,
    ) -> Optional[Union[bytes, Iterator[Any]]]:
        """Return an archive of the repository.

        Args:
            sha: ID of the commit (default branch by default)
            streamed: If True the data will be processed by chunks of
                `chunk_size` and each chunk is passed to `action` for
                treatment
            iterator: If True directly return the underlying response
                iterator
            action: Callable responsible of dealing with chunk of
                data
            chunk_size: Size of each chunk
            format: file format (tar.gz by default)
            path: The subpath of the repository to download (all files by default)
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabListError: If the server failed to perform the request

        Returns:
            The binary data of the archive
        """
        url_path = f"/projects/{self.encoded_id}/repository/archive"
        if format:
            url_path += "." + format
        query_data = {}
        if sha:
            query_data["sha"] = sha
        if path is not None:
            query_data["path"] = path
        result = self.manager.gitlab.http_get(
            url_path, query_data=query_data, raw=True, streamed=streamed, **kwargs
        )
        if TYPE_CHECKING:
            assert isinstance(result, requests.Response)
        return utils.response_content(
            result, streamed, action, chunk_size, iterator=iterator
        )

    @cli.register_custom_action("Project", ("refs",))
    @exc.on_http_error(exc.GitlabGetError)
    def repository_merge_base(
        self, refs: List[str], **kwargs: Any
    ) -> Union[Dict[str, Any], requests.Response]:
        """Return a diff between two branches/commits.

        Args:
            refs: The refs to find the common ancestor of. Multiple refs can be passed.
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetError: If the server failed to perform the request

        Returns:
            The common ancestor commit (*not* a RESTObject)
        """
        path = f"/projects/{self.encoded_id}/repository/merge_base"
        query_data, _ = utils._transform_types(
            data={"refs": refs},
            custom_types={"refs": types.ArrayAttribute},
            transform_data=True,
        )
        return self.manager.gitlab.http_get(path, query_data=query_data, **kwargs)

    @cli.register_custom_action("Project")
    @exc.on_http_error(exc.GitlabDeleteError)
    def delete_merged_branches(self, **kwargs: Any) -> None:
        """Delete merged branches.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabDeleteError: If the server failed to perform the request
        """
        path = f"/projects/{self.encoded_id}/repository/merged_branches"
        self.manager.gitlab.http_delete(path, **kwargs)
