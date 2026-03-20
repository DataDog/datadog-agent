"""E2E-specific failure extractors for UTOF.

These extractors handle infrastructure-level error formats (e.g. Pulumi) that
only appear in E2E test output.  They conform to the ``FailureExtractor``
protocol and can be passed to ``build_attempts(custom_extractors=[...])``.
"""

from __future__ import annotations

import re

# Pulumi resource section header: "  kubernetes:helm.sh/v3:Release (dda-linux):"
_RE_PULUMI_RESOURCE_SECTION = re.compile(r'^\s+(\w[\w./-]*:\w[\w./-]*:\w[\w./ -]*)\s+\(([^)]+)\):\s*$')

# Pulumi error bullet: "    \t* Helm release ..."
_RE_PULUMI_ERROR_BULLET = re.compile(r'^\s+\t\*\s+(.+)')


def _extract_pulumi_errors(raw_output_lines: list[str]) -> list[str]:
    """Extract Pulumi resource error messages from Go test output.

    Pulumi formats infrastructure failures as::

        resource:type:Name (logical-name):
            error: N error(s) occurred:
            \\t* <actual error message>

    Returns a deduplicated list of ``"resource (name): error"`` strings,
    or an empty list if no Pulumi error bullets are found.
    """
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


def pulumi_extractor(raw_output_lines: list[str]) -> tuple[str, str] | None:
    """FailureExtractor for Pulumi infrastructure errors.

    Returns ``("infrastructure", message)`` when Pulumi error bullets are
    found, or ``None`` to let the generic parser handle the output.

    Usage::

        from tasks.libs.testing.utof.e2e.extractors import pulumi_extractor

        attempts = build_attempts(actions, custom_extractors=[pulumi_extractor])
    """
    errors = _extract_pulumi_errors(raw_output_lines)
    if errors:
        return ("infrastructure", "\n".join(errors))
    return None
