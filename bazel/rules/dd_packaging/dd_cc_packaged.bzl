"""dd_cc_packaged — packaging-aware wrapper around cc_shared_library or cc_binary."""

load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("@rules_cc//cc/common:cc_info.bzl", "CcInfo")
load("@rules_cc//cc/common:cc_shared_library_info.bzl", "CcSharedLibraryInfo")
load("@rules_pkg//pkg:mappings.bzl", "pkg_files")
load("@rules_pkg//pkg:providers.bzl", "PackageFilegroupInfo", "PackageFilesInfo")
load("//bazel/rules:so_symlink.bzl", "so_symlink")
load("//bazel/rules/dd_packaging:dd_packaging_info.bzl", "DdPackagingInfo")
load("//bazel/rules/rewrite_rpath:rewrite_rpath.bzl", "otool_dir_action", "otool_file_action", "patchelf_dir_action", "patchelf_file_action", "rewrite_rpath")

def _is_os(ctx, constraint):
    return ctx.target_platform_has_constraint(constraint[platform_common.ConstraintValueInfo])

def _dd_packaged_files_impl(ctx):
    is_linux = _is_os(ctx, ctx.attr._linux_constraint)
    is_macos = _is_os(ctx, ctx.attr._macos_constraint)

    rpath = ctx.attr.rpath.format(install_dir = ctx.attr._install_dir[BuildSettingInfo].value)
    dest_src_map = {}

    for src, prefix in ctx.attr.srcs.items():
        for f in src.files.to_list():
            if f.is_directory:
                if is_linux:
                    out = ctx.actions.declare_directory("patched_dirs/" + f.basename)
                    patchelf_dir_action(ctx, f, out, rpath)
                elif is_macos:
                    out = ctx.actions.declare_directory("patched_dirs/" + f.basename)
                    otool_dir_action(ctx, f, out, rpath)
                else:
                    out = f
                dest = prefix
            else:
                if is_linux:
                    out = ctx.actions.declare_file("patched/" + f.basename)
                    patchelf_file_action(ctx, f, out, rpath)
                elif is_macos:
                    out = ctx.actions.declare_file("patched/" + f.basename)
                    otool_file_action(ctx, f, out, rpath)
                else:
                    out = f
                dest = (prefix + "/" + f.basename) if prefix else f.basename
            dest_src_map[dest] = out

    return [PackageFilesInfo(
        dest_src_map = dest_src_map,
        attributes = {"mode": "0755"},
    )]

_dd_packaged_files_rule = rule(
    implementation = _dd_packaged_files_impl,
    attrs = {
        "srcs": attr.label_keyed_string_dict(
            doc = "Dict mapping file/directory labels to their installation prefix",
            allow_files = True,
        ),
        "rpath": attr.string(
            default = "{install_dir}/embedded/lib",
        ),
        "_linux_constraint": attr.label(default = "@platforms//os:linux"),
        "_macos_constraint": attr.label(default = "@platforms//os:macos"),
        "_script": attr.label(
            default = "@@//bazel/rules/rewrite_rpath:macos.sh",
            allow_single_file = True,
            cfg = "exec",
        ),
        "_install_dir": attr.label(default = "@@//:install_dir"),
    },
    toolchains = [
        "@@//bazel/toolchains/patchelf:patchelf_toolchain_type",
        "@@//bazel/toolchains/otool:otool_toolchain_type",
    ],
)

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

def _dd_cc_packaged_impl(name, input, version = "", installed_files = [], installed_executables = {}, libname = "", visibility = None, **kwargs):
    patched_name = "{}_patched".format(name)
    rewrite_rpath(
        name = patched_name,
        inputs = [input],
        package_metadata = [],
    )
    extra_files = []
    if installed_executables:
        exec_files_name = "{}_exec_files".format(name)
        _dd_packaged_files_rule(
            name = exec_files_name,
            srcs = installed_executables,
            package_metadata = [],
            visibility = visibility,
        )
        extra_files.append(":{}".format(exec_files_name))
    packaged_lib = "{}_packaged".format(name)
    resolved_libname = libname if libname else "lib" + input.name
    if version:
        so_symlink(
            name = packaged_lib,
            src = ":{}".format(patched_name),
            libname = resolved_libname,
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
    extra_files.append(":{}".format(packaged_lib))
    _dd_cc_packaged_rule(
        name = name,
        input = input,
        installed_files = extra_files + installed_files,
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

    If installed_executables is provided, each entry is rpath-patched and
    installed with mode 0755. Individual files are installed as prefix/basename;
    directory artifacts are installed as the prefix itself (contents copied into it).
    Use this instead of wrapping files in pkg_files and passing them via
    installed_files, so that rpath rewriting is not skipped.

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
            configurable = True,
        ),
        "installed_executables": attr.label_keyed_string_dict(
            doc = "Dict mapping labels to installation prefixes. Each entry is rpath-patched and installed with mode 0755.",
            configurable = True,
        ),
        "version": attr.string(
            default = "",
            configurable = False,
        ),
        "libname": attr.string(
            default = "",
            configurable = False,
        ),
    },
    implementation = _dd_cc_packaged_impl,
)
