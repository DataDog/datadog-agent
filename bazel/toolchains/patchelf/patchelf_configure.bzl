"""Repository rule to autoconfigure a toolchain using the system patchelf."""

# NOTE: this must match the name used by register_toolchains in consuming
# MODULE.bazel files.  It seems like we should have a better interface that
# allows for this module name to be specified from a single point.
NAME = "patchelf_toolchains"

def _write_build(rctx, path, version):
    if not path:
        path = ""
    rctx.template(
        "BUILD",
        Label("@@//bazel/toolchains/patchelf:BUILD.tpl"),
        substitutions = {
            "{GENERATOR}": "@@//bazel/toolchains/patchelf/patchelf_configure.bzl%find_system_patchelf",
            "{PATCHELF_PATH}": str(path),
            "{PATCHELF_VERSION}": version,
        },
        executable = False,
    )

def _build_repo_for_patchelf_toolchain_impl(rctx):
    patchelf_path = rctx.which("patchelf")
    if rctx.attr.verbose:
        if patchelf_path:
            print("Found patchelf at '%s'" % patchelf_path)  # buildifier: disable=print
        else:
            print("No system patchelf found.")  # buildifier: disable=print

    version = "unknown"
    if patchelf_path:
        res = rctx.execute([patchelf_path, "--version"])
        if res.return_code == 0:
            # expect stdout like:  patchelf 0.18.0
            version = res.stdout.strip().split(" ")[1]

    _write_build(
        rctx = rctx,
        path = patchelf_path,
        version = version,
    )

build_repo_for_patchelf_toolchain = repository_rule(
    implementation = _build_repo_for_patchelf_toolchain_impl,
    doc = """Create a repository that defines an patchelf toolchain based on the system patchelf.""",
    local = True,
    environ = ["PATH"],
    attrs = {
        "verbose": attr.bool(
            doc = "If true, print status messages.",
        ),
    },
)

find_system_patchelf = module_extension(
    implementation = lambda ctx: build_repo_for_patchelf_toolchain(name = NAME),
)
