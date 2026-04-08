"""
CI Visibility helpers for creating custom spans in Datadog CI Visibility.

Uses `datadog-ci trace span` to create custom spans that appear in CI Visibility flamegraphs.
All helpers are no-ops outside of CI to avoid breaking local development.
"""

from __future__ import annotations

import subprocess
import time
from contextlib import contextmanager
from dataclasses import dataclass, field

from tasks.libs.common.utils import running_in_ci


def _shell_quote(s: str) -> str:
    """Quote a string for safe shell usage."""
    return "'" + s.replace("'", "'\\''") + "'"


def _current_time_ms() -> int:
    """Return current time in milliseconds since epoch."""
    return int(time.time() * 1000)


@dataclass
class CIVisibilitySection:
    """Represents a custom CI Visibility span."""

    # Module-level accumulator for batch sending
    _accumulated: list[CIVisibilitySection] = field(default_factory=list, init=False, repr=False)

    name: str
    start_time_ms: int
    end_time_ms: int
    tags: dict[str, str] = field(default_factory=dict)

    def __post_init__(self):
        # Ensure tags always include the custom span marker
        self.tags.setdefault("agent-custom-span", "true")

    @classmethod
    def create(cls, name: str, start_time_ms: int, end_time_ms: int, tags: dict[str, str] | None = None) -> None:
        """Register a section for batch sending later."""
        section = cls(name=name, start_time_ms=start_time_ms, end_time_ms=end_time_ms, tags=tags or {})
        _SECTIONS.append(section)

    def send(self) -> bool:
        """Send a single span via datadog-ci trace span."""
        tag_args = " ".join(f"--tags {k}:{v}" for k, v in self.tags.items())
        cmd = f"datadog-ci trace span --name {_shell_quote(self.name)} --start-time {self.start_time_ms} --end-time {self.end_time_ms} {tag_args}"

        try:
            subprocess.run(cmd, shell=True, check=True, capture_output=True, timeout=30)
            return True
        except (subprocess.CalledProcessError, subprocess.TimeoutExpired, FileNotFoundError) as e:
            print(f"Warning: failed to send CI visibility span '{self.name}': {e}")
            return False

    @classmethod
    def send_all(cls) -> None:
        """Send all accumulated sections individually via datadog-ci trace span."""
        if not _SECTIONS or not running_in_ci():
            _SECTIONS.clear()
            return

        for section in _SECTIONS:
            section.send()

        _SECTIONS.clear()


# Module-level list, declared after class to avoid forward reference issues
_SECTIONS: list[CIVisibilitySection] = []


@contextmanager
def ci_visibility_section(name: str, tags: dict[str, str] | None = None):
    """Context manager that records timing and creates a CI Visibility section.

    No-op outside of CI environments.

    Usage:
        with ci_visibility_section("my-operation", tags={"agent-category": "e2e"}):
            do_something()
    """
    if not running_in_ci():
        yield
        return

    all_tags = dict(tags) if tags else {}
    all_tags.setdefault("agent-custom-span", "true")

    start = _current_time_ms()
    try:
        yield
    finally:
        end = _current_time_ms()
        CIVisibilitySection.create(name=name, start_time_ms=start, end_time_ms=end, tags=all_tags)
