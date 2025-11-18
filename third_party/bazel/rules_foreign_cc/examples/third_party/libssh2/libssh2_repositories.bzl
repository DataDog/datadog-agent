"""A module defining the third party dependency libssh2"""

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")

def libssh2_repositories():
    maybe(
        http_archive,
        name = "libssh2",
        urls = [
            "https://mirror.bazel.build/github.com/libssh2/libssh2/releases/download/libssh2-1.9.0/libssh2-1.9.0.tar.gz",
            "https://github.com/libssh2/libssh2/releases/download/libssh2-1.9.0/libssh2-1.9.0.tar.gz",
        ],
        type = "tar.gz",
        sha256 = "d5fb8bd563305fd1074dda90bd053fb2d29fc4bce048d182f96eaa466dfadafd",
        strip_prefix = "libssh2-1.9.0",
        build_file = Label("//libssh2:BUILD.libssh2.bazel"),
    )
