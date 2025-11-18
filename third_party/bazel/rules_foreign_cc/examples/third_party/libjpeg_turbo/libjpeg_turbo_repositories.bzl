"""A module defining the third party dependency libjpeg_turbo"""

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("@bazel_tools//tools/build_defs/repo:utils.bzl", "maybe")

def libjpeg_turbo_repositories():
    maybe(
        http_archive,
        name = "libjpeg_turbo",
        sha256 = "6a965adb02ad898b2ae48214244618fe342baea79db97157fdc70d8844ac6f09",
        strip_prefix = "libjpeg-turbo-2.0.90",
        urls = [
            "https://mirror.bazel.build/github.com/libjpeg-turbo/libjpeg-turbo/archive/2.0.90.tar.gz",
            "https://github.com/libjpeg-turbo/libjpeg-turbo/archive/2.0.90.tar.gz",
        ],
        build_file = Label("//libjpeg_turbo:BUILD.libjpeg_turbo.bazel"),
    )
