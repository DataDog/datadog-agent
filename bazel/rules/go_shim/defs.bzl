"""Shim to run a tool with the hermetic Go SDK's `go` first in $PATH, from the current working directory.

A (thin) wrapper is unavoidable because prepending to the inherited PATH cannot be expressed declaratively.
The SDK's `go` is used over @rules_go//go, because the latter changes the current directory again, breaking subcommands.

References:
- https://github.com/bazel-contrib/bazel-lib/blob/main/lib/private/bats.bzl (is_windows)
- https://github.com/bazel-contrib/rules_multitool/blob/main/multitool/private/run_in.bzl (template)
"""

load("@bazel_skylib//lib:paths.bzl", "paths")

_GO_TOOLCHAIN = "@rules_go//go:toolchain"

def _impl(ctx):
    sdk = ctx.toolchains[_GO_TOOLCHAIN].sdk
    go_dir = paths.dirname(sdk.go.short_path)
    goroot = paths.dirname(sdk.root_file.short_path)
    is_windows = ctx.target_platform_has_constraint(ctx.attr._windows_constraint[platform_common.ConstraintValueInfo])
    template = ctx.file._template_bat if is_windows else ctx.file._template_sh
    tool = ctx.executable.tool.short_path
    wrapper = ctx.actions.declare_file("{}.{}".format(ctx.label.name, template.extension))
    ctx.actions.expand_template(
        is_executable = True,
        output = wrapper,
        substitutions = {
            "{{go_dir}}": go_dir.replace("/", "\\") if is_windows else go_dir,
            "{{goroot}}": goroot.replace("/", "\\") if is_windows else goroot,
            "{{tool}}": tool.replace("/", "\\") if is_windows else tool,
        },
        template = template,
    )
    return [DefaultInfo(
        executable = wrapper,
        runfiles = ctx.runfiles(
            [sdk.go],
            transitive_files = depset(transitive = [sdk.headers, sdk.libs, sdk.srcs, sdk.tools]),
        ).merge(ctx.attr.tool[DefaultInfo].default_runfiles),
    )]

go_shim = rule(
    implementation = _impl,
    attrs = {
        "_template_bat": attr.label(allow_single_file = True, default = ":template.bat"),
        "_template_sh": attr.label(allow_single_file = True, default = ":template.sh"),
        "_windows_constraint": attr.label(default = "@platforms//os:windows"),
        "tool": attr.label(cfg = "exec", executable = True, mandatory = True),
    },
    executable = True,
    toolchains = [_GO_TOOLCHAIN],
)
