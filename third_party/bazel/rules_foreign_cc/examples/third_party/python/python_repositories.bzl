"""A module defining the third party dependency Python"""

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")

# buildifier: disable=unnamed-macro
def python_repositories():
    maybe(
        http_archive,
        name = "python3",
        build_file = Label("//python:BUILD.python3.bazel"),
        strip_prefix = "Python-3.10.1",
        urls = [
            "https://www.python.org/ftp/python/3.10.1/Python-3.10.1.tgz",
        ],
        sha256 = "b76117670e7c5064344b9c138e141a377e686b9063f3a8a620ff674fa8ec90d3",
    )
