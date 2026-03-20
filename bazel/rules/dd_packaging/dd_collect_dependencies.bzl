"""dd_collect_dependencies — collect transitive DdPackagingInfo for installation."""

load("@rules_pkg//pkg:providers.bzl", "PackageFilegroupInfo")
load("//bazel/rules/dd_packaging:dd_packaging_info.bzl", "DdPackagingInfo")

_CollectedPackagingInfo = provider(
    doc = "Internal provider used by _collect_dd_packaging_aspect to accumulate PackageFilegroupInfo instances.",
    fields = {
        # Depset of PackageFilegroupInfo gathered from this node and all nodes
        # reachable through dynamic_deps / input edges.
        "pkg_filegroups": "depset of PackageFilegroupInfo accumulated transitively",
    },
)

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

def _collect_dd_packaging_aspect_impl(target, ctx):
    direct = target[DdPackagingInfo].installed_files if DdPackagingInfo in target else []
    transitive = [
        dep[_CollectedPackagingInfo].pkg_filegroups
        for dep in _get_deps(ctx, ["dynamic_deps", "input"])
        if _CollectedPackagingInfo in dep
    ]
    return [_CollectedPackagingInfo(
        pkg_filegroups = depset(direct, transitive = transitive),
    )]

_collect_dd_packaging_aspect = aspect(
    implementation = _collect_dd_packaging_aspect_impl,
    doc = """
        Traverses two edge types to walk the full CC dependency graph:
        - dynamic_deps: cc_shared_library -> cc_shared_library edges
        - input: _dd_cc_packaged_rule -> cc_shared_library edges (bridges a
          packaged target back to its underlying cc_shared_library)
    """,
    attr_aspects = ["dynamic_deps", "input"],
)

def _dd_collect_dependencies_impl(ctx):
    pkg_files = []
    pkg_dirs = []
    pkg_symlinks = []

    for src in ctx.attr.srcs:
        if _CollectedPackagingInfo in src:
            for fg in src[_CollectedPackagingInfo].pkg_filegroups.to_list():
                pkg_files.extend(fg.pkg_files)
                pkg_dirs.extend(fg.pkg_dirs)
                pkg_symlinks.extend(fg.pkg_symlinks)

    # The same content can be packaged by multiple dependencies so we deduplicate
    pkg_files = depset(pkg_files).to_list()
    pkg_dirs = depset(pkg_dirs).to_list()
    pkg_symlinks = depset(pkg_symlinks).to_list()

    all_files = depset([
        f
        for pkg_files_info, _ in pkg_files
        for f in pkg_files_info.dest_src_map.values()
    ])

    return [
        PackageFilegroupInfo(
            pkg_files = pkg_files,
            pkg_dirs = pkg_dirs,
            pkg_symlinks = pkg_symlinks,
        ),
        DefaultInfo(files = all_files),
    ]

dd_collect_dependencies = rule(
    implementation = _dd_collect_dependencies_impl,
    doc = """
        Walks the build graph to collect all DdPackagingInfo providers to merge them
        into a single PackageFilegroupInfo to be installed with the final artifact.

        Intended to be used once at the top-level binary/library of a package
        to gather all installed files (headers, configuration files, ...) that were
        declared by dd_cc_packaged targets anywhere in the dependency graph.
        The result can be passed directly to pkg_filegroup or pkg_install.
    """,
    attrs = {
        "srcs": attr.label_list(
            doc = "Top-level CC targets which dependency graph should be searched for DdPackagingInfo.",
            aspects = [_collect_dd_packaging_aspect],
        ),
    },
    provides = [PackageFilegroupInfo],
)
