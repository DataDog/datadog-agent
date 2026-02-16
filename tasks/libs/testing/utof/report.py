"""Human-readable colored report formatter for UTOF documents."""

from __future__ import annotations

from tasks.libs.common.color import color_message
from tasks.libs.testing.utof.models import UTOFDocument, UTOFFailure


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
    """
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
    failed = [t for t in doc.tests if t.status == "fail"]
    if failed:
        lines.append(color_message(f"--- Failures ({len(failed)}) ---", "red"))
        lines.append("")
        for t in failed:
            badge = _status_badge(t.status)
            name = color_message(f"{_short_package(t.package)} :: {t.name}", "bold")
            lines.append(f"  {badge}  {name}")
            if t.failure:
                lines.extend(_render_failure(t.failure, "         "))
        lines.append("")

    # -- Retried tests --
    retried = [t for t in doc.tests if t.retry_count > 0]
    if retried:
        lines.append(color_message(f"--- Retried ({len(retried)}) ---", "orange"))
        lines.append("")
        for t in retried:
            badge = _status_badge(t.status)
            name = color_message(f"{_short_package(t.package)} :: {t.name}", "bold")
            retry_info = color_message(f"({t.retry_count} retry, final: {t.status})", "grey")
            lines.append(f"  {badge}  {name}  {retry_info}")
            if t.attempts:
                for a in t.attempts:
                    if a.status == "fail":
                        marker = color_message("[x]", "red")
                        attempt_status = color_message(f"attempt {a.attempt}: fail", "red")
                    else:
                        marker = color_message("[v]", "green")
                        attempt_status = color_message(f"attempt {a.attempt}: pass", "green")
                    dur = color_message(f"({_format_duration(a.duration_seconds)})", "grey")
                    lines.append(f"         {marker} {attempt_status} {dur}")
                    if a.failure:
                        lines.extend(_render_failure(a.failure, "              "))
            elif t.failure and t.failure.message:
                lines.append(f"         initial failure: {t.failure.message}")
        lines.append("")

    # -- Flaky tests (not already shown above) --
    flaky = [t for t in doc.tests if t.status in ("flaky_pass", "flaky_fail") and t.retry_count == 0]
    if flaky:
        lines.append(color_message(f"--- Flaky ({len(flaky)}) ---", "orange"))
        lines.append("")
        for t in flaky:
            badge = _status_badge(t.status)
            name = color_message(f"{_short_package(t.package)} :: {t.name}", "bold")
            source = color_message(f"(source: {t.flaky.source})", "grey") if t.flaky else ""
            lines.append(f"  {badge}  {name}  {source}")
            if t.failure:
                lines.extend(_render_failure(t.failure, "         "))
        lines.append("")

    return "\n".join(lines)
