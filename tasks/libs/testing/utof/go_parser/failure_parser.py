"""Failure extraction from raw Go test output lines."""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import TYPE_CHECKING

from tasks.libs.testing.result_json import ActionType
from tasks.libs.testing.utof.models import UTOFFailure

if TYPE_CHECKING:
    from tasks.libs.testing.result_json import ResultJsonLine

# Regex for testify "Error:" lines, e.g.:
#   \tError:      \tExpected nil, but got: ...
_RE_TESTIFY_ERROR = re.compile(r'^\s+Error:\s+(.+)')

# Regex for testify multi-line values that continue an Error: block, e.g.:
#   \t            \texpected: 5
#   \t            \tactual  : 3
_RE_TESTIFY_CONTINUATION = re.compile(r'^\s+\t\s{12,}\t(.+)')

# Regex for standard Go test t.Error / t.Fatal output, e.g.:
#   some_test.go:42: expected X, got Y
_RE_GO_TEST_ERROR = re.compile(r'^\s+\w[\w./]*\.go:\d+:\s*(.+)')

# Regex for testify "Error Trace:" lines to extract file:line for stacktrace
_RE_TESTIFY_TRACE = re.compile(r'^\s+Error Trace:\s+(.+)')

# Regex for testify "Messages:" lines — extra context attached to an assertion, e.g.:
#   \tMessages:   \t'diskspd.exe' process not found in payloads
_RE_TESTIFY_MESSAGES = re.compile(r'^\s+Messages:\s+(.+)')

# Error texts emitted by testify wrapper constructs (e.g. EventuallyWithT).
# These blocks don't add information when an inner assertion already explains the failure.
_EVENTUALLY_WRAPPER_ERRORS = {"Condition never satisfied"}

# Pulumi resource section header: "  kubernetes:helm.sh/v3:Release (dda-linux):"
_RE_PULUMI_RESOURCE_SECTION = re.compile(r'^\s+(\w[\w./-]*:\w[\w./-]*:\w[\w./ -]*)\s+\(([^)]+)\):\s*$')

# Pulumi error bullet: "    \t* Helm release ..."
_RE_PULUMI_ERROR_BULLET = re.compile(r'^\s+\t\*\s+(.+)')


@dataclass
class _AssertionBlock:
    """One assertion failure parsed from raw Go test output."""

    trace: str  # file:line location (may be full path)
    error_lines: list[str]  # error message parts


def _parse_assertion_blocks(raw_output_lines: list[str]) -> list[_AssertionBlock]:
    """Parse raw output into individual assertion blocks.

    Each block starts at an "Error Trace:" line and contains the
    corresponding "Error:" content. Returns an empty list when no
    testify-style assertion blocks are found.
    """
    blocks: list[_AssertionBlock] = []
    current_trace = ""
    current_error: list[str] = []
    in_error = False

    for raw_line in raw_output_lines:
        # "Error Trace:" starts a new assertion block
        m_trace = _RE_TESTIFY_TRACE.match(raw_line)
        if m_trace:
            # Save previous block if any
            if current_error:
                blocks.append(_AssertionBlock(trace=current_trace, error_lines=current_error))
                current_error = []
            current_trace = m_trace.group(1).strip()
            in_error = False
            continue

        # "Error:" starts the error content within the current block
        m_err = _RE_TESTIFY_ERROR.match(raw_line)
        if m_err:
            current_error.append(m_err.group(1).strip())
            in_error = True
            continue

        # "Messages:" provides extra context attached to the current assertion, e.g.:
        #   'diskspd.exe' process not found in payloads
        # Truncate to avoid pulling in huge data dumps (e.g. full process payloads).
        m_msg = _RE_TESTIFY_MESSAGES.match(raw_line)
        if m_msg and current_trace:
            content = m_msg.group(1).strip()
            if len(content) > 120:
                content = content[:120] + "…"
            current_error.append(content)
            # Stop continuation capture — Messages: values can be multi-line
            # data dumps (e.g. full process payload JSON) that we don't want.
            in_error = False
            continue

        # Continuation lines (indented values like "expected: 5")
        if in_error:
            m_cont = _RE_TESTIFY_CONTINUATION.match(raw_line)
            if m_cont:
                current_error.append(m_cont.group(1).strip())
                continue
            in_error = False

    # Don't forget the last block
    if current_error:
        blocks.append(_AssertionBlock(trace=current_trace, error_lines=current_error))

    return blocks


def _short_location(trace: str) -> str:
    """Shorten an absolute path to just filename:line."""
    return re.sub(r'^.*/', '', trace) if trace else ""


def _drop_eventually_wrappers(blocks: list[_AssertionBlock]) -> list[_AssertionBlock]:
    """Remove EventuallyWithT wrapper blocks when real failures exist alongside them.

    testify's EventuallyWithT emits a "Condition never satisfied" block in
    addition to the real assertion failure(s). That wrapper adds no information
    — the other blocks already explain what went wrong — so we strip it
    wherever it appears, as long as at least one non-wrapper block remains.
    """
    real = [b for b in blocks if not (b.error_lines and b.error_lines[0] in _EVENTUALLY_WRAPPER_ERRORS)]
    return real if real else blocks


def _extract_pulumi_errors(raw_output_lines: list[str]) -> list[str]:
    """Extract Pulumi resource error messages from Go test output.

    Pulumi formats infrastructure failures as:
        resource:type:Name (logical-name):
            error: N error(s) occurred:
            \\t* <actual error message>

    Returns a deduplicated list of "resource (name): error" strings,
    or an empty list if no Pulumi error bullets are found.
    """
    # resource -> last error message seen (later retries overwrite earlier ones)
    by_resource: dict[str, str] = {}
    order: list[str] = []
    current_resource = ""

    for line in raw_output_lines:
        m_res = _RE_PULUMI_RESOURCE_SECTION.match(line)
        if m_res:
            current_resource = f"{m_res.group(1)} ({m_res.group(2)})"
            continue

        m_bullet = _RE_PULUMI_ERROR_BULLET.match(line)
        if m_bullet:
            msg = m_bullet.group(1).strip()
            key = current_resource or ""
            if key not in by_resource:
                order.append(key)
            by_resource[key] = msg

    return [f"{key}: {by_resource[key]}" if key else by_resource[key] for key in order]


def _extract_message_from_raw_output(raw_output_lines: list[str]) -> str:
    """Parse raw Go test output lines to extract a meaningful error message.

    Handles:
    1. Multiple testify assertions — each formatted with its own file:line
    2. Single testify assertion — file:line: error message
    3. Standard t.Error/t.Fatal — file.go:42: message
    4. Falls back to empty string
    """
    # Try testify assertion blocks first
    blocks = _drop_eventually_wrappers(_parse_assertion_blocks(raw_output_lines))
    if blocks:
        if len(blocks) == 1:
            b = blocks[0]
            loc = _short_location(b.trace)
            error_text = ", ".join(b.error_lines)
            return f"{loc}: {error_text}" if loc else error_text

        # Multiple assertions — number each one
        parts = []
        for i, b in enumerate(blocks, 1):
            loc = _short_location(b.trace)
            error_text = ", ".join(b.error_lines)
            entry = f"{loc}: {error_text}" if loc else error_text
            parts.append(f"  [{i}] {entry}")
        return f"{len(blocks)} assertions failed\n" + "\n".join(parts)

    # Fall back to standard Go test error lines (file.go:N: message)
    for raw_line in raw_output_lines:
        m = _RE_GO_TEST_ERROR.match(raw_line)
        if m:
            msg = m.group(1).strip()
            if msg:
                # Include the file:line location
                loc_match = re.match(r'^\s+(\w[\w./]*\.go:\d+):', raw_line)
                loc = loc_match.group(1) if loc_match else ""
                return f"{loc}: {msg}" if loc else msg

    return ""


def _extract_stacktrace_from_raw_output(raw_output_lines: list[str]) -> str:
    """Extract all failure locations from raw output for assertion failures."""
    # Collect all testify Error Trace: locations
    traces = []
    for raw_line in raw_output_lines:
        m = _RE_TESTIFY_TRACE.match(raw_line)
        if m:
            traces.append(m.group(1).strip())
    if traces:
        return "\n".join(traces)

    # Standard Go test file:line reference
    for raw_line in raw_output_lines:
        m = _RE_GO_TEST_ERROR.match(raw_line)
        if m:
            loc_match = re.match(r'^\s+(\w[\w./]*\.go:\d+):', raw_line)
            if loc_match:
                return loc_match.group(1)

    return ""


def _extract_failure_info(lines: list[ResultJsonLine]) -> UTOFFailure | None:
    """Extract structured failure information from test output lines."""
    has_failure = False
    failure_type = ""
    message = ""
    stacktrace_lines: list[str] = []
    raw_output_lines: list[str] = []
    in_stacktrace = False

    for line in lines:
        if line.action == ActionType.FAIL:
            has_failure = True
        if line.action == ActionType.BUILD_FAIL:
            has_failure = True
            failure_type = "build"

        if not line.output:
            continue

        output = line.output

        # Detect panic
        if "panic:" in output:
            has_failure = True
            failure_type = "panic"
            message = output.strip()
            in_stacktrace = True
            raw_output_lines.append(output.rstrip("\n"))
            continue

        # Collect stacktrace lines (goroutine headers, file:line patterns)
        if in_stacktrace:
            stripped = output.strip()
            if stripped and (
                stripped.startswith("goroutine ")
                or re.match(r'\t.*:\d+', stripped)
                or stripped.startswith("created by ")
                or re.match(r'.*\.\w+\(', stripped)
            ):
                stacktrace_lines.append(output.rstrip("\n"))
                raw_output_lines.append(output.rstrip("\n"))
                continue
            elif stripped == "":
                # Empty line can be part of the stacktrace
                stacktrace_lines.append("")
                raw_output_lines.append("")
                continue
            else:
                in_stacktrace = False

        # Detect assertion failure from --- FAIL: lines
        if "--- FAIL:" in output and not failure_type:
            has_failure = True
            failure_type = "assertion"
            raw_output_lines.append(output.rstrip("\n"))
            continue

        # Collect output lines that look like test failure output
        if line.action == ActionType.OUTPUT:
            raw_output_lines.append(output.rstrip("\n"))

    if not has_failure:
        return None

    if not failure_type:
        failure_type = "assertion"

    # For non-panic failures, parse the raw output to extract a useful message.
    direct = failure_type == "panic"
    if failure_type != "panic" and not message:
        pulumi_errors = _extract_pulumi_errors(raw_output_lines)
        if pulumi_errors:
            message = "\n".join(pulumi_errors)
            failure_type = "infrastructure"
            direct = True
        else:
            blocks = _drop_eventually_wrappers(_parse_assertion_blocks(raw_output_lines))
            if blocks:
                direct = True
            message = _extract_message_from_raw_output(raw_output_lines)

    # For assertion failures, extract a stacktrace from the output if we
    # don't already have one (panics already have goroutine traces)
    if failure_type == "assertion" and not stacktrace_lines:
        trace = _extract_stacktrace_from_raw_output(raw_output_lines)
        if trace:
            stacktrace_lines = [trace]

    return UTOFFailure(
        message=message,
        type=failure_type,
        stacktrace="\n".join(stacktrace_lines) if stacktrace_lines else "",
        raw_output="\n".join(raw_output_lines) if raw_output_lines else "",
        direct=direct,
    )
