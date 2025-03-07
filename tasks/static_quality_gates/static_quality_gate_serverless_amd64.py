from tasks.static_quality_gates.lib.package_agent_lib import generic_package_agent_quality_gate


# TODO
def entrypoint(**kwargs):
    generic_package_agent_quality_gate(
        "static_quality_gate_serverless", "amd64", "serverless", "serverless", **kwargs
    )
