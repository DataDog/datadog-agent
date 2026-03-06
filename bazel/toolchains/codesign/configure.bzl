"""Repository rule to autoconfigure a toolchain using the system codesign."""

load("//bazel/toolchains/common:defs.bzl", "make_repo_builder", "write_toolchain_repo")

# NOTE: this must match the name used by register_toolchains in consuming
# MODULE.bazel files.  It seems like we should have a better interface that
# allows for this module name to be specified from a single point.
NAME = "codesign"

def _build_repo_for_toolchain_impl(rctx):
    tool_name = rctx.original_name
    tool_path = rctx.which(tool_name)
    if rctx.attr.verbose:
        if tool_path:
            print("Found %s at '%s'" % (tool_name, tool_path))  # buildifier: disable=print
        else:
            print("No system %s found." % tool_name)  # buildifier: disable=print

    write_toolchain_repo(
        rctx = rctx,
        tool_name = NAME,
        tool_path = tool_path,
    )

build_repo_for_toolchain = make_repo_builder(name = NAME, impl = _build_repo_for_toolchain_impl)

find_system_codesign = module_extension(
    implementation = lambda ctx: build_repo_for_toolchain(name = NAME),
)
