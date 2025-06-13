from tasks.static_quality_gates.lib.package_agent_lib import (
    generic_debug_package_agent_quality_gate,
    generic_package_agent_quality_gate,
)


def entrypoint(**kwargs):
    generic_package_agent_quality_gate(
        "static_quality_gate_dogstatsd_suse_amd64", "amd64", "suse", "datadog-dogstatsd", **kwargs
    )


def debug_entrypoint(**kwargs):
    generic_debug_package_agent_quality_gate(
        "amd64", "suse", "datadog-dogstatsd", build_job_name="dogstatsd_suse-x64", **kwargs
    )
