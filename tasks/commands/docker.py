from __future__ import annotations

from typing import TYPE_CHECKING

from tasks.commands.interface import CLI

AGENT_REPOSITORY_PATH = '/workspaces/datadog-agent'

if TYPE_CHECKING:
    from typing import Iterable


class DockerCLI(CLI):
    """
    CLI interface to run command lines directly in a docker container.
    """
    def __init__(self, container_name: str):
        self._container_name = container_name

    def _format_command(self, command: Iterable[str]) -> str:
        docker_command = ['docker', 'exec']

        if self._isatty():
            docker_command += ['-it']

        docker_command += ['-w', AGENT_REPOSITORY_PATH, self._container_name, *command]

        return super()._format_command(docker_command)
