"""A module defining default toolchain info for the foreign_cc framework"""

def _toolchain_mapping(file, repo_name, exec_compatible_with = []):
    """Mapping of toolchain definition files to platform constraints

    Args:
        file (str): Toolchain definition file
        repo_name (str): name of repository to create for this toolchain
        exec_compatible_with (list): A list of compatible execution platform constraints.

    Returns:
        struct: A collection of toolchain data
    """
    return struct(
        file = file,
        repo_name = repo_name,
        exec_compatible_with = exec_compatible_with,
    )

# This list is the single entrypoint for all foreign_cc framework toolchains.
TOOLCHAIN_MAPPINGS = [
    _toolchain_mapping(
        repo_name = "rules_foreign_cc_framework_toolchain_linux",
        exec_compatible_with = [
            "@platforms//os:linux",
        ],
        file = "@rules_foreign_cc//foreign_cc/private/framework/toolchains:linux_commands.bzl",
    ),
    _toolchain_mapping(
        repo_name = "rules_foreign_cc_framework_toolchain_freebsd",
        exec_compatible_with = [
            "@platforms//os:freebsd",
        ],
        file = "@rules_foreign_cc//foreign_cc/private/framework/toolchains:freebsd_commands.bzl",
    ),
    _toolchain_mapping(
        repo_name = "rules_foreign_cc_framework_toolchain_windows",
        exec_compatible_with = [
            "@platforms//os:windows",
        ],
        file = "@rules_foreign_cc//foreign_cc/private/framework/toolchains:windows_commands.bzl",
    ),
    _toolchain_mapping(
        repo_name = "rules_foreign_cc_framework_toolchain_macos",
        exec_compatible_with = [
            "@platforms//os:macos",
        ],
        file = "@rules_foreign_cc//foreign_cc/private/framework/toolchains:macos_commands.bzl",
    ),
]
