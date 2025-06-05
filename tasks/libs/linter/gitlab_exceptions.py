"""Definitions for gitlabci linting-related exceptions"""

from dataclasses import dataclass
from enum import Enum

from tasks.libs.common.color import Color, color_message


class FailureLevel(int, Enum):
    """Enum for different criticalities of gitlabci linting failures"""

    CRITICAL = 3  # Something went wrong while linting
    ERROR = 2  # The linter found something wrong with the file being linted
    WARNING = 1  # Same as error, but this failure is accepted and should not fail anything

    def pretty_print(self) -> str:
        """Outputs a nice string detailing the failure level, meant for CLI output."""
        return f'{color_message(self._name_, FAILURE_LEVEL_COLORS[self])}'


FAILURE_LEVEL_COLORS = {
    FailureLevel.CRITICAL: Color.RED,
    FailureLevel.ERROR: Color.RED,
    FailureLevel.WARNING: Color.ORANGE,
}


@dataclass
class GitlabLintFailure(Exception):
    """Custom exception used to handle gitlabci linting errors easily"""

    details: str
    level: FailureLevel

    # Can be None in case no specific job failed, for example if the linter applies to the whole config
    failing_job_name: str | None = None
    # The entry point of the config for this linting job, if applicable
    entry_point: str | None = None

    def pretty_print(self) -> str:
        """Outputs a nice string detailing the failure, meant for CLI output."""
        level_out = self.level.pretty_print()

        # Build the entrypoint/job name string
        entrypoint_job = ""
        if self.entry_point:
            entrypoint_job = color_message(self.entry_point, Color.BOLD)
            if self.failing_job_name:
                entrypoint_job = f"{entrypoint_job}/"

        if self.failing_job_name:
            entrypoint_job = f"{entrypoint_job}{self.failing_job_name}"

        return f'[{level_out}] {entrypoint_job} : {self.details}'

    @property
    def exit_code(self) -> int:
        return 0 if self.level == FailureLevel.WARNING else 1


@dataclass
class MultiGitlabLintFailure(Exception):
    failures: list[GitlabLintFailure]
    """Custom exception used to handle simultaneous gitlabci linting errors easily"""

    def pretty_print(self) -> str:
        """Outputs a nice string detailing the failure, meant for CLI output."""
        level_out = self.level.pretty_print()
        if len(self.entry_points) > 1:
            entry_point = ", ".join(self.entry_points)
        elif len(self.entry_points) == 1:
            entry_point = next(iter(self.entry_points))
        else:
            entry_point = "global"
        entry_point = color_message(entry_point, Color.BOLD)
        return f'[{level_out}] {entry_point} - Multiple failures:\n{self.details})'

    @property
    def details(self):
        out = [f"    - {failure.pretty_print()}" for failure in self.failures]
        return "\n".join(out)

    @property
    def level(self) -> FailureLevel:
        """Returns the highest level of failure"""
        return max(failure.level for failure in self.failures)

    @property
    def entry_points(self) -> set[str]:
        return {failure.entry_point for failure in self.failures if failure.entry_point}

    @property
    def exit_code(self) -> int:
        return 0 if self.level == FailureLevel.WARNING else 1
