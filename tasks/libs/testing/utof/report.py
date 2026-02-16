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
        "flaky_fail": color_message(" FLAKY FAIL ", "orange"),
    }
    return badges.get(status, status.upper())


def _render_failure(failure: UTOFFailure, prefix: str) -> list[str]:
    """Render a failure block as indented lines.

    For assertion failures the file locations are already embedded in the
    message (e.g. "[1] value_test.go:62: ..."), so we only show the
    stacktrace for panics where it contains a goroutine trace.

    When a failure has no message and isn't a panic, it's just Go
    propagating a subtest failure to the parent — skip it entirely.
    """
    if not failure.message and failure.type != "panic":
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


def _collect_all_tests(tests: list[UTOFTestResult]) -> list[UTOFTestResult]:
    """Flatten a tree of tests into a single list (for backward-compat counting)."""
    result: list[UTOFTestResult] = []

    def _walk(t: UTOFTestResult):
        result.append(t)
        if t.subtests:
            for sub in t.subtests:
                _walk(sub)

    for t in tests:
        _walk(t)
    return result


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

    # -- Helper to render a test entry with subtests --
    def _tree_prefix(depth: int) -> str:
        return "    " * depth + "└─ " if depth > 0 else "  "

    def _failure_prefix(depth: int) -> str:
        return "    " * depth + "         "

    # -- Failed tests --
    def fail_pred(t: UTOFTestResult) -> bool:
        return t.status == "fail"

    all_tests = _collect_all_tests(doc.tests)
    fail_count = sum(1 for t in all_tests if fail_pred(t) and not t.subtests)
    failed_roots = [t for t in doc.tests if _has_matching_descendant(t, fail_pred)]
    if failed_roots:

        def _render_failed(t: UTOFTestResult, depth: int = 0) -> list[str]:
            out: list[str] = []
            prefix = _tree_prefix(depth)
            fp = _failure_prefix(depth)
            badge = _status_badge(t.status)
            if depth == 0:
                name = color_message(f"{_short_package(t.package)} :: {t.name}", "bold")
            else:
                name = color_message(t.name, "bold")
            out.append(f"{prefix}{badge}  {name}")
            if t.failure:
                out.extend(_render_failure(t.failure, fp))
            if t.subtests:
                out.extend(_render_subtests(t.subtests, fail_pred, depth + 1, _render_failed))
            return out

        lines.append(color_message(f"--- Failures ({fail_count}) ---", "red"))
        lines.append("")
        for t in failed_roots:
            lines.extend(_render_failed(t))
        lines.append("")

    # -- Retried tests --
    def retry_pred(t: UTOFTestResult) -> bool:
        return t.retry_count > 0

    retry_count = sum(1 for t in all_tests if retry_pred(t) and not t.subtests)
    retried_roots = [t for t in doc.tests if _has_matching_descendant(t, retry_pred)]
    if retried_roots:

        def _render_retried(t: UTOFTestResult, depth: int = 0) -> list[str]:
            out: list[str] = []
            prefix = _tree_prefix(depth)
            fp = _failure_prefix(depth)
            badge = _status_badge(t.status)
            if depth == 0:
                name = color_message(f"{_short_package(t.package)} :: {t.name}", "bold")
            else:
                name = color_message(t.name, "bold")
            if t.retry_count > 0:
                retry_info = color_message(f"({t.retry_count} retry, final: {t.status})", "grey")
                out.append(f"{prefix}{badge}  {name}  {retry_info}")
                # Skip per-attempt detail on parent tests — subtests show their own
                if not t.subtests:
                    if t.attempts:
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
                    elif t.failure and t.failure.message:
                        out.append(f"{fp}initial failure: {t.failure.message}")
            else:
                out.append(f"{prefix}{badge}  {name}")
            if t.subtests:
                out.extend(_render_subtests(t.subtests, retry_pred, depth + 1, _render_retried))
            return out

        lines.append(color_message(f"--- Retried ({retry_count}) ---", "orange"))
        lines.append("")
        for t in retried_roots:
            lines.extend(_render_retried(t))
        lines.append("")

    # -- Flaky tests (not already shown above) --
    def flaky_pred(t: UTOFTestResult) -> bool:
        return t.status in ("flaky_pass", "flaky_fail") and t.retry_count == 0

    flaky_count = sum(1 for t in all_tests if flaky_pred(t) and not t.subtests)
    flaky_roots = [t for t in doc.tests if _has_matching_descendant(t, flaky_pred)]
    if flaky_roots:

        def _render_flaky(t: UTOFTestResult, depth: int = 0) -> list[str]:
            out: list[str] = []
            prefix = _tree_prefix(depth)
            fp = _failure_prefix(depth)
            badge = _status_badge(t.status)
            if depth == 0:
                name = color_message(f"{_short_package(t.package)} :: {t.name}", "bold")
            else:
                name = color_message(t.name, "bold")
            source = color_message(f"(source: {t.flaky.source})", "grey") if t.flaky else ""
            out.append(f"{prefix}{badge}  {name}  {source}")
            if t.failure:
                out.extend(_render_failure(t.failure, fp))
            if t.subtests:
                out.extend(_render_subtests(t.subtests, flaky_pred, depth + 1, _render_flaky))
            return out

        lines.append(color_message(f"--- Flaky ({flaky_count}) ---", "orange"))
        lines.append("")
        for t in flaky_roots:
            lines.extend(_render_flaky(t))
        lines.append("")

    return "\n".join(lines)
