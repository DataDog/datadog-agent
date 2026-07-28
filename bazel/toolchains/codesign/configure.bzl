"""Repository rule to autoconfigure a toolchain using the system codesign."""

load("//bazel/toolchains/common:defs.bzl", "make_repo_builder")

# NOTE: this must match the name used by register_toolchains in consuming
# MODULE.bazel files.  It seems like we should have a better interface that
# allows for this module name to be specified from a single point.
NAME = "codesign"

build_repo_for_toolchain = make_repo_builder(name = NAME)

find_system_codesign = module_extension(
    implementation = lambda ctx: build_repo_for_toolchain(name = NAME),
)
