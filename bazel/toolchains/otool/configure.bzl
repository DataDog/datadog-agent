"""Repository rule to autoconfigure a toolchain using the system otool."""

load("//bazel/toolchains/common:defs.bzl", "make_repo_builder", "write_toolchain_repo")

# NOTE: this must match the name used by register_toolchains in consuming
# MODULE.bazel files.  It seems like we should have a better interface that
# allows for this module name to be specified from a single point.
NAME = "otool"

# TODO: We could templatize this by having a builder function that takes the
# version finding code as a lambda.  That's for a day when we have too much
# time on our hands.
def _build_repo_for_toolchain_impl(rctx):
    tool_name = rctx.original_name
    tool_path = rctx.which(tool_name)
    if rctx.attr.verbose:
        if tool_path:
            print("Found %s at '%s'" % (tool_name, tool_path))  # buildifier: disable=print
        else:
            print("No system %s found." % tool_name)  # buildifier: disable=print

    version = "unknown"
    if tool_path:
        res = rctx.execute([tool_path, "--version"])
        if res.return_code == 0:
            # expect stderr like:
            #   llvm-otool(1): Apple Inc. version cctools-1030.6.3
            #   otool(1): Apple Inc. version cctools-1030.6.3
            #   disassembler: LLVM version 17.0.0
            for word in res.stderr.strip().replace("\n", " ").split(" "):
                if word.startswith("cctools-"):
                    version = word
                    break

    write_toolchain_repo(
        rctx = rctx,
        tool_name = NAME,
        tool_path = tool_path,
        tool_version = version,
    )

build_repo_for_toolchain = make_repo_builder(name = NAME, impl = _build_repo_for_toolchain_impl)

find_system_otool = module_extension(
    implementation = lambda ctx: build_repo_for_toolchain(name = NAME),
)
