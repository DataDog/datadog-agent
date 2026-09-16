"""Import release.json so we can use these global constants.

It can also take variables from the environment to replace values given for
the "dependencies" field, such that they can be overridden
(requires --repo_env=VAR_NAME for each field for this to have an effect)

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

def read_effective_release_json(rctx, release_json_label):
    """Read contents release.json file with environment overrides applied.

    This is intended as a common entry point for repository rules to use
    to get values from release.json.

    Requires --repo_env=VAR_NAME for variables to be actually overridden.
    """
    release_json = json.decode(rctx.read(rctx.path(release_json_label)))

    # Override dependencies with values from the environment
    release_json["dependencies"] = {
        dep_key: rctx.getenv(dep_key) or release_json["dependencies"][dep_key]
        for dep_key in release_json["dependencies"]
    }
    return release_json

def _release_json_impl(rctx):
    rctx.file("BUILD.bazel", BUILD_FILE_CONTENT)
    release_json = read_effective_release_json(rctx, rctx.attr._release_json)
    rctx.file("release_json.bzl", """release_json = %s\n""" % str(release_json))

release_json = repository_rule(
    implementation = _release_json_impl,
    doc = """Import release.json as a .bzl file.""",
    attrs = {
        "_release_json": attr.label(default = "//:release.json", allow_single_file = True),
    },
)
