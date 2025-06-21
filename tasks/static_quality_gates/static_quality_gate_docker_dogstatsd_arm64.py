from tasks.static_quality_gates.lib.docker_agent_lib import (
    generic_debug_docker_agent_quality_gate,
    generic_docker_agent_quality_gate,
)


def entrypoint(**kwargs):
    generic_docker_agent_quality_gate(
        gate_name="static_quality_gate_docker_dogstatsd_arm64", arch="arm64", flavor="dogstatsd", **kwargs
    )


def debug_entrypoint(**kwargs):
    generic_debug_docker_agent_quality_gate(arch="arm64", flavor="dogstatsd", **kwargs)
