"""Generate a repository containing Starlark constants.

Usage:
    constants_repo = use_repo_rule("//bazel/repo:constants_repo.bzl", "constants_repo")
    constants_repo(
        name = "my_constants",
        items = {
            "FOO": "bar",
        },
    )

    load("@my_constants//:constants.bzl", "FOO")
"""

BUILD_FILE_CONTENT = """
exports_files(["constants.bzl"])
"""

def _constants_repo_impl(rctx):
    rctx.file("BUILD.bazel", BUILD_FILE_CONTENT)

    content = "".join([
        "{} = {}\n".format(key, json.encode(value))
        for key, value in sorted(rctx.attr.items.items())
    ])
    rctx.file("constants.bzl", content)

constants_repo = repository_rule(
    implementation = _constants_repo_impl,
    attrs = {
        "items": attr.string_dict(mandatory = True, doc = "Map of constant names to values."),
    },
    doc = """Generate a repository containing a constants.bzl file.""",
)
