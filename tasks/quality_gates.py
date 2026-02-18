import os
import random
import re
import traceback
import typing
from collections.abc import Callable
from dataclasses import dataclass

import yaml
from invoke import task
from invoke.exceptions import Exit

from tasks.git import get_ancestor
from tasks.github_tasks import pr_commenter
from tasks.libs.ciproviders.github_api import GithubAPI, create_datadog_agent_pr
from tasks.libs.common.color import color_message
from tasks.libs.common.datadog_api import query_metrics
from tasks.libs.common.git import (
    create_tree,
    get_commit_sha,
    is_a_release_branch,
)
from tasks.libs.common.utils import running_in_ci
from tasks.libs.package.size import InfraError
from tasks.static_quality_gates.experimental_gates import (
    measure_image_local as _measure_image_local,
)
from tasks.static_quality_gates.experimental_gates import (
    measure_package_local as _measure_package_local,
)
from tasks.static_quality_gates.gates import (
    GateMetricHandler,
    QualityGateFactory,
    StaticQualityGate,
    StaticQualityGateError,
    byte_to_string,
)
from tasks.static_quality_gates.gates_reporter import QualityGateOutputFormatter

BUFFER_SIZE = 1000000
FAIL_CHAR = "❌"
SUCCESS_CHAR = "✅"
WARNING_CHAR = "⚠️"
GATE_CONFIG_PATH = "test/static/static_quality_gates.yml"


@dataclass
class GateMetricsData:
    """Metrics for a single quality gate."""

    current_on_disk_size: int | None = None
    current_on_wire_size: int | None = None
    max_on_disk_size: int | None = None
    max_on_wire_size: int | None = None
    relative_on_disk_size: int | None = None
    relative_on_wire_size: int | None = None


def _extract_gate_name_from_scope(scope: str) -> str | None:
    """Extract gate_name from scope string like 'gate_name:static_quality_gate_agent_deb_amd64'."""
    for part in scope.split(","):
        if part.startswith("gate_name:"):
            return part.split(":", 1)[1]
    return None


def _get_latest_value_from_pointlist(pointlist: list) -> float | None:
    """Get the latest non-null value from a pointlist of Point objects.

    Point.value returns [timestamp, metric_value], so we access index 1.
    """
    if not pointlist:
        return None
    for point in reversed(pointlist):
        if point and point.value and point.value[1] is not None:
            return point.value[1]
    return None


def fetch_pr_metrics(pr_number: int) -> dict[str, GateMetricsData]:
    """
    Fetch metrics for a specific PR from Datadog.

    Uses a single API call to fetch all 4 metric types at once.

    Returns a dict mapping gate_name to GateMetricsData.
    """
    # Fetch all metrics in a single query using comma-separated metric names
    metrics_data: dict[str, GateMetricsData] = {}

    # Map metric names to attribute names
    metric_map = {
        "on_disk_size": "current_on_disk_size",
        "on_wire_size": "current_on_wire_size",
        "max_allowed_on_disk_size": "max_on_disk_size",
        "max_allowed_on_wire_size": "max_on_wire_size",
        "relative_on_disk_size": "relative_on_disk_size",
        "relative_on_wire_size": "relative_on_wire_size",
    }

    # Single query with all metrics (comma-separated)
    queries = ",".join(
        f"avg:datadog.agent.static_quality_gate.{m}{{pr_number:{pr_number}}} by {{gate_name}}" for m in metric_map
    )
    result = query_metrics(queries, from_time="now-1d", to_time="now")

    for series in result:
        gate_name = _extract_gate_name_from_scope(series.get("scope", ""))
        if not gate_name:
            continue

        if gate_name not in metrics_data:
            metrics_data[gate_name] = GateMetricsData()

        # Determine which metric this series is for from the expression
        expression = series.get("expression", "")
        for metric_suffix, attr_name in metric_map.items():
            if f".{metric_suffix}" in expression:
                latest_value = _get_latest_value_from_pointlist(series.get("pointlist", []))
                if latest_value is not None:
                    setattr(metrics_data[gate_name], attr_name, int(latest_value))
                break

    return metrics_data


def fetch_main_headroom(failing_gates: list[str]) -> dict[str, dict[str, int]]:
    """
    Fetch main branch metrics to calculate headroom (max - current).

    Only fetches metrics for the specified failing gates to minimize API footprint.

    Returns a dict mapping gate_name to {'disk_headroom': int, 'wire_headroom': int}.
    """
    if not failing_gates:
        return {}

    main_metrics: dict[str, dict[str, int]] = {}

    # Map metric names to keys
    metric_map = {
        "on_disk_size": "current_disk",
        "on_wire_size": "current_wire",
        "max_allowed_on_disk_size": "max_disk",
        "max_allowed_on_wire_size": "max_wire",
    }

    # Build gate filter - only query for failing gates
    gate_filter = " OR ".join(f"gate_name:{g}" for g in failing_gates)

    # Single query with all metrics for failing gates only
    queries = ",".join(
        f"avg:datadog.agent.static_quality_gate.{m}{{git_ref:main AND ({gate_filter})}} by {{gate_name}}"
        for m in metric_map
    )
    result = query_metrics(queries, from_time="now-1d", to_time="now")

    for series in result:
        gate_name = _extract_gate_name_from_scope(series.get("scope", ""))
        if not gate_name:
            continue

        if gate_name not in main_metrics:
            main_metrics[gate_name] = {}

        # Determine which metric this series is for from the expression
        expression = series.get("expression", "")
        for metric_suffix, key in metric_map.items():
            if f".{metric_suffix}" in expression:
                latest_value = _get_latest_value_from_pointlist(series.get("pointlist", []))
                if latest_value is not None:
                    main_metrics[gate_name][key] = int(latest_value)
                break

    # Calculate headroom for each gate
    headroom: dict[str, dict[str, int]] = {}
    for gate_name, metrics in main_metrics.items():
        disk_headroom = metrics.get("max_disk", 0) - metrics.get("current_disk", 0)
        wire_headroom = metrics.get("max_wire", 0) - metrics.get("current_wire", 0)
        headroom[gate_name] = {
            "disk_headroom": max(0, disk_headroom),
            "wire_headroom": max(0, wire_headroom),
        }

    return headroom


def identify_failing_gates(pr_metrics: dict[str, GateMetricsData]) -> dict[str, GateMetricsData]:
    """
    Identify gates that are failing (current > max).

    Returns only the gates that need threshold bumps.
    """
    failing: dict[str, GateMetricsData] = {}

    for gate_name, metrics in pr_metrics.items():
        disk_failing = (
            metrics.current_on_disk_size is not None
            and metrics.max_on_disk_size is not None
            and metrics.current_on_disk_size > metrics.max_on_disk_size
        )
        wire_failing = (
            metrics.current_on_wire_size is not None
            and metrics.max_on_wire_size is not None
            and metrics.current_on_wire_size > metrics.max_on_wire_size
        )

        if disk_failing or wire_failing:
            failing[gate_name] = metrics

    return failing


# Threshold for considering a size change as meaningful (not noise)
# Changes below this threshold are considered neutral and won't trigger a bump
SIZE_INCREASE_THRESHOLD_BYTES = 2 * 1024  # 2 KiB


def identify_gates_with_size_increase(pr_metrics: dict[str, GateMetricsData]) -> dict[str, GateMetricsData]:
    """
    Identify all gates that have a meaningful on-disk size increase.

    This is used for exception bumps where we want to bump ALL gates
    with size increases, not just the ones that are currently failing.

    A gate is included if relative_on_disk_size > SIZE_INCREASE_THRESHOLD_BYTES.

    Returns gates where on_disk_size has increased beyond the noise threshold.
    """
    gates_to_bump: dict[str, GateMetricsData] = {}

    for gate_name, metrics in pr_metrics.items():
        # Check if there's a meaningful size increase
        if metrics.relative_on_disk_size is not None and metrics.relative_on_disk_size > SIZE_INCREASE_THRESHOLD_BYTES:
            gates_to_bump[gate_name] = metrics

    return gates_to_bump


def get_pr_for_branch(branch: str):
    """
    Get PR info for a branch. Returns the PR object or None.

    This function is used to cache PR lookup results for reuse across:
    - Adding PR number as a metric tag
    - Displaying PR comments

    Args:
        branch: The branch name to look up

    Returns:
        The PR object if found, None otherwise
    """
    try:
        github = GithubAPI()
        prs = list(github.get_pr_for_branch(branch))
        return prs[0] if prs else None
    except Exception as e:
        print(color_message(f"[WARN] Failed to get PR for branch {branch}: {e}", "orange"))
        return None


def get_pr_number_from_commit(ctx) -> str | None:
    """
    Extract PR number from the HEAD commit message.

    On main branch, merged commits typically end with (#XXXXX).
    Example: "Fix bug in quality gates (#44462)"

    Args:
        ctx: Invoke context for running git commands

    Returns:
        The PR number as a string, or None if not found.
    """
    try:
        # Get the first line of the HEAD commit message
        result = ctx.run("git log -1 --pretty=%s HEAD", hide=True)
        commit_message = result.stdout.strip()

        # Match pattern like "(#12345)" at the end of the message
        match = re.search(r'\(#(\d+)\)\s*$', commit_message)
        if match:
            return match.group(1)
        return None
    except Exception as e:
        print(color_message(f"[WARN] Failed to extract PR number from commit: {e}", "orange"))
        return None


def get_pr_author(pr_number: str) -> str | None:
    """
    Get the author (login) of a PR by its number.

    Args:
        pr_number: The PR number as a string

    Returns:
        The PR author's GitHub login, or None if not found.
    """
    try:
        github = GithubAPI()
        pr = github.get_pr(int(pr_number))
        return pr.user.login if pr and pr.user else None
    except Exception as e:
        print(color_message(f"[WARN] Failed to get PR author for PR #{pr_number}: {e}", "orange"))
        return None


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


def should_bypass_failure(gate_name: str, metric_handler: GateMetricHandler) -> bool:
    """
    Check if a gate failure should be non-blocking because on-disk size delta is 0 or negative.

    A failure is considered non-blocking if the on-disk size hasn't increased from the ancestor,
    meaning the issue existed before this PR and wasn't introduced by the current changes.

    Note: Only on-disk size is checked because it's the primary metric for package size impact.

    Args:
        gate_name: The name of the quality gate to check
        metric_handler: The metric handler containing relative size metrics

    Returns:
        True if on-disk size delta is effectively <= 0 (bypass eligible), False otherwise
    """
    gate_metrics = metric_handler.metrics.get(gate_name, {})
    disk_delta = gate_metrics.get("relative_on_disk_size")

    # If we don't have delta data (e.g., no ancestor report), can't bypass
    if disk_delta is None:
        return False

    # Threshold: values smaller than 2 KiB are treated as 0
    # Small variations due to build non-determinism should not block PRs
    delta_threshold_bytes = 2 * 1024  # 2 KiB

    # Bypass if on-disk size hasn't meaningfully increased from ancestor
    return disk_delta <= delta_threshold_bytes


def display_pr_comment(
    ctx,
    final_state: bool,
    gate_states: list[dict[str, typing.Any]],
    metric_handler: GateMetricHandler,
    ancestor: str,
    pr,
):
    """
    Display a comment on a PR with results from our static quality gates checks
    :param ctx: Invoke task context
    :param final_state: Boolean that represents the overall state of quality gates checks
    :param gate_states: State of each quality gate
    :param metric_handler: Precise metrics of each quality gate
    :param ancestor: Ancestor used for relative size comparaison
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

            # This is probably way more convoluted than it should be, but the best we can do
            # without refactoring the data structures involved
            if gate_metrics.get("current_on_wire_size", 0) > gate_metrics.get("max_on_wire_size", float('inf')):
                body_error += f"|{status_char}|{gate_name} (on wire)|{wire_change_str}|{wire_limit_bounds}|\n"
            if gate_metrics.get("current_on_disk_size", 0) > gate_metrics.get("max_on_disk_size", float('inf')):
                body_error += f"|{status_char}|{gate_name} (on disk)|{change_str}|{limit_bounds}|\n"

            # Add to wire table for errors too
            body_wire += f"|{status_char}|{gate_name}|{wire_change_str}|{wire_limit_bounds}|\n"

            error_message = gate['message'].replace('\n', '<br>')
            blocking_note = "" if is_blocking else " (non-blocking: size unchanged from ancestor)"
            body_error_footer += f"|{gate_name}|{gate['error_type']}{blocking_note}|{error_message}|\n"

            if is_blocking:
                with_blocking_error = True
            else:
                with_non_blocking_error = True

    if with_blocking_error:
        body_error_footer += "\n</details>\n\nStatic quality gates prevent the PR to merge!\nYou can check the static quality gates [confluence page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4805854687/Static+Quality+Gates) for guidance. We also have a [toolbox page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4887448722/Static+Quality+Gates+Toolbox) available to list tools useful to debug the size increase.\n"
        final_error_body = body_error + body_error_footer
    elif with_non_blocking_error:
        body_error_footer += "\n</details>\n\nNote: Some gates exceeded limits but are non-blocking because the size hasn't increased from the ancestor commit.\n"
        final_error_body = body_error + body_error_footer
    else:
        final_error_body = ""

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

    body = f"{SUCCESS_CHAR if final_state else FAIL_CHAR} Please find below the results from static quality gates\n{ancestor_info}{dashboard_link}{job_link}{retry_hint}{final_error_body}\n\n{success_section}\n{wire_section}"

    pr_commenter(ctx, title=title, body=body, pr=pr)


def _print_quality_gates_report(gate_states: list[dict[str, typing.Any]]):
    print(color_message("======== Static Quality Gates Report ========", "magenta"))
    for gate in sorted(gate_states, key=lambda x: x["error_type"] is not None):
        if gate["error_type"] is None:
            print(color_message(f"Gate {gate['name']} succeeded {SUCCESS_CHAR}", "blue"))
        elif gate["error_type"] == "AssertionError":
            print(
                color_message(
                    f"Gate {gate['name']} failed {FAIL_CHAR} because of the following assertion failures :\n{gate['message']}",
                    "orange",
                )
            )
        else:
            print(
                color_message(
                    f"Gate {gate['name']} failed {FAIL_CHAR} with the following stack trace :\n{gate['message']}",
                    "orange",
                )
            )


@task
def parse_and_trigger_gates(ctx, config_path: str = GATE_CONFIG_PATH) -> list[StaticQualityGate]:
    """
    Parse and executes static quality gates using composition pattern
    :param ctx: Invoke context
    :param config_path: Static quality gates configuration file path
    :return: List of quality gates
    """
    final_state = "success"
    gate_states = []
    metric_handler = GateMetricHandler(
        git_ref=os.environ["CI_COMMIT_REF_SLUG"], bucket_branch=os.environ["BUCKET_BRANCH"]
    )
    gate_list = QualityGateFactory.create_gates_from_config(config_path)

    # python 3.11< does not allow to use \n in f-strings
    delimiter = '\n'
    print(color_message(f"Starting {len(gate_list)} quality gates...", "cyan"))
    print(color_message(f"Gates to run: {delimiter.join(gate.config.gate_name for gate in gate_list)}", "cyan"))

    nightly_run = os.environ.get("BUCKET_BRANCH") == "nightly"
    branch = os.environ["CI_COMMIT_BRANCH"]

    # Early PR lookup - cache for later use in metrics and PR comment
    # Skip for release branches since they don't have associated PRs
    pr = None
    pr_number = None
    pr_author = None
    if not is_a_release_branch(ctx, branch):
        pr = get_pr_for_branch(branch)
        if pr:
            print(color_message(f"Found PR #{pr.number}: {pr.title}", "cyan"))
            pr_number = str(pr.number)
            # Extract author directly from PR object
            if pr.user:
                pr_author = pr.user.login
                print(color_message(f"PR author: {pr_author}", "cyan"))
        else:
            # On main branch (or when no open PR), extract PR number from commit message
            pr_number = get_pr_number_from_commit(ctx)
            if pr_number:
                print(color_message(f"Extracted PR #{pr_number} from commit message", "cyan"))
                # Fetch author for the PR number
                pr_author = get_pr_author(pr_number)
                if pr_author:
                    print(color_message(f"PR author: {pr_author}", "cyan"))

    for gate in gate_list:
        result = None
        try:
            result = gate.execute_gate(ctx)
            if not result.success:
                violation_messages = []
                for violation in result.violations:
                    current_mb = violation.current_size / (1024 * 1024)
                    max_mb = violation.max_size / (1024 * 1024)
                    excess_mb = violation.excess_bytes / (1024 * 1024)
                    if excess_mb < 1:
                        excess_kb = violation.excess_bytes / 1024
                        excess_str = f"{excess_kb:.1f} KB"
                    else:
                        excess_str = f"{excess_mb:.1f} MB"
                    violation_messages.append(
                        f"{violation.measurement_type.title()} size {current_mb:.1f} MB "
                        f"exceeds limit of {max_mb:.1f} MB by {excess_str}"
                    )
                error_message = f"{gate.config.gate_name} failed!\n" + "\n".join(violation_messages)
                print(color_message(error_message, "red"))
                raise StaticQualityGateError(error_message)
            gate_states.append({"name": result.config.gate_name, "state": True, "error_type": None, "message": None})
        except StaticQualityGateError as e:
            final_state = "failure"
            gate_states.append(
                {
                    "name": gate.config.gate_name,
                    "state": False,
                    "error_type": "StaticQualityGateFailed",
                    "message": str(e),
                    "blocking": True,  # May be updated to False if delta=0 after relative size calculation
                }
            )
        except InfraError as e:
            print(color_message(f"Gate {gate.config.gate_name} flaked ! (InfraError)\n Restarting the job...", "red"))
            for line in traceback.format_exception(e):
                print(color_message(line, "red"))
            ctx.run("datadog-ci tag --level job --tags static_quality_gates:\"restart\"")
            raise Exit(code=42) from e
        except Exception:
            final_state = "failure"
            gate_states.append(
                {
                    "name": gate.config.gate_name,
                    "state": False,
                    "error_type": "StackTrace",
                    "message": traceback.format_exc(),
                    "blocking": True,  # StackTrace errors are always blocking
                }
            )
        finally:
            # Build tags dict - only include pr_number and pr_author if we have a PR
            gate_tags = {
                "gate_name": gate.config.gate_name,
                "arch": gate.config.arch,
                "os": gate.config.os,
                "pipeline_id": os.environ["CI_PIPELINE_ID"],
                "ci_commit_ref_slug": os.environ["CI_COMMIT_REF_SLUG"],
                "ci_commit_sha": os.environ["CI_COMMIT_SHA"],
            }
            if pr_number:
                gate_tags["pr_number"] = pr_number
            if pr_author:
                gate_tags["pr_author"] = pr_author

            metric_handler.register_gate_tags(gate.config.gate_name, **gate_tags)
            metric_handler.register_metric(gate.config.gate_name, "max_on_wire_size", gate.config.max_on_wire_size)
            metric_handler.register_metric(gate.config.gate_name, "max_on_disk_size", gate.config.max_on_disk_size)

            # Only register current sizes if gate executed successfully and we have a result
            if result is not None:
                metric_handler.register_metric(
                    gate.config.gate_name, "current_on_wire_size", result.measurement.on_wire_size
                )
                metric_handler.register_metric(
                    gate.config.gate_name, "current_on_disk_size", result.measurement.on_disk_size
                )

    ctx.run(f"datadog-ci tag --level job --tags static_quality_gates:\"{final_state}\"")

    # Calculate relative sizes (delta from ancestor) before sending metrics
    # This is done for all branches to include delta metrics in Datadog
    # Use get_ancestor_base_branch to correctly handle PRs targeting release branches
    ancestor = get_ancestor(ctx, branch)
    current_commit = get_commit_sha(ctx)
    is_on_main_branch = ancestor == current_commit
    metric_handler.generate_relative_size(ancestor=ancestor)

    # Post-process gate failures: mark as non-blocking if delta <= 0
    # This tolerance only applies to PRs - on main branch, failures should always block unconditionally
    # This means on PRs, the size issue existed before this PR and wasn't introduced by current changes
    if not is_on_main_branch:
        for gate_state in gate_states:
            if gate_state["state"] is False and gate_state.get("blocking", True):
                # Only StaticQualityGateFailed errors are eligible for bypass (not StackTrace errors)
                if gate_state["error_type"] == "StaticQualityGateFailed":
                    if should_bypass_failure(gate_state["name"], metric_handler):
                        gate_state["blocking"] = False
                        print(
                            color_message(
                                f"Gate {gate_state['name']} failure is non-blocking (size unchanged from ancestor)",
                                "orange",
                            )
                        )

    # Reporting part
    # Send metrics to Datadog (now includes delta metrics)
    # and then print the summary table in the job's log
    metric_handler.send_metrics_to_datadog()

    # Print summary table directly with composition-based gates and metric handler
    QualityGateOutputFormatter.print_summary_table(gate_list, gate_states, metric_handler)

    # Then print the traditional report for any failures
    if final_state != "success":
        _print_quality_gates_report(gate_states)

    # We don't need a PR notification nor gate failures on release branches
    if not is_a_release_branch(ctx, branch):
        # Determine if there are blocking failures (non-blocking failures have delta=0)
        has_blocking_failures = any(gs["state"] is False and gs.get("blocking", True) for gs in gate_states)

        # Reuse cached PR lookup from earlier
        if pr:
            # Pass True for final_state if there are no blocking failures
            display_pr_comment(ctx, not has_blocking_failures, gate_states, metric_handler, ancestor, pr)

        # Nightly pipelines have different package size and gates thresholds are unreliable for nightly pipelines
        # Only fail for blocking failures (non-blocking failures have delta=0 and don't block the PR)
        if has_blocking_failures and not nightly_run:
            metric_handler.generate_metric_reports(ctx, branch=branch, is_nightly=nightly_run)
            raise Exit(code=1)
    # We are generating our metric reports at the end to include relative size metrics
    metric_handler.generate_metric_reports(ctx, branch=branch, is_nightly=nightly_run)

    return gate_list


def get_gate_new_limit_threshold(current_gate, current_key, max_key, metric_handler, exception_bump=False):
    # The new limit is decreased when the difference between current and max value is greater than the `BUFFER_SIZE`
    # unless it is an exception bump where we will bump gates by the amount increased
    curr_size = metric_handler.metrics[current_gate][current_key]
    max_curr_size = metric_handler.metrics[current_gate][max_key]
    if exception_bump:
        bump_amount = max(0, metric_handler.metrics[current_gate][current_key.replace("current", "relative")])
        return max_curr_size + bump_amount, -bump_amount

    remaining_allowed_size = max_curr_size - curr_size
    gate_limit = max_curr_size
    saved_amount = 0
    if remaining_allowed_size > BUFFER_SIZE:
        saved_amount = remaining_allowed_size - BUFFER_SIZE
        gate_limit -= saved_amount
    return gate_limit, saved_amount


def generate_new_quality_gate_config(file_descriptor, metric_handler, exception_bump=False):
    config_content = yaml.safe_load(file_descriptor)
    total_saved_amount = 0
    for gate in config_content.keys():
        on_wire_new_limit, wire_saved_amount = get_gate_new_limit_threshold(
            gate, "current_on_wire_size", "max_on_wire_size", metric_handler, exception_bump
        )
        config_content[gate]["max_on_wire_size"] = byte_to_string(on_wire_new_limit, unit_power=2)
        on_disk_new_limit, disk_saved_amount = get_gate_new_limit_threshold(
            gate, "current_on_disk_size", "max_on_disk_size", metric_handler, exception_bump
        )
        config_content[gate]["max_on_disk_size"] = byte_to_string(on_disk_new_limit, unit_power=2)
        total_saved_amount += wire_saved_amount + disk_saved_amount
    return config_content, total_saved_amount


def update_quality_gates_threshold(ctx, metric_handler, github):
    # Update quality gates threshold config
    with open(GATE_CONFIG_PATH) as f:
        file_content, total_size_saved = generate_new_quality_gate_config(f, metric_handler)

    if total_size_saved == 0:
        return

    # Create new branch
    branch_name = f"static_quality_gates/threshold_update_{os.environ['CI_COMMIT_SHORT_SHA']}"
    current_branch = github.repo.get_branch(os.environ["CI_COMMIT_BRANCH"])
    ctx.run(f"git checkout -b {branch_name}")
    ctx.run(
        f"git remote set-url origin https://x-access-token:{github._auth.token}@github.com/DataDog/datadog-agent.git",
        hide=True,
    )
    ctx.run(f"git push --set-upstream origin {branch_name}")

    # Push changes
    commit_message = "feat(gate): update static quality gates thresholds"
    if running_in_ci():
        # Update config locally and add it to the stage
        with open(GATE_CONFIG_PATH, "w") as f:
            yaml.dump(file_content, f)
        ctx.run(f"git add {GATE_CONFIG_PATH}")
        print("Creating signed commits using Github API")
        tree = create_tree(ctx, current_branch.name)
        github.commit_and_push_signed(branch_name, commit_message, tree)
    else:
        print("Creating commits using your local git configuration, please make sure to sign them")
        contents = github.repo.get_contents("test/static/static_quality_gates.yml", ref=branch_name)
        github.repo.update_file(
            GATE_CONFIG_PATH,
            commit_message,
            yaml.dump(file_content),
            contents.sha,
            branch=branch_name,
        )

    # Create pull request
    milestone_version = list(github.latest_unreleased_release_branches())[0].name.replace("x", "0")
    return create_datadog_agent_pr(
        "[automated] Static quality gates threshold update",
        current_branch.name,
        branch_name,
        milestone_version,
        ["team/agent-build", "qa/no-code-change", "changelog/no-changelog"],
    )


def notify_threshold_update(pr_url):
    from slack_sdk import WebClient

    client = WebClient(os.environ['SLACK_DATADOG_AGENT_BOT_TOKEN'])
    emojis = client.emoji_list()
    waves = [emoji for emoji in emojis.data['emoji'] if 'wave' in emoji and 'microwave' not in emoji]
    message = f'Hello :{random.choice(waves)}:\nA new quality gates threshold <{pr_url}/s|update PR> has been generated !\nPlease take a look, thanks !'
    client.chat_postMessage(channel='#agent-build-reviews', text=message)


@task
def manual_threshold_update(self, filename="static_gate_report.json"):
    metric_handler = GateMetricHandler(
        git_ref=os.environ["CI_COMMIT_REF_SLUG"], bucket_branch=os.environ["BUCKET_BRANCH"], filename=filename
    )
    github = GithubAPI()
    pr_url = update_quality_gates_threshold(self, metric_handler, github)
    notify_threshold_update(pr_url)


@task(positional=["pr_number"], help={"pr_number": "The PR number to bump thresholds for"})
def exception_threshold_bump(ctx, pr_number):
    """
    Bump quality gate thresholds for a PR that has been granted an exception.

    This task queries Datadog metrics to:
    1. Find which gates are failing for this PR
    2. Get the current headroom on main (max - current)
    3. Set new thresholds = PR's current size + main's headroom

    Usage:
        dd-auth -- dda inv quality-gates.exception-threshold-bump <pr_number>
    """
    pr_number = int(pr_number)
    print(color_message(f"Fetching metrics for PR #{pr_number}...", "cyan"))

    # Step 1: Fetch PR metrics from Datadog
    pr_metrics = fetch_pr_metrics(pr_number)
    if not pr_metrics:
        print(color_message(f"[ERROR] No metrics found for PR #{pr_number} in the last 24 hours.", "red"))
        print(color_message("", "red"))
        print(color_message("This usually means one of the following:", "orange"))
        print(color_message("  1. The PR branch is stale and needs to be updated", "orange"))
        print(color_message("  2. The static_quality_gates job hasn't run recently", "orange"))
        print(color_message("  3. The PR number is incorrect", "orange"))
        print(color_message("", "red"))
        print(color_message("Recommended actions:", "cyan"))
        print(color_message("  - Update your branch: git fetch origin main && git rebase origin/main", "cyan"))
        print(color_message("  - Push to trigger a new pipeline run", "cyan"))
        print(color_message("  - Wait for static_quality_gates job to complete", "cyan"))
        print(color_message("  - Re-run this command", "cyan"))
        raise Exit(code=1)

    print(color_message(f"Found metrics for {len(pr_metrics)} gates", "cyan"))

    # Step 2: Identify gates with size increase (not just failing gates)
    gates_to_bump = identify_gates_with_size_increase(pr_metrics)
    if not gates_to_bump:
        print(color_message("[INFO] No gates with size increase found - nothing to bump!", "green"))
        return

    print(color_message(f"Found {len(gates_to_bump)} gates with size increase:", "orange"))
    for gate_name, metrics in gates_to_bump.items():
        short_name = gate_name.replace("static_quality_gate_", "")
        disk_delta = metrics.relative_on_disk_size or 0
        wire_delta = metrics.relative_on_wire_size or 0
        print(
            color_message(
                f"  - {short_name}: disk +{byte_to_string(disk_delta)}, wire +{byte_to_string(wire_delta)}", "orange"
            )
        )

    # Step 3: Fetch main branch headroom (for gates with size increase)
    print(color_message("Fetching main branch metrics for headroom calculation...", "cyan"))
    main_headroom = fetch_main_headroom(list(gates_to_bump.keys()))

    if not main_headroom:
        print(color_message("[ERROR] Unable to fetch main branch metrics from Datadog.", "red"))
        print(color_message("Please check your Datadog API credentials and try again.", "orange"))
        raise Exit(code=1)

    # Step 4: Load current config
    with open(GATE_CONFIG_PATH) as f:
        config = yaml.safe_load(f)

    # Step 5: Calculate and apply new thresholds for gates with size increase
    updated_gates = []
    for gate_name, pr_gate_metrics in gates_to_bump.items():
        if gate_name not in config:
            print(color_message(f"[WARN] Gate {gate_name} not found in config, skipping", "orange"))
            continue

        headroom = main_headroom.get(gate_name, {"disk_headroom": 0, "wire_headroom": 0})

        # Calculate new thresholds: PR's current + main's headroom
        short_name = gate_name.replace("static_quality_gate_", "")
        updates = []

        if pr_gate_metrics.current_on_disk_size is not None:
            disk_headroom = headroom["disk_headroom"]
            new_disk_threshold = pr_gate_metrics.current_on_disk_size + disk_headroom
            old_disk = config[gate_name].get("max_on_disk_size", "N/A")
            config[gate_name]["max_on_disk_size"] = byte_to_string(new_disk_threshold, unit_power=2)
            updates.append(f"disk: {old_disk} → {config[gate_name]['max_on_disk_size']}")

        if pr_gate_metrics.current_on_wire_size is not None:
            wire_headroom = headroom["wire_headroom"]
            new_wire_threshold = pr_gate_metrics.current_on_wire_size + wire_headroom
            old_wire = config[gate_name].get("max_on_wire_size", "N/A")
            config[gate_name]["max_on_wire_size"] = byte_to_string(new_wire_threshold, unit_power=2)
            updates.append(f"wire: {old_wire} → {config[gate_name]['max_on_wire_size']}")

        if updates:
            updated_gates.append((short_name, updates))

    # Step 6: Write updated config
    if updated_gates:
        with open(GATE_CONFIG_PATH, "w") as f:
            yaml.dump(config, f)

        print(color_message(f"\n[SUCCESS] Updated {len(updated_gates)} gate thresholds:", "green"))
        for gate_name, updates in updated_gates:
            for update in updates:
                print(color_message(f"  - {gate_name}: {update}", "green"))
    else:
        print(color_message("[WARN] No gates were updated", "orange"))


@task
def measure_package_local(
    ctx,
    package_path,
    gate_name,
    config_path="test/static/static_quality_gates.yml",
    output_path=None,
    build_job_name="local_test",
    debug=False,
    filter: Callable[[str], bool] = None,
):
    """
    Run the in-place package measurer locally for testing and development.

    This task allows you to test the measurement functionality on local packages
    without requiring a full CI environment.

    Args:
        package_path: Path to the package file to measure
        gate_name: Quality gate name from the configuration file
        config_path: Path to quality gates configuration (default: test/static/static_quality_gates.yml)
        output_path: Path to save the measurement report (default: {gate_name}_report.yml)
        build_job_name: Simulated build job name (default: local_test)
        debug: Enable debug logging for troubleshooting (default: false)

    Example:
        dda inv quality-gates.measure-package-local --package-path /path/to/package.deb --gate-name static_quality_gate_agent_deb_amd64
    """
    return _measure_package_local(
        ctx=ctx,
        package_path=package_path,
        gate_name=gate_name,
        config_path=config_path,
        output_path=output_path,
        build_job_name=build_job_name,
        debug=debug,
        filter=filter,
    )


@task
def measure_image_local(
    ctx,
    image_ref,
    gate_name,
    config_path="test/static/static_quality_gates.yml",
    output_path=None,
    build_job_name="local_test",
    include_layer_analysis=True,
    debug=False,
):
    """
    Run the in-place Docker image measurer locally for testing and development.

    This task allows you to test the Docker image measurement functionality on local images
    without requiring a full CI environment.

    Args:
        image_ref: Docker image reference (tag, digest, or image ID)
        gate_name: Quality gate name from the configuration file
        config_path: Path to quality gates configuration (default: test/static/static_quality_gates.yml)
        output_path: Path to save the measurement report (default: {gate_name}_image_report.yml)
        build_job_name: Simulated build job name (default: local_test)
        include_layer_analysis: Whether to analyze individual layers (default: true)
        debug: Enable debug logging for troubleshooting (default: false)

    Example:
        dda inv quality-gates.measure-image-local --image-ref nginx:latest --gate-name static_quality_gate_docker_agent_amd64
    """
    return _measure_image_local(
        ctx=ctx,
        image_ref=image_ref,
        gate_name=gate_name,
        config_path=config_path,
        output_path=output_path,
        build_job_name=build_job_name,
        include_layer_analysis=include_layer_analysis,
        debug=debug,
    )
