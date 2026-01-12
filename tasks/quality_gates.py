import os
import random
import re
import tempfile
import traceback
import typing

import gitlab
import yaml
from invoke import task
from invoke.exceptions import Exit

from tasks.github_tasks import pr_commenter
from tasks.libs.ciproviders.github_api import GithubAPI, create_datadog_agent_pr
from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.common.color import color_message
from tasks.libs.common.git import (
    create_tree,
    get_ancestor_base_branch,
    get_commit_sha,
    get_common_ancestor,
    get_current_branch,
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
        # For neutral changes, just show the current size (bolded)
        limit_bounds_str = f"**{current_mib:.3f}** MiB"
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

    # Sort gates by error_types to group in between NoError, AssertionError and StackTrace
    for gate in sorted(gate_states, key=lambda x: x["error_type"] is None):
        gate_name = gate['name'].replace("static_quality_gate_", "")

        # Get change metrics for on-disk (delta with percentage and limit bounds)
        change_str, limit_bounds, is_neutral = get_change_metrics(gate['name'], metric_handler, metric_type="disk")

        # Get change metrics for on-wire
        wire_change_str, wire_limit_bounds, _ = get_change_metrics(gate['name'], metric_handler, metric_type="wire")

        if gate["error_type"] is None:
            if is_neutral:
                # Neutral changes go to collapsed section (just show current size)
                gate_metrics = metric_handler.metrics.get(gate['name'], {})
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

            body_error += f"|{status_char}|{gate_name}|{change_str}|{limit_bounds}|\n"

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

    body = f"{SUCCESS_CHAR if final_state else FAIL_CHAR} Please find below the results from static quality gates\n{ancestor_info}{dashboard_link}{final_error_body}\n\n{success_section}\n{wire_section}"

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
    if not is_a_release_branch(ctx, branch):
        pr = get_pr_for_branch(branch)
        if pr:
            print(color_message(f"Found PR #{pr.number}: {pr.title}", "cyan"))
            pr_number = str(pr.number)
        else:
            # On main branch (or when no open PR), extract PR number from commit message
            pr_number = get_pr_number_from_commit(ctx)
            if pr_number:
                print(color_message(f"Extracted PR #{pr_number} from commit message", "cyan"))

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
            # Build tags dict - only include pr_number if we have a PR
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
    base_branch = get_ancestor_base_branch(branch)
    ancestor = get_common_ancestor(ctx, "HEAD", base_branch, try_fetch=True)
    current_commit = get_commit_sha(ctx)
    # When on main/release branch, get_common_ancestor returns HEAD itself since merge-base of HEAD and origin/<branch>
    # is the current commit. In this case, use the parent commit as the ancestor instead.
    if ancestor == current_commit:
        ancestor = get_commit_sha(ctx, commit="HEAD~1")
        print(color_message(f"On main branch, using parent commit {ancestor} as ancestor", "cyan"))
    metric_handler.generate_relative_size(ctx, ancestor=ancestor, report_path="ancestor_static_gate_report.json")

    # Post-process gate failures: mark as non-blocking if delta <= 0
    # This means the size issue existed before this PR and wasn't introduced by current changes
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


@task()
def exception_threshold_bump(ctx, pipeline_id):
    """
    When a PR is exempt of static quality gates, they have to use this invoke task to adjust the quality gates thresholds accordingly to the exempted added size.

    Note: This invoke task must be run on a pipeline that has finished running static quality gates
    :param ctx:
    :param pipeline_id: pipeline ID we want to fetch the artifact from to bump gates
    :return:
    """
    current_branch_name = get_current_branch(ctx)
    repo = get_gitlab_repo()
    with tempfile.TemporaryDirectory() as extract_dir, ctx.cd(extract_dir):
        cur_pipeline = repo.pipelines.get(pipeline_id)
        gate_job_id = next(
            job.id for job in cur_pipeline.jobs.list(iterator=True) if job.name == "static_quality_gates"
        )
        gate_job = repo.jobs.get(id=gate_job_id)
        with open(f"{extract_dir}/gate_archive.zip", "wb") as f:
            try:
                f.write(gate_job.artifacts())
            except gitlab.exceptions.GitlabGetError as e:
                print(
                    color_message(
                        "[ERROR] Unable to fetch the last artifact of the static_quality_gates job. Details :", "red"
                    )
                )
                print(repr(e))
                raise Exit(code=1) from e
        ctx.run(f"unzip gate_archive.zip -d {extract_dir}", hide=True)
        static_gate_report_path = f"{extract_dir}/static_gate_report.json"
        if os.path.isfile(static_gate_report_path):
            metric_handler = GateMetricHandler(
                git_ref=current_branch_name, bucket_branch="dev", filename=static_gate_report_path
            )
            with open("test/static/static_quality_gates.yml") as f:
                file_content, total_size_saved = generate_new_quality_gate_config(f, metric_handler, True)

            if total_size_saved == 0:
                print(color_message("[WARN] No gates needs to be changed.", "orange"))

            with open("test/static/static_quality_gates.yml", "w") as f:
                f.write(yaml.dump(file_content))

            print(
                color_message(
                    f"[SUCCESS] Static Quality gate have been updated ! Total gate threshold impact : {byte_to_string(-total_size_saved)}",
                    "green",
                )
            )
        else:
            print(
                color_message(
                    "[ERROR] Unable to find static_gate_report.json inside of the last artifact of the static_quality_gates job",
                    "red",
                )
            )
            raise Exit(code=1)


@task
def measure_package_local(
    ctx,
    package_path,
    gate_name,
    config_path="test/static/static_quality_gates.yml",
    output_path=None,
    build_job_name="local_test",
    debug=False,
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
