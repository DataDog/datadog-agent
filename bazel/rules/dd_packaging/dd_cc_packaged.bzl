"""dd_cc_packaged — packaging-aware wrapper around cc_shared_library or cc_binary."""

load("@rules_cc//cc/common:cc_info.bzl", "CcInfo")
load("@rules_cc//cc/common:cc_shared_library_info.bzl", "CcSharedLibraryInfo")
load("@rules_pkg//pkg:mappings.bzl", "pkg_files")
load("@rules_pkg//pkg:providers.bzl", "PackageFilegroupInfo", "PackageFilesInfo")
load("//bazel/rules/dd_packaging:dd_packaging_info.bzl", "DdPackagingInfo")
load("//bazel/rules/rewrite_rpath:rewrite_rpath.bzl", "rewrite_rpath")
load("//bazel/rules:so_symlink.bzl", "so_symlink")

def _dd_cc_packaged_rule_impl(ctx):
    installed = []
    for dep in ctx.attr.installed_files:
        if PackageFilegroupInfo in dep:
            installed.append(dep[PackageFilegroupInfo])
        elif PackageFilesInfo in dep:
            installed.append(PackageFilegroupInfo(
                pkg_files = [(dep[PackageFilesInfo], dep.label)],
                pkg_dirs = [],
                pkg_symlinks = [],
            ))
    providers = [
        DdPackagingInfo(installed_files = installed),
        DefaultInfo(files = depset([ctx.file.patched])),
    ]
    if CcSharedLibraryInfo in ctx.attr.input:
        providers.append(ctx.attr.input[CcSharedLibraryInfo])
    return providers

_dd_cc_packaged_rule = rule(
    implementation = _dd_cc_packaged_rule_impl,
    attrs = {
        "input": attr.label(
            mandatory = True,
            providers = [[CcInfo], [CcSharedLibraryInfo]],
        ),
        "patched": attr.label(
            mandatory = True,
            allow_single_file = True,
        ),
        "installed_files": attr.label_list(providers = [[PackageFilesInfo], [PackageFilegroupInfo]]),
    },
)

def _dd_cc_packaged_impl(name, input, version = "", installed_files = [], visibility = None, **kwargs):
    patched_name = "{}_patched".format(name)
    rewrite_rpath(
        name = patched_name,
        inputs = [input],
    )
    rule_installed_files = list(installed_files)
    packaged_lib = "{}_packaged".format(name)
    if version:
        so_symlink(
            name = packaged_lib,
            src = ":{}".format(patched_name),
            libname = "lib" + input.name,
            version = version,
            visibility = visibility,
        )
    else:
        pkg_files(
            name = packaged_lib,
            srcs = [":{}".format(patched_name)],
            prefix = "lib",
            visibility = visibility,
        )
    rule_installed_files.append(":{}".format(packaged_lib))
    _dd_cc_packaged_rule(
        name = name,
        input = input,
        patched = ":{}".format(patched_name),
        installed_files = rule_installed_files,
        visibility = visibility,
        **kwargs,
    )

dd_cc_packaged = macro(
    attrs = {
        "input": attr.label(
            mandatory = True,
            configurable = False,
        ),
        "version": attr.string(
            default = "",
            configurable = False,
        ),
        "installed_files": attr.label_list(
            configurable = False,
        ),
    },
    implementation = _dd_cc_packaged_impl,
)
