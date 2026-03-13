"""Check files considered by Bazelisk are present and Bazel version against .bazelversion file.

This implements the idiom described in the Bazelisk README for ensuring users don't mistakenly bypass Bazelisk by having
a Bazel binary in their PATH:
- https://github.com/bazelbuild/bazelisk?tab=readme-ov-file#ensuring-that-your-developers-use-bazelisk-rather-than-bazel
- https://gerrit.googlesource.com/prolog-cafe/+/master/tools/bzl/bazelisk_version.bzl
"""

_template = """
load("@bazel_skylib//lib:versions.bzl", "versions")

def check_bazel_version():
    versions.check(
        bazel_version = "{current_version}",
        maximum_bazel_version = "{required_version}",
        minimum_bazel_version = "{required_version}",
    )
""".strip()

def _impl(repository_ctx):
    file_to_path = {
        f: repository_ctx.path(Label("//:" + f))
        for f in (".bazelversion", "tools/bazel", "tools/bazel.bat")
    }

    for required_file, path in file_to_path.items():
        if not path.exists or path.is_dir:
            fail("Required file not found: `{}` - did you (re)move it?".format(required_file))
        repository_ctx.watch(path)

    repository_ctx.file("BUILD.bazel")
    repository_ctx.file(
        "defs.bzl",
        content = _template.format(
            current_version = native.bazel_version,
            required_version = repository_ctx.read(file_to_path[".bazelversion"]).strip(),
        ),
    )

bazelisk_check = repository_rule(configure = True, implementation = _impl, local = True)
