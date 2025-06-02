from tasks.static_quality_gates.lib.docker_agent_lib import generic_docker_agent_quality_gate


def entrypoint(**kwargs):
    generic_docker_agent_quality_gate(
        gate_name="static_quality_gate_docker_cws_instrumentation_arm64",
        arch="arm64",
        flavor="cws-instrumentation",
        **kwargs,
    )
