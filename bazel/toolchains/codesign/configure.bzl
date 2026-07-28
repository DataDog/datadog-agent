"""Repository rule to autoconfigure a toolchain using the system codesign."""

load("//bazel/toolchains/common:defs.bzl", "make_repo_builder")

# This must match the name used by register_toolchains in consuming MODULE.bazel files.
NAME = "macos_codesign"

find_macos_codesign = make_repo_builder(name = NAME, tool_name = "codesign")
