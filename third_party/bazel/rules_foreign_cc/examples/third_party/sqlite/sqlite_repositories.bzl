"""A module defining the third party dependency sqlite"""

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")

def sqlite_repositories():
    maybe(
        http_archive,
        name = "sqlite",
        build_file = Label("//sqlite:BUILD.sqlite.bazel"),
        sha256 = "f52b72a5c319c3e516ed7a92e123139a6e87af08a2dc43d7757724f6132e6db0",
        strip_prefix = "sqlite-autoconf-3350500",
        urls = [
            "https://mirror.bazel.build/www.sqlite.org/2021/sqlite-autoconf-3350500.tar.gz",
            "https://www.sqlite.org/2021/sqlite-autoconf-3350500.tar.gz",
        ],
    )
