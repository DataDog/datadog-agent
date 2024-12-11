"""
SELinux namespaced tasks
"""

import os

from invoke import task

DEFAULT_SYSPROBE_POLICY_TE_FILE = os.path.join(
    os.path.dirname(os.path.realpath(__file__)), "..", "cmd", "agent", "selinux", "system_probe_policy.te"
)
DEFAULT_SYSPROBE_POLICY_OUTPUT_DIRECTORY = os.path.join(
    os.path.dirname(os.path.realpath(__file__)), "..", "cmd", "agent", "selinux"
)


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

    # Compile the module
    command = f"checkmodule -M -m -o {temp_module_name} {te_file}"
    ctx.run(command)

    # Package the module
    command = f"semodule_package -o {output_module_name} -m {temp_module_name}"
    ctx.run(command)
