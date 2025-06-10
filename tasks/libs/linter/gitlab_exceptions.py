"""Definitions for gitlabci linting-related exceptions."""

from abc import ABC, abstractmethod
from dataclasses import dataclass
from enum import Enum

from tasks.libs.common.color import Color, color_message


class FailureLevel(int, Enum):
    """Enum for different criticalities of gitlabci linting failures."""

    CRITICAL = 3  # Something went wrong while linting
    ERROR = 2  # The linter found something wrong with the file being linted
    WARNING = 1  # Same as error, but this failure is accepted and should not fail anything
    IGNORED = -1  # The linter found something wrong, but it is ignored by the config

    def pretty_print(self) -> str:
        """Outputs a nice string detailing the failure level, meant for CLI output."""
        return f'{color_message(self._name_, FAILURE_LEVEL_COLORS[self])}'


FAILURE_LEVEL_COLORS = {
    FailureLevel.CRITICAL: Color.RED,
    FailureLevel.ERROR: Color.RED,
    FailureLevel.WARNING: Color.ORANGE,
    FailureLevel.IGNORED: Color.GREY,
}


class GitlabLintFailure(ABC, Exception):
    """Base abstract class for representing gitlabci linting failures."""

    _level_override: FailureLevel | None = None

    @property
    @abstractmethod
    def details(self) -> str:
        """Details about the failure."""

    @property
    @abstractmethod
    def level(self) -> FailureLevel:
        """The level of the failure, WARNING, ERROR, or CRITICAL."""

    def override_level(self, level: FailureLevel) -> None:
        """Override the level of the failure."""
        self._level_override = level

    def ignore(self) -> None:
        """Mark the failure as ignored."""
        self.override_level(FailureLevel.IGNORED)

    @abstractmethod
    def pretty_print(self, min_level: FailureLevel = FailureLevel.IGNORED) -> str:
        """Outputs a nice string detailing the failure, meant for CLI output."""

    @abstractmethod
    def get_individual_failures(self) -> list["SingleGitlabLintFailure"]:
        """Returns a list of individual failures if this is a multi-failure exception."""

    @property
    def exit_code(self) -> int:
        """Returns the exit code for this failure based on the failure level."""
        return 0 if self.level in (FailureLevel.WARNING, FailureLevel.IGNORED) else 1


@dataclass
class SingleGitlabLintFailure(GitlabLintFailure):
    """Custom exception used to handle single gitlabci linting errors easily."""

    _details: str
    _level: FailureLevel

    # Can be None in case no specific job failed, for example if the linter applies to the whole config
    failing_job_name: str | None = None
    # The entry point of the config for this linting job, if applicable
    entry_point: str | None = None

    @property
    def details(self) -> str:
        """Details about the failure."""
        return self._details

    @property
    def level(self) -> FailureLevel:
        """Details about the failure."""
        return self._level_override or self._level

    def pretty_print(self, min_level: FailureLevel = FailureLevel.IGNORED) -> str:
        """Outputs a nice string detailing the failure, meant for CLI output."""
        if self.level < min_level:
            return ""
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

    def get_individual_failures(self) -> list["SingleGitlabLintFailure"]:
        """Returns a list of individual failures if this is a multi-failure exception."""
        return [self]


@dataclass
class MultiGitlabLintFailure(GitlabLintFailure):
    """Custom exception used to handle simultaneous gitlabci linting errors easily."""

    failures: list[SingleGitlabLintFailure]

    def pretty_print(self, min_level: FailureLevel = FailureLevel.IGNORED) -> str:
        """Outputs a nice string detailing the failure, meant for CLI output."""
        level_out = self.level.pretty_print()
        if len(self.entry_points) > 1:
            entry_point = ", ".join(self.entry_points)
        elif len(self.entry_points) == 1:
            entry_point = next(iter(self.entry_points))
        else:
            entry_point = "global"
        entry_point = color_message(entry_point, Color.BOLD)

        shown_failures = "\n".join(
            f"    - {failure.pretty_print()}" for failure in self.failures if failure.level >= min_level
        )
        return f'[{level_out}] {entry_point} - Multiple failures:\n{shown_failures}'

    @property
    def details(self):
        out = [f"    - {failure.pretty_print()}" for failure in self.failures]
        return "\n".join(out)

    @property
    def level(self) -> FailureLevel:
        """Returns the highest level of failure."""
        return self._level_override or max(failure.level for failure in self.failures)

    def get_individual_failures(self) -> list[SingleGitlabLintFailure]:
        """Returns a list of individual failures if this is a multi-failure exception."""
        return self.failures

    @property
    def entry_points(self) -> set[str]:
        return {failure.entry_point for failure in self.failures if failure.entry_point}
