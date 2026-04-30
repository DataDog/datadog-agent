import os
import traceback
import typing
from collections.abc import Callable
from concurrent.futures import ThreadPoolExecutor, as_completed

import yaml
from invoke import task
from invoke.exceptions import Exit

from tasks.git import get_ancestor
from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.color import color_message
from tasks.libs.common.git import (
    get_commit_sha,
    is_a_release_branch,
)
from tasks.libs.package.size import InfraError
from tasks.static_quality_gates.decisions import (
    PER_PR_THRESHOLD,
    ExceptionApprovalChecker,
    identify_gates_exceeding_pr_threshold,
    should_bypass_failure,
)
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
    byte_to_string,
)
from tasks.static_quality_gates.gates_reporter import QualityGateOutputFormatter
from tasks.static_quality_gates.github import get_pr_author, get_pr_for_branch, get_pr_number_from_commit
from tasks.static_quality_gates.metrics import (
    fetch_main_headroom,
    fetch_pr_metrics,
)
from tasks.static_quality_gates.pr_comment import (
    FAIL_CHAR,
    SUCCESS_CHAR,
    display_pr_comment,
)
from tasks.static_quality_gates.thresholds import (
    GATE_CONFIG_PATH,
    identify_gates_with_size_increase,
    notify_threshold_update,
    update_quality_gates_threshold,
)


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


def _run_gate(ctx, gate: StaticQualityGate):
    try:
        result = gate.execute_gate(ctx)
        error_message = None
        error_type = None
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
            error_type = "StaticQualityGateFailed"
            print(color_message(error_message, "red"))
        return {
            "name": result.config.gate_name,
            "state": result.success,
            "error_type": error_type,
            "message": error_message,
            "result": result,
            "blocking": not result.success,
        }
    # re-raise the InfraError as is, don't swallow it as Exception
    except InfraError:
        raise
    except Exception:
        return {
            "name": gate.config.gate_name,
            "state": False,
            "error_type": "StackTrace",
            "message": traceback.format_exc(),
            "blocking": True,  # StackTrace errors are always blocking
        }


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

    if os.environ.get("SKIP_WINDOWS") == "true":
        gate_list = [gate for gate in gate_list if gate.config.os != "windows"]
        print(color_message("SKIP_WINDOWS is set: skipping Windows MSI quality gates", "orange"))

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

    # Run all gates in parallel (I/O-bound: pulling images, measuring packages)
    gate_results: dict[StaticQualityGate, dict] = {}
    executor = ThreadPoolExecutor()
    future_to_gate = {executor.submit(_run_gate, ctx, gate): gate for gate in gate_list}
    try:
        for future in as_completed(future_to_gate):
            gate_results[future_to_gate[future]] = future.result()
    except InfraError as e:
        # Cancel queued futures; running threads cannot be interrupted but will be abandoned
        executor.shutdown(wait=False, cancel_futures=True)
        gate = future_to_gate[future]
        print(color_message(f"Gate {gate.config.gate_name} flaked ! (InfraError)\n Restarting the job...", "red"))
        for line in traceback.format_exception(e):
            print(color_message(line, "red"))
        ctx.run("datadog-ci tag --level job --tags static_quality_gates:\"restart\"")
        raise Exit(code=42) from e
    executor.shutdown(wait=False)

    # Process results in original gate order
    for gate in gate_list:
        gate_result = gate_results[gate]
        gate_states.append(gate_result)
        if 'blocking' in gate_result and gate_result['blocking']:
            final_state = "failure"
        result = gate_result.get('result')
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

    # Calculate relative sizes (delta from ancestor) before sending metrics
    # This is done for all branches to include delta metrics in Datadog
    # Use get_ancestor_base_branch to correctly handle PRs targeting release branches
    ancestor = get_ancestor(ctx, branch)
    current_commit = get_commit_sha(ctx)
    is_on_main_branch = ancestor == current_commit
    is_merge_queue = branch.startswith("mq-working-branch-")
    metric_handler.generate_relative_size(ancestor=ancestor)

    # Post-process gate failures: mark as non-blocking if delta <= 0
    # This tolerance only applies to PRs - on main branch, failures should always block unconditionally
    # This means on PRs, the size issue existed before this PR and wasn't introduced by current changes
    exception_checker = ExceptionApprovalChecker(pr)

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

        # Check per-PR threshold: if any gate increased by more than PER_PR_THRESHOLD, mark it as failing
        # Skip for merge queue jobs: the MQ working branch is an ephemeral merge preview, not a PR
        if not is_merge_queue:
            per_pr_exceeding = identify_gates_exceeding_pr_threshold(metric_handler)
            if per_pr_exceeding:
                threshold_str = byte_to_string(PER_PR_THRESHOLD)
                print(
                    color_message(
                        f"Per-PR threshold ({threshold_str}) exceeded by: {', '.join(per_pr_exceeding)}",
                        "red",
                    )
                )
                for gate_state in gate_states:
                    if gate_state["name"] in per_pr_exceeding and gate_state["state"] is True:
                        delta = metric_handler.metrics.get(gate_state["name"], {}).get("relative_on_disk_size", 0)
                        gate_state["state"] = False
                        gate_state["error_type"] = "PerPRThresholdExceeded"
                        gate_state["message"] = (
                            f"On-disk size increase of {byte_to_string(delta)} exceeds the per-PR threshold of {threshold_str}"
                        )
                        gate_state["blocking"] = exception_checker.get() is None

    # Recompute final_state now that all post-processing is done (bypass logic and per-PR threshold
    # can both change gate states after the initial gate execution loop).
    if any(gs["state"] is False for gs in gate_states):
        final_state = "failure"
    ctx.run(f"datadog-ci tag --level job --tags static_quality_gates:\"{final_state}\"")

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
            display_pr_comment(
                ctx, not has_blocking_failures, gate_states, metric_handler, ancestor, pr, exception_checker.get()
            )

        # Nightly pipelines have different package size and gates thresholds are unreliable for nightly pipelines
        # Only fail for blocking failures (non-blocking failures have delta=0 and don't block the PR)
        if has_blocking_failures and not nightly_run:
            metric_handler.generate_metric_reports(ctx, branch=branch, is_nightly=nightly_run)
            raise Exit(code=1)
    # We are generating our metric reports at the end to include relative size metrics
    metric_handler.generate_metric_reports(ctx, branch=branch, is_nightly=nightly_run)

    return gate_list


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
    filter: Callable[[str], bool] = lambda _: True,
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
