"""split_symbols — split a pkg_filegroup into a stripped and debug symbols pair.

Walks a pkg_filegroup looking for DdStripInfo instances.  When one is found
put the stripped variant in the default output and the debug symbols in
the debug output.
"""

load("@bazel_skylib//lib:paths.bzl", "paths")
load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load("@rules_pkg//pkg:providers.bzl", "PackageFilegroupInfo", "PackageFilesInfo")
load("//bazel/rules/dd_strip:dd_strip_info.bzl", "DdStripInfo")

_CollectedPackagingInfo = provider(
    doc = "Internal provider used to accumulate PackageFilegroupInfo instances.",
    fields = {
        "strip_info": "depset of DdStripInfo",
    },
)

# Aspects walked by dd_strip collector
# - dynamic_deps: cc_shared_library -> cc_shared_library edges
# - input: _dd_cc_packaged_rule -> cc_shared_library edges (bridges a
#   packaged target back to its underlying cc_shared_library)
# - shared_library: cc_import -> _dd_cc_packaged_rule edges (a cc_import
#   bridge pointed at an already-packaged shared library)
# - embed, deps: go_library/go_binary edges, so a walk can start from a
#   real Go binary
# - cdeps: go_library/go_binary -> cc_library edges (the cgo boundary)
# - data: cc_library -> _dd_cc_packaged_rule edges (a runtime, dlopen'd
#   dependency rather than a link-time one)
# - installed_files: inputs to _dd_cc_packaged_rule via dd_cc_packaged macro
_WALKED_ATTRS = ["srcs", "dynamic_deps", "input", "shared_library", "embed", "deps", "cdeps", "data", "installed_files"]

def _get_deps(ctx, attr_names):
    deps = []
    for attr_name in attr_names:
        val = getattr(ctx.rule.attr, attr_name, None)
        if val == None:
            continue
        if type(val) == "list":
            deps.extend(val)
        else:
            deps.append(val)
    return deps

def _collect_dd_strip_aspect_impl(target, ctx):
    strip_info_direct = [target[DdStripInfo]] if DdStripInfo in target else []

    deps = _get_deps(ctx, _WALKED_ATTRS)
    strip_info_transitive = [
        dep[_CollectedPackagingInfo].strip_info
        for dep in deps
        if _CollectedPackagingInfo in dep
    ]
    return [_CollectedPackagingInfo(
        strip_info = depset(direct = strip_info_direct, transitive = strip_info_transitive),
    )]

_collect_dd_strip_aspect = aspect(
    implementation = _collect_dd_strip_aspect_impl,
    doc = "Collects stripping information from the graph.",
    attr_aspects = _WALKED_ATTRS,
)

def _get_debug_symbols_impl(ctx):
    """Find the targets in the pacakge with DdStripInfo and build a pkg_filegroup of the debug parts.

    The algorithm is subtle:
    - all_strip_info <= The list of all DdStripInfo providers.
    - create a map of original targets to the the info
    - walk the overall package file group to find the effective destinations of originals.
    - use those destinations to determine where to place the debug symbols.
    """

    # Merge individual per-src depsets to deduplicate
    all_strip_info = depset(transitive = [
        src[_CollectedPackagingInfo].strip_info
        for src in ctx.attr.srcs
        if _CollectedPackagingInfo in src
    ]).to_list()

    originals_to_include = {strip_info.packaged_file: strip_info for strip_info in all_strip_info}

    # Go through the dest/source map and spot the originals we want.
    # Note that we can not have a single binary appear twice and map to corresponding distinct
    # locations for the debug symbols. That is an acceptable limitation.
    original_src_to_dest = {}
    for pkg_src in ctx.attr.srcs:
        if PackageFilegroupInfo in pkg_src:
            for pfi, _ in pkg_src[PackageFilegroupInfo].pkg_files:
                for dest, src in pfi.dest_src_map.items():
                    if src in originals_to_include:
                        original_src_to_dest[src] = dest

    # Build a single pkg_files for all the debug symbols
    prefix = paths.normalize(ctx.attr.prefix.format(install_dir = ctx.attr._install_dir[BuildSettingInfo].value))
    dest_src_map = {}
    for strip_info in all_strip_info:
        debug_file = strip_info.debug_file
        if strip_info.packaged_file in original_src_to_dest:
            original_folder = paths.dirname(original_src_to_dest[strip_info.packaged_file])

            # Notes:
            # - the lstrip guarantees the path is relative to avoid accidental path resets when joining paths
            # - using debug_file's basename guarantees that the name matches the reference stored in the stripped binary
            path = paths.join(prefix, original_folder.lstrip("/"), debug_file.basename)
        else:
            fail("Have debug symbol object with no original for", debug_file.short_path, "from", strip_info.original)
        dest_src_map[path] = debug_file

    pfi = PackageFilesInfo(
        dest_src_map = dest_src_map,
        attributes = {"mode": "0644"},
    )

    return [
        DefaultInfo(files = depset(dest_src_map.values())),
        PackageFilegroupInfo(pkg_files = [(pfi, ctx.label)], pkg_dirs = [], pkg_symlinks = []),
    ]

get_debug_symbols = rule(
    implementation = _get_debug_symbols_impl,
    doc = """
        Walks a pkg_filegroup for DdStripInfo providers and pulls the debug outputs
        into a new pkg_filegroup of just those.
    """,
    attrs = {
        "srcs": attr.label_list(
            doc = "Set of pkg_filegroup targets to examine",
            aspects = [_collect_dd_strip_aspect],
            providers = [[PackageFilegroupInfo]],
        ),
        "prefix": attr.string(
            doc = """
        Standard pkg_* rules prefix. Allows {install_dir} placeholder and
        defaults to {install_dir}/.debug/{install_dir}.
        """,
            default = "{install_dir}/.debug/{install_dir}",
        ),
        "_install_dir": attr.label(
            doc = "Private label used for interpolationg the {install_dir} placeholder",
            default = "@@//:install_dir",
        ),
    },
    provides = [PackageFilegroupInfo],
)
