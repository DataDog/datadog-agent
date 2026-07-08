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

def _merge_release_config(base, override):
    """Merge override into base, deep-merging one level of nested dicts.

    Starlark forbids recursion, so this merges the top level and one level of
    nested dicts (e.g. "dependencies"), which covers the release.json structure.
    Override values win.
    """
    result = dict(base)
    for key, value in override.items():
        if key in result and type(result[key]) == type({}) and type(value) == type({}):
            merged = dict(result[key])
            merged.update(value)
            result[key] = merged
        else:
            result[key] = value
    return result

def read_effective_release_json(rctx, release_json_label, shard_labels = []):
    """Read contents release.json file with environment overrides applied.

    This is intended as a common entry point for repository rules to use
    to get values from release.json.

    Requires --repo_env=VAR_NAME for variables to be actually overridden.

    Args:
        rctx: The repository context.
        release_json_label: Label for release.json (holds top-level metadata).
        shard_labels: Labels for per-project dependency shard files under
            release.d/. Each shard is merged into the result, treating it like
            a conf.d-style config system. When empty, falls back to reading a
            "dependencies" key directly from release.json (for tests or callers
            that embed dependencies).
    """
    release_json = json.decode(rctx.read(rctx.path(release_json_label)))

    if shard_labels:
        # Merge each shard into the result.
        for label in shard_labels:
            shard = json.decode(rctx.read(rctx.path(label)))
            release_json = _merge_release_config(release_json, shard)

    # Override with values from the environment (dependencies keys only).
    if "dependencies" in release_json and type(release_json["dependencies"]) == type({}):
        release_json["dependencies"] = {
            dep_key: rctx.getenv(dep_key) or release_json["dependencies"][dep_key]
            for dep_key in release_json["dependencies"]
        }

    return release_json

def _release_json_impl(rctx):
    rctx.file("BUILD.bazel", BUILD_FILE_CONTENT)
    release_json_path = rctx.path(rctx.attr._release_json)
    release_json_data = json.decode(rctx.read(release_json_path))

    # Auto-discover all shards from release.d/ and merge them, treating it
    # like a conf.d-style config system.
    release_d_dir = release_json_path.dirname.get_child("release.d")
    for shard_path in sorted(release_d_dir.readdir(), key = lambda p: str(p)):
        if not str(shard_path).endswith(".json"):
            continue
        shard = json.decode(rctx.read(shard_path), default = {})
        release_json_data = _merge_release_config(release_json_data, shard)

    # Override with environment variables (dependencies keys only).
    if "dependencies" in release_json_data and type(release_json_data["dependencies"]) == type({}):
        release_json_data["dependencies"] = {
            dep_key: rctx.getenv(dep_key) or release_json_data["dependencies"][dep_key]
            for dep_key in release_json_data["dependencies"]
        }

    rctx.file("release_json.bzl", "release_json = %s\n" % str(release_json_data))

release_json = repository_rule(
    implementation = _release_json_impl,
    doc = """Import release.json as a .bzl file.""",
    attrs = {
        "_release_json": attr.label(default = "//:release.json", allow_single_file = True),
    },
)
