"""Human-readable colored report formatter for UTOF documents."""

from __future__ import annotations

from collections.abc import Callable

from tasks.libs.common.color import color_message
from tasks.libs.testing.utof.models import UTOFDocument, UTOFFailure, UTOFTestResult


def _short_package(package: str) -> str:
    """Strip the common github.com/DataDog/datadog-agent/ prefix."""
    prefix = "github.com/DataDog/datadog-agent/"
    return package[len(prefix) :] if package.startswith(prefix) else package


def _format_duration(seconds: float) -> str:
    if seconds < 1:
        return f"{seconds * 1000:.0f}ms"
    if seconds < 60:
        return f"{seconds:.1f}s"
    minutes = int(seconds) // 60
    secs = seconds - minutes * 60
    return f"{minutes}m{secs:.0f}s"


def _indent(text: str, prefix: str = "    ") -> str:
    return "\n".join(f"{prefix}{line}" for line in text.splitlines())


def _status_badge(status: str) -> str:
    """Return a colored status badge for display."""
    badges = {
        "pass": color_message(" PASS ", "green"),
        "fail": color_message(" FAIL ", "red"),
        "skip": color_message(" SKIP ", "grey"),
        "flaky_pass": color_message(" FLAKY PASS ", "orange"),
        "flaky_fail": color_message(" FLAKY FAIL ", "magenta"),
    }
    return badges.get(status, status.upper())


def _get_test_failure(t: UTOFTestResult) -> UTOFFailure | None:
    """Resolve the most relevant failure for a test from its attempts list.

    For a final fail: the last failed attempt's failure (most recent signal).
    For a pass/flaky after retries: the first failed attempt's failure (initial cause).
    """
    if t.status == "fail":
        for attempt in reversed(t.attempts):
            if attempt.status == "fail" and attempt.failure:
                return attempt.failure
    else:
        for attempt in t.attempts:
            if attempt.status == "fail" and attempt.failure:
                return attempt.failure
    return None


def _render_failure(failure: UTOFFailure, prefix: str) -> list[str]:
    """Render a failure block as indented lines.

    For assertion failures the file locations are already embedded in the
    message, so we only show the stacktrace for panics where it contains
    a runtime crash trace.

    When a failure has no message and no type, the failure carries no
    useful diagnostic information — skip it entirely.
    """
    if not failure.message and not failure.type:
        return []
    out: list[str] = []
    if failure.type:
        out.append(color_message(f"{prefix}type: {failure.type}", "grey"))
    if failure.message:
        for msg_line in failure.message.splitlines():
            out.append(f"{prefix}{msg_line}")
    # Only show stacktrace for panics — assertion locations are in the message
    if failure.type == "panic" and failure.stacktrace:
        out.append(color_message(_indent(failure.stacktrace, prefix), "grey"))
    return out


def _has_matching_descendant(t: UTOFTestResult, predicate: Callable[[UTOFTestResult], bool]) -> bool:
    """Check if test or any of its subtests (recursively) match the predicate."""
    if predicate(t):
        return True
    if t.subtests:
        return any(_has_matching_descendant(sub, predicate) for sub in t.subtests)
    return False


def _render_subtests(
    subtests: list[UTOFTestResult],
    predicate: Callable[[UTOFTestResult], bool],
    depth: int,
    render_fn: Callable[[UTOFTestResult, int], list[str]],
) -> list[str]:
    """Render matching subtests recursively with tree indentation."""
    out: list[str] = []
    matching = [s for s in subtests if _has_matching_descendant(s, predicate)]
    for sub in matching:
        out.extend(render_fn(sub, depth))
    return out


def _tree_prefix(depth: int) -> str:
    return "    " * depth + "└─ " if depth > 0 else "  "


def _failure_prefix(depth: int) -> str:
    return "    " * depth + "         "


def _test_name(t: UTOFTestResult, depth: int) -> str:
    if depth == 0:
        return color_message(f"{_short_package(t.package)} :: {t.name}", "bold")
    return color_message(t.name, "bold")


def _fail_pred(t: UTOFTestResult) -> bool:
    return t.status == "fail"


def _retry_pred(t: UTOFTestResult) -> bool:
    return t.retry_count > 0


def _flaky_pred(t: UTOFTestResult) -> bool:
    return t.status in ("flaky_pass", "flaky_fail") and t.retry_count == 0


def _render_failed(t: UTOFTestResult, depth: int = 0) -> list[str]:
    out: list[str] = []
    prefix = _tree_prefix(depth)
    fp = _failure_prefix(depth)
    out.append(f"{prefix}{_status_badge(t.status)}  {_test_name(t, depth)}")
    # Suppress parent failure when subtests already explain the cause,
    # but only if the failure is inferred (not a direct assertion).
    # A direct assertion (testify blocks, panic, infrastructure) on the
    # parent must always be shown even when subtests also failed.
    has_failing_subtests = t.subtests and any(_has_matching_descendant(s, _fail_pred) for s in t.subtests)
    failure = _get_test_failure(t)
    if failure and (not has_failing_subtests or failure.direct):
        out.extend(_render_failure(failure, fp))
    if t.subtests:
        out.extend(_render_subtests(t.subtests, _fail_pred, depth + 1, _render_failed))
    return out


def _render_retried(t: UTOFTestResult, depth: int = 0) -> list[str]:
    out: list[str] = []
    prefix = _tree_prefix(depth)
    fp = _failure_prefix(depth)
    name = _test_name(t, depth)
    badge = _status_badge(t.status)
    if t.retry_count > 0:
        retry_info = color_message(f"({t.retry_count} retry, final: {t.status})", "grey")
        out.append(f"{prefix}{badge}  {name}  {retry_info}")
        if not t.subtests:
            for a in t.attempts:
                if a.status == "fail":
                    marker = color_message("[x]", "red")
                    attempt_status = color_message(f"attempt {a.attempt}: fail", "red")
                else:
                    marker = color_message("[v]", "green")
                    attempt_status = color_message(f"attempt {a.attempt}: pass", "green")
                dur = color_message(f"({_format_duration(a.duration_seconds)})", "grey")
                out.append(f"{fp}{marker} {attempt_status} {dur}")
                if a.failure:
                    out.extend(_render_failure(a.failure, fp + "     "))
    else:
        out.append(f"{prefix}{badge}  {name}")
    if t.subtests:
        out.extend(_render_subtests(t.subtests, _retry_pred, depth + 1, _render_retried))
    return out


def _render_flaky(t: UTOFTestResult, depth: int = 0) -> list[str]:
    out: list[str] = []
    prefix = _tree_prefix(depth)
    fp = _failure_prefix(depth)
    name = _test_name(t, depth)
    source = color_message(f"(source: {t.flaky.source})", "grey") if t.flaky else ""
    out.append(f"{prefix}{_status_badge(t.status)}  {name}  {source}")
    has_flaky_subtests = t.subtests and any(_has_matching_descendant(s, _flaky_pred) for s in t.subtests)
    failure = _get_test_failure(t)
    if failure and (not has_flaky_subtests or failure.direct):
        out.extend(_render_failure(failure, fp))
    if t.subtests:
        out.extend(_render_subtests(t.subtests, _flaky_pred, depth + 1, _render_flaky))
    return out


def format_report(doc: UTOFDocument) -> str:
    """Produce a human-friendly, color-coded text report from a UTOF document.

    Designed to be printed in CI logs. Sections are ordered by
    severity: failures first, then retried tests, then flaky, then
    a compact summary. Uses ANSI colors to make failures immediately
    visible in terminal / CI output.
    """
    lines: list[str] = []
    s = doc.summary

    # -- Header --
    separator = "=" * 60
    if s.status == "pass":
        title = f"  Test Report ({doc.metadata.test_system}) — PASSED  "
        lines.append(color_message(separator, "green"))
        lines.append(color_message(title, "green"))
        lines.append(color_message(separator, "green"))
    else:
        title = f"  Test Report ({doc.metadata.test_system}) — FAILED  "
        lines.append(color_message(separator, "red"))
        lines.append(color_message(title, "red"))
        lines.append(color_message(separator, "red"))
    lines.append("")

    # -- Summary bar --
    parts = [color_message(f"{s.total} total", "bold")]
    if s.passed:
        parts.append(color_message(f"{s.passed} passed", "green"))
    if s.failed:
        parts.append(color_message(f"{s.failed} failed", "red"))
    if s.skipped:
        parts.append(color_message(f"{s.skipped} skipped", "grey"))
    if s.flaky:
        parts.append(color_message(f"{s.flaky} flaky", "orange"))
    if s.retried:
        parts.append(color_message(f"{s.retried} retried", "orange"))
    if doc.metadata.duration_seconds:
        parts.append(f"in {_format_duration(doc.metadata.duration_seconds)}")
    lines.append("  ".join(parts))
    lines.append("")

    # -- Failed tests --
    failed_roots = [t for t in doc.tests if _has_matching_descendant(t, _fail_pred)]
    if failed_roots:
        lines.append(color_message(f"--- Failures ({s.failed}) ---", "red"))
        lines.append("")
        for t in failed_roots:
            lines.extend(_render_failed(t))
        lines.append("")

    # -- Retried tests --
    retried_roots = [t for t in doc.tests if _has_matching_descendant(t, _retry_pred)]
    if retried_roots:
        lines.append(color_message(f"--- Retried ({s.retried}) ---", "orange"))
        lines.append("")
        for t in retried_roots:
            lines.extend(_render_retried(t))
        lines.append("")

    # -- Flaky tests (not already shown above) --
    flaky_roots = [t for t in doc.tests if _has_matching_descendant(t, _flaky_pred)]
    if flaky_roots:
        lines.append(color_message(f"--- Flaky ({s.flaky}) ---", "orange"))
        lines.append("")
        for t in flaky_roots:
            lines.extend(_render_flaky(t))
        lines.append("")

    return "\n".join(lines)
