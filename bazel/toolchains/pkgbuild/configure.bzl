"""Repository rule to autoconfigure a toolchain using the system pkgbuild."""

load("//bazel/toolchains/common:defs.bzl", "make_repo_builder", "write_toolchain_repo")

NAME = "pkgbuild"

build_repo_for_toolchain = make_repo_builder(name = NAME)

find_system_pkgbuild = module_extension(
    implementation = lambda ctx: build_repo_for_toolchain(name = NAME),
)
