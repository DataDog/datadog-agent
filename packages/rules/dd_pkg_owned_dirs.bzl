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

def _files_destinations(pfi):
    declared = []
    trees = []
    owns_tree_dirs = pfi.attributes.get("rpm_own_tree_dirs", False)
    for dest, src in pfi.dest_src_map.items():
        if src.is_directory and not owns_tree_dirs:
            trees.append(dest)
        else:
            declared.append(dest)
    return declared, trees

def _destinations(src):
    declared = []
    trees = []
    pfis = []
    if PackageFilegroupInfo in src:
        pfg = src[PackageFilegroupInfo]
        pfis.extend([pfi for pfi, _ in pfg.pkg_files])
        declared.extend([psi.destination for psi, _ in pfg.pkg_symlinks])
        declared.extend([d for pdi, _ in pfg.pkg_dirs for d in pdi.dirs])
    if PackageFilesInfo in src:
        pfis.append(src[PackageFilesInfo])
    if PackageSymlinkInfo in src:
        declared.append(src[PackageSymlinkInfo].destination)
    if PackageDirsInfo in src:
        declared.extend(src[PackageDirsInfo].dirs)
    for pfi in pfis:
        pfi_declared, pfi_trees = _files_destinations(pfi)
        declared.extend(pfi_declared)
        trees.extend(pfi_trees)
    return declared, trees

def owned_parent_dirs(declared, trees, roots):
    """Returns the directories under one of `roots` that the package must own.

    Args:
      declared: normalized absolute paths the package already provides.
      trees: normalized absolute destinations of TreeArtifacts the package
        installs without owning them (no `rpm_own_tree_dirs` attribute).
      roots: normalized absolute paths of the directories the package owns.

    Returns:
      Sorted list of the parent directories of `declared` and of `trees`,
      along with `trees` themselves, up to and including each root, minus
      the paths in `declared`.
    """
    provided = {d: True for d in declared}
    owned = {}
    for start in [paths.dirname(d) for d in declared] + trees:
        for root in roots:
            if start != root and not start.startswith(root + "/"):
                continue
            parent = start
            for _ in range(start.count("/") + 1):
                if parent in owned or parent in provided:
                    break
                owned[parent] = True
                if parent == root:
                    break
                parent = paths.dirname(parent)
    return sorted(owned.keys())

def _dd_pkg_owned_dirs_impl(ctx):
    declared = []
    trees = []
    for src in ctx.attr.srcs:
        src_declared, src_trees = _destinations(src)
        declared.extend([_normalize(d) for d in src_declared])
        trees.extend([_normalize(d) for d in src_trees])

    # TODO(ABLD-598): use the package's own install prefix, installer and dogstatsd don't install under //:install_dir
    roots = [_normalize(ctx.attr._install_dir[BuildSettingInfo].value)]
    return [
        PackageDirsInfo(
            dirs = owned_parent_dirs(declared, trees, roots),
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
