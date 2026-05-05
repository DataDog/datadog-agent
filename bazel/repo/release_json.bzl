"""Import release.json so we can use these global constants.

Usage:
    release_json = use_repo_rule("//bazel/repo:release_json.bzl", "release_json")
    # Give it a private name so that we don't run the risk of conflicting with a future OSS module.
    release_json(name = "dd_release_json")

    load("@dd_release_json//:release_json.bzl", "release_json")

    milestone = release_json.get("current_milestone")
"""

BUILD_FILE_CONTENT = """
exports_files(
    ["release_json.bzl"],
    # We may expand this in the future, but let's limit inventiveness for now.
    visibility = [
        "@agent//bazel/rules:__subpackages__",
        "@agent//packages:__subpackages__",
    ],
)
"""

def _release_json_impl(rctx):
    rctx.file("BUILD.bazel", BUILD_FILE_CONTENT)
    release_json = json.decode(rctx.read(rctx.path(rctx.attr._release_json)))
    rctx.file("release_json.bzl", """release_json = %s\n""" % str(release_json))

release_json = repository_rule(
    implementation = _release_json_impl,
    doc = """Import release.json as a .bzl file.""",
    attrs = {
        "_release_json": attr.label(default = "//:release.json", allow_single_file = True),
    },
)
