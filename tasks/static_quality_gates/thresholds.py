from __future__ import annotations

import os
import random

import yaml

from tasks.libs.ciproviders.github_api import create_datadog_agent_pr
from tasks.libs.common.git import create_tree
from tasks.libs.common.utils import running_in_ci
from tasks.static_quality_gates.gates import byte_to_string
from tasks.static_quality_gates.metrics import GateMetricsData

BUFFER_SIZE = 1000000

# Threshold for considering a size change as meaningful (not noise)
# Changes below this threshold are considered neutral and won't trigger a bump
SIZE_INCREASE_THRESHOLD_BYTES = 2 * 1024  # 2 KiB

GATE_CONFIG_PATH = "test/static/static_quality_gates.yml"


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
        if metrics.relative_on_disk_size is not None and metrics.relative_on_disk_size > SIZE_INCREASE_THRESHOLD_BYTES:
            gates_to_bump[gate_name] = metrics

    return gates_to_bump


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
