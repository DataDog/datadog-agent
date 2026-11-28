"""dd_rust_check: a Rust shared-library check and its packaging."""

load("@rules_pkg//pkg:mappings.bzl", "pkg_attributes")
load("@rules_rust//rust:defs.bzl", "rust_shared_library")
load("//bazel/rules/dd_packaging:agent_binary.bzl", "agent_binary_for_package")

_LINUX = ["@platforms//os:linux"]

def dd_rust_check(name, **kwargs):
    """Builds a Rust check cdylib and declares how it is packaged.

    The library is named libdatadog-agent-<name>.so, as the shared-library
    check loader expects. <name>_package installs it into checks.d with its
    rpath set to the Agent's embedded lib dir, stripped. Root + root-group
    read/exec, no "other": group-readable so init containers can copy them on
    OpenShift, where containers run with a random UID in the root group.

    Args:
      name: the check name.
      **kwargs: forwarded to rust_shared_library.
    """
    rust_shared_library(
        name = name,
        lib_name = "datadog-agent-" + name,
        target_compatible_with = _LINUX,
        **kwargs
    )

    agent_binary_for_package(
        name = name + "_package",
        attributes = pkg_attributes(mode = "0550"),
        patch_rpath = True,
        prefix = "checks.d",
        target = ":" + name,
        target_compatible_with = _LINUX,
        visibility = ["//pkg/collector/sharedlibrary/rustchecks:__pkg__"],
    )
