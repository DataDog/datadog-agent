"""Write a manifest of rules_pkg providers, without materialising any files.

Consumers like the healthcheck need to know the destination path of every
file that will be installed, and be able to read each one's own content
(wherever Bazel already put it), without copying anything into a combined
tree-artifact directory.

Usage:

    load("//bazel/rules/pkg_manifest:manifest.bzl", "write_manifest")

    result = write_manifest(ctx, "my_manifest", [":embedded", ":etc"])
    # result.manifest: the written JSON manifest File.
    # result.files: source Files referenced by the manifest; add these to
    #   the consuming action's inputs.
"""

load("@rules_pkg//pkg:providers.bzl", "PackageFilegroupInfo", "PackageFilesInfo", "PackageSymlinkInfo")

def write_manifest(ctx, name, srcs):
    """Write a JSON manifest (dest path -> source file / symlink target) for pkg_filegroup / pkg_files / pkg_symlinks providers.

    Args:
      ctx: the rule context.
      name: base name for the declared manifest file.
      srcs: list of targets carrying PackageFilegroupInfo / PackageFilesInfo /
        PackageSymlinkInfo providers to include in the manifest.

    Returns:
      A struct with `manifest` (the written File) and `files` (the list of
      source Files referenced by the manifest; the caller must add these to
      any action that reads the manifest, since Bazel doesn't know about
      them otherwise).
    """
    files = {}
    symlinks = {}

    def _add_pkg_files(pkg_files):
        for dest, src in pkg_files.dest_src_map.items():
            files[dest] = src

    def _add_pkg_symlink(pkg_symlink):
        symlinks[pkg_symlink.destination] = pkg_symlink.target

    for dep in srcs:
        if PackageFilegroupInfo in dep:
            pfg = dep[PackageFilegroupInfo]
            for pkg_files, _ in pfg.pkg_files:
                _add_pkg_files(pkg_files)
            for pkg_symlink, _ in pfg.pkg_symlinks:
                _add_pkg_symlink(pkg_symlink)
        elif PackageFilesInfo in dep:
            _add_pkg_files(dep[PackageFilesInfo])
        elif PackageSymlinkInfo in dep:
            _add_pkg_symlink(dep[PackageSymlinkInfo])

    # "files": regular files (dest -> real source path to read).
    # "directories": directory artifacts (dest -> real source dir); contents
    # aren't known until the action producing the directory actually runs, so
    # the reader walks it at load time rather than this Starlark code doing it.
    # "symlinks": dest -> literal link target, as it will be installed.
    file_entries = {}
    dir_entries = {}
    for dest, src in files.items():
        if src.is_directory:
            dir_entries[dest] = src.path
        else:
            file_entries[dest] = src.path

    for dest, target in symlinks.items():
        if type(target) != "string":
            fail("write_manifest: symlink target must be a string, got {} for {}".format(target, dest))

    manifest = ctx.actions.declare_file(name + ".manifest.json")
    ctx.actions.write(manifest, json.encode({
        "files": file_entries,
        "directories": dir_entries,
        "symlinks": symlinks,
    }))

    return struct(manifest = manifest, files = list(files.values()))
