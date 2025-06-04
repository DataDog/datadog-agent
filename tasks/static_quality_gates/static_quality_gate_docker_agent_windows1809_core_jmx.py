from tasks.static_quality_gates.lib.docker_agent_lib import generic_docker_agent_quality_gate


def entrypoint(**kwargs):
    generic_docker_agent_quality_gate(
        gate_name="static_quality_gate_docker_agent_windows1809_core_jmx",
        arch="amd64",
        jmx=True,
        image_suffix="-win1809-servercore",
        **kwargs,
    )
