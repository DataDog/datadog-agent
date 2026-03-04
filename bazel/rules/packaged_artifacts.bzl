"""Rules for declaring and collecting installation files for dependencies.

When a target is a transitive dynamic dependency, its files (shared objects,
versioned symlinks, data files, …) must be installed alongside the primary
artifact.  This module provides:

  packaged_artifacts  — wraps any dependency target and carries a reference
                        to the pkg_filegroup that should be installed with it
                        (typically the output of so_symlink, already
                        rpath-patched).

  collect_packaged_artifacts — traverses the dependency graph (via srcs, deps,
                        and dynamic_deps attributes) and merges the
                        PackageFilegroupInfo from every packaged_artifacts
                        target it encounters into a single target usable in
                        pkg_filegroup.srcs.

Typical usage in a dependency's BUILD file:

    rewrite_rpath(
        name = "foo_rpath_patched",
        inputs = [":foo_lib"],
    )

    so_symlink(
        name = "lib_files",
        src = ":foo_rpath_patched",
        libname = "libfoo",
        version = VERSION,
        prefix = "embedded",
    )

    packaged_artifacts(
        name = "foo",               # same label used in dynamic_deps elsewhere
        src = ":foo_lib",           # the underlying dependency target
        install_filegroups = [
            ":lib_files",           # shared object + versioned symlinks
            ":bin_files",           # executables shipped with the dependency (if any)
        ],
    )

Typical usage in the consumer's BUILD file:

    collect_packaged_artifacts(
        name = "all_deps_shipping",
        srcs = [":my_cc_shared_library"],
    )

    pkg_filegroup(
        name = "all_files",
        srcs = [
            ":all_deps_shipping",
            ":bin_files",
            ":lib_files",
            ...
        ],
        prefix = "embedded",
    )
"""

load("@rules_cc//cc/common:cc_shared_library_info.bzl", "CcSharedLibraryInfo")
load("@rules_pkg//pkg:providers.bzl", "PackageFilegroupInfo", "PackageFilesInfo")

# ── Public provider ───────────────────────────────────────────────────────────

PackagedArtifactsInfo = provider(
    doc = """Associates a dependency target with its installation pkg_filegroups.

    Add this (via packaged_artifacts) to any target that is used as a
    dependency so that collect_packaged_artifacts can automatically gather all
    the files that must be installed alongside the primary binary.""",
    fields = {
        "install_filegroups": "list of Targets each providing PackageFilegroupInfo (e.g. so_symlink output, bin_files, …)",
    },
)

# ── packaged_artifacts rule ───────────────────────────────────────────────────

def _packaged_artifacts_impl(ctx):
    src = ctx.attr.src
    providers = [
        src[DefaultInfo],
        PackagedArtifactsInfo(
            install_filegroups = ctx.attr.install_filegroups,
        ),
    ]
    # Re-export CcSharedLibraryInfo when present so the target remains usable
    # in other cc_shared_library dynamic_deps attributes.
    if CcSharedLibraryInfo in src:
        providers.append(src[CcSharedLibraryInfo])
    if CcInfo in src:
        providers.append(src[CcInfo])
    return providers

packaged_artifacts = rule(
    implementation = _packaged_artifacts_impl,
    doc = """Wraps a dependency target and declares its installation files.

    The resulting target is a drop-in replacement for the wrapped target in
    other rules' dynamic_deps (or deps / srcs) attributes: it re-exports
    CcSharedLibraryInfo and CcInfo when present, and always provides
    PackagedArtifactsInfo so that collect_packaged_artifacts can find the
    associated packaging target.""",
    attrs = {
        "src": attr.label(
            doc = "The underlying dependency target to wrap.",
            mandatory = True,
        ),
        "install_filegroups": attr.label_list(
            doc = """pkg_filegroup (or similar) targets whose PackageFilegroupInfo
            should be included whenever this target appears as a transitive
            dependency.  Typically includes the output of so_symlink (built from
            the rpath-patched shared object) and, where applicable, a bin_files
            target for any executables shipped with the dependency.""",
            mandatory = True,
        ),
    },
)

# ── Aspect + collector rule ───────────────────────────────────────────────────

_TransitivePackagedArtifactsInfo = provider(
    doc = "Internal: accumulates install_filegroup targets reached via the dependency graph.",
    fields = {
        "shipping_targets": "depset of Targets (each providing PackageFilegroupInfo)",
    },
)

def _packaged_artifacts_aspect_impl(target, ctx):
    transitive = []
    direct = []

    for attr_name in ["srcs", "deps", "dynamic_deps"]:
        if hasattr(ctx.rule.attr, attr_name):
            attr_value = getattr(ctx.rule.attr, attr_name)
            if type(attr_value) != "list":
                continue
            for dep in attr_value:
                if _TransitivePackagedArtifactsInfo in dep:
                    transitive.append(dep[_TransitivePackagedArtifactsInfo].shipping_targets)

    if PackagedArtifactsInfo in target:
        direct.extend(target[PackagedArtifactsInfo].install_filegroups)

    return [_TransitivePackagedArtifactsInfo(
        shipping_targets = depset(direct, transitive = transitive),
    )]

_packaged_artifacts_aspect = aspect(
    implementation = _packaged_artifacts_aspect_impl,
    attr_aspects = ["srcs", "deps", "dynamic_deps"],
    provides = [_TransitivePackagedArtifactsInfo],
    doc = """Traverses dynamic_deps (and srcs/deps for pkg_filegroup hierarchies)
    and collects PackagedArtifactsInfo.install_filegroups from every
    packaged_artifacts target encountered.""",
)

def _collect_packaged_artifacts_impl(ctx):
    all_pkg_files = []
    all_pkg_dirs = []
    all_pkg_symlinks = []
    all_actual_files = []
    seen = {}

    for dep in ctx.attr.srcs:
        if _TransitivePackagedArtifactsInfo not in dep:
            continue
        for target in dep[_TransitivePackagedArtifactsInfo].shipping_targets.to_list():
            # Deduplicate: the same dep can appear via multiple paths.
            label = str(target.label)
            if label in seen:
                continue
            seen[label] = True

            if PackageFilegroupInfo in target:
                info = target[PackageFilegroupInfo]
                all_pkg_files.extend(info.pkg_files)
                all_pkg_dirs.extend(info.pkg_dirs)
                all_pkg_symlinks.extend(info.pkg_symlinks)
                for pkg_files_info, _ in info.pkg_files:
                    all_actual_files.extend(pkg_files_info.dest_src_map.values())
            elif PackageFilesInfo in target:
                # install_filegroups may point directly to pkg_files targets
                # (which provide PackageFilesInfo rather than PackageFilegroupInfo).
                info = target[PackageFilesInfo]
                all_pkg_files.append((info, target.label))
                all_actual_files.extend(info.dest_src_map.values())
            else:
                continue

    return [
        DefaultInfo(files = depset(all_actual_files)),
        PackageFilegroupInfo(
            pkg_files = all_pkg_files,
            pkg_dirs = all_pkg_dirs,
            pkg_symlinks = all_pkg_symlinks,
        ),
    ]

collect_packaged_artifacts = rule(
    implementation = _collect_packaged_artifacts_impl,
    doc = """Collects installation files from all transitive packaged_artifacts dependencies.

    Apply this to the top-level cc_shared_library (or binary, or any target
    with dependencies) of a component. The rule walks the dependency graph via
    srcs, deps, and dynamic_deps attributes, finds every packaged_artifacts
    target, and merges their associated PackageFilegroupInfo (shared objects,
    versioned symlinks, data files …) into a single target.

    The result provides PackageFilegroupInfo and can therefore appear directly
    in pkg_filegroup.srcs, pkg_tar.srcs, pkg_deb.data, etc.""",
    attrs = {
        "srcs": attr.label_list(
            doc = "Top-level targets whose dependency subgraph to search.",
            aspects = [_packaged_artifacts_aspect],
        ),
    },
)
