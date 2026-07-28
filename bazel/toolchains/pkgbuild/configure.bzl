"""Repository rule to autoconfigure a toolchain using the system pkgbuild."""

load("//bazel/toolchains/common:defs.bzl", "make_repo_builder")

# This must match the name used by register_toolchains in MODULE.bazel.
NAME = "macos_pkgbuild"

find_macos_pkgbuild = make_repo_builder(name = NAME, tool_name = "pkgbuild")
