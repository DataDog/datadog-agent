"""dd_pkg_owned_dirs - declare the implicit parent directories of packaged content."""

load("@bazel_skylib//lib:paths.bzl", "paths")
load("@bazel_skylib//rules:common_settings.bzl", "BuildSettingInfo")
load(
    "@rules_pkg//pkg:providers.bzl",
    "PackageDirsInfo",
    "PackageFilegroupInfo",
    "PackageFilesInfo",
    "PackageSymlinkInfo",
)

def _normalize(path):
    return "/" + paths.normalize(path).lstrip("/")

def _destinations(src):
    dests = []
    if PackageFilegroupInfo in src:
        pfg = src[PackageFilegroupInfo]
        dests.extend([d for pfi, _ in pfg.pkg_files for d in pfi.dest_src_map.keys()])
        dests.extend([psi.destination for psi, _ in pfg.pkg_symlinks])
        dests.extend([d for pdi, _ in pfg.pkg_dirs for d in pdi.dirs])
    if PackageFilesInfo in src:
        dests.extend(src[PackageFilesInfo].dest_src_map.keys())
    if PackageSymlinkInfo in src:
        dests.append(src[PackageSymlinkInfo].destination)
    if PackageDirsInfo in src:
        dests.extend(src[PackageDirsInfo].dirs)
    return dests

def owned_parent_dirs(destinations, roots):
    """Returns the parent directories of `destinations` that lie under one of `roots`.

    Args:
      destinations: normalized absolute paths of the packaged entries.
      roots: normalized absolute paths of the directories the package owns.

    Returns:
      Sorted list of directories, each root included, minus the paths
      already present in `destinations`.
    """
    declared = {d: True for d in destinations}
    owned = {}
    for dest in destinations:
        for root in roots:
            if not dest.startswith(root + "/"):
                continue
            parent = paths.dirname(dest)
            for _ in range(dest.count("/")):
                if parent in owned or parent in declared:
                    break
                owned[parent] = True
                if parent == root:
                    break
                parent = paths.dirname(parent)
    return sorted(owned.keys())

def _dd_pkg_owned_dirs_impl(ctx):
    destinations = [
        _normalize(d)
        for src in ctx.attr.srcs
        for d in _destinations(src)
    ]
    roots = [_normalize(ctx.attr._install_dir[BuildSettingInfo].value)]
    return [
        PackageDirsInfo(
            dirs = owned_parent_dirs(destinations, roots),
            attributes = {"mode": "0755"},
        ),
        DefaultInfo(),
    ]

dd_pkg_owned_dirs = rule(
    implementation = _dd_pkg_owned_dirs_impl,
    doc = """
        Declares every directory between the content of `srcs` and the install
        directory, so a package manager that only removes directories it owns
        (rpm) cleans them up on uninstall. Directories already declared by
        `srcs` are skipped.
    """,
    attrs = {
        "srcs": attr.label_list(
            doc = "Package content, as passed to pkg_rpm's srcs.",
        ),
        "_install_dir": attr.label(default = "//:install_dir"),
    },
    provides = [PackageDirsInfo],
)
