"""Packaging declaration for a single Agent binary.

This macro centralizes the standard treatment a binary gets on its way into
a distributable package.  For one binary it generates a three-target chain:

    <binary>
      -> <name>_rpath      rewrite_rpath; only when patch_rpath = True
      -> <name>_stripped   dd_strip_symbols: split into a stripped binary and
                           a debug-symbols file
      -> <name>            pkg_files installing the stripped binary

The public target is the pkg_files one, so package filegroups reference it
exactly as they did when these declarations were hand-written.  The
intermediates are generated plumbing with deterministic names;
`get_debug_symbols` finds the debug twin through the DdStripInfo provider on
the `<name>_stripped` target.

Binaries are linked unstripped (repo-wide `--strip=never`, see .bazelrc);
stripping happens exclusively in this chain.  Non-packaging consumers of the
same binary (tests, `bazel run`, dev tooling) therefore keep full debug info
and intact Go providers.

Transform order matters: rpath rewriting runs on the unstripped binary and
the strip/split runs last, so the debug-symbols file corresponds to the
binary that actually ships (build-id included).
"""

load("@rules_pkg//pkg:mappings.bzl", "pkg_attributes", "pkg_files")
load("//bazel/rules/dd_strip:dd_strip.bzl", "dd_strip_symbols")
load("//bazel/rules/rewrite_rpath:rewrite_rpath.bzl", "rewrite_rpath")

def agent_binary_for_package(
        name,
        target,
        prefix,
        patch_rpath = False,
        destination = None,
        renames = None,
        mode = "0755",
        use_relative_rpaths = False):
    """Packages one binary with the standard treatment chain.

    The chain is: rpath rewrite (opt-in) -> strip/symbol-split -> pkg_files.

    Args:
      name: name of the generated pkg_files target.  Also the base for the
          generated "<name>_rpath" and "<name>_stripped" intermediates.
      target: label of the binary to package (e.g. "//cmd/agent").
      prefix: install path of the binary, relative to the package root
          (passed to pkg_files).  May be a select().
      patch_rpath: whether to rewrite the binary's rpath for its install location.
      destination: absolute install path of the binary, used only for
          relative-rpath computation (passed to rewrite_rpath).  Defaults to
          "{install_dir}/<prefix>"; must be given explicitly when patch_rpath is
          True and prefix is a select().
      renames: pkg_files renames, for when the installed filename differs
          from the built one.  Keys must reference the generated
          ":<name>_stripped" target.
      mode: file mode of the installed binary.
      use_relative_rpaths: forwarded to rewrite_rpath; bool or select.  Only
          used when rpath = True.
    """
    input = target

    if patch_rpath:
        if destination == None:
            if type(prefix) != "string":
                fail(
                    "agent_binary_for_package '%s': prefix is a select(); " % name +
                    "pass an explicit destination when rpath is True",
                )
            destination = "{install_dir}/" + prefix
        rewrite_rpath(
            name = name + "_rpath",
            inputs = [input],
            destination = destination,
            use_relative_rpaths = use_relative_rpaths,
        )
        input = ":" + name + "_rpath"

    stripped_name = name + "_stripped"
    dd_strip_symbols(
        name = stripped_name,
        src = input,
    )

    pkg_files(
        name = name,
        srcs = [":" + stripped_name],
        attributes = pkg_attributes(mode = mode),
        prefix = prefix,
        renames = renames,
    )
