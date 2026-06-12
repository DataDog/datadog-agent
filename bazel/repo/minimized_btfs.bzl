"""Repo rule to capture the minimized-btfs.tar.xz archive built outside the Bazel graph.

Usage:

minimized_btfs = use_repo_rule("//bazel/repo:minimized_btfs.bzl", "minimized_btfs")

minimized_btfs(name = "minimized_btfs_archive")

- Reads the environment variable MINIMIZED_BTFS_PATH, which should point to a
  directory that contains minimized-btfs.tar.xz (equivalent to $SYSTEM_PROBE_BIN
  in the omnibus build).
- If the archive is found, creates a filegroup target exposing it.
- If not found, creates an empty filegroup stub so local builds that do not need
  the BTF archive can still succeed.
"""

_ARCHIVE_NAME = "minimized-btfs.tar.xz"

_TARGET_NAME = "archive"

_BUILD_FILE_FOUND = """\
package(default_visibility = ["//visibility:public"])

filegroup(
    name = "archive",
    srcs = ["{archive_name}"],
)
"""

_BUILD_FILE_NOT_FOUND = """\
package(default_visibility = ["//visibility:public"])

filegroup(
    name = "archive",
    srcs = [],
)
"""

def _minimized_btfs_impl(rctx):
    """Implementation of the minimized_btfs repository rule."""

    rctx.file("MODULE.bazel", "module(name = \"{name}\")\n".format(name = rctx.original_name))

    btfs_dir = rctx.os.environ.get("MINIMIZED_BTFS_PATH")

    archive_found = False
    if btfs_dir:
        archive_path = rctx.path(btfs_dir + "/" + _ARCHIVE_NAME)
        rctx.watch(archive_path)
        if archive_path.exists:
            rctx.report_progress("Found BTF archive: %s" % str(archive_path))
            rctx.symlink(archive_path, _ARCHIVE_NAME)
            archive_found = True

    if archive_found:
        rctx.file("BUILD.bazel", _BUILD_FILE_FOUND.format(archive_name = _ARCHIVE_NAME))
    else:
        rctx.file("BUILD.bazel", _BUILD_FILE_NOT_FOUND.format(archive_name = _ARCHIVE_NAME))

    return rctx.repo_metadata(reproducible = False)

minimized_btfs = repository_rule(
    implementation = _minimized_btfs_impl,
    doc = "Captures the minimized-btfs.tar.xz archive from an external build directory.",
    environ = ["MINIMIZED_BTFS_PATH"],
)
