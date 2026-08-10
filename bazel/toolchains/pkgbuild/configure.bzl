"""Repository rule to autoconfigure a toolchain using the system pkgbuild."""

load("//bazel/toolchains/common:defs.bzl", "make_toolchain_repository_rule")

# This must match the name used by register_toolchains in MODULE.bazel.
NAME = "macos_pkgbuild"

find_macos_pkgbuild = make_toolchain_repository_rule(name = NAME, tool_name = "pkgbuild")
