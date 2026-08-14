"""Shim to run a tool with @rules_go//go first in $PATH, from the current working directory like rules_go//go does.

A (thin) wrapper is unavoidable because prepending to the inherited PATH cannot be expressed declaratively.

References:
- https://github.com/bazel-contrib/bazel-lib/blob/main/lib/private/bats.bzl (is_windows)
- https://github.com/bazel-contrib/rules_go/blob/master/go/tools/go_bin_runner/main.go (BUILD_WORKING_DIRECTORY)
- https://github.com/bazel-contrib/rules_multitool/blob/main/multitool/private/run_in.bzl (template)
"""

load("@bazel_skylib//lib:paths.bzl", "paths")

def _go_shim_impl(ctx):
    go_dir = paths.dirname(ctx.executable._go.short_path)
    is_windows = ctx.target_platform_has_constraint(ctx.attr._windows_constraint[platform_common.ConstraintValueInfo])
    template = ctx.file._template_bat if is_windows else ctx.file._template_sh
    tool = ctx.executable.tool.short_path
    wrapper = ctx.actions.declare_file("{}.{}".format(ctx.label.name, template.extension))
    ctx.actions.expand_template(
        is_executable = True,
        output = wrapper,
        substitutions = {
            "{{go_dir}}": go_dir.replace("/", "\\") if is_windows else go_dir,
            "{{tool}}": tool.replace("/", "\\") if is_windows else tool,
        },
        template = template,
    )
    return [DefaultInfo(
        executable = wrapper,
        runfiles = ctx.runfiles([ctx.executable._go, ctx.executable.tool]).merge_all([
            ctx.attr._go[DefaultInfo].default_runfiles,
            ctx.attr.tool[DefaultInfo].default_runfiles,
        ]),
    )]

go_shim = rule(
    implementation = _go_shim_impl,
    executable = True,
    attrs = {
        "_go": attr.label(cfg = "target", default = "@rules_go//go", executable = True),
        "_template_bat": attr.label(allow_single_file = True, default = ":template.bat"),
        "_template_sh": attr.label(allow_single_file = True, default = ":template.sh"),
        "_windows_constraint": attr.label(default = "@platforms//os:windows"),
        "tool": attr.label(cfg = "exec", executable = True, mandatory = True),
    },
)
