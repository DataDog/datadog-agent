# buildifier: disable=module-docstring
load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")

def libpng_repositories():
    """Load all repositories needed for libpng"""
    maybe(
        http_archive,
        name = "libpng",
        build_file = Label("//libpng:BUILD.libpng.bazel"),
        sha256 = "",
        strip_prefix = "libpng-1.6.43",
        urls = [
            "https://downloads.sourceforge.net/project/libpng/libpng16/1.6.43/libpng-1.6.43.tar.xz",
        ],
    )
