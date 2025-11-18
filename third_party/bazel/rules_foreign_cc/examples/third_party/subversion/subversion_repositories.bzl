"""A module defining the third party dependency subversion"""

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")

def subversion_repositories():
    maybe(
        http_archive,
        name = "subversion",
        build_file = Label("//subversion:BUILD.subversion.bazel"),
        sha256 = "dee2796abaa1f5351e6cc2a60b1917beb8238af548b20d3e1ec22760ab2f0cad",
        strip_prefix = "subversion-1.14.1",
        urls = [
            "https://mirror.bazel.build/downloads.apache.org/subversion/subversion-1.14.1.tar.gz",
            "https://downloads.apache.org/subversion/subversion-1.14.1.tar.gz",
        ],
    )
