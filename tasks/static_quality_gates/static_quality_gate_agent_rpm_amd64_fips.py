from tasks.static_quality_gates.lib.package_agent_lib import (
    generic_debug_package_agent_quality_gate,
    generic_package_agent_quality_gate,
)


def entrypoint(**kwargs):
    generic_package_agent_quality_gate(
        "static_quality_gate_agent_rpm_amd64_fips", "amd64", "centos", "datadog-fips-agent", **kwargs
    )


def debug_entrypoint(**kwargs):
    generic_debug_package_agent_quality_gate(
        "amd64", "centos", "datadog-fips-agent", build_job_name="agent_rpm-x64-a7-fips", **kwargs
    )
