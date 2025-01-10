import yaml
from invoke import task
import traceback

FAIL_CHAR = "❌"
SUCCESS_CHAR = "✅"


def generate_pr_comment(finalState, gateStates):
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
            gateStates.append({"name": gate, "state": True})
        except Exception:
            print(f"Gate {gate} failed with the following stack trace :")
            print(traceback.format_exc())
            finalState = False
            gateStates.append({"name": gate, "state": False})
    if not finalState:
        ctx.run("datadog-ci tag --level job --tags static_quality_gates:\"failed\"")
    else:
        ctx.run("datadog-ci tag --level job --tags static_quality_gates:\"passed\"")
