from tasks.static_quality_gates.lib.package_agent_lib import (
    generic_debug_package_agent_quality_gate,
    generic_package_agent_quality_gate,
)


def entrypoint(**kwargs):
    generic_package_agent_quality_gate(
        "static_quality_gate_agent_deb_arm64_fips", "arm64", "debian", "datadog-fips-agent", **kwargs
    )


def debug_entrypoint(**kwargs):
    generic_debug_package_agent_quality_gate(
        "arm64", "debian", "datadog-fips-agent", build_job_name="agent_deb-arm64-a7-fips", **kwargs
    )
