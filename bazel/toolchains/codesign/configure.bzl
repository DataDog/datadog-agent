"""Repository rule to autoconfigure a toolchain using the system codesign."""

load("//bazel/toolchains/common:defs.bzl", "make_toolchain_repository_rule")

# This must match the repository name used by register_toolchains in consuming MODULE.bazel files.
NAME = "macos_codesign"

find_macos_codesign = make_toolchain_repository_rule(name = NAME, tool_name = "codesign")
