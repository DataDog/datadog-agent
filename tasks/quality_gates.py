import os
import random
import traceback
import typing

import yaml
from invoke import task
from invoke.exceptions import Exit

from tasks.github_tasks import pr_commenter
from tasks.libs.ciproviders.github_api import GithubAPI, create_datadog_agent_pr
from tasks.libs.common.color import color_message
from tasks.libs.common.utils import is_conductor_scheduled_pipeline
from tasks.libs.package.size import InfraError
from tasks.static_quality_gates.lib.gates_lib import GateMetricHandler, byte_to_string, is_first_commit_of_the_day

BUFFER_SIZE = 1000000
FAIL_CHAR = "❌"
SUCCESS_CHAR = "✅"

body_pattern = """### {}

|Result|Quality gate|On disk size|On disk size limit|On wire size|On wire size limit|
|----|----|----|----|----|----|
"""

body_error_footer_pattern = """<details>
<summary>Gate failure full details</summary>

|Quality gate|Error type|Error message|
|----|---|--------|
"""


def display_pr_comment(
    ctx, final_state: bool, gate_states: list[dict[str, typing.Any]], metric_handler: GateMetricHandler
):
    """
    Display a comment on a PR with results from our static quality gates checks
    :param ctx: Invoke task context
    :param final_state: Boolean that represents the overall state of quality gates checks
    :param gate_states: State of each quality gate
    :param metric_handler: Precise metrics of each quality gate
    :return:
    """
    title = f"Static quality checks {SUCCESS_CHAR if final_state else FAIL_CHAR}"
    body_info = "<details>\n<summary>Successful checks</summary>\n\n" + body_pattern.format("Info")
    body_error = body_pattern.format("Error")
    body_error_footer = body_error_footer_pattern

    with_error = False
    with_info = False
    # Sort gates by error_types to group in between NoError, AssertionError and StackTrace
    for gate in sorted(gate_states, key=lambda x: x["error_type"] is None):

        def getMetric(metric_name, gate_name=gate['name']):
            try:
                return metric_handler.get_formatted_metric(gate_name, metric_name)
            except KeyError:
                return "DataNotFound"

        if gate["error_type"] is None:
            body_info += f"|{SUCCESS_CHAR}|{gate['name']}|{getMetric('current_on_disk_size')}|{getMetric('max_on_disk_size')}|{getMetric('current_on_wire_size')}|{getMetric('max_on_wire_size')}|\n"
            with_info = True
        else:
            body_error += f"|{FAIL_CHAR}|{gate['name']}|{getMetric('current_on_disk_size')}|{getMetric('max_on_disk_size')}|{getMetric('current_on_wire_size')}|{getMetric('max_on_wire_size')}|\n"
            error_message = gate['message'].replace('\n', '<br>')
            body_error_footer += f"|{gate['name']}|{gate['error_type']}|{error_message}|\n"
            with_error = True

    body_error_footer += "\n</details>\n\nStatic quality gates prevent the PR to merge! You can check the static quality gates [confluence page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4805854687/Static+Quality+Gates) for guidance. We also have a [toolbox page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4887448722/Static+Quality+Gates+Toolbox) available to list tools useful to debug the size increase.\n"
    body_info += "\n</details>\n"
    body = f"Please find below the results from static quality gates\n{body_error+body_error_footer if with_error else ''}\n\n{body_info if with_info else ''}"

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
def parse_and_trigger_gates(ctx, config_path="test/static/static_quality_gates.yml"):
    """
    Parse and executes static quality gates
    :param ctx: Invoke context
    :param config_path: Static quality gates configuration file path
    :return:
    """
    with open(config_path) as file:
        config = yaml.safe_load(file)

    gate_list = list(config.keys())
    quality_gates_mod = __import__("tasks.static_quality_gates", fromlist=gate_list)
    print(f"{config_path} correctly parsed !")
    metric_handler = GateMetricHandler(
        git_ref=os.environ["CI_COMMIT_REF_SLUG"], bucket_branch=os.environ["BUCKET_BRANCH"]
    )
    newline_tab = "\n\t"
    print(f"The following gates are going to run:{newline_tab}- {(newline_tab+'- ').join(gate_list)}")
    final_state = "success"
    gate_states = []

    threshold_update_run = False
    nightly_run = False
    branch = os.environ["CI_COMMIT_BRANCH"]
    bucket_branch = os.environ["BUCKET_BRANCH"]
    # we avoid nightly pipelines because they have different package size than the main branch
    if branch == "main" and bucket_branch != "nightly" and is_first_commit_of_the_day(ctx):
        threshold_update_run = True

    DDR_WORKFLOW_ID = os.environ.get("DDR_WORKFLOW_ID")
    if DDR_WORKFLOW_ID and branch == "main" and is_conductor_scheduled_pipeline():
        nightly_run = True

    for gate in gate_list:
        gate_inputs = config[gate]
        gate_inputs["ctx"] = ctx
        gate_inputs["metricHandler"] = metric_handler
        gate_inputs["nightly"] = nightly_run
        try:
            gate_mod = getattr(quality_gates_mod, gate)
            gate_mod.entrypoint(**gate_inputs)
            print(f"Gate {gate} succeeded !")
            gate_states.append({"name": gate, "state": True, "error_type": None, "message": None})
        except AssertionError as e:
            print(f"Gate {gate} failed ! (AssertionError)")
            final_state = "failure"
            gate_states.append({"name": gate, "state": False, "error_type": "AssertionError", "message": str(e)})
        except InfraError as e:
            print(f"Gate {gate} flaked ! (InfraError)\n Restarting the job...")
            ctx.run("datadog-ci tag --level job --tags static_quality_gates:\"restart\"")
            raise Exit(code=42) from e
        except Exception:
            print(f"Gate {gate} failed ! (StackTrace)")
            final_state = "failure"
            gate_states.append(
                {"name": gate, "state": False, "error_type": "StackTrace", "message": traceback.format_exc()}
            )
    ctx.run(f"datadog-ci tag --level job --tags static_quality_gates:\"{final_state}\"")

    _print_quality_gates_report(gate_states)

    metric_handler.send_metrics_to_datadog()

    github = GithubAPI()
    if github.get_pr_for_branch(branch).totalCount > 0:
        display_pr_comment(ctx, final_state == "success", gate_states, metric_handler)

    # Generate PR to update static quality gates threshold once per day (scheduled main pipeline by conductor)
    if threshold_update_run:
        pr_url = update_quality_gates_threshold(ctx, metric_handler, github)
        notify_threshold_update(pr_url)

    # Nightly pipelines have different package size and gates thresholds are unreliable for nightly pipelines
    if final_state != "success" and not nightly_run:
        raise Exit(code=1)


def get_gate_new_limit_threshold(current_gate, current_key, max_key, metric_handler):
    # The new limit is decreased when the difference between current and max value is greater than the `BUFFER_SIZE`
    curr_size = metric_handler.metrics[current_gate][current_key]
    max_curr_size = metric_handler.metrics[current_gate][max_key]
    remaining_allowed_size = max_curr_size - curr_size
    gate_limit = max_curr_size
    saved_amount = 0
    if remaining_allowed_size > BUFFER_SIZE:
        saved_amount = remaining_allowed_size - BUFFER_SIZE
        gate_limit -= saved_amount
    return gate_limit, saved_amount


def generate_new_quality_gate_config(file_descriptor, metric_handler):
    config_content = yaml.safe_load(file_descriptor)
    total_saved_amount = 0
    for gate in config_content.keys():
        on_wire_new_limit, wire_saved_amount = get_gate_new_limit_threshold(
            gate, "current_on_wire_size", "max_on_wire_size", metric_handler
        )
        config_content[gate]["max_on_wire_size"] = byte_to_string(on_wire_new_limit)
        on_disk_new_limit, disk_saved_amount = get_gate_new_limit_threshold(
            gate, "current_on_disk_size", "max_on_disk_size", metric_handler
        )
        config_content[gate]["max_on_disk_size"] = byte_to_string(on_disk_new_limit)
        total_saved_amount += wire_saved_amount + disk_saved_amount
    return config_content, total_saved_amount


def update_quality_gates_threshold(ctx, metric_handler, github):
    # Update quality gates threshold config
    with open("test/static/static_quality_gates.yml") as f:
        file_content, total_size_saved = generate_new_quality_gate_config(f, metric_handler)

    if total_size_saved == 0:
        return

    # Create new branch
    branch_name = f"static_quality_gates/threshold_update_{os.environ['CI_COMMIT_SHORT_SHA']}"
    current_branch = github.repo.get_branch(os.environ["CI_COMMIT_BRANCH"])
    github.repo.create_git_ref(ref=f'refs/heads/{branch_name}', sha=current_branch.commit.sha)

    # Update static_quality_gates.yml config file
    contents = github.repo.get_contents("test/static/static_quality_gates.yml", ref=branch_name)
    github.repo.update_file(
        "test/static/static_quality_gates.yml",
        "feat(gate): update static quality gates thresholds",
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
        ["team/agent-delivery", "qa/skip-qa", "changelog/no-changelog"],
    )


def notify_threshold_update(pr_url):
    from slack_sdk import WebClient

    client = WebClient(os.environ['SLACK_DATADOG_AGENT_BOT_TOKEN'])
    emojis = client.emoji_list()
    waves = [emoji for emoji in emojis.data['emoji'] if 'wave' in emoji and 'microwave' not in emoji]
    message = f'Hello :{random.choice(waves)}:\nA new quality gates threshold <{pr_url}/s|update PR> has been generated !\nPlease take a look, thanks !'
    client.chat_postMessage(channel='#agent-delivery-reviews', text=message)
