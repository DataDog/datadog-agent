import os
import traceback
import typing

import yaml
from invoke import task

from tasks.github_tasks import pr_commenter
from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.color import color_message
from tasks.static_quality_gates.lib.gates_lib import GateMetricHandler

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
    body_info = body_pattern.format("Info")
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
            body_error_footer += f"|{gate['name']}|{gate['error_type']}|{gate['message'].replace('\n', '<br>')}|\n"
            with_error = True

    body_error_footer += "\n</details>\n"
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

    print(f"The following gates are going to run:\n\t- {"\n\t- ".join(gate_list)}")
    final_state = "success"
    gate_states = []
    for gate in gate_list:
        gate_inputs = config[gate]
        gate_inputs["ctx"] = ctx
        gate_inputs["metricHandler"] = metric_handler
        try:
            gate_mod = getattr(quality_gates_mod, gate)
            gate_mod.entrypoint(**gate_inputs)
            print(f"Gate {gate} succeeded !")
            gate_states.append({"name": gate, "state": True, "error_type": None, "message": None})
        except AssertionError as e:
            print(f"Gate {gate} failed ! (AssertionError)")
            final_state = "failure"
            gate_states.append({"name": gate, "state": False, "error_type": "AssertionError", "message": str(e)})
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
    branch = os.environ["CI_COMMIT_BRANCH"]
    if github.get_pr_for_branch(branch).totalCount > 0:
        display_pr_comment(ctx, final_state == "success", gate_states, metric_handler)
