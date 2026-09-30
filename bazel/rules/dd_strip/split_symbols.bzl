"""split_symbols — split a pkg_filegroup into a stripped and debug symbols pair.

Walks a pkg_filegroup looking for DdStripInfo instances.  When one is found
put the stripped variant in the default output and the debug symbols in
the debug output.
"""

load("@rules_pkg//pkg:providers.bzl", "PackageFilegroupInfo", "PackageFilesInfo")
load("//bazel/rules/dd_strip:dd_strip_info.bzl", "DdStripInfo")

_CollectedPackagingInfo = provider(
    doc = "Internal provider used to accumulate PackageFilegroupInfo instances.",
    fields = {
        "pkg_filegroups": "depset of PackageFilegroupInfo accumulated transitively",
        "strip_info": "depset of DdStripInfo",
    },
)

def _collect_dd_strip_aspect_impl(target, ctx):
    direct = target[_CollectedPackagingInfo] if _CollectedPackagingInfo in target else []
    transitive = [
        dep[_CollectedPackagingInfo].pkg_filegroups
        for dep in getattr(ctx.rule.attr, "srcs", [])
        if _CollectedPackagingInfo in dep
    ]
    strip_info_direct = [target[DdStripInfo]] if DdStripInfo in target else []
    strip_info_transitive = [
        dep[_CollectedPackagingInfo].strip_info
        for dep in getattr(ctx.rule.attr, "srcs", [])
        if _CollectedPackagingInfo in dep
    ]
    return [_CollectedPackagingInfo(
        pkg_filegroups = depset(direct, transitive = transitive),
        strip_info = depset(direct = strip_info_direct, transitive = strip_info_transitive),
    )]

_collect_dd_strip_aspect = aspect(
    implementation = _collect_dd_strip_aspect_impl,
    doc = """
        Traverses these edge types to walk the full CC dependency graph:
        - dynamic_deps: cc_shared_library -> cc_shared_library edges
        - input: _dd_cc_packaged_rule -> cc_shared_library edges (bridges a
          packaged target back to its underlying cc_shared_library)
        - shared_library: cc_import -> _dd_cc_packaged_rule edges (a cc_import
          bridge pointed at an already-packaged shared library)
        - embed, deps: go_library/go_binary edges, so a walk can start from a
          real Go binary
        - cdeps: go_library/go_binary -> cc_library edges (the cgo boundary)
        - data: cc_library -> _dd_cc_packaged_rule edges (a runtime, dlopen'd
          dependency rather than a link-time one)
    """,
    attr_aspects = ["srcs"],
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

    # Index by original file. We do this for stripped_file and original_file for now
    # because the semantics of dd_go_binary + dd_strip_symbols is still in flux.
    originals_to_include = {strip_info.stripped_file: strip_info for strip_info in all_strip_info}
    originals_to_include.update({strip_info.original: strip_info for strip_info in all_strip_info})

    # Go through the dest/source map and spot the originals we want.
    # Note that we can not have a single binary appear twice and map to corresponding distinct
    # locations for the debug symbols. That is an acceptable limitation.
    original_src_to_dest = {}
    for pkg_src in ctx.attr.srcs:
        if PackageFilegroupInfo in pkg_src:
            for pfi, _ in pkg_src[PackageFilegroupInfo].pkg_files:
                # print(pfi, origin)  # buildifier: disable=print
                for dest, src in pfi.dest_src_map.items():
                    # print(" Source", src, "->", dest)  # buildifier: disable=print
                    if src in originals_to_include:
                        # print(" WINNER:", src, "->", dest)  # buildifier: disable=print
                        original_src_to_dest[src] = dest

    # Build a single pkg_files for all the debug symbols
    prefix = ctx.attr.prefix
    if prefix and not prefix.endswith("/"):
        prefix = prefix + "/"
    dest_src_map = {}
    for strip_info in all_strip_info:
        debug_file = strip_info.debug_file
        if strip_info.stripped_file in original_src_to_dest:
            original_dest_dir = "/".join(original_src_to_dest[strip_info.stripped_file].split("/")[0:-1])

            # TODO: We might need to add extension for windows.
            path = prefix + original_dest_dir + "/" + debug_file.basename
        else:
            path = prefix + debug_file.short_path

            # buildifier: disable=print
            print("Warning: Have debug symbol object with no original at", path, strip_info)
        dest_src_map[path] = debug_file

    pfi = PackageFilesInfo(
        dest_src_map = dest_src_map,
        attributes = {"mode": "0644"},
    )

    return [
        DefaultInfo(files = depset(dest_src_map.values())),
        PackageFilegroupInfo(pkg_files = [(pfi, ctx.label)]),
    ]

get_debug_symbols = rule(
    implementation = _get_debug_symbols_impl,
    doc = """
        Walks a pkg_filegroup for DdStripInfo providers and pulls the debug outputs
        into a new pkg_filegroup of just those.1
    """,
    attrs = {
        "srcs": attr.label_list(
            doc = "Set of pkg_filegroup targets to examine",
            aspects = [_collect_dd_strip_aspect],
            providers = [[PackageFilegroupInfo]],
        ),
        "prefix": attr.string(doc = "standard pkg_* rules prefix"),
    },
    provides = [PackageFilegroupInfo],
)
