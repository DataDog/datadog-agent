# buildifier: disable=module-docstring
load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")

def bison_repositories():
    """Load all repositories needed for bison"""

    maybe(
        http_archive,
        name = "bison",
        build_file = Label("//bison:BUILD.bison.bazel"),
        strip_prefix = "bison-3.8.2",
        urls = [
            "https://mirror.bazel.build/ftp.gnu.org/gnu/bison/bison-3.8.2.tar.gz",
            "https://ftp.gnu.org/gnu/bison/bison-3.8.2.tar.gz",
        ],
        sha256 = "",
    )
