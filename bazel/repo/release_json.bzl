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

def read_effective_release_json(rctx, release_json_label, shard_labels = []):
    """Read contents release.json file with environment overrides applied.

    This is intended as a common entry point for repository rules to use
    to get values from release.json.

    Requires --repo_env=VAR_NAME for variables to be actually overridden.

    Args:
        rctx: The repository context.
        release_json_label: Label for release.json (holds top-level metadata).
        shard_labels: Labels for per-project dependency shard files under
            release.d/. Each shard has the form {"dependencies": {...}} and its
            keys are merged into the returned dependencies dict. When empty,
            falls back to reading a "dependencies" key directly from release.json
            (for tests or callers that embed dependencies in release.json).
    """
    release_json = json.decode(rctx.read(rctx.path(release_json_label)))

    if shard_labels:
        # Merge dependency keys from per-project shard files.
        dependencies = {}
        for label in shard_labels:
            shard = json.decode(rctx.read(rctx.path(label)))
            dependencies.update(shard.get("dependencies", {}))
    else:
        # Fallback: read from a "dependencies" key embedded in release.json.
        # Used by tests and any callers that have not yet been migrated.
        dependencies = release_json.get("dependencies", {})

    # Override with values from the environment
    release_json["dependencies"] = {
        dep_key: rctx.getenv(dep_key) or dependencies[dep_key]
        for dep_key in dependencies
    }
    return release_json

def _release_json_impl(rctx):
    rctx.file("BUILD.bazel", BUILD_FILE_CONTENT)
    release_json_path = rctx.path(rctx.attr._release_json)
    release_json_data = json.decode(rctx.read(release_json_path))

    # Auto-discover all shards from release.d/ so new shards are picked up
    # without needing to update this file.
    release_d_dir = release_json_path.dirname.get_child("release.d")
    dependencies = {}
    for shard_path in sorted(release_d_dir.readdir(), key = lambda p: str(p)):
        if not str(shard_path).endswith(".json"):
            continue
        shard = json.decode(rctx.read(shard_path))
        dependencies.update(shard.get("dependencies", {}))

    release_json_data["dependencies"] = {
        dep_key: rctx.getenv(dep_key) or dependencies[dep_key]
        for dep_key in dependencies
    }
    rctx.file("release_json.bzl", "release_json = %s\n" % str(release_json_data))

release_json = repository_rule(
    implementation = _release_json_impl,
    doc = """Import release.json as a .bzl file.""",
    attrs = {
        "_release_json": attr.label(default = "//:release.json", allow_single_file = True),
    },
)
