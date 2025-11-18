"""A module defining the third party dependency libgit2"""

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")

def libgit2_repositories():
    maybe(
        http_archive,
        name = "libgit2",
        build_file = Label("//libgit2:BUILD.libgit2.bazel"),
        sha256 = "192eeff84596ff09efb6b01835a066f2df7cd7985e0991c79595688e6b36444e",
        strip_prefix = "libgit2-1.3.0",
        urls = [
            "https://mirror.bazel.build/github.com/libgit2/libgit2/archive/refs/tags/v1.3.0.tar.gz",
            "https://github.com/libgit2/libgit2/archive/refs/tags/v1.3.0.tar.gz",
        ],
    )
