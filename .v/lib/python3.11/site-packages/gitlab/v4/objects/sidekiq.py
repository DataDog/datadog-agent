from typing import Any, Dict, Union

import requests

from gitlab import cli
from gitlab import exceptions as exc
from gitlab.base import RESTManager

__all__ = [
    "SidekiqManager",
]


class SidekiqManager(RESTManager):
    """Manager for the Sidekiq methods.

    This manager doesn't actually manage objects but provides helper function
    for the sidekiq metrics API.
    """

    @cli.register_custom_action("SidekiqManager")
    @exc.on_http_error(exc.GitlabGetError)
    def queue_metrics(self, **kwargs: Any) -> Union[Dict[str, Any], requests.Response]:
        """Return the registered queues information.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetError: If the information couldn't be retrieved

        Returns:
            Information about the Sidekiq queues
        """
        return self.gitlab.http_get("/sidekiq/queue_metrics", **kwargs)

    @cli.register_custom_action("SidekiqManager")
    @exc.on_http_error(exc.GitlabGetError)
    def process_metrics(
        self, **kwargs: Any
    ) -> Union[Dict[str, Any], requests.Response]:
        """Return the registered sidekiq workers.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetError: If the information couldn't be retrieved

        Returns:
            Information about the register Sidekiq worker
        """
        return self.gitlab.http_get("/sidekiq/process_metrics", **kwargs)

    @cli.register_custom_action("SidekiqManager")
    @exc.on_http_error(exc.GitlabGetError)
    def job_stats(self, **kwargs: Any) -> Union[Dict[str, Any], requests.Response]:
        """Return statistics about the jobs performed.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetError: If the information couldn't be retrieved

        Returns:
            Statistics about the Sidekiq jobs performed
        """
        return self.gitlab.http_get("/sidekiq/job_stats", **kwargs)

    @cli.register_custom_action("SidekiqManager")
    @exc.on_http_error(exc.GitlabGetError)
    def compound_metrics(
        self, **kwargs: Any
    ) -> Union[Dict[str, Any], requests.Response]:
        """Return all available metrics and statistics.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetError: If the information couldn't be retrieved

        Returns:
            All available Sidekiq metrics and statistics
        """
        return self.gitlab.http_get("/sidekiq/compound_metrics", **kwargs)
