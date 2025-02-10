from tasks.static_quality_gates.lib.package_agent_lib import generic_docker_agent_quality_gate


def entrypoint(**kwargs):
    generic_docker_agent_quality_gate(
        "static_quality_gate_agent_rpm_amd64", "amd64", "centos", "datadog-agent", **kwargs
    )
