from tasks.static_quality_gates.lib.docker_agent_lib import (
    generic_debug_docker_agent_quality_gate,
    generic_docker_agent_quality_gate,
)


def entrypoint(**kwargs):
    generic_docker_agent_quality_gate(
        gate_name="static_quality_gate_docker_agent_windows2022_core",
        arch="amd64",
        image_suffix="-winltsc2022-servercore",
        **kwargs,
    )


def debug_entrypoint(**kwargs):
    generic_debug_docker_agent_quality_gate(
        arch="amd64",
        image_suffix="-winltsc2022-servercore",
        **kwargs,
    )
