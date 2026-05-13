"""E2E-specific failure extractors for UTOF.

These extractors handle infrastructure-level error formats (e.g. Pulumi) that
only appear in e2e test output. They conform to the ``FailureExtractor``
protocol and can be passed to ``build_attempts(custom_extractors=[...])``.
"""

from __future__ import annotations

import re

# Pulumi resource section header: "  kubernetes:helm.sh/v3:Release (dda-linux):"
_RE_PULUMI_RESOURCE_SECTION = re.compile(r'^\s+(\w[\w./-]*:\w[\w./-]*:\w[\w./ -]*)\s+\(([^)]+)\):\s*$')


def _extract_pulumi_errors(raw_output_lines: list[str]) -> list[str]:
    """Extract Pulumi resource error blocks from Go test output.

    Pulumi formats infrastructure failures inside the ``Diagnostics:`` block as::

        resource:type:Name (logical-name):
            error: <message>
            <continuation context — stderr, package lists, kubectl response …>

    or, for resources that aggregate multiple errors::

        resource:type:Name (logical-name):
            error: N error(s) occurred:
            \\t* <error 1>
            \\t* <error 2>

    A block runs from a resource header to the next blank line or the next
    resource header. Every indented line in between is kept — those are the
    `error:` line plus the continuation context that explains *why* the
    resource failed, which is what an operator actually needs to triage.

    Returns a list of formatted multi-line strings (``"resource (name):\\n  …"``),
    one per resource, in source order. Empty list when no Pulumi-style resource
    sections are found.
    """
    blocks: list[tuple[str, list[str]]] = []
    current_resource: str | None = None
    current_lines: list[str] = []

    def flush() -> None:
        nonlocal current_resource, current_lines
        if current_resource is not None and current_lines:
            blocks.append((current_resource, current_lines))
        current_resource = None
        current_lines = []

    for line in raw_output_lines:
        m_res = _RE_PULUMI_RESOURCE_SECTION.match(line)
        if m_res:
            flush()
            current_resource = f"{m_res.group(1)} ({m_res.group(2)})"
            continue

        if current_resource is None:
            continue

        if line.strip() == "":
            flush()
            continue

        current_lines.append(line.strip())

    flush()

    return [f"{resource}:\n" + "\n".join(f"  {line}" for line in lines) for resource, lines in blocks]


def pulumi_extractor(raw_output_lines: list[str]) -> tuple[str, str] | None:
    """FailureExtractor for Pulumi infrastructure errors.

    Returns ``("infrastructure", message)`` when Pulumi error blocks are
    found, or ``None`` to let the generic parser handle the output.
    """
    errors = _extract_pulumi_errors(raw_output_lines)
    if errors:
        return ("infrastructure", "\n\n".join(errors))
    return None
