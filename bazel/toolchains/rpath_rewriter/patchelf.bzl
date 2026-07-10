"""Toolchain to provide a hermetic patchelf-based rpath patcher."""

def _patchelf_rpath_rewriter_impl(ctx):
    rewriter_tool = ctx.actions.declare_file(ctx.label.name + "_rewriter_tool.sh")
    ctx.actions.write(
        output = rewriter_tool,
        is_executable = True,
        content = """#!/usr/bin/env bash
set -euo pipefail

INPUT=$1
RPATH=$2
OUTPUT=$3

"{patchelf}" --set-rpath "$RPATH" --force-rpath "$INPUT" --output "$OUTPUT"
""".format(patchelf = ctx.executable.patchelf.path),
    )

    return DefaultInfo(
        executable = rewriter_tool,
        files = depset([rewriter_tool, ctx.executable.patchelf]),
    )

patchelf_rpath_rewriter = rule(
    implementation = _patchelf_rpath_rewriter_impl,
    attrs = {
        "patchelf": attr.label(
            doc = "A valid label of a target pointing at a patchelf executable.",
            cfg = "exec",
            executable = True,
            allow_files = True,
            mandatory = True,
        ),
    },
)
