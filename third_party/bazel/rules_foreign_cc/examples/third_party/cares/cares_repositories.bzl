# buildifier: disable=module-docstring
load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")

def cares_repositories():
    """Load all repositories needed for cares"""
    maybe(
        http_archive,
        name = "cares",
        build_file = Label("//cares:BUILD.cares.bazel"),
        sha256 = "62dd12f0557918f89ad6f5b759f0bf4727174ae9979499f5452c02be38d9d3e8",
        strip_prefix = "c-ares-cares-1_14_0",
        urls = [
            "https://mirror.bazel.build/github.com/c-ares/c-ares/archive/cares-1_14_0.tar.gz",
            "https://github.com/c-ares/c-ares/archive/cares-1_14_0.tar.gz",
        ],
    )
