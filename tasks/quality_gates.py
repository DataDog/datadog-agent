import os
import random
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
from tasks.libs.common.git import create_tree, get_common_ancestor, get_current_branch, is_a_release_branch
from tasks.libs.common.utils import running_in_ci
from tasks.libs.package.size import InfraError
from tasks.static_quality_gates.experimental_gates import measure_package_local as _measure_package_local
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
GATE_CONFIG_PATH = "test/static/static_quality_gates.yml"

body_pattern = """### {}

||Quality gate|Delta|On disk size (MiB)|Delta|On wire size (MiB)|
|--|--|--|--|--|--|
"""

body_error_footer_pattern = """<details>
<summary>Gate failure full details</summary>

|Quality gate|Error type|Error message|
|----|---|--------|
"""


def display_pr_comment(
    ctx, final_state: bool, gate_states: list[dict[str, typing.Any]], metric_handler: GateMetricHandler, ancestor: str
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
    body_info = "<details>\n<summary>Successful checks</summary>\n\n" + body_pattern.format("Info")
    body_error = body_pattern.format("Error")
    body_error_footer = body_error_footer_pattern

    with_error = False
    with_info = False
    # Sort gates by error_types to group in between NoError, AssertionError and StackTrace
    for gate in sorted(gate_states, key=lambda x: x["error_type"] is None):

        def getMetric(*metric_names, gate_name=gate['name']):
            try:
                metric_number = len(metric_names)
                if metric_number == 1:
                    return metric_handler.get_formatted_metric(gate_name, metric_names[0], with_unit=False)
                elif metric_number == 2:
                    return metric_handler.get_formatted_metric_comparison(gate_name, *metric_names)
                else:
                    return "InvalidMetricParam"
            except KeyError:
                return "DataNotFound"

        gate_name = gate['name'].replace("static_quality_gate_", "")
        relative_disk_size, relative_wire_size = (
            getMetric("relative_on_disk_size"),
            getMetric("relative_on_wire_size"),
        )

        if gate["error_type"] is None:
            body_info += f"|{SUCCESS_CHAR}|{gate_name}|{relative_disk_size}|{getMetric('current_on_disk_size', 'max_on_disk_size')}|{relative_wire_size}|{getMetric('current_on_wire_size', 'max_on_wire_size')}|\n"
            with_info = True
        else:
            body_error += f"|{FAIL_CHAR}|{gate_name}|{relative_disk_size}|{getMetric('current_on_disk_size', 'max_on_disk_size')}|{relative_wire_size}|{getMetric('current_on_wire_size', 'max_on_wire_size')}|\n"
            error_message = gate['message'].replace('\n', '<br>')
            body_error_footer += f"|{gate_name}|{gate['error_type']}|{error_message}|\n"
            with_error = True
    if with_error:
        body_error_footer += "\n</details>\n\nStatic quality gates prevent the PR to merge!\nYou can check the static quality gates [confluence page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4805854687/Static+Quality+Gates) for guidance. We also have a [toolbox page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4887448722/Static+Quality+Gates+Toolbox) available to list tools useful to debug the size increase.\n"
        final_error_body = body_error + body_error_footer
    else:
        final_error_body = ""
    body_info += "\n</details>\n"
    body = f"{SUCCESS_CHAR if final_state else FAIL_CHAR} Please find below the results from static quality gates\n{ancestor_info}{final_error_body}\n\n{body_info if with_info else ''}"

    pr_commenter(ctx, title=title, body=body)


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
                    violation_messages.append(
                        f"{violation.measurement_type.title()} size {current_mb:.1f} MB "
                        f"exceeds limit of {max_mb:.1f} MB by {excess_mb:.1f} MB"
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
                }
            )
        finally:
            metric_handler.register_gate_tags(
                gate.config.gate_name,
                gate_name=gate.config.gate_name,
                arch=gate.config.arch,
                os=gate.config.os,
                pipeline_id=os.environ["CI_PIPELINE_ID"],
                ci_commit_ref_slug=os.environ["CI_COMMIT_REF_SLUG"],
                ci_commit_sha=os.environ["CI_COMMIT_SHA"],
            )
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

    # Reporting part
    # Send metrics to Datadog
    # and then print the summary table
    # in the job's log
    metric_handler.send_metrics_to_datadog()

    # Print summary table directly with composition-based gates and metric handler
    QualityGateOutputFormatter.print_summary_table(gate_list, gate_states, metric_handler)

    # Then print the traditional report for any failures
    if final_state != "success":
        _print_quality_gates_report(gate_states)

    # We don't need a PR notification nor gate failures on release branches
    if not is_a_release_branch(ctx, branch):
        github = GithubAPI()
        if github.get_pr_for_branch(branch).totalCount > 0:
            ancestor = get_common_ancestor(ctx, "HEAD")
            metric_handler.generate_relative_size(
                ctx, ancestor=ancestor, report_path="ancestor_static_gate_report.json"
            )
            display_pr_comment(ctx, final_state == "success", gate_states, metric_handler, ancestor)

        # Nightly pipelines have different package size and gates thresholds are unreliable for nightly pipelines
        if final_state != "success" and not nightly_run:
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
        tree = create_tree(ctx, f"origin/{current_branch.name}")
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
    max_files=20000,
    no_checksums=False,
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
        max_files: Maximum number of files to process in inventory (default: 10000)
        no_checksums: Skip checksum generation for faster processing (default: false)
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
        max_files=max_files,
        no_checksums=no_checksums,
        debug=debug,
    )
