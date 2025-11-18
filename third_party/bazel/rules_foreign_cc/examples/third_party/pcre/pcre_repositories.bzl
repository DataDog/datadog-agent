"""A module defining the third party dependency PCRE"""

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")

def pcre_repositories():
    maybe(
        http_archive,
        name = "pcre",
        build_file = Label("//pcre:BUILD.pcre.bazel"),
        strip_prefix = "pcre2-10.37",
        sha256 = "04e214c0c40a97b8a5c2b4ae88a3aa8a93e6f2e45c6b3534ddac351f26548577",
        urls = [
            "https://github.com/PCRE2Project/pcre2/releases/download/pcre2-10.37/pcre2-10.37.tar.gz",
        ],
    )
