import os
import traceback

import yaml
from invoke import task

from tasks.github_tasks import pr_commenter
from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.color import bash_color_to_html, color_message
from tasks.static_quality_gates.lib.gates_lib import GateMetricHandler

FAIL_CHAR = "❌"
SUCCESS_CHAR = "✅"

body_info_pattern = """### Info

<details>
<summary>Full details</summary>

|Result|Quality gate|Details|Run|
|----|----|----|----|
"""
body_error_pattern = """### Error

<details>
<summary>Full details</summary>

|Result|Quality gate|Details|Run|
|----|----|----|----|
"""


def format_gate_message(message):
    return bash_color_to_html(message).replace("\n", "<br>")


def display_pr_comment(ctx, finalState, gateStates):
    title = f"Static quality checks {SUCCESS_CHAR if finalState else FAIL_CHAR}"
    body_info = body_info_pattern
    body_error = body_error_pattern
    withError = False
    withInfo = False
    for gate in sorted(gateStates, key=lambda x: x["error_type"] is None):
        if gate["error_type"] is None:
            body_info += f"|✅|{gate['name']}|{format_gate_message(gate['message'])}| WIP |\n"
            withInfo = True
        else:
            body_error += f"|❌|{gate['name']}|{format_gate_message(gate['message'])}| WIP |\n"
            withError = True
    body_info += "\n</details>\n"
    body_error += "\n</details>\n"

    body = f"Please find below the results from static quality gates\n{body_error if withError else ""}\n\n{body_info if withInfo else ""}"

    pr_commenter(ctx, title=title, body=body)


def _print_quality_gates_report(gateStates):
    print(color_message("======== Static Quality Gates Report ========", "magenta"))
    for gate in sorted(gateStates, key=lambda x: x["error_type"] is not None):
        if gate["error_type"] is None:
            print(color_message(f"Gate {gate['name']} succeeded ✅", "blue"))
        elif gate["error_type"] == "AssertionError":
            print(
                color_message(
                    f"Gate {gate['name']} failed ❌ because of the following assertion failures :\n{gate['message']}",
                    "orange",
                )
            )
        else:
            print(
                color_message(
                    f"Gate {gate['name']} failed ❌ with the following stack trace :\n{gate['message']}",
                    "orange",
                )
            )


@task
def parse_and_trigger_gates(ctx, config_path="test/static/static_quality_gates.yml"):
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
        ctx.run("datadog-ci tag --level job --tags static_quality_gates:\"failed\"")
    else:
        ctx.run("datadog-ci tag --level job --tags static_quality_gates:\"passed\"")

    _print_quality_gates_report(gateStates)

    metricHandler.send_metrics()

    github = GithubAPI()
    branch = os.environ["CI_COMMIT_BRANCH"]
    if github.get_pr_for_branch(branch).totalCount > 0:
        display_pr_comment(ctx, finalState, gateStates)
