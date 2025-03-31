# Defines a custom C++ MINGGW toolchain
#
# The point of this file is to show how we're defining a toolchain that triggers
# with //platforms:windows_platform. These match because they both specify
# @platforms//cpu:x86_64.

# This is C++-specific overhead required for all C++ toolchains. See
# toolchain_config.bzl for details.
load("//bazel/toolchains/config:toolchain_config.bzl", "glibc_cc_toolchain_config")

glibc_cc_toolchain_config(
    name = "glibc-cc-toolchain-config_x86",
    arch = "x86_64",
    gcc_version = "11.4.0",
    path = "/opt/toolchains/x86_64",
)

filegroup(
    name = "toolchain_files",
)

# Define our cc_toolchain
# (https://bazel.build/reference/be/c-cpp#cc_toolchain).
# The cc_toolchain rule is pre-defined by the C++ rule owners. It uses these
# parameters to construct a ToolchainInfo provider, as required by Bazel's
# platform/toolchain APIs.
cc_toolchain(
    name = "glibc_cc_toolchain_definition_x86",
    all_files = ":toolchain_files",
    compiler_files = ":toolchain_files",
    dwp_files = ":toolchain_files",
    linker_files = ":toolchain_files",
    objcopy_files = ":toolchain_files",
    strip_files = ":toolchain_files",
    toolchain_config = ":glibc-cc-toolchain-config_x86",
)

# Bazel's platform/toolchain APIs require this wrapper around the actual
# toolchain defined above. It serves two purposes: declare which
# constraint_values it supports (which can be matched to appropriate platforms)
# and tell Bazel what language this toolchain is for.
#
# So when you're building a cc_binary, Bazel has all the info it needs to give
# that cc_binary the right toolchain: it knows cc_binary requires a "C++-type
# toolchain" (this is encoded in the cc_binary rule definition) and needs to
# use a toolchain that matches whatever you set --platforms to at the command
# line.
toolchain(
    name = "glibc_cc_toolchain_x86",
    target_compatible_with = ["@platforms//cpu:x86_64"],
    toolchain = ":glibc_cc_toolchain_definition_x86",
    toolchain_type = "@bazel_tools//tools/cpp:toolchain_type",
)
