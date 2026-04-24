from __future__ import annotations

import os
import typing

from tasks.github_tasks import pr_commenter
from tasks.static_quality_gates.decisions import PER_PR_THRESHOLD
from tasks.static_quality_gates.gates import GateMetricHandler, byte_to_string

FAIL_CHAR = "❌"
SUCCESS_CHAR = "✅"
WARNING_CHAR = "⚠️"

# Main table pattern for on-disk metrics (primary view)
body_pattern = """### {}

||Quality gate|Change|Size (prev → **curr** → max)|
|--|--|--|--|
"""

# Collapsed table pattern for successful checks with minimal changes
body_collapsed_pattern = """<details>
<summary>{} successful checks with minimal change (&lt; 2 KiB)</summary>

||Quality gate|Current Size|
|--|--|--|
"""

# Collapsed table pattern for on-wire sizes
body_wire_pattern = """<details>
<summary>On-wire sizes (compressed)</summary>

||Quality gate|Change|Size (prev → **curr** → max)|
|--|--|--|--|
"""

body_error_footer_pattern = """<details>
<summary>Gate failure full details</summary>

|Quality gate|Error type|Error message|
|----|---|--------|
"""


# Threshold for considering a size change as "neutral" (not meaningful)
# Changes below this threshold are collapsed and shown with simplified display
NEUTRAL_THRESHOLD_BYTES = 2 * 1024  # 2 KiB


def get_change_metrics(
    gate_name: str, metric_handler: GateMetricHandler, metric_type: str = "disk"
) -> tuple[str, str, bool]:
    """
    Calculate change metrics for a gate.

    Args:
        gate_name: The name of the quality gate
        metric_handler: The metric handler containing gate metrics
        metric_type: Either "disk" for on-disk sizes or "wire" for on-wire/compressed sizes

    Returns:
        Tuple of (change_str, limit_bounds_str, is_neutral) for display in PR comment.
        - change_str: e.g., "neutral", "-58.7 KiB (0.29% reduction)", "+98.3 KiB (1.35% increase)"
        - limit_bounds_str: e.g., "**707.163** MiB" for neutral, "707.000 → **707.163** → 707.240" for changes
        - is_neutral: True if the change is below the threshold (< 2 KiB)
    """
    gate_metrics = metric_handler.metrics.get(gate_name, {})

    # Select metric keys based on type
    current_key = f"current_on_{metric_type}_size"
    max_key = f"max_on_{metric_type}_size"
    relative_key = f"relative_on_{metric_type}_size"

    current_size = gate_metrics.get(current_key)
    max_size = gate_metrics.get(max_key)
    relative_size = gate_metrics.get(relative_key)

    # If we don't have the required metrics, return N/A
    if current_size is None or max_size is None:
        return "N/A", "N/A", False

    # Calculate baseline (ancestor size) from current - relative
    if relative_size is not None:
        baseline_size = current_size - relative_size
    else:
        baseline_size = None

    # Convert to MiB for display
    current_mib = current_size / (1024 * 1024)
    max_mib = max_size / (1024 * 1024)
    baseline_mib = baseline_size / (1024 * 1024) if baseline_size is not None else None

    # Determine if change is neutral (below threshold)
    is_neutral = relative_size is not None and abs(relative_size) < NEUTRAL_THRESHOLD_BYTES

    # Build limit bounds string based on whether change is neutral
    if is_neutral:
        # For neutral changes, show the current size (bolded) → limit
        limit_bounds_str = f"**{current_mib:.3f}** MiB → {max_mib:.3f}"
    elif baseline_mib is not None:
        # For meaningful changes, show: baseline → current (bold) → limit
        limit_bounds_str = f"{baseline_mib:.3f} → **{current_mib:.3f}** → {max_mib:.3f}"
    else:
        limit_bounds_str = f"N/A → **{current_mib:.3f}** → {max_mib:.3f}"

    # Build change string with delta and percentage
    if baseline_size is None or relative_size is None:
        change_str = "N/A"
    elif is_neutral:
        change_str = "neutral"
    else:
        # Format the delta in human-readable units
        delta_str = byte_to_string(relative_size)

        if baseline_size > 0:
            # Calculate percentage change relative to baseline
            pct_change = abs(relative_size / baseline_size) * 100

            if relative_size > 0:
                change_str = f"+{delta_str} ({pct_change:.2f}% increase)"
            else:
                change_str = f"{delta_str} ({pct_change:.2f}% reduction)"
        else:
            # Baseline is 0, can't calculate percentage
            if relative_size > 0:
                change_str = f"+{delta_str} (new)"
            else:
                change_str = f"{delta_str} (reduction)"

    return change_str, limit_bounds_str, is_neutral


def display_pr_comment(
    ctx,
    final_state: bool,
    gate_states: list[dict[str, typing.Any]],
    metric_handler: GateMetricHandler,
    ancestor: str,
    pr,
    exception_granted_by: str | None = None,
):
    """
    Display a comment on a PR with results from our static quality gates checks
    :param ctx: Invoke task context
    :param final_state: Boolean that represents the overall state of quality gates checks
    :param gate_states: State of each quality gate
    :param metric_handler: Precise metrics of each quality gate
    :param ancestor: Ancestor used for relative size comparaison
    :param exception_granted_by: Login of the reviewer who granted a per-PR threshold exception, or None
    :return:
    """
    title = "Static quality checks"
    ancestor_info = (
        f"Comparison made with [ancestor](https://github.com/DataDog/datadog-agent/commit/{ancestor}) {ancestor}\n"
    )
    dashboard_link = (
        "[📊 Static Quality Gates Dashboard](https://app.datadoghq.com/dashboard/5np-man-vak/static-quality-gates)\n"
    )
    job_url = os.environ.get("CI_JOB_URL", "")
    job_link = f"[🔗 SQG Job]({job_url})\n" if job_url else ""

    # Main tables for on-disk metrics
    body_info = ""
    body_info_collapsed = ""
    body_error = body_pattern.format("Error")
    body_error_footer = body_error_footer_pattern

    # On-wire sizes table (separate collapsed section)
    body_wire = ""

    with_blocking_error = False
    with_non_blocking_error = False
    significant_success_count = 0
    collapsed_success_count = 0
    has_na_change = False

    # Sort gates by error_types to group in between NoError, AssertionError and StackTrace
    for gate in sorted(gate_states, key=lambda x: x["error_type"] is None):
        gate_name = gate['name'].replace("static_quality_gate_", "")
        gate_metrics = metric_handler.metrics.get(gate['name'], {})

        # Get change metrics for on-disk (delta with percentage and limit bounds)
        change_str, limit_bounds, is_neutral = get_change_metrics(gate['name'], metric_handler, metric_type="disk")
        if change_str == "N/A":
            has_na_change = True

        # Get change metrics for on-wire
        wire_change_str, wire_limit_bounds, _ = get_change_metrics(gate['name'], metric_handler, metric_type="wire")

        if gate["error_type"] is None:
            if is_neutral:
                # Neutral changes go to collapsed section (just show current size)
                current_disk = gate_metrics.get("current_on_disk_size")
                if current_disk is not None:
                    current_mib = current_disk / (1024 * 1024)
                    current_size_str = f"**{current_mib:.3f}** MiB"
                else:
                    current_size_str = "N/A"
                body_info_collapsed += f"|{SUCCESS_CHAR}|{gate_name}|{current_size_str}|\n"
                collapsed_success_count += 1
            else:
                # Significant changes shown in main section
                if significant_success_count == 0:
                    body_info = "<details open>\n<summary>Successful checks</summary>\n\n" + body_pattern.format("Info")
                body_info += f"|{SUCCESS_CHAR}|{gate_name}|{change_str}|{limit_bounds}|\n"
                significant_success_count += 1

            # All successful gates go to wire table
            body_wire += f"|{SUCCESS_CHAR}|{gate_name}|{wire_change_str}|{wire_limit_bounds}|\n"
        else:
            # Check if this is a blocking or non-blocking failure
            is_blocking = gate.get("blocking", True)
            status_char = FAIL_CHAR if is_blocking else WARNING_CHAR

            if gate["error_type"] == "PerPRThresholdExceeded":
                body_error += f"|{status_char}|{gate_name} (per-PR threshold)|{change_str}|{limit_bounds}|\n"
            else:
                # This is probably way more convoluted than it should be, but the best we can do
                # without refactoring the data structures involved
                if gate_metrics.get("current_on_wire_size", 0) > gate_metrics.get("max_on_wire_size", float('inf')):
                    body_error += f"|{status_char}|{gate_name} (on wire)|{wire_change_str}|{wire_limit_bounds}|\n"
                if gate_metrics.get("current_on_disk_size", 0) > gate_metrics.get("max_on_disk_size", float('inf')):
                    body_error += f"|{status_char}|{gate_name} (on disk)|{change_str}|{limit_bounds}|\n"

            # Add to wire table for errors too
            body_wire += f"|{status_char}|{gate_name}|{wire_change_str}|{wire_limit_bounds}|\n"

            error_message = gate['message'].replace('\n', '<br>')
            if not is_blocking and gate["error_type"] == "PerPRThresholdExceeded":
                blocking_note = f" (non-blocking: exception granted by @{exception_granted_by})"
            elif not is_blocking:
                blocking_note = " (non-blocking: size unchanged from ancestor)"
            else:
                blocking_note = ""
            body_error_footer += f"|{gate_name}|{gate['error_type']}{blocking_note}|{error_message}|\n"

            if is_blocking:
                with_blocking_error = True
            else:
                with_non_blocking_error = True

    if with_blocking_error:
        body_error_footer += (
            "\n</details>\n\n"
            "Static quality gates prevent the PR to merge!\n"
            "You can check the static quality gates [confluence page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4805854687/Static+Quality+Gates) for guidance. "
            "We also have a [toolbox page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4887448722/Static+Quality+Gates+Toolbox) available to list tools useful to debug the size increase.\n"
            "Please either fix the size violation or [request an exception](https://datadoghq.atlassian.net/wiki/spaces/ABLD/pages/6034456675/Static+Quality+Gates+runbooks#Exception-process).\n"
        )
        final_error_body = body_error + body_error_footer
    elif with_non_blocking_error:
        body_error_footer += "\n</details>\n\nNote: Some gates exceeded limits but are non-blocking because the size hasn't increased from the ancestor commit.\n"
        final_error_body = body_error + body_error_footer
    else:
        final_error_body = ""

    exception_banner = ""
    if exception_granted_by:
        per_pr_excepted = [
            gs
            for gs in gate_states
            if gs.get("error_type") == "PerPRThresholdExceeded" and not gs.get("blocking", True)
        ]
        if per_pr_excepted:
            threshold_str = byte_to_string(PER_PR_THRESHOLD)
            exception_banner = f"{WARNING_CHAR} **Exception granted by @{exception_granted_by}**: this PR exceeds the per-PR size threshold ({threshold_str}) but will not be blocked.\n"

    # Build successful checks section
    success_section = ""
    if significant_success_count > 0:
        body_info += "\n</details>\n"
        success_section += body_info

    if collapsed_success_count > 0:
        success_section += body_collapsed_pattern.format(collapsed_success_count)
        success_section += body_info_collapsed
        success_section += "\n</details>\n"

    # Build on-wire sizes section (collapsed)
    wire_section = ""
    if body_wire:
        wire_section = body_wire_pattern
        wire_section += body_wire
        wire_section += "\n</details>\n"

    # Add retry hint if some deltas are N/A (ancestor metrics not yet available due to race condition)
    retry_hint = ""
    if has_na_change and job_url:
        retry_hint = f"SOME SIZE DELTAS ARE N/A (ANCESTOR METRICS NOT YET AVAILABLE). [RETRY JOB]({job_url})\n"

    body = f"{SUCCESS_CHAR if final_state else FAIL_CHAR} Please find below the results from static quality gates\n{ancestor_info}{dashboard_link}{job_link}{retry_hint}{exception_banner}{final_error_body}\n\n{success_section}\n{wire_section}"

    pr_commenter(ctx, title=title, body=body, pr=pr)
