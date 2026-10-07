"""Wine's wmc behind the GNU windmc command line used by windows_resources.bzl."""

_SCRIPT = """#!/usr/bin/env bash
set -euo pipefail
wmc="$0.runfiles/{wmc}"
rc_dir=.
h_dir=.
while [[ $# -gt 1 ]]; do
    case "$1" in
        --target) shift 2 ;;
        -r) rc_dir="$2"; shift 2 ;;
        -h) h_dir="$2"; shift 2 ;;
        *) echo "windmc: unsupported argument: $1" >&2; exit 1 ;;
    esac
done
wmc="$(realpath "$wmc")"
input="$(realpath "$1")"
name="$(basename "$input" .mc)"
h_dir="$(realpath "$h_dir")"
cd "$rc_dir"
exec "$wmc" -H "$h_dir/$name.h" -o "$name.rc" "$input"
"""

def _runfiles_path(ctx, file):
    if file.short_path.startswith("../"):
        return file.short_path[len("../"):]
    return ctx.workspace_name + "/" + file.short_path

def _wine_windmc_impl(ctx):
    wmc = [f for f in ctx.files._wmc if f.basename == "wmc"][0]
    script = ctx.actions.declare_file(ctx.label.name + ".sh")
    ctx.actions.write(
        output = script,
        content = _SCRIPT.format(wmc = _runfiles_path(ctx, wmc)),
        is_executable = True,
    )
    return DefaultInfo(executable = script, runfiles = ctx.runfiles(files = ctx.files._wmc))

wine_windmc = rule(
    implementation = _wine_windmc_impl,
    doc = "Runs Wine's wmc with the GNU windmc arguments used by windows_resources.bzl.",
    executable = True,
    attrs = {
        "_wmc": attr.label(default = "@wine_linux_x86_64//:wmc", cfg = "exec"),
    },
)
