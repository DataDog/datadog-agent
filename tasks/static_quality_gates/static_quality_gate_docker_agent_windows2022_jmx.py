from tasks.static_quality_gates.lib.docker_agent_lib import generic_docker_agent_quality_gate


def entrypoint(**kwargs):
    generic_docker_agent_quality_gate(
        gate_name="static_quality_gate_docker_agent_windows2022_jmx",
        arch="amd64",
        jmx=True,
        image_suffix="-winltsc2022",
        **kwargs,
    )
