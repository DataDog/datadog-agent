from tasks.static_quality_gates.lib.package_agent_lib import generic_docker_agent_quality_gate


def entrypoint(**kwargs):
    generic_docker_agent_quality_gate(
        "static_quality_gate_iot_agent_deb_arm64", "arm64", "debian", "iot-agent", **kwargs
    )
