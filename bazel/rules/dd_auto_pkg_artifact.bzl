"""Provides dd_auto_pkg_artifact and collect_dd_auto_pkg_artifacts rules.

Overview
--------
Each C/C++ dependency that must be shipped with the agent declares a
dd_auto_pkg_artifact target next to its cc_shared_library / cc_binary.  It
associates the build target with the set of packaging targets (so_symlink
outputs, pkg_files, …) that describe exactly what gets installed.

At the dependency level the typical pattern is:

    rewrite_rpath(
        name   = "mylib_rpath_patched",
        inputs = [":mylib"],
    )

    so_symlink(
        name    = "lib_files",
        src     = ":mylib_rpath_patched",
        libname = "libmylib",
        version = VERSION,
        prefix  = "embedded",
    )

    dd_auto_pkg_artifact(
        name          = "mylib_pkg",   # used in dynamic_deps by consumers
        src           = ":mylib",      # re-exports CcSharedLibraryInfo for linking
        install_files = [":lib_files"],
    )

rpath patching must be applied explicitly at the dep level (via rewrite_rpath)
before files are passed to so_symlink or pkg_files — collect_dd_auto_pkg_artifacts
does not patch anything.

At the packaging level a single collect_dd_auto_pkg_artifacts rule crawls the
transitive dependency graph starting from the declared srcs and returns a merged
PackageFilegroupInfo:

    collect_dd_auto_pkg_artifacts(
        name = "all_openscap_deps",
        srcs = ["@openscap//:openscap_pkg", ...],
    )

The result can be used directly in pkg_filegroup.srcs, pkg_tar, etc.
"""

load("@rules_cc//cc/common:cc_shared_library_info.bzl", "CcSharedLibraryInfo")
load("@rules_pkg//pkg:providers.bzl", "PackageFilegroupInfo", "PackageFilesInfo")

# ── Public provider ───────────────────────────────────────────────────────────

DdAutoPkgArtifactInfo = provider(
    doc = """Associates a dependency target with the files to install alongside it.

    install_files is a list of targets each providing PackageFilegroupInfo or
    PackageFilesInfo (e.g. the output of so_symlink or pkg_files).  The files
    carry the correct install prefix; rpath patching must be applied explicitly
    at the dep level via rewrite_rpath before files are passed here.""",
    fields = {
        "install_files": "list of Targets providing PackageFilegroupInfo or PackageFilesInfo",
    },
)

# ── dd_auto_pkg_artifact rule ─────────────────────────────────────────────────

def _dd_auto_pkg_artifact_impl(ctx):
    src = ctx.attr.src
    has_shared_lib = CcSharedLibraryInfo in src

    # cc_binary provides an executable in DefaultInfo but no CcSharedLibraryInfo.
    is_binary = (
        not has_shared_lib and
        src[DefaultInfo].files_to_run != None and
        src[DefaultInfo].files_to_run.executable != None
    )

    # For cc_binary, do NOT propagate DefaultInfo.executable — dd_auto_pkg_artifact
    # is not itself an executable rule and must not expose another rule's executable.
    # Expose just the files so Bazel knows what to build.
    if is_binary:
        default_info = DefaultInfo(files = src[DefaultInfo].files)
    else:
        default_info = src[DefaultInfo]

    providers = [
        default_info,
        DdAutoPkgArtifactInfo(
            install_files = ctx.attr.install_files,
        ),
    ]
    # Re-export CC providers so this target is a drop-in for dynamic_deps.
    if has_shared_lib:
        providers.append(src[CcSharedLibraryInfo])
    if CcInfo in src:
        providers.append(src[CcInfo])
    return providers

dd_auto_pkg_artifact = rule(
    implementation = _dd_auto_pkg_artifact_impl,
    doc = """Wraps a cc_shared_library or cc_binary and declares its installation files.

    The target re-exports CcSharedLibraryInfo (when present) so it can replace
    the wrapped target in dynamic_deps without any change to the build graph.
    collect_dd_auto_pkg_artifacts uses the install_files to gather everything
    that must be shipped alongside the artifact.  rpath patching must be applied
    explicitly at the dep level via rewrite_rpath before files are passed here.""",
    attrs = {
        "src": attr.label(
            doc = "The underlying cc_shared_library or cc_binary to wrap.",
            mandatory = True,
        ),
        "install_files": attr.label_list(
            doc = """Targets (so_symlink outputs, pkg_files, pkg_filegroup, …)
            whose PackageFilegroupInfo / PackageFilesInfo will be merged into the
            collected result.  Files must already have the correct install prefix
            and must already have rpath patched via rewrite_rpath if needed.""",
            mandatory = True,
        ),
    },
)

# ── Aspect + collector ────────────────────────────────────────────────────────

_TransitiveDdAutoPkgArtifactsInfo = provider(
    doc = "Internal: depset of DdAutoPkgArtifactInfo Targets accumulated by the aspect.",
    fields = {"artifacts": "depset of Targets providing DdAutoPkgArtifactInfo"},
)

def _dd_auto_pkg_artifact_aspect_impl(target, ctx):
    transitive = []

    # Walk all relevant dependency attributes; handle both list and single-label.
    for attr_name in ["src", "srcs", "deps", "dynamic_deps"]:
        if hasattr(ctx.rule.attr, attr_name):
            v = getattr(ctx.rule.attr, attr_name)
            deps = v if type(v) == "list" else ([v] if v else [])
            for dep in deps:
                if _TransitiveDdAutoPkgArtifactsInfo in dep:
                    transitive.append(dep[_TransitiveDdAutoPkgArtifactsInfo].artifacts)

    direct = [target] if DdAutoPkgArtifactInfo in target else []
    return [_TransitiveDdAutoPkgArtifactsInfo(
        artifacts = depset(direct, transitive = transitive),
    )]

_dd_auto_pkg_artifact_aspect = aspect(
    implementation = _dd_auto_pkg_artifact_aspect_impl,
    attr_aspects = [
        # dd_auto_pkg_artifact: the wrapped cc_shared_library or cc_binary.
        "src",
        # collect_dd_auto_pkg_artifacts: the root targets to collect from.
        "srcs",
        # cc_library / cc_binary: transitive compile/link dependencies.
        "deps",
        # cc_shared_library / cc_binary / configure_make: shared libraries
        # linked at runtime.
        "dynamic_deps",
        # foreign_cc_shared_wrapper: the wrapped rules_foreign_cc target
        # (e.g. configure_make). These targets expose their wrapped output via
        # `input` rather than `src` or `deps`.
        "input",
    ],
    provides = [_TransitiveDdAutoPkgArtifactsInfo],
)

def _process_pkg_files_info(pfi, lbl, all_actual_files, seen_dests):
    """Return (PackageFilesInfo, label), skipping dests already seen.

    Returns None if all dests were already seen (deduplication across transitive artifacts).
    """
    filtered = {dest: src for dest, src in pfi.dest_src_map.items() if dest not in seen_dests}
    if not filtered:
        return None
    for dest in filtered:
        seen_dests[dest] = True
    all_actual_files.extend(filtered.values())
    return (PackageFilesInfo(dest_src_map = filtered, attributes = pfi.attributes), lbl)

def _collect_dd_auto_pkg_artifacts_impl(ctx):
    all_pkg_files = []
    all_pkg_dirs = []
    all_pkg_symlinks = []
    all_actual_files = []
    seen = {}
    seen_dests = {}

    for dep in ctx.attr.srcs:
        if _TransitiveDdAutoPkgArtifactsInfo not in dep:
            continue
        for target in dep[_TransitiveDdAutoPkgArtifactsInfo].artifacts.to_list():
            label = str(target.label)
            if label in seen:
                continue
            seen[label] = True

            for fg in target[DdAutoPkgArtifactInfo].install_files:
                if PackageFilegroupInfo in fg:
                    info = fg[PackageFilegroupInfo]
                    for pfi, lbl in info.pkg_files:
                        result = _process_pkg_files_info(pfi, lbl, all_actual_files, seen_dests)
                        if result != None:
                            all_pkg_files.append(result)
                    all_pkg_dirs.extend(info.pkg_dirs)
                    all_pkg_symlinks.extend(info.pkg_symlinks)
                elif PackageFilesInfo in fg:
                    result = _process_pkg_files_info(fg[PackageFilesInfo], fg.label, all_actual_files, seen_dests)
                    if result != None:
                        all_pkg_files.append(result)

    return [
        # Expose the File objects so Bazel schedules their producing actions.
        DefaultInfo(files = depset(all_actual_files)),
        PackageFilegroupInfo(
            pkg_files = all_pkg_files,
            pkg_dirs = all_pkg_dirs,
            pkg_symlinks = all_pkg_symlinks,
        ),
    ]

collect_dd_auto_pkg_artifacts = rule(
    implementation = _collect_dd_auto_pkg_artifacts_impl,
    doc = """Collects the install_files from every dd_auto_pkg_artifact reachable
    through the transitive dependency graph of srcs and returns a merged
    PackageFilegroupInfo.

    rpath patching must be applied explicitly at the dep level via rewrite_rpath
    before files are passed to so_symlink or pkg_files — this rule does not patch
    anything.

    Returns PackageFilegroupInfo suitable for use in pkg_filegroup.srcs,
    pkg_tar.srcs, etc.""",
    attrs = {
        "srcs": attr.label_list(
            doc = "Root targets whose transitive dd_auto_pkg_artifact deps to collect.",
            aspects = [_dd_auto_pkg_artifact_aspect],
        ),
    },
)
