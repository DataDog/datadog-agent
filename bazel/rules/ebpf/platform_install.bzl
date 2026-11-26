"""pkg_install for a payload built for a given platform.

`bazel run --platforms=X <pkg_install>` also builds the installer for X, which
cannot run on a host of another architecture. Transitioning only the payload
keeps the installer on the host.
"""

load("@rules_pkg//pkg:install.bzl", "pkg_install")
load("@rules_pkg//pkg:providers.bzl", "PackageFilegroupInfo", "PackageFilesInfo")

_PLATFORMS = {
    "arm64": "//bazel/platforms:linux_arm64",
    "x86_64": "//bazel/platforms:linux_x86_64",
}

def _platform_transition_impl(_settings, attr):
    return {"//command_line_option:platforms": str(attr.platform)}

_platform_transition = transition(
    implementation = _platform_transition_impl,
    inputs = [],
    outputs = ["//command_line_option:platforms"],
)

def _platform_pkg_files_impl(ctx):
    return [
        PackageFilegroupInfo(
            pkg_files = [(src[PackageFilesInfo], src.label) for src in ctx.attr.srcs],
            pkg_dirs = [],
            pkg_symlinks = [],
        ),
        DefaultInfo(files = depset(transitive = [src[DefaultInfo].files for src in ctx.attr.srcs])),
    ]

_platform_pkg_files = rule(
    implementation = _platform_pkg_files_impl,
    attrs = {
        "srcs": attr.label_list(
            mandatory = True,
            providers = [PackageFilesInfo],
            cfg = _platform_transition,
            doc = "pkg_files targets, built for `platform`.",
        ),
        "platform": attr.label(
            mandatory = True,
            providers = [platform_common.PlatformInfo],
            doc = "Platform the srcs are built for.",
        ),
    },
    doc = "Re-exports pkg_files targets built for another platform.",
)

def ebpf_pkg_install(name, srcs, **kwargs):
    """Declares a pkg_install of `srcs`, plus `name`_<cpu> installs of it for each supported target CPU.

    Args:
      name: name of the native install, and prefix of the per-CPU ones.
      srcs: pkg_files targets to install.
      **kwargs: forwarded to pkg_install.
    """
    pkg_install(name = name, srcs = srcs, **kwargs)
    for cpu, platform in _PLATFORMS.items():
        _platform_pkg_files(
            name = "{}_{}_payload".format(name, cpu),
            srcs = srcs,
            platform = platform,
            tags = ["manual"],
            visibility = ["//visibility:private"],
        )
        pkg_install(
            name = "{}_{}".format(name, cpu),
            srcs = [":{}_{}_payload".format(name, cpu)],
            **kwargs
        )
