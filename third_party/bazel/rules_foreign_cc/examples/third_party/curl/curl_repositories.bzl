"""A module defining the third party dependency curl"""

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")

def curl_repositories():
    maybe(
        http_archive,
        name = "curl",
        urls = [
            "https://mirror.bazel.build/curl.se/download/curl-7.74.0.tar.gz",
            "https://curl.se/download/curl-7.74.0.tar.gz",
            "https://github.com/curl/curl/releases/download/curl-7_74_0/curl-7.74.0.tar.gz",
        ],
        type = "tar.gz",
        sha256 = "e56b3921eeb7a2951959c02db0912b5fcd5fdba5aca071da819e1accf338bbd7",
        strip_prefix = "curl-7.74.0",
        build_file = Label("//curl:BUILD.curl.bazel"),
    )
