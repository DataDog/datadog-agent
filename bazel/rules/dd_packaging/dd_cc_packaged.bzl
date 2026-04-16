"""dd_cc_packaged — packaging-aware wrapper around cc_shared_library or cc_binary."""

load("@rules_cc//cc/common:cc_info.bzl", "CcInfo")
load("@rules_cc//cc/common:cc_shared_library_info.bzl", "CcSharedLibraryInfo")
load("@rules_pkg//pkg:mappings.bzl", "pkg_files")
load("@rules_pkg//pkg:providers.bzl", "PackageFilegroupInfo", "PackageFilesInfo")
load("//bazel/rules:so_symlink.bzl", "so_symlink")
load("//bazel/rules/dd_packaging:dd_packaging_info.bzl", "DdPackagingInfo")
load("//bazel/rules/rewrite_rpath:rewrite_rpath.bzl", "rewrite_rpath")

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
    ]
    if CcSharedLibraryInfo in ctx.attr.input:
        providers.append(ctx.attr.input[CcSharedLibraryInfo])
    return providers

_dd_cc_packaged_rule = rule(
    implementation = _dd_cc_packaged_rule_impl,
    doc = """
        Provides a convenience wrapper on top of a CcInfo or CcSharedLibraryInfo
        This wrapper can be used just like the providers it wraps, but also
        includes a list of files that should be installed alongside the wrapped object.
        It is meant to be provided to dd_collect_dependencies which will walk the tree
        to gather all installed files from all dependencies.
    """,
    attrs = {
        "input": attr.label(
            doc = "A CcInfo or CcSharedLibrary provider representing a shared library or an executable",
            mandatory = True,
            providers = [[CcInfo], [CcSharedLibraryInfo]],
        ),
        "installed_files": attr.label_list(
            doc = "A list of files that should be installed alongside the packaged dependency",
            providers = [[PackageFilesInfo], [PackageFilegroupInfo]],
        ),
    },
)

def _dd_cc_packaged_impl(name, input, version = "", installed_files = [], visibility = None, **kwargs):
    patched_name = "{}_patched".format(name)
    rewrite_rpath(
        name = patched_name,
        inputs = [input],
        package_metadata = [],
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
            package_metadata = [],
        )
    rule_installed_files.append(":{}".format(packaged_lib))
    _dd_cc_packaged_rule(
        name = name,
        input = input,
        installed_files = rule_installed_files,
        visibility = visibility,
        package_metadata = [],
        **kwargs
    )

dd_cc_packaged = macro(
    doc = """
    A macro used to prepare a cc_shared_library or cc_binary for packaging.

    If installed_files is provided, these files will be installed with the
    packaged object at installation time if the final artifact
    depends, directly or indirectly, on the wrapped binary.

    If a version is provided and the input is a cc_shared_library, the library
    will be installed along with the versioned symlink (see so_symlink).
    This will also handle the rpath rewriting at install time.

    The returned object transparently provides the input, which can be used as `dynamic_deps`
    if the input is a cc_shared_library.
    """,
    attrs = {
        "input": attr.label(
            mandatory = True,
            configurable = False,
            providers = [[CcInfo], [CcSharedLibraryInfo]],
        ),
        "installed_files": attr.label_list(
            configurable = False,
        ),
        "version": attr.string(
            default = "",
            configurable = False,
        ),
    },
    implementation = _dd_cc_packaged_impl,
)
