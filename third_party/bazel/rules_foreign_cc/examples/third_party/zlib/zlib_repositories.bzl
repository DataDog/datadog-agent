"""A module defining the third party dependency zlib"""

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")

def zlib_repositories():
    maybe(
        http_archive,
        name = "zlib",
        build_file = Label("//zlib:BUILD.zlib.bazel"),
        sha256 = "9a93b2b7dfdac77ceba5a558a580e74667dd6fede4585b91eefb60f03b72df23",
        strip_prefix = "zlib-1.3.1",
        patches = [
            # This patch modifies zlib so that on windows the generated lib matches that stated in the generated pkgconfig pc file for consumption by dependent rules
            # Similar patches are used in vcpkg and conan to resolve the same issue
            Label("//zlib:zlib.patch"),
        ],
        urls = [
            "https://zlib.net/zlib-1.3.1.tar.gz",
            "https://zlib.net/archive/zlib-1.3.1.tar.gz",
        ],
    )
