"""Toolchain to provide a macOS install_name_tool-based rpath patcher."""

def _macos_rpath_rewriter_impl(ctx):
    rewriter_tool = ctx.actions.declare_file(ctx.label.name + "_rewriter_tool.sh")
    ctx.actions.write(
        output = rewriter_tool,
        is_executable = True,
        content = """#!/usr/bin/env bash
set -euo pipefail

INPUT=$1
RPATH=$2
OUTPUT=$3

"{script}" "{helper}" "{install_name_tool}" /usr/bin/otool "$RPATH" "$INPUT" "$OUTPUT"
""".format(
            helper = ctx.file._helper.path,
            install_name_tool = ctx.executable.install_name_tool.path,
            script = ctx.file._script.path,
        ),
    )

    return DefaultInfo(
        executable = rewriter_tool,
        files = depset([rewriter_tool, ctx.file._helper, ctx.file._script, ctx.executable.install_name_tool]),
    )

def _macos_tree_rpath_rewriter_impl(ctx):
    tree_rewriter_tool = ctx.actions.declare_file(ctx.label.name + "_tree_rewriter_tool.sh")
    ctx.actions.write(
        output = tree_rewriter_tool,
        is_executable = True,
        content = """#!/usr/bin/env bash
set -euo pipefail

INPUT=$1
RPATH=$2
OUTPUT=$3

"{tree_script}" "{helper}" "{file_script}" "{install_name_tool}" /usr/bin/otool "$RPATH" "$INPUT" "$OUTPUT"
""".format(
            file_script = ctx.file._file_script.path,
            helper = ctx.file._helper.path,
            install_name_tool = ctx.executable.install_name_tool.path,
            tree_script = ctx.file._tree_script.path,
        ),
    )

    return DefaultInfo(
        executable = tree_rewriter_tool,
        files = depset([
            tree_rewriter_tool,
            ctx.file._file_script,
            ctx.file._helper,
            ctx.file._tree_script,
            ctx.executable.install_name_tool,
        ]),
    )

macos_rpath_rewriter = rule(
    implementation = _macos_rpath_rewriter_impl,
    attrs = {
        "install_name_tool": attr.label(
            doc = "A valid label of a target pointing at an install_name_tool executable.",
            cfg = "exec",
            executable = True,
            allow_files = True,
            mandatory = True,
        ),
        "_script": attr.label(
            default = "//bazel/toolchains/rpath_rewriter:rewrite_with_install_name_tool.sh",
            allow_single_file = True,
        ),
        "_helper": attr.label(
            default = "//bazel/toolchains/rpath_rewriter:rpath_helpers.sh",
            allow_single_file = True,
        ),
    },
)

macos_tree_rpath_rewriter = rule(
    implementation = _macos_tree_rpath_rewriter_impl,
    attrs = {
        "install_name_tool": attr.label(
            doc = "A valid label of a target pointing at an install_name_tool executable.",
            cfg = "exec",
            executable = True,
            allow_files = True,
            mandatory = True,
        ),
        "_file_script": attr.label(
            default = "//bazel/toolchains/rpath_rewriter:rewrite_with_install_name_tool.sh",
            allow_single_file = True,
        ),
        "_tree_script": attr.label(
            default = "//bazel/toolchains/rpath_rewriter:rewrite_tree_with_install_name_tool.sh",
            allow_single_file = True,
        ),
        "_helper": attr.label(
            default = "//bazel/toolchains/rpath_rewriter:rpath_helpers.sh",
            allow_single_file = True,
        ),
    },
)
