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

def _patchelf_tree_rpath_rewriter_impl(ctx):
    rewriter_tool = ctx.actions.declare_file(ctx.label.name + "_tree_rewriter_tool.sh")
    ctx.actions.write(
        output = rewriter_tool,
        is_executable = True,
        content = r"""#!/usr/bin/env bash
set -euo pipefail

INPUT=$1
RPATH=$2
OUTPUT=$3

# /. copies the contents of input rather than nesting it under output
# (Bazel pre-creates output via declare_directory). chmod restores
# owner-write so patchelf can rewrite files installed as 0555.
cp -rL "$INPUT/." "$OUTPUT"
chmod -R u+w "$OUTPUT"
find "$OUTPUT" \
        -type f \( -name '*.so' -o -name '*.so.*' \) \
        -exec "{patchelf}" \
        --set-rpath "$RPATH" --force-rpath {{}} \;
""".format(patchelf = ctx.executable.patchelf.path),
    )

    return DefaultInfo(
        executable = rewriter_tool,
        files = depset([rewriter_tool, ctx.executable.patchelf]),
    )

patchelf_tree_rpath_rewriter = rule(
    implementation = _patchelf_tree_rpath_rewriter_impl,
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
