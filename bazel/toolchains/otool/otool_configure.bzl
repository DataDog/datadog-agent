"""Repository rule to autoconfigure a toolchain using the system otool."""

# NOTE: this must match the name used by register_toolchains in consuming
# MODULE.bazel files.  It seems like we should have a better interface that
# allows for this module name to be specified from a single point.
NAME = "otool"

def _write_build(rctx, path, version):
    if not path:
        path = ""
    rctx.template(
        "BUILD",
        Label("@@//bazel/toolchains/otool:BUILD.tpl"),
        substitutions = {
            "{GENERATOR}": "@@//bazel/toolchains/otool/otool_configure.bzl%find_system_otool",
            "{OTOOL_PATH}": str(path),
            "{OTOOL_VERSION}": version,
        },
        executable = False,
    )

def _build_repo_for_otool_toolchain_impl(rctx):
    otool_path = rctx.which("otool")
    if rctx.attr.verbose:
        if otool_path:
            print("Found otool at '%s'" % otool_path)  # buildifier: disable=print
        else:
            print("No system otool found.")  # buildifier: disable=print

    version = "unknown"
    if otool_path:
        res = rctx.execute([otool_path, "--version"])
        if res.return_code == 0:
            # expect stderr like:
            #   llvm-otool(1): Apple Inc. version cctools-1030.6.3
            #   otool(1): Apple Inc. version cctools-1030.6.3
            #   disassembler: LLVM version 17.0.0
            for word in res.stderr.strip().replace("\n", " ").split(" "):
                if word.startswith("cctools-"):
                    version = word
                    break

    _write_build(
        rctx = rctx,
        path = otool_path,
        version = version,
    )

build_repo_for_otool_toolchain = repository_rule(
    implementation = _build_repo_for_otool_toolchain_impl,
    doc = """Create a repository that defines an otool toolchain based on the system otool.""",
    local = True,
    environ = ["PATH"],
    attrs = {
        "verbose": attr.bool(
            doc = "If true, print status messages.",
        ),
    },
)

find_system_otool = module_extension(
    implementation = lambda ctx: build_repo_for_otool_toolchain(name = NAME),
)
