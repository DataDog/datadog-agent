from typing import Any, Callable, cast, Dict, Iterator, Optional, TYPE_CHECKING, Union

import requests

from gitlab import cli
from gitlab import exceptions as exc
from gitlab import utils
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import RefreshMixin, RetrieveMixin
from gitlab.types import ArrayAttribute

__all__ = [
    "ProjectJob",
    "ProjectJobManager",
]


class ProjectJob(RefreshMixin, RESTObject):
    @cli.register_custom_action("ProjectJob")
    @exc.on_http_error(exc.GitlabJobCancelError)
    def cancel(self, **kwargs: Any) -> Dict[str, Any]:
        """Cancel the job.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabJobCancelError: If the job could not be canceled
        """
        path = f"{self.manager.path}/{self.encoded_id}/cancel"
        result = self.manager.gitlab.http_post(path, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(result, dict)
        return result

    @cli.register_custom_action("ProjectJob")
    @exc.on_http_error(exc.GitlabJobRetryError)
    def retry(self, **kwargs: Any) -> Dict[str, Any]:
        """Retry the job.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabJobRetryError: If the job could not be retried
        """
        path = f"{self.manager.path}/{self.encoded_id}/retry"
        result = self.manager.gitlab.http_post(path, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(result, dict)
        return result

    @cli.register_custom_action("ProjectJob")
    @exc.on_http_error(exc.GitlabJobPlayError)
    def play(self, **kwargs: Any) -> None:
        """Trigger a job explicitly.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabJobPlayError: If the job could not be triggered
        """
        path = f"{self.manager.path}/{self.encoded_id}/play"
        self.manager.gitlab.http_post(path, **kwargs)

    @cli.register_custom_action("ProjectJob")
    @exc.on_http_error(exc.GitlabJobEraseError)
    def erase(self, **kwargs: Any) -> None:
        """Erase the job (remove job artifacts and trace).

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabJobEraseError: If the job could not be erased
        """
        path = f"{self.manager.path}/{self.encoded_id}/erase"
        self.manager.gitlab.http_post(path, **kwargs)

    @cli.register_custom_action("ProjectJob")
    @exc.on_http_error(exc.GitlabCreateError)
    def keep_artifacts(self, **kwargs: Any) -> None:
        """Prevent artifacts from being deleted when expiration is set.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabCreateError: If the request could not be performed
        """
        path = f"{self.manager.path}/{self.encoded_id}/artifacts/keep"
        self.manager.gitlab.http_post(path, **kwargs)

    @cli.register_custom_action("ProjectJob")
    @exc.on_http_error(exc.GitlabCreateError)
    def delete_artifacts(self, **kwargs: Any) -> None:
        """Delete artifacts of a job.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabDeleteError: If the request could not be performed
        """
        path = f"{self.manager.path}/{self.encoded_id}/artifacts"
        self.manager.gitlab.http_delete(path, **kwargs)

    @cli.register_custom_action("ProjectJob")
    @exc.on_http_error(exc.GitlabGetError)
    def artifacts(
        self,
        streamed: bool = False,
        action: Optional[Callable[..., Any]] = None,
        chunk_size: int = 1024,
        *,
        iterator: bool = False,
        **kwargs: Any,
    ) -> Optional[Union[bytes, Iterator[Any]]]:
        """Get the job artifacts.

        Args:
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
            GitlabGetError: If the artifacts could not be retrieved

        Returns:
            The artifacts if `streamed` is False, None otherwise.
        """
        path = f"{self.manager.path}/{self.encoded_id}/artifacts"
        result = self.manager.gitlab.http_get(
            path, streamed=streamed, raw=True, **kwargs
        )
        if TYPE_CHECKING:
            assert isinstance(result, requests.Response)
        return utils.response_content(
            result, streamed, action, chunk_size, iterator=iterator
        )

    @cli.register_custom_action("ProjectJob")
    @exc.on_http_error(exc.GitlabGetError)
    def artifact(
        self,
        path: str,
        streamed: bool = False,
        action: Optional[Callable[..., Any]] = None,
        chunk_size: int = 1024,
        *,
        iterator: bool = False,
        **kwargs: Any,
    ) -> Optional[Union[bytes, Iterator[Any]]]:
        """Get a single artifact file from within the job's artifacts archive.

        Args:
            path: Path of the artifact
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
            GitlabGetError: If the artifacts could not be retrieved

        Returns:
            The artifacts if `streamed` is False, None otherwise.
        """
        path = f"{self.manager.path}/{self.encoded_id}/artifacts/{path}"
        result = self.manager.gitlab.http_get(
            path, streamed=streamed, raw=True, **kwargs
        )
        if TYPE_CHECKING:
            assert isinstance(result, requests.Response)
        return utils.response_content(
            result, streamed, action, chunk_size, iterator=iterator
        )

    @cli.register_custom_action("ProjectJob")
    @exc.on_http_error(exc.GitlabGetError)
    def trace(
        self,
        streamed: bool = False,
        action: Optional[Callable[..., Any]] = None,
        chunk_size: int = 1024,
        *,
        iterator: bool = False,
        **kwargs: Any,
    ) -> Dict[str, Any]:
        """Get the job trace.

        Args:
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
            GitlabGetError: If the artifacts could not be retrieved

        Returns:
            The trace
        """
        path = f"{self.manager.path}/{self.encoded_id}/trace"
        result = self.manager.gitlab.http_get(
            path, streamed=streamed, raw=True, **kwargs
        )
        if TYPE_CHECKING:
            assert isinstance(result, requests.Response)
        return_value = utils.response_content(
            result, streamed, action, chunk_size, iterator=iterator
        )
        if TYPE_CHECKING:
            assert isinstance(return_value, dict)
        return return_value


class ProjectJobManager(RetrieveMixin, RESTManager):
    _path = "/projects/{project_id}/jobs"
    _obj_cls = ProjectJob
    _from_parent_attrs = {"project_id": "id"}
    _list_filters = ("scope",)
    _types = {"scope": ArrayAttribute}

    def get(self, id: Union[str, int], lazy: bool = False, **kwargs: Any) -> ProjectJob:
        return cast(ProjectJob, super().get(id=id, lazy=lazy, **kwargs))
