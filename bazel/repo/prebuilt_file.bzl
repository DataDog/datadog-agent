"""Repo rule to capture a file that was built outside the bazel graph as if it was a real target.

Usage:

prebuilt_file = use_repo_rule("//bazel/repo:prebuilt_file.bzl", "prebuilt_file")

prebuilt_file(
    name = "agent_file",
    # Label of the prebuilt, as if there were really a target for it.
    target_label = "@@//:bin/agent/agent",
    # Name to give the target within this module.
    target_name = "agent",
)

- Creates a module named "agent_file"
- with the target name "agent"
- that points to the file bin/agent/agent if bin/agent/agent exists
  else it points to an empty file.
- and a constraint "file_exists", which can be used to tell if the file existed.
"""

_BUILD_FILE_FOUND = """\
load("@@//bazel/repo:prebuilt_exists.bzl", "file_exists")
package(default_visibility = ["//visibility:public"])

exports_files(glob(["**"]))

filegroup(
    name = "{target_name}",
    srcs = ["{target_path}"],
)

file_exists(
    name = "exists",
    build_setting_default = True,
)
"""

_BUILD_FILE_NOT_FOUND = """\
load("@@//bazel/repo:prebuilt_exists.bzl", "file_exists")
package(default_visibility = ["//visibility:public"])

exports_files(glob(["**"]))

filegroup(
    name = "{target_name}",
    srcs = [],
)

file_exists(
    name = "exists",
    build_setting_default = False,
)
"""

def _prebuilt_file(rctx):
    """Implementation of the prebuilt_file rule."""

    rctx.file("MODULE.bazel", "module(name = \"{name}\")\n".format(name = rctx.original_name))

    actual_label = rctx.attr.target_label
    target_path = rctx.path(actual_label)
    # Windows will probably have a .exe extension, so we can fall back to that.
    if not target_path.exists and rctx.os.name == "windows":
        l = rctx.attr.target_label
        actual_label = l.same_package_label(l.name + ".exe")
        target_path = rctx.path(actual_label)
    rctx.watch(target_path)
    if target_path.exists:
       rctx.report_progress("Found built file: %s" % str(target_path))
       rctx.file("BUILD", _BUILD_FILE_FOUND.format(
           target_name = rctx.attr.target_name,
           target_path = str(actual_label),
       ))
    else:
       rctx.file("BUILD", _BUILD_FILE_NOT_FOUND.format(
           target_name = rctx.attr.target_name,
           target_path = target_path,
       ))
    return rctx.repo_metadata(reproducible = False)

prebuilt_file = repository_rule(
    implementation = _prebuilt_file,
    attrs = {
        "target_name": attr.string(
            doc = "Name to give target in this module",
            mandatory = True,
        ),
        "target_label": attr.label(
            doc = "Path to check for existence of the prebuilt",
            mandatory = True,
        ),
    },
)
