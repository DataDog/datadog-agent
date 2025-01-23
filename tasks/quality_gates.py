import os
import traceback
import typing

import yaml
from invoke import task

from tasks.github_tasks import pr_commenter
from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.color import bash_color_to_html, color_message
from tasks.static_quality_gates.lib.gates_lib import GateMetricHandler

FAIL_CHAR = "❌"
SUCCESS_CHAR = "✅"

body_info_pattern = """### Info

|Result|Quality gate|On disk size|Maximum on disk size|On wire size|Maximum on wire size|
|----|----|----|----|----|----|
"""
body_error_pattern = """### Error

|Result|Quality gate|On disk size|Maximum on disk size|On wire size|Maximum on wire size|
|----|----|----|----|----|----|
"""

body_error_feet_pattern = """<details>
<summary>Gate failure full details</summary>

|Quality gate|Error type|Error message|
|----|---|--------|
"""


def format_gate_message(message: str):
    """
    Format a quality gate message from bash colors to html to show in PRs
    :param message: message with bash colors
    :return: formated message
    """
    return bash_color_to_html(message).replace("\n", "<br>")


def display_pr_comment(
    ctx, finalState: bool, gateStates: list[dict[str, typing.Any]], metricHandler: GateMetricHandler
):
    """
    Display a comment on a PR with results from our static quality gates checks
    :param ctx: Invoke task context
    :param finalState: Boolean that represents the overall state of quality gates checks
    :param gateStates: State of each quality gate
    :param metricHandler: Precise metrics of each quality gate
    :return:
    """
    title = f"Static quality checks {SUCCESS_CHAR if finalState else FAIL_CHAR}"
    body_info = body_info_pattern
    body_error = body_error_pattern
    body_error_feet = body_error_feet_pattern

    withError = False
    withInfo = False
    # Sort gates by error_types to group in between NoError, AssertionError and StackTrace
    for gate in sorted(gateStates, key=lambda x: x["error_type"] is None):

        def getMetric(metric_name, gate_name=gate['name']):
            try:
                return metricHandler.get_formated_metric(gate_name, metric_name)
            except KeyError:
                return "DataNotFound"

        if gate["error_type"] is None:
            body_info += f"|{SUCCESS_CHAR}|{gate['name']}|{getMetric('current_on_disk_size')}|{getMetric('max_on_disk_size')}|{getMetric('current_on_wire_size')}|{getMetric('max_on_wire_size')}|\n"
            withInfo = True
        else:
            body_error += f"|{FAIL_CHAR}|{gate['name']}|{getMetric('current_on_disk_size')}|{getMetric('max_on_disk_size')}|{getMetric('current_on_wire_size')}|{getMetric('max_on_wire_size')}|\n"
            body_error_feet += f"|{gate['name']}|{gate['error_type']}|{format_gate_message(gate['message'])}|\n"
            withError = True

    body_error_feet += "\n</details>\n"
    body = f"Please find below the results from static quality gates\n{body_error+body_error_feet if withError else ""}\n\n{body_info if withInfo else ""}"

    pr_commenter(ctx, title=title, body=body)


def _print_quality_gates_report(gateStates: list[dict[str, typing.Any]]):
    print(color_message("======== Static Quality Gates Report ========", "magenta"))
    for gate in sorted(gateStates, key=lambda x: x["error_type"] is not None):
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

    gateList = list(config.keys())
    quality_gates_mod = __import__("tasks.static_quality_gates", fromlist=gateList)
    print(f"{config_path} correctly parsed !")
    metricHandler = GateMetricHandler(
        git_ref=os.environ["CI_COMMIT_REF_SLUG"], bucket_branch=os.environ["BUCKET_BRANCH"]
    )

    print(f"The following gates are going to run:\n\t- {"\n\t- ".join(gateList)}")
    finalState = True
    gateStates = []
    for gate in gateList:
        gateInputs = config[gate]
        gateInputs["ctx"] = ctx
        gateInputs["metricHandler"] = metricHandler
        try:
            gate_mod = getattr(quality_gates_mod, gate)
            gate_mod.entrypoint(**gateInputs)
            print(f"Gate {gate} succeeded !")
            gateStates.append({"name": gate, "state": True, "error_type": None, "message": None})
        except AssertionError as e:
            print(f"Gate {gate} failed ! (AssertionError)")
            finalState = False
            gateStates.append({"name": gate, "state": False, "error_type": "AssertionError", "message": str(e)})
        except Exception:
            print(f"Gate {gate} failed ! (StackTrace)")
            finalState = False
            gateStates.append(
                {"name": gate, "state": False, "error_type": "StackTrace", "message": traceback.format_exc()}
            )
    if not finalState:
        ctx.run("datadog-ci tag --level job --tags static_quality_gates:\"failure\"")
    else:
        ctx.run("datadog-ci tag --level job --tags static_quality_gates:\"success\"")

    _print_quality_gates_report(gateStates)

    metricHandler.send_metrics_to_datadog()

    github = GithubAPI()
    branch = os.environ["CI_COMMIT_BRANCH"]
    if github.get_pr_for_branch(branch).totalCount > 0:
        display_pr_comment(ctx, finalState, gateStates, metricHandler)
