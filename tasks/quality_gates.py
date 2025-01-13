import traceback

import yaml
from invoke import task

from tasks.libs.common.color import color_message

FAIL_CHAR = "❌"
SUCCESS_CHAR = "✅"


def display_pr_comment(ctx, finalState, gateStates):
    body_head = f"""
    <details>
    <summary><h1>
    Static checks {SUCCESS_CHAR if finalState else FAIL_CHAR}
    </h1></summary>
    <ul dir:"auto">
    """
    body_gates = "\n".join(
        [f"<li>{gate['name']} {SUCCESS_CHAR if gate['state'] else FAIL_CHAR}</li>" for gate in gateStates]
    )
    body_feet = """
    </ul>
    </details>
    """
    return body_head + body_gates + body_feet
    # pr_commenter(ctx, title="Uncompressed package size comparison", body=body)


def _print_quality_gates_report(gateStates):
    print(color_message("======== Static Quality Gates Report ========", "magenta"))
    for gate in sorted(gateStates, key=lambda x: x["error_type"] is not None):
        if gate["error_type"] is None:
            print(color_message(f"Gate {gate['name']} succeeded ✅", "green"))
        elif gate["error_type"] == "AssertionError":
            print(
                color_message(
                    f"Gate {gate['name']} failed ❌ because of the following assertion failures :\n{gate['error_message']}",
                    "red",
                )
            )
        else:
            print(
                color_message(
                    f"Gate {gate['name']} failed ❌ with the following stack trace :\n{gate['error_message']}",
                    "red",
                )
            )


@task
def parse_and_trigger_gates(ctx, config_path="test/static/static_quality_gates.yml"):
    with open(config_path) as file:
        config = yaml.safe_load(file)

    gateList = list(config.keys())
    quality_gates_mod = __import__("tasks.static_quality_gates", fromlist=gateList)
    print(f"{config_path} correctly parsed !")
    print(f"The following gates are going to run:\n\t- {"\n\t- ".join(gateList)}")
    finalState = True
    gateStates = []
    for gate in gateList:
        gateInputs = config[gate]
        gateInputs["ctx"] = ctx
        try:
            getattr(quality_gates_mod, gate).entrypoint(**gateInputs)
            print(f"Gate {gate} succeeded !")
            gateStates.append({"name": gate, "state": True, "error_type": None, "error_message": None})
        except AssertionError as e:
            print(f"Gate {gate} failed ! (AssertionError)")
            finalState = False
            gateStates.append({"name": gate, "state": False, "error_type": "AssertionError", "error_message": str(e)})
        except Exception:
            print(f"Gate {gate} failed ! (StackTrace)")
            finalState = False
            gateStates.append(
                {"name": gate, "state": False, "error_type": "StackTrace", "error_message": traceback.format_exc()}
            )
    if not finalState:
        ctx.run("datadog-ci tag --level job --tags static_quality_gates:\"failed\"")
    else:
        ctx.run("datadog-ci tag --level job --tags static_quality_gates:\"passed\"")

    _print_quality_gates_report(gateStates)
