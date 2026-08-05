"""Check files considered by Bazelisk are present and Bazel version against .bazelversion file.

This implements the idiom described in the Bazelisk README for ensuring users don't mistakenly bypass Bazelisk by having
a Bazel binary in their PATH:
- https://github.com/bazelbuild/bazelisk?tab=readme-ov-file#ensuring-that-your-developers-use-bazelisk-rather-than-bazel
- https://gerrit.googlesource.com/prolog-cafe/+/master/tools/bzl/bazelisk_version.bzl
"""

def _impl(mctx):
    file_to_path = {
        f: mctx.path(Label("//:" + f))
        for f in (".bazelversion", "tools/bazel", "tools/bazel.bat", "tools/bazelisk.md")
    }

    for required_file, path in file_to_path.items():
        if not path.exists or path.is_dir:
            fail("Required file not found: `{}` - did you (re)move it?".format(required_file))
        mctx.watch(path)

    required_version = mctx.read(file_to_path[".bazelversion"]).strip()
    if native.bazel_version != required_version:
        fail("""Bazel version mismatch: expected {}, actual {}.
{}""".format(
            required_version,
            native.bazel_version,
            mctx.read(file_to_path["tools/bazelisk.md"]),
        ))

bazelisk_check = module_extension(implementation = _impl)
