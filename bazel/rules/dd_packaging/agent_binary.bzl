"""Packaging declaration for a single Agent binary.

This macro centralizes the standard treatment a binary gets on its way into
a distributable package.  For one binary it generates a three-target chain:

    <binary>
      -> <name>_rpath      rewrite_rpath; only when patch_rpath = True
      -> <name>_stripped   dd_strip_symbols: split into a stripped binary and
                           a debug-symbols file
      -> <name>            pkg_files installing the stripped binary

The public target is the pkg_files one, so package filegroups reference it
exactly as they did when these declarations were hand-written. The
intermediates are generated plumbing with deterministic names, private to the
macro; `get_debug_symbols` finds the debug twin through the DdStripInfo
provider on the `<name>_stripped` target.

Binaries are linked unstripped (repo-wide `--strip=never`, see .bazelrc);
stripping happens exclusively in this chain. Non-packaging consumers of the
same binary (tests, `bazel run`, dev tooling) therefore keep full debug info
and intact Go providers.
"""

load("@rules_pkg//pkg:mappings.bzl", "pkg_attributes", "pkg_files")
load("//bazel/rules/dd_strip:dd_strip.bzl", "dd_strip_symbols")
load("//bazel/rules/rewrite_rpath:rewrite_rpath.bzl", "rewrite_rpath")

def _agent_binary_for_package_impl(
        name,
        target,
        prefix,
        patch_rpath = False,
        destination = None,
        use_relative_rpaths = False,
        target_compatible_with = None,
        visibility = None):
    """Packages one binary with the standard treatment chain.

    The chain is: rpath rewrite (opt-in) -> strip/symbol-split -> pkg_files.
    """
    input = target

    if patch_rpath:
        rewrite_rpath(
            name = name + "_rpath",
            inputs = [input],
            destination = destination,
            use_relative_rpaths = use_relative_rpaths,
            tags = ["manual"],
            target_compatible_with = target_compatible_with,
        )
        input = ":" + name + "_rpath"

    stripped_name = name + "_stripped"
    dd_strip_symbols(
        name = stripped_name,
        src = input,
        tags = ["manual"],
        target_compatible_with = target_compatible_with,
    )

    pkg_files(
        name = name,
        srcs = [":" + stripped_name],
        attributes = pkg_attributes(mode = "0755"),
        prefix = prefix,
        target_compatible_with = target_compatible_with,
        visibility = visibility,
    )

agent_binary_for_package = macro(
    doc = """Packages one binary with the standard treatment chain.

The chain is: rpath rewrite (opt-in) -> strip/symbol-split -> pkg_files.
""",
    attrs = {
        "target": attr.label(
            doc = """Label of the binary to package (e.g. "//cmd/agent").""",
            mandatory = True,
            configurable = False,
        ),
        "prefix": attr.string(
            doc = """Install path of the binary, relative to the package root (passed to pkg_files).""",
            mandatory = True,
        ),
        "patch_rpath": attr.bool(
            doc = "Whether to rewrite the binary's rpath for its install location.",
            default = False,
            configurable = False,
        ),
        "destination": attr.string(
            doc = """Absolute install path of the binary, used only for
            relative-rpath computation (passed to rewrite_rpath). Required
            when patch_rpath is True.""",
        ),
        "use_relative_rpaths": attr.bool(
            doc = """Forwarded to rewrite_rpath; only used when patch_rpath is True.""",
            default = False,
        ),
        "target_compatible_with": attr.label_list(
            doc = """Standard Bazel constraint, forwarded to every generated target.""",
        ),
    },
    implementation = _agent_binary_for_package_impl,
)
