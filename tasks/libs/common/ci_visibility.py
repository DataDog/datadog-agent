"""
CI Visibility helpers for creating custom spans in Datadog CI Visibility.

Uses `datadog-ci trace span` to create custom spans that appear in CI Visibility flamegraphs.
All helpers are no-ops outside of CI to avoid breaking local development.
"""

from __future__ import annotations

import json
import os
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


def create_spans_from_test_json(result_json_path: str, tags: dict[str, str] | None = None) -> int:
    """Parse a gotestsum JSON output file and create a CI Visibility span for each test.

    Each test gets a span from its "run" event to its terminal event (pass/fail/skip).
    Returns the number of spans created.
    """
    if not running_in_ci():
        return 0

    if not os.path.isfile(result_json_path):
        print(f"Warning: test result JSON not found at {result_json_path}, skipping span creation")
        return 0

    base_tags = dict(tags) if tags else {}
    base_tags.setdefault("agent-custom-span", "true")

    # Collect per-test events: (package, test) -> {start_time, end_time, action}
    # We track the "run" time as start and the terminal action time as end.
    test_runs: dict[tuple[str, str], dict] = {}

    with open(result_json_path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue

            test_name = event.get("Test")
            if not test_name:
                # Package-level event, skip
                continue

            package = event.get("Package", "")
            action = event.get("Action")
            event_time = event.get("Time")

            if not event_time or not action:
                continue

            key = (package, test_name)

            if action == "run":
                test_runs[key] = {"start_time": event_time, "package": package, "test": test_name}
            elif action in ("pass", "fail", "skip") and key in test_runs:
                test_runs[key]["end_time"] = event_time
                test_runs[key]["result"] = action
                # Capture elapsed if present (more precise than timestamp diff)
                if "Elapsed" in event:
                    test_runs[key]["elapsed"] = event["Elapsed"]

    count = 0
    for (package, test_name), data in test_runs.items():
        if "end_time" not in data:
            # Test started but never finished (e.g. panic/timeout), skip
            continue

        start_ms = _iso_to_ms(data["start_time"])
        end_ms = _iso_to_ms(data["end_time"])

        if start_ms is None or end_ms is None:
            continue

        # Use elapsed field for more precise end time if available
        if "elapsed" in data and start_ms is not None:
            end_ms = start_ms + int(data["elapsed"] * 1000)

        span_tags = dict(base_tags)
        span_tags["test.name"] = test_name
        span_tags["test.package"] = package
        span_tags["test.result"] = data.get("result", "unknown")

        # Use short package name for the span name
        short_pkg = package.rsplit("/", 1)[-1] if "/" in package else package
        span_name = f"{short_pkg}/{test_name}"

        CIVisibilitySection.create(name=span_name, start_time_ms=start_ms, end_time_ms=end_ms, tags=span_tags)
        count += 1

    return count


def _iso_to_ms(iso_time: str) -> int | None:
    """Convert an ISO 8601 / RFC 3339 timestamp to milliseconds since epoch."""
    from datetime import datetime, timezone

    try:
        dt = datetime.fromisoformat(iso_time)
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)
        return int(dt.timestamp() * 1000)
    except (ValueError, OSError):
        return None


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
