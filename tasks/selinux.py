"""
SELinux namespaced tasks
"""

import os

from invoke import task

from tasks.libs.build.bazel import bazel

DEFAULT_SYSPROBE_POLICY_TE_FILE = os.path.join(
    os.path.dirname(os.path.realpath(__file__)), "..", "cmd", "agent", "selinux", "system_probe_policy.te"
)
DEFAULT_SYSPROBE_POLICY_OUTPUT_DIRECTORY = os.path.join(
    os.path.dirname(os.path.realpath(__file__)), "..", "cmd", "agent", "selinux"
)

CHECKMODULE = "@checkpolicy//:checkmodule"
SEMODULE_PACKAGE = "@semodule_utils//:semodule_package"
# The tools run here, so they must not follow a cross-compiling --platforms.
HOST_PLATFORM = "--platforms=@platforms//host"


def _bazel_tool(target: str) -> str:
    output = bazel("cquery", "--output=files", HOST_PLATFORM, target, capture_output=True).strip()
    lines = [line for line in output.splitlines() if line]
    if len(lines) != 1:
        raise SystemExit(f"Expected exactly one output file for {target}, got: {lines}")
    execroot = bazel("info", "execution_root", capture_output=True).strip()
    return os.path.join(execroot, lines[0])


@task
def compile_system_probe_policy_file(
    ctx, te_file=DEFAULT_SYSPROBE_POLICY_TE_FILE, output_directory=DEFAULT_SYSPROBE_POLICY_OUTPUT_DIRECTORY
):
    """
    Takes a SELinux .te policy file and compiles it into a .pp policy module.
    """

    # Get the filename without the .te extension
    policy_filename = os.path.splitext(te_file)[0]

    # Compute the .mod module name
    temp_module_name = ".".join([policy_filename, "mod"])

    # Compute the .pp packaged module name
    output_module_name = ".".join([os.path.join(output_directory, os.path.basename(policy_filename)), "pp"])

    bazel("build", HOST_PLATFORM, CHECKMODULE, SEMODULE_PACKAGE)
    checkmodule = _bazel_tool(CHECKMODULE)
    semodule_package = _bazel_tool(SEMODULE_PACKAGE)

    # Compile the module targeting modular policy version 19, the highest version
    # supported by libsepol on RHEL 7 (range 4-19). Newer distributions remain
    # backward-compatible, so a v19 module loads on all supported RHEL releases.
    ctx.run(f"{checkmodule} -M -m -c 19 -o {temp_module_name} {te_file}")
    ctx.run(f"{semodule_package} -o {output_module_name} -m {temp_module_name}")
