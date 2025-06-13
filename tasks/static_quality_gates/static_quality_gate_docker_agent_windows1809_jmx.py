from tasks.static_quality_gates.lib.docker_agent_lib import (
    generic_debug_docker_agent_quality_gate,
    generic_docker_agent_quality_gate,
)


def entrypoint(**kwargs):
    generic_docker_agent_quality_gate(
        gate_name="static_quality_gate_docker_agent_windows1809_jmx",
        arch="amd64",
        jmx=True,
        image_suffix="-win1809",
        **kwargs,
    )


def debug_entrypoint(**kwargs):
    generic_debug_docker_agent_quality_gate(
        arch="amd64",
        jmx=True,
        image_suffix="-win1809",
        **kwargs,
    )
