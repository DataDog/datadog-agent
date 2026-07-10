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

INSTALL_NAME_TOOL=\"{install_name_tool}\"

# RPATH is the full rpath (e.g. /opt/datadog-agent/embedded/lib); use as-is.
cp \"$INPUT\" \"$OUTPUT\"
# Restore owner-write so install_name_tool can modify dylibs installed as
# read-only by their build system (e.g. Python lib-dynload modules).
chmod u+w \"$OUTPUT\"

# Match Linux patchelf --set-rpath semantics: the packaged output should have
# exactly the packaging rpath, to avoid leaving in existing ones that may
# point to build-time paths.
\"$INSTALL_NAME_TOOL\" -delete_all_rpaths -add_rpath \"$RPATH\" \"$OUTPUT\"
dylib_name=$(basename \"$OUTPUT\")
new_id=\"$RPATH/$dylib_name\"

\"$INSTALL_NAME_TOOL\" -id \"$new_id\" \"$OUTPUT\"

# Dylibs built in the Bazel sandbox record their dependencies with absolute
# sandbox paths as install names (e.g. bazel-out/.../libfoo.dylib). Those paths
# vanish after the build, so rewrite them to $RPATH/<basename> so the dynamic
# linker can find them via the rpath we just added. Leave everything else
# (system libraries, @rpath/... references) untouched.
/usr/bin/otool -L \"$OUTPUT\" | tail -n +2 | awk '{{print $1}}' | while read -r dep; do
    if [[ \"$dep\" == *\"sandbox\"* ]] || [[ \"$dep\" == *\"bazel-out\"* ]]; then
        dep_name=$(basename \"$dep\")
        new_dep=\"$RPATH/$dep_name\"
        \"$INSTALL_NAME_TOOL\" -change \"$dep\" \"$new_dep\" \"$OUTPUT\" 2>/dev/null || true
    fi
done

# Re-sign with an ad-hoc signature after modification as install_name_tool invalidates
# any existing code signature.
/usr/bin/codesign --sign - --force \"$OUTPUT\"
""".format(
            install_name_tool = ctx.executable.install_name_tool.path,
        ),
    )

    return DefaultInfo(
        executable = rewriter_tool,
        files = depset([rewriter_tool, ctx.executable.install_name_tool]),
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
    },
)
