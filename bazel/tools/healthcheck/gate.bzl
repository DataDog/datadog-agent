"""Gate a pkg_filegroup / pkg_files / pkg_symlinks target on the healthcheck.

Wraps a Package*Info-providing target, passing its providers through
unchanged, and attaches a validation action that runs the standalone
healthcheck against a manifest of the wrapped target's contents. Validation
actions run automatically whenever anything depending on the wrapped target
is built, and are deduplicated by Bazel like any other action.
"""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("@rules_pkg//pkg:providers.bzl", "PackageFilegroupInfo", "PackageFilesInfo", "PackageSymlinkInfo")
load("//bazel/rules/pkg_manifest:manifest.bzl", "write_manifest")

def _is_os(ctx, constraint):
    return ctx.target_platform_has_constraint(constraint[platform_common.ConstraintValueInfo])

def _healthcheck_gate_impl(ctx):
    is_linux = _is_os(ctx, ctx.attr._linux_constraint)
    is_macos = _is_os(ctx, ctx.attr._macos_constraint)
    if not is_linux and not is_macos:
        fail("{}: unsupported platform (Linux and macOS only)".format(ctx.label))
    os = "linux" if is_linux else "macos"

    result = write_manifest(ctx, ctx.attr.name, [ctx.attr.srcs])

    stamp = ctx.actions.declare_file(ctx.attr.name + ".stamp")
    arguments = [result.manifest.path, os, stamp.path]
    inputs = [result.manifest] + result.files
    if is_linux:
        install_prefix = ctx.attr._install_dir[BuildSettingInfo].value
        arguments += [install_prefix, ctx.executable._readelf.path]
        inputs.append(ctx.executable._readelf)

    ctx.actions.run(
        executable = ctx.executable._gate,
        arguments = arguments,
        inputs = depset(direct = inputs),
        outputs = [stamp],
        mnemonic = "HealthcheckGate",
        progress_message = "Running install-tree healthcheck for %{label}",
    )

    src = ctx.attr.srcs
    providers = [OutputGroupInfo(_validation = depset([stamp])), src[DefaultInfo]]
    if PackageFilegroupInfo in src:
        providers.append(src[PackageFilegroupInfo])
    if PackageFilesInfo in src:
        providers.append(src[PackageFilesInfo])
    if PackageSymlinkInfo in src:
        providers.append(src[PackageSymlinkInfo])
    return providers

healthcheck_gate = rule(
    implementation = _healthcheck_gate_impl,
    doc = """Wraps a pkg_filegroup / pkg_files / pkg_symlinks target so anything
    depending on it also runs the standalone healthcheck as a build-time
    validation action, once, without the check's output ending up in package
    content.""",
    attrs = {
        "srcs": attr.label(
            mandatory = True,
            providers = [[PackageFilegroupInfo], [PackageFilesInfo], [PackageSymlinkInfo]],
            doc = "The pkg_filegroup / pkg_files / pkg_symlinks target to check and pass through unchanged.",
        ),
        "_gate": attr.label(
            default = "//bazel/tools/healthcheck:gate",
            executable = True,
            cfg = "exec",
        ),
        "_install_dir": attr.label(
            doc = "The configured final install location (e.g. /opt/datadog-agent).",
            default = "@@//:install_dir",
        ),
        "_readelf": attr.label(
            default = "//bazel/tools:readelf",
            executable = True,
            cfg = "exec",
        ),
        "_linux_constraint": attr.label(
            default = "@platforms//os:linux",
        ),
        "_macos_constraint": attr.label(
            default = "@platforms//os:macos",
        ),
    },
)
