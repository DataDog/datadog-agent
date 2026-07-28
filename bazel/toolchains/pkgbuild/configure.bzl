"""Repository rule to autoconfigure a toolchain using the system pkgbuild."""

load("//bazel/toolchains/common:defs.bzl", "make_repo_builder")

# This must match the name used by register_toolchains in MODULE.bazel.
NAME = "macos_pkgbuild"

build_repo_for_toolchain = make_repo_builder(name = NAME)

find_macos_pkgbuild = module_extension(
    implementation = lambda ctx: build_repo_for_toolchain(name = NAME),
)
