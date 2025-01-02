import traceback

import yaml
from invoke import task
from invoke.exceptions import Exit


@task
def parse_and_trigger_gates(ctx, config_path="test/static/static_quality_gates.yml"):
    with open(config_path) as file:
        config = yaml.safe_load(file)

    gateList = list(config.keys())
    quality_gates_mod = __import__("tasks.static_quality_gates", fromlist=gateList)
    print(f"{config_path} correctly parsed !")
    print(f"The following gates are going to run:\n\t- {"\n\t- ".join(gateList)}")
    finalState = True
    for gate in gateList:
        gateInputs = config[gate]
        gateInputs["ctx"] = ctx
        try:
            getattr(quality_gates_mod, gate).entrypoint(**gateInputs)
            print(f"Gate {gate} succeeded !")
        except Exception as e:
            print(traceback.format_exc())
            print(f"Gate {gate} failed with the following message :")
            print(e)
            finalState = False
    if not finalState:
        raise Exit(code=1)
